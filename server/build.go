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
	NpmPackage
	Exports []string `json:"exports"`
	Types   string   `json:"types"`
}

type buildOptions struct {
	packages moduleSlice
	target   string
	dev      bool
}

type buildResult struct {
	buildID    string
	importMeta map[string]*ImportMeta
	single     bool
}

func build(storageDir string, options buildOptions) (ret buildResult, err error) {
	buildLock.Lock()
	defer buildLock.Unlock()

	n := len(options.packages)
	if n == 0 {
		err = fmt.Errorf("no packages")
		return
	}

	ret.single = n == 1
	if ret.single {
		pkg := options.packages[0]
		filename := path.Base(pkg.name)
		if pkg.submodule != "" {
			filename = pkg.submodule
		}
		if options.dev {
			filename += ".development"
		}
		ret.buildID = fmt.Sprintf("%s@%s/%s/%s", pkg.name, pkg.version, options.target, filename)
	} else {
		hasher := sha1.New()
		sort.Sort(options.packages)
		fmt.Fprintf(hasher, "%s %s %v", options.packages.String(), options.target, options.dev)
		ret.buildID = "bundle-" + strings.ToLower(base32.StdEncoding.EncodeToString(hasher.Sum(nil)))
	}

	p, err := db.Get(q.Alias(ret.buildID), q.K("hash", "importMeta"))
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
			NpmPackage: p,
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
			meta.Main = pkg.ImportPath()
			meta.Module = ""
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
	if options.dev {
		env = "development"
	}

	start = time.Now()
	codeBuf := bytes.NewBuffer(nil)
	codeBuf.WriteString(`
		const fs = require("fs");
		const meta = {};
		const isObject = v => typeof v === 'object' && v !== null;
	`)
	for _, pkg := range options.packages {
		importPath := pkg.ImportPath()
		meta := importMeta[importPath]
		if pkg.submodule == "" && meta.Module != "" {
			exports, pass, err := parseModuleExports(path.Join(buildDir, "node_modules", meta.Name, ensureExt(meta.Module, ".js")))
			if err != nil && os.IsNotExist(err) {
				exports, pass, err = parseModuleExports(path.Join(buildDir, "node_modules", meta.Name, meta.Module, "index.js"))
			}
			if pass {
				meta.Exports = exports
				continue
			}
			// fake module
			meta.Module = ""
		}
		if pkg.submodule != "" {
			exports, pass, err := parseModuleExports(path.Join(buildDir, "node_modules", meta.Name, ensureExt(meta.Main, ".js")))
			if err != nil && os.IsNotExist(err) {
				exports, pass, err = parseModuleExports(path.Join(buildDir, "node_modules", meta.Name, meta.Main, "index.js"))
			}
			if pass {
				// es submodule
				meta.Module = meta.Main
				meta.Exports = exports
				continue
			}
		}
		// export commonjs exports
		importIdentifier := identify(importPath)
		fmt.Fprintf(codeBuf, `
			try {
				const %s = require("%s");
				meta["%s"] = {exports: isObject(%s) ? Object.keys(%s) : ['default'] };
			} catch(e) {}
		`, importIdentifier, importPath, importPath, importIdentifier, importIdentifier)
	}
	codeBuf.WriteString(`
		fs.writeFileSync('./peer.output.json', JSON.stringify(meta))
		process.exit(0);
	`)

	cmd := exec.Command("node")
	cmd.Stdin = codeBuf
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
		if pkg.submodule == "" {
			if meta.Types == "" && meta.Typings == "" && !strings.HasPrefix(pkg.name, "@") {
				var info NpmPackage
				err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", "@types/"+pkg.name, "package.json"), &info)
				if err == nil {
					types = getTypesPath(info)
				} else if !os.IsNotExist(err) {
					return
				}
			}
			if types == "" {
				types = getTypesPath(meta.NpmPackage)
			}
		} else {
			var p NpmPackage
			err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", pkg.name, pkg.submodule, "package.json"), &p)
			if err == nil {
				var tp string
				if p.Types != "" {
					tp = p.Types
				} else if p.Typings != "" {
					tp = p.Typings
				} else if p.Main != "" {
					tp = strings.TrimSuffix(p.Main, ".js")
				}
				if tp != "" {
					types = fmt.Sprintf("%s/%s", nv, ensureExt(path.Join(pkg.submodule, tp), ".d.ts"))
				}
				if p.PeerDependencies != nil {
					for name := range p.PeerDependencies {
						peerPackages[name] = NpmPackage{
							Name: name,
						}
					}
				}
			}
			if types == "" {
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
		}
		if types != "" {
			err = copyDTS(path.Join(buildDir, "node_modules"), path.Join(storageDir, "types"), types)
			if err != nil {
				err = fmt.Errorf("copyDTS(%s): %v", types, err)
				return
			}
			meta.Types = "/" + types
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
		if p.Main != "" || p.Module != "" {
			peerPackages[name] = p
			externals[i] = name
			i++
		}
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

	codeBuf = bytes.NewBuffer(nil)
	if ret.single {
		pkg := options.packages[0]
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
			fmt.Fprintf(codeBuf, `export * from "%s";%s`, importPath, EOL)
			if hasDefaultExport {
				fmt.Fprintf(codeBuf, `export {default} from "%s";`, importPath)
			}
		} else if meta.Main != "" {
			if hasDefaultExport {
				fmt.Fprintf(codeBuf, `import %s from "%s";%s`, importIdentifier, importPath, EOL)
			} else {
				fmt.Fprintf(codeBuf, `import * as %s from "%s";%s`, importIdentifier, importPath, EOL)
			}
			fmt.Fprintf(codeBuf, `export default %s;`, importIdentifier)
		} else {
			fmt.Fprintf(codeBuf, `export default null;`)
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
				fmt.Fprintf(codeBuf, `export * as %s_star from "%s";%s`, importIdentifier, importPath, EOL)
				if hasDefaultExport {
					fmt.Fprintf(codeBuf, `export {default as %s_default} from "%s";`, importIdentifier, importPath)
				}
			} else if meta.Main != "" {
				if hasDefaultExport {
					fmt.Fprintf(codeBuf, `import %s from "%s";%s`, importIdentifier, importPath, EOL)
				} else {
					fmt.Fprintf(codeBuf, `import * as %s from "%s";%s`, importIdentifier, importPath, EOL)
				}
				fmt.Fprintf(codeBuf, `export {%s as %s_default};`, importIdentifier, importIdentifier)
			} else {
				fmt.Fprintf(codeBuf, `export const %s_default = null;`, importIdentifier)
			}
		}
	}
	input := &api.StdinOptions{
		Contents:   codeBuf.String(),
		ResolveDir: buildDir,
		Sourcefile: "export.js",
	}
	minify := !options.dev
	defines := map[string]string{
		"process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	missingResolved := map[string]struct{}{}
esbuild:
	start = time.Now()
	peerCommonjsModules := map[string]string{}
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
				plugin.SetName("rewrite-path")
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
							if options.dev {
								filename += ".development"
							}
							esmPath := fmt.Sprintf("/%s@%s/%s/%s", resolvePath, p.Version, options.target, ensureExt(filename, ".js"))
							if esm {
								resolvePath = esmPath
							} else {
								peerCommonjsModules[resolvePath] = esmPath
							}
						} else {
							if esm {
								resolvePath = fmt.Sprintf("/_error.js?type=resolve&name=%s", url.QueryEscape(resolvePath))
							} else {
								peerCommonjsModules[resolvePath] = ""
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
			log.Debug(w.Text)
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

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* %s - esbuild bundle(%s) %s %s */%s`, jsCopyrightName, options.packages.String(), strings.ToLower(options.target), env, EOL)

	var peerModules []string
	var eol, indent string
	if options.dev {
		indent = "  "
		eol = EOL
	}

	if len(peerCommonjsModules) > 0 {
		for name, importPath := range peerCommonjsModules {
			if importPath != "" {
				identifier := identify(name)
				peerModules = append(peerModules, fmt.Sprintf(`"%s": %s`, name, identifier))
				fmt.Fprintf(jsContentBuf, `import %s from "%s";%s`, identifier, importPath, eol)
			}
		}
		fmt.Fprintf(jsContentBuf, `var __peerModules = `)
		if len(peerModules) > 0 {
			fmt.Fprintf(jsContentBuf, `{%s`, eol)
			fmt.Fprintf(jsContentBuf, `%s%s%s`, indent, strings.Join(peerModules, fmt.Sprintf(",%s%s", eol, indent)), eol)
			fmt.Fprintf(jsContentBuf, `};%s`, eol)
		} else {
			fmt.Fprintf(jsContentBuf, `{};%s`, eol)
		}
		fmt.Fprintf(jsContentBuf, `var require = name => {%s`, eol)
		fmt.Fprintf(jsContentBuf, `%sif (name in __peerModules) {%s`, indent, eol)
		fmt.Fprintf(jsContentBuf, `%s%sreturn __peerModules[name];%s`, indent, indent, eol)
		fmt.Fprintf(jsContentBuf, `%s}%s`, indent, eol)
		fmt.Fprintf(jsContentBuf, `%sthrow new Error("[%s] Could not resolve \"" + name + "\"");%s`, indent, jsCopyrightName, eol)
		fmt.Fprintf(jsContentBuf, `};%s`, eol)
	}

	// nodejs compatibility
	outputContent := result.OutputFiles[0].Contents
	if containsExp(outputContent, "global.") || containsExp(outputContent, "global[") {
		fmt.Fprintf(jsContentBuf, `if (typeof global === 'undefined') var global = window;%s`, eol)
	}
	if containsExp(outputContent, "process.") {
		fmt.Fprintf(jsContentBuf, `import process from "/_process_browser.js?env=%s";%s`, env, eol)
	}
	if containsExp(outputContent, "Buffer.") {
		fmt.Fprintf(jsContentBuf, `import Buffer from "/buffer";%s`, eol)
	}

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

func containsExp(content []byte, exp string) bool {
	i := bytes.Index(content, []byte(exp))
	if i > 0 {
		c := content[i-1]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '.' {
			return true
		}
	}
	return false
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
