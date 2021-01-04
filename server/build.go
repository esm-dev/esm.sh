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

var (
	buildVersion = 1
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
	external moduleSlice
	target   string
	isDev    bool
}

type buildResult struct {
	buildID    string
	importMeta map[string]*ImportMeta
}

func build(storageDir string, hostname string, options buildOptions) (ret buildResult, err error) {
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
		target := options.target
		if len(options.external) > 0 {
			target = fmt.Sprintf("external=%s/%s", strings.ReplaceAll(options.external.String(), "/", "_"), target)
		}
		if pkg.submodule != "" {
			filename = pkg.submodule
		}
		if options.isDev {
			filename += ".development"
		}
		ret.buildID = fmt.Sprintf("v%d/%s@%s/%s/%s", buildVersion, pkg.name, pkg.version, target, filename)
	} else {
		hash := sha1.New()
		sort.Sort(options.packages)
		sort.Sort(options.external)
		fmt.Fprintf(hash, "v%d/%s/%s/%s/%v", buildVersion, options.packages.String(), options.external.String(), options.target, options.isDev)
		ret.buildID = "bundle-" + strings.ToLower(base32.StdEncoding.EncodeToString(hash.Sum(nil)))
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
			// built
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
			if name == "react" && p.Name == "react-dom" {
				version = p.Version
			}
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
			for _, m := range options.external {
				if m.name == name {
					version = m.version
					break
				}
			}
			installList = append(installList, name+"@"+version)
		}
	}

	log.Debugf("parse importMeta in %v", time.Now().Sub(start))

	buildDir := path.Join(os.TempDir(), "esmd-build", rs.Hex.String(16))
	nodeModulesDir := path.Join(buildDir, "node_modules")
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
		pkgDir := path.Join(nodeModulesDir, meta.Name)
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
			types = getTypesPath(nodeModulesDir, *meta.NpmPackage, "")
		} else if pkg.submodule == "" {
			if fileExists(path.Join(nodeModulesDir, pkg.name, "index.d.ts")) {
				types = fmt.Sprintf("%s/%s", nv, "index.d.ts")
			} else if !strings.HasPrefix(pkg.name, "@") {
				var info NpmPackage
				err = utils.ParseJSONFile(path.Join(nodeModulesDir, "@types", pkg.name, "package.json"), &info)
				if err == nil {
					types = getTypesPath(nodeModulesDir, info, "")
				} else if !os.IsNotExist(err) {
					return
				}
			}
		} else {
			if fileExists(path.Join(nodeModulesDir, pkg.name, pkg.submodule, "index.d.ts")) {
				types = fmt.Sprintf("%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
			} else if fileExists(path.Join(nodeModulesDir, pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
				types = fmt.Sprintf("%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
			} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, pkg.submodule, "index.d.ts")) {
				types = fmt.Sprintf("@types/%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
			} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
				types = fmt.Sprintf("@types/%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
			}
		}
		if types != "" {
			err = copyDTS(options.external, hostname, nodeModulesDir, path.Join(storageDir, "types", fmt.Sprintf("v%d", buildVersion)), types)
			if err != nil {
				err = fmt.Errorf("copyDTS(%s): %v", types, err)
				return
			}
			meta.Dts = "/" + types
		}
	}
	log.Debug("copy dts in", time.Now().Sub(start))

	externals := make([]string, len(peerPackages)+len(builtInNodeModules)+len(options.external))
	i := 0
	for name := range peerPackages {
		var p NpmPackage
		err = utils.ParseJSONFile(path.Join(nodeModulesDir, name, "package.json"), &p)
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
	for _, m := range options.external {
		var self bool
		for _, pkg := range options.packages {
			if pkg.name == m.name {
				self = true
			}
		}
		if !self {
			externals[i] = m.name
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
	define := map[string]string{
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, env),
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	missingResolved := newStringSet()
esbuild:
	start = time.Now()
	peerModulesForCommonjs := newStringMap()
	result := api.Build(api.BuildOptions{
		Stdin:             input,
		Bundle:            true,
		Write:             false,
		Target:            targets[options.target],
		Format:            api.FormatESModule,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Define:            define,
		Plugins: []api.Plugin{
			api.Plugin{
				Name: "rewrite-external-path",
				Setup: func(plugin api.PluginBuild) {
					plugin.OnResolve(
						api.OnResolveOptions{Filter: fmt.Sprintf("^(%s)$", strings.Join(externals, "|"))},
						func(args api.OnResolveArgs) (api.OnResolveResult, error) {
							_, esm, _ := parseModuleExports(args.Importer)
							resolvePath := args.Path
							var version string
							var ok bool
							if !ok {
								m, yes := options.external.Get(resolvePath)
								if yes {
									version = m.version
									ok = true
								}
							}
							if !ok {
								p, yes := peerPackages[resolvePath]
								if yes {
									version = p.Version
									ok = true
								}
							}
							if !ok {
								polyfill, yes := polyfilledBuiltInNodeModules[resolvePath]
								if yes {
									p, err := nodeEnv.getPackageInfo(polyfill, "latest")
									if err == nil {
										resolvePath = polyfill
										version = p.Version
										ok = true
									} else {
										return api.OnResolveResult{Path: resolvePath}, err
									}
								} else {
									_, yes := polyfills[fmt.Sprintf("node_%s.js", resolvePath)]
									if yes {
										pathname := fmt.Sprintf("/v%d/_node_%s.js", buildVersion, resolvePath)
										if esm {
											resolvePath = pathname
										} else {
											peerModulesForCommonjs.Set(resolvePath, pathname)
										}
										return api.OnResolveResult{Path: resolvePath, External: true, Namespace: "http"}, nil
									}
								}
							}
							if ok {
								packageName := resolvePath
								if !strings.HasPrefix(packageName, "@") {
									packageName, _ = utils.SplitByFirstByte(packageName, '/')
								}
								filename := path.Base(resolvePath)
								if options.isDev {
									filename += ".development"
								}
								pathname := fmt.Sprintf("/v%d/%s@%s/%s/%s", buildVersion, packageName, version, options.target, ensureExt(filename, ".js"))
								if esm {
									resolvePath = pathname
								} else {
									peerModulesForCommonjs.Set(resolvePath, pathname)
								}
							} else {
								if esm {
									if hostname != "localhost" {
										resolvePath = fmt.Sprintf("https://%s/_error.js?type=resolve&name=%s", hostname, url.QueryEscape(resolvePath))
									} else {
										resolvePath = fmt.Sprintf("/_error.js?type=resolve&name=%s", url.QueryEscape(resolvePath))
									}
								} else {
									peerModulesForCommonjs.Set(resolvePath, "")
								}
							}
							return api.OnResolveResult{Path: resolvePath, External: true, Namespace: "http"}, nil
						},
					)
				},
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
				if !missingResolved.Has(missingModule) {
					err = yarnAdd(missingModule)
					if err != nil {
						return
					}
					missingResolved.Set(missingModule)
					goto esbuild // rebuild
				}
			}
		}
		err = errors.New("esbuild: " + fe.Text)
		return
	}

	log.Debugf("esbuild %s %s %s in %v", options.packages.String(), options.target, env, time.Now().Sub(start))

	var eol string
	if options.isDev {
		eol = EOL
	}

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* %s - esbuild bundle(%s) %s %s */%s`, jsCopyrightName, options.packages.String(), strings.ToLower(options.target), env, EOL)
	if options.isDev {
		for _, pkg := range options.packages {
			importPath := pkg.ImportPath()
			meta := importMeta[importPath]
			if len(meta.Dependencies) > 0 {
				if single {
					fmt.Fprintf(jsContentBuf, `/*%s * bundled dependencies:%s`, EOL, EOL)
				} else {
					fmt.Fprintf(jsContentBuf, `/*%s * bundled dependencies of %s:%s`, EOL, pkg.name, EOL)
				}
				for name, version := range meta.Dependencies {
					fmt.Fprintf(jsContentBuf, ` *   - %s: %s%s`, name, version, EOL)
				}
				fmt.Fprintf(jsContentBuf, ` */%s`, EOL)
			}
		}
	}

	// nodejs compatibility
	outputContent := result.OutputFiles[0].Contents
	if regProcess.Match(outputContent) {
		fmt.Fprintf(jsContentBuf, `import process from "/v%d/_process_browser.js";%sprocess.env.NODE_ENV="%s";%s`, buildVersion, eol, env, eol)
	}
	if regBuffer.Match(outputContent) {
		fmt.Fprintf(jsContentBuf, `import { Buffer } from "/v%d/_node_buffer.js";%s`, buildVersion, eol)
	}
	if peerModulesForCommonjs.Size() > 0 {
		for _, entry := range peerModulesForCommonjs.Entries() {
			name, importPath := entry[0], entry[1]
			if importPath != "" {
				identifier := identify(name)
				fmt.Fprintf(jsContentBuf, `import __%s$ from "%s";%s`, identifier, importPath, eol)
				outputContent = bytes.ReplaceAll(outputContent, []byte(fmt.Sprintf("require(\"%s\")", name)), []byte(fmt.Sprintf("__%s$", identifier)))
			}
		}
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

func getTypesPath(nodeModulesDir string, p NpmPackage, subpath string) string {
	types := ""
	if subpath != "" {
		var subpkg NpmPackage
		var subtypes string
		subpkgJSONFile := path.Join(nodeModulesDir, p.Name, subpath, "package.json")
		if fileExists(subpkgJSONFile) && utils.ParseJSONFile(subpkgJSONFile, &subpkg) == nil {
			if subpkg.Types != "" {
				subtypes = subpkg.Types
			} else if subpkg.Typings != "" {
				subtypes = subpkg.Typings
			}
		}
		if subtypes != "" {
			types = path.Join("/", subpath, subtypes)
		} else {
			types = subpath
		}
	} else {
		if p.Types != "" {
			types = p.Types
		} else if p.Typings != "" {
			types = p.Typings
		} else if p.Main != "" {
			types = strings.TrimSuffix(p.Main, ".js")
		}
	}
	if types != "" {
		return fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(path.Join("/", types), ".d.ts"))
	}
	return ""
}
