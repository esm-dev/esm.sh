package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/crypto/rs"
	"github.com/ije/gox/utils"
	"github.com/postui/postdb"
	"github.com/postui/postdb/q"
)

const (
	jsCopyrightName = "esm.sh"
)

var targets = map[string]api.Target{
	"es2015": api.ES2015,
	"es2016": api.ES2016,
	"es2017": api.ES2017,
	"es2018": api.ES2018,
	"es2019": api.ES2019,
	"es2020": api.ES2020,
}

// todo: use queue to replace lock
var buildLock sync.Mutex

// ImportMeta defines import meta
type ImportMeta struct {
	*NpmPackage
	Exports []string `json:"exports"`
	Dts     string   `json:"dts"`
}

type buildOptions struct {
	packages moduleSlice
	target   string
	isDev    bool
}

type buildResult struct {
	buildID    string
	importMeta map[string]*ImportMeta
}

func build(storageDir string, options buildOptions) (ret buildResult, err error) {
	buildLock.Lock()
	defer buildLock.Unlock()

	n := len(options.packages)
	if n == 0 {
		err = fmt.Errorf("no packages")
		return
	}

	single := n == 1
	if single {
		pkg := options.packages[0]
		filename := path.Base(pkg.name)
		if pkg.submodule != "" {
			filename = pkg.submodule
		}
		if options.isDev {
			filename += ".development"
		}
		ret.buildID = fmt.Sprintf("%s@%s/%s/%s", pkg.name, pkg.version, options.target, filename)
	} else {
		hasher := sha1.New()
		sort.Sort(options.packages)
		fmt.Fprintf(hasher, "%s %s %v", options.packages.String(), options.target, options.isDev)
		ret.buildID = "bundle-" + strings.ToLower(base32.StdEncoding.EncodeToString(hasher.Sum(nil)))
	}

	p, err := db.Get(q.Alias(ret.buildID), q.K("importMeta"))
	if err == nil {
		err = json.Unmarshal(p.KV.Get("importMeta"), &ret.importMeta)
		if err != nil {
			_, err = db.Delete(q.Alias(ret.buildID))
			if err != nil {
				return
			}
		}

		_, err = os.Stat(path.Join(storageDir, "builds", ret.buildID+".js"))
		if err == nil || os.IsExist(err) {
			return
		}

		_, err = db.Delete(q.Alias(ret.buildID))
		if err != nil {
			return
		}
	}
	if err != nil && err != postdb.ErrNotFound {
		return
	}

	installList := []string{}
	for _, pkg := range options.packages {
		installList = append(installList, pkg.name+"@"+pkg.version)
	}

	start := time.Now()
	importMeta := map[string]*ImportMeta{}
	peerDependencies := map[string]string{}
	for _, pkg := range options.packages {
		var p NpmPackage
		p, err = nodeEnv.getPackageInfo(pkg.name, pkg.version)
		if err != nil {
			return
		}
		meta := &ImportMeta{
			NpmPackage: &p,
		}
		for name, version := range p.PeerDependencies {
			peerDependencies[name] = version
		}
		if meta.Types == "" && meta.Typings == "" && !strings.HasPrefix(pkg.name, "@") {
			var info NpmPackage
			info, err = nodeEnv.getPackageInfo("@types/"+pkg.name, "latest")
			if err == nil {
				if info.Types != "" || info.Typings != "" || info.Main != "" {
					installList = append(installList, fmt.Sprintf("%s@%s", info.Name, info.Version))
				}
			} else if err.Error() != fmt.Sprintf("npm: package '@types/%s' not found", pkg.name) {
				return
			}
		}
		if pkg.submodule != "" {
			meta.Main = pkg.submodule
			meta.Module = ""
			meta.Types = ""
			meta.Typings = ""
		}
		importMeta[pkg.ImportPath()] = meta
	}

	peerPackages := map[string]NpmPackage{}
	for name, version := range peerDependencies {
		peer := true
		for _, pkg := range options.packages {
			if pkg.name == name {
				peer = false
				break
			}
		}
		if peer {
			for _, meta := range importMeta {
				for dep := range meta.Dependencies {
					if dep == name {
						peer = false
						break
					}
				}
			}
		}
		if peer {
			peerPackages[name] = NpmPackage{
				Name: name,
			}
			installList = append(installList, name+"@"+version)
		}
	}

	log.Debugf("parse importMeta in %v", time.Now().Sub(start))

	buildDir := path.Join(os.TempDir(), "esmd-build", rs.Hex.String(16))
	ensureDir(buildDir)
	defer os.RemoveAll(buildDir)

	err = os.Chdir(buildDir)
	if err != nil {
		return
	}

	err = yarnAdd(installList...)
	if err != nil {
		return
	}

	env := "production"
	if options.isDev {
		env = "development"
	}

	start = time.Now()
	buf := bytes.NewBuffer(nil)
	buf.WriteString(`
		const fs = require("fs");
		const meta = {};
		const isObject = v => typeof v === 'object' && v !== null;
	`)
	for _, pkg := range options.packages {
		importPath := pkg.ImportPath()
		meta := importMeta[importPath]
		pkgDir := path.Join(buildDir, "node_modules", meta.Name)
		if pkg.submodule != "" {
			if fileExists(path.Join(pkgDir, pkg.submodule, "package.json")) {
				var p NpmPackage
				err = utils.ParseJSONFile(path.Join(pkgDir, pkg.submodule, "package.json"), &p)
				if err != nil {
					return
				}
				if p.Main != "" {
					meta.Main = path.Join(pkg.submodule, p.Main)
				}
				if p.Module != "" {
					meta.Module = path.Join(pkg.submodule, p.Module)
				}
				if p.Types != "" {
					meta.Types = path.Join(pkg.submodule, p.Types)
				}
				if p.Typings != "" {
					meta.Typings = path.Join(pkg.submodule, p.Typings)
				}
			} else {
				exports, pass, err := parseModuleExports(path.Join(pkgDir, ensureExt(meta.Main, ".js")))
				if err != nil && os.IsNotExist(err) {
					exports, pass, err = parseModuleExports(path.Join(pkgDir, meta.Main, "index.js"))
				}
				if pass {
					meta.Module = meta.Main
					meta.Exports = exports
					continue
				}
			}
		}
		if meta.Module != "" {
			exports, pass, err := parseModuleExports(path.Join(pkgDir, ensureExt(meta.Module, ".js")))
			if err != nil && os.IsNotExist(err) {
				exports, pass, err = parseModuleExports(path.Join(pkgDir, meta.Module, "index.js"))
			}
			if pass {
				meta.Exports = exports
				continue
			}
			// fake module
			meta.Module = ""
		}
		// export commonjs exports
		importIdentifier := identify(importPath)
		fmt.Fprintf(buf, `
			try {
				const %s = require("%s");
				meta["%s"] = {exports: isObject(%s) ? Object.keys(%s) : ['default'] };
			} catch(e) {}
		`, importIdentifier, importPath, importPath, importIdentifier, importIdentifier)
	}
	buf.WriteString(`
		fs.writeFileSync('./peer.output.json', JSON.stringify(meta))
		process.exit(0);
	`)

	cmd := exec.Command("node")
	cmd.Stdin = buf
	cmd.Env = append(os.Environ(), fmt.Sprintf(`NODE_ENV=%s`, env))
	output, err := cmd.CombinedOutput()
	if err == nil {
		var m map[string]ImportMeta
		err = utils.ParseJSONFile("./peer.output.json", &m)
		if err != nil {
			return
		}
		for name, meta := range m {
			_meta, ok := importMeta[name]
			if ok {
				_meta.Exports = meta.Exports
			}
		}
	} else {
		err = fmt.Errorf("nodejs: %s", string(output))
		return
	}

	log.Debug("node peer.js in", time.Now().Sub(start))

	start = time.Now()
	for _, pkg := range options.packages {
		var types string
		meta := importMeta[pkg.ImportPath()]
		nv := fmt.Sprintf("%s@%s", meta.Name, meta.Version)
		if meta.Types != "" || meta.Typings != "" {
			types = getTypesPath(*meta.NpmPackage)
		} else if pkg.submodule == "" {
			if fileExists(path.Join(buildDir, "node_modules", pkg.name, "index.d.ts")) {
				types = fmt.Sprintf("%s/%s", nv, "index.d.ts")
			} else if !strings.HasPrefix(pkg.name, "@") {
				var info NpmPackage
				err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", "@types/"+pkg.name, "package.json"), &info)
				if err == nil {
					types = getTypesPath(info)
				} else if !os.IsNotExist(err) {
					return
				}
			}
		} else {
			if fileExists(path.Join(buildDir, "node_modules", pkg.name, pkg.submodule, "index.d.ts")) {
				types = fmt.Sprintf("%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
			} else if fileExists(path.Join(buildDir, "node_modules", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
				types = fmt.Sprintf("%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
			} else if fileExists(path.Join(buildDir, "node_modules/@types", pkg.name, pkg.submodule, "index.d.ts")) {
				types = fmt.Sprintf("@types/%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
			} else if fileExists(path.Join(buildDir, "node_modules/@types", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
				types = fmt.Sprintf("@types/%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
			}
		}
		if types != "" {
			err = copyDTS(path.Join(buildDir, "node_modules"), path.Join(storageDir, "types"), types)
			if err != nil {
				err = fmt.Errorf("copyDTS(%s): %v", types, err)
				return
			}
			meta.Dts = "/" + types
		}
	}
	log.Debug("copy dts in", time.Now().Sub(start))

	externals := make([]string, len(peerPackages)+len(builtInNodeModules))
	i := 0
	for name := range peerPackages {
		var p NpmPackage
		err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", name, "package.json"), &p)
		if err != nil {
			return
		}
		peerPackages[name] = p
		externals[i] = name
		i++
	}
	for name := range builtInNodeModules {
		var self bool
		for _, pkg := range options.packages {
			if pkg.name == name {
				self = true
			}
		}
		if !self {
			externals[i] = name
			i++
		}
	}
	externals = externals[:i]

	buf = bytes.NewBuffer(nil)
	if single {
		pkg := options.packages[0]
		importPath := pkg.ImportPath()
		importIdentifier := identify(importPath)
		meta := importMeta[importPath]
		exports := []string{}
		hasDefaultExport := false
		for _, name := range meta.Exports {
			if name == "default" {
				hasDefaultExport = true
			} else if name != "import" {
				exports = append(exports, name)
			}
		}
		if meta.Module != "" {
			fmt.Fprintf(buf, `export * from "%s";%s`, importPath, EOL)
			if hasDefaultExport {
				fmt.Fprintf(buf, `export {default} from "%s";`, importPath)
			}
		} else {
			if hasDefaultExport {
				fmt.Fprintf(buf, `import %s from "%s";%s`, importIdentifier, importPath, EOL)
			} else {
				fmt.Fprintf(buf, `import * as %s from "%s";%s`, importIdentifier, importPath, EOL)
			}
			fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(exports, ","), importIdentifier, EOL)
			fmt.Fprintf(buf, `export default %s;`, importIdentifier)
		}
	} else {
		for _, pkg := range options.packages {
			importPath := pkg.ImportPath()
			importIdentifier := identify(importPath)
			meta := importMeta[importPath]
			hasDefaultExport := false
			for _, name := range meta.Exports {
				if name == "default" {
					hasDefaultExport = true
					break
				}
			}
			if meta.Module != "" {
				fmt.Fprintf(buf, `export * as %s_star from "%s";%s`, importIdentifier, importPath, EOL)
				if hasDefaultExport {
					fmt.Fprintf(buf, `export {default as %s_default} from "%s";`, importIdentifier, importPath)
				}
			} else if meta.Main != "" {
				if hasDefaultExport {
					fmt.Fprintf(buf, `import %s from "%s";%s`, importIdentifier, importPath, EOL)
				} else {
					fmt.Fprintf(buf, `import * as %s from "%s";%s`, importIdentifier, importPath, EOL)
				}
				fmt.Fprintf(buf, `export {%s as %s_default};`, importIdentifier, importIdentifier)
			} else {
				fmt.Fprintf(buf, `export const %s_default = null;`, importIdentifier)
			}
		}
	}
	input := &api.StdinOptions{
		Contents:   buf.String(),
		ResolveDir: buildDir,
		Sourcefile: "export.js",
	}
	minify := !options.isDev
	defines := map[string]string{
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, env),
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	missingResolved := map[string]struct{}{}
esbuild:
	start = time.Now()
	peerModulesForCommonjs := map[string]string{}
	result := api.Build(api.BuildOptions{
		Stdin:             input,
		Bundle:            true,
		Write:             false,
		Target:            targets[options.target],
		Format:            api.FormatESModule,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Defines:           defines,
		Plugins: []func(api.Plugin){
			func(plugin api.Plugin) {
				plugin.SetName("rewrite-external-path")
				plugin.AddResolver(
					api.ResolverOptions{Filter: fmt.Sprintf("^(%s)$", strings.Join(externals, "|"))},
					func(args api.ResolverArgs) (api.ResolverResult, error) {
						_, esm, _ := parseModuleExports(args.Importer)
						resolvePath := args.Path
						p, ok := peerPackages[resolvePath]
						if !ok {
							polyfill, yes := polyfilledBuiltInNodeModules[resolvePath]
							if yes {
								var err error
								p, err = nodeEnv.getPackageInfo(polyfill, "latest")
								if err == nil {
									resolvePath = polyfill
									ok = true
								}
							}
						}
						if ok {
							filename := path.Base(resolvePath)
							if options.isDev {
								filename += ".development"
							}
							esmPath := fmt.Sprintf("/%s@%s/%s/%s", resolvePath, p.Version, options.target, ensureExt(filename, ".js"))
							if esm {
								resolvePath = esmPath
							} else {
								peerModulesForCommonjs[resolvePath] = esmPath
							}
						} else {
							if esm {
								resolvePath = fmt.Sprintf("/_error.js?type=resolve&name=%s", url.QueryEscape(resolvePath))
							} else {
								peerModulesForCommonjs[resolvePath] = ""
							}
						}
						return api.ResolverResult{Path: resolvePath, External: true, Namespace: "http"}, nil
					},
				)
			},
		},
	})
	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Text, `Indirect calls to "require" will not be bundled`) {
			log.Warn(w.Text)
		}
	}
	if len(result.Errors) > 0 {
		fe := result.Errors[0]
		if strings.HasPrefix(fe.Text, `Could not resolve "`) {
			missingModule := strings.Split(fe.Text, `"`)[1]
			if missingModule != "" {
				_, ok := missingResolved[missingModule]
				if !ok {
					err = yarnAdd(missingModule)
					if err != nil {
						return
					}
					missingResolved[missingModule] = struct{}{}
					goto esbuild // rebuild
				}
			}
		}
		err = errors.New("esbuild: " + fe.Text)
		return
	}

	log.Debugf("esbuild %s %s %s in %v", options.packages.String(), options.target, env, time.Now().Sub(start))

	var eol, indent string
	if options.isDev {
		indent = "  "
		eol = EOL
	}

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* %s - esbuild bundle(%s) %s %s */%s`, jsCopyrightName, options.packages.String(), strings.ToLower(options.target), env, EOL)

	// nodejs compatibility
	outputContent := result.OutputFiles[0].Contents
	if regProcess.Match(outputContent) {
		fmt.Fprintf(jsContentBuf, `import process from "/_process_browser.js?env=%s";%s`, env, eol)
	}
	if regBuffer.Match(outputContent) {
		p, err := nodeEnv.getPackageInfo("buffer", "latest")
		if err == nil {
			fmt.Fprintf(jsContentBuf, `import Buffer from "/buffer@%s/%s/buffer.js";%s`, p.Version, options.target, eol)
		} else {
			fmt.Fprintf(jsContentBuf, `import Buffer from "/buffer";%s`, eol)
		}
	}
	if len(peerModulesForCommonjs) > 0 {
		var cases []string
		for name, importPath := range peerModulesForCommonjs {
			if importPath != "" {
				identifier := identify(name)
				cases = append(cases, fmt.Sprintf(`case "%s":%s%s%s%sreturn __%s;`, name, eol, indent, indent, indent, identifier))
				fmt.Fprintf(jsContentBuf, `import __%s from "%s";%s`, identifier, importPath, eol)
			}
		}
		fmt.Fprintf(jsContentBuf, `var require = name => {%s`, eol)
		fmt.Fprintf(jsContentBuf, `%sswitch (name) {%s`, indent, eol)
		for _, c := range cases {
			fmt.Fprintf(jsContentBuf, `%s%s%s%s`, indent, indent, c, eol)
		}
		fmt.Fprintf(jsContentBuf, `%s%sdefault:%s`, indent, indent, eol)
		fmt.Fprintf(jsContentBuf, `%s%s%sthrow new Error("[%s] Could not resolve \"" + name + "\"");%s`, indent, indent, indent, jsCopyrightName, eol)
		fmt.Fprintf(jsContentBuf, `%s}%s`, indent, eol)
		fmt.Fprintf(jsContentBuf, `};%s`, eol)
	}
	if regGlobal.Match(outputContent) {
		fmt.Fprintf(jsContentBuf, `if (typeof global === "undefined") var global = window;%s`, eol)
	}

	// esbuild output
	jsContentBuf.Write(outputContent)

	saveFilePath := path.Join(storageDir, "builds", ret.buildID+".js")
	ensureDir(path.Dir(saveFilePath))
	file, err := os.Create(saveFilePath)
	if err != nil {
		return
	}
	defer file.Close()

	_, err = io.Copy(file, jsContentBuf)
	if err != nil {
		return
	}

	db.Put(
		q.Alias(ret.buildID),
		q.Tags("build"),
		q.KV{
			"importMeta": utils.MustEncodeJSON(importMeta),
		},
	)

	ret.importMeta = importMeta
	return
}

func identify(importPath string) string {
	p := []byte(importPath)
	for i, c := range p {
		switch c {
		case '/', '-', '@', '.':
			p[i] = '_'
		default:
			p[i] = c
		}
	}
	return string(p)
}

func getTypesPath(p NpmPackage) string {
	types := ""
	if p.Types != "" {
		types = p.Types
	} else if p.Typings != "" {
		types = p.Typings
	} else if p.Main != "" {
		types = strings.TrimSuffix(p.Main, ".js")
	}
	if types != "" {
		return fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(path.Join("/", types), ".d.ts"))
	}
	return ""
}
