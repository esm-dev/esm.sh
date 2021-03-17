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
	jsCopyrightName    = "esm.sh"
	denoStdNodeVersion = "0.90.0"
)

var (
	buildVersion = 1
)

var targets = map[string]api.Target{
	"deno":   api.ESNext,
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
	Exports            []string `json:"exports"`
	ExportsFromDefault []string `json:"exportsFromDefault"`
	Dts                string   `json:"dts"`
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
	hasCSS     bool
}

func build(storageDir string, hostname string, options buildOptions) (ret buildResult, err error) {
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

	p, err := db.Get(q.Alias(ret.buildID), q.K("importMeta", "css"))
	if err == nil {
		err = json.Unmarshal(p.KV.Get("importMeta"), &ret.importMeta)
		if err != nil {
			_, err = db.Delete(q.Alias(ret.buildID))
			if err != nil {
				return
			}
		}

		if val := p.KV.Get("css"); len(val) == 1 && val[0] == 1 {
			ret.hasCSS = fileExists(path.Join(storageDir, "builds", ret.buildID+".css"))
		}

		if fileExists(path.Join(storageDir, "builds", ret.buildID+".js")) {
			// has built
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

	buildLock.Lock()
	defer buildLock.Unlock()

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

	commonjsModules := newStringSet()
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
				exports, esm, err := parseModuleExports(path.Join(pkgDir, ensureExt(meta.Main, ".js")))
				if err != nil && os.IsNotExist(err) {
					exports, esm, err = parseModuleExports(path.Join(pkgDir, meta.Main, "index.js"))
				}
				if esm {
					meta.Module = meta.Main
					meta.Exports = exports
					continue
				}
			}
		}
		if meta.Module != "" {
			exports, esm, err := parseModuleExports(path.Join(pkgDir, ensureExt(meta.Module, ".js")))
			if err != nil && os.IsNotExist(err) {
				exports, esm, err = parseModuleExports(path.Join(pkgDir, meta.Module, "index.js"))
			}
			if esm {
				meta.Exports = exports
				continue
			}
			// fake module
			meta.Module = ""
		}
		commonjsModules.Add(importPath)
	}

	if commonjsModules.Size() > 0 {
		start := time.Now()
		buf := bytes.NewBuffer(nil)
		buf.WriteString(`
			const fs = require("fs");
			const meta = {};
			const isObject = v => typeof v === 'object' && v !== null && !Array.isArray(v);
		`)
		for _, importPath := range commonjsModules.Values() {
			// export commonjs exports
			js := `
				try {
					const $MOD = require("$PATH");
					const safe = name => !["arguments"].includes(name)
					
					if (isObject($MOD)) {
						if (isObject($MOD.default)) {
							const exports = Object.keys($MOD).filter(safe);
							const exportsFromDefault = Object.keys($MOD.default).filter(safe);
							const onlyExportsFromDefault = exportsFromDefault.filter(d => exports.includes(d));
							meta["$PATH"] = { exports, exportsFromDefault: onlyExportsFromDefault };
						} else {
							const exports = Object.keys($MOD).filter(safe);
							meta["$PATH"] = { exports };
						}
					} else {
						meta["$PATH"] = { exports: ['default'] };
					}
				} catch(e) {}
			`
			js = strings.ReplaceAll(js, "$PATH", importPath)
			js = strings.ReplaceAll(js, "$MOD", identify(importPath))
			buf.WriteString(js)
		}
		buf.WriteString(`
			fs.writeFileSync('./peer.output.json', JSON.stringify(meta))
			process.exit(0);
		`)

		cmd := exec.Command("node")
		cmd.Stdin = buf
		cmd.Env = append(os.Environ(), fmt.Sprintf(`NODE_ENV=%s`, env))
		var output []byte
		output, err = cmd.CombinedOutput()
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
	}

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

	buf := bytes.NewBuffer(nil)
	if single {
		pkg := options.packages[0]
		importPath := pkg.ImportPath()
		importIdentifier := "__" + identify(importPath)
		meta := importMeta[importPath]
		exports := []string{}
		exportsFromDefault := []string{}
		hasDefaultExport := false
		for _, name := range meta.Exports {
			if name == "default" {
				hasDefaultExport = true
			} else if name != "import" {
				exports = append(exports, name)
			}
		}

		for _, name := range meta.ExportsFromDefault {
			if name != "import" {
				exportsFromDefault = append(exportsFromDefault, name)
			}
		}

		if meta.Module != "" {
			fmt.Fprintf(buf, `export * from "%s";%s`, importPath, EOL)
			if hasDefaultExport {
				fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
			}
		} else {
			fmt.Fprintf(buf, `import * as %s from "%s";%s`, importIdentifier, importPath, EOL)

			fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(exports, ","), importIdentifier, EOL)

			if hasDefaultExport {
				if len(exportsFromDefault) > 0 {
					fmt.Fprintf(buf, `export const { %s } = %s.default;%s`, strings.Join(exportsFromDefault, ","), importIdentifier, EOL)
				}
			}

			fmt.Fprintf(buf, `export default %s.default;`, importIdentifier)
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
		"__filename":                  fmt.Sprintf(`"https://%s/%s.js"`, hostname, ret.buildID),
		"__dirname":                   fmt.Sprintf(`"https://%s/%s"`, hostname, path.Dir(ret.buildID)),
		"global":                      "__global$",
		"process":                     "__process$",
		"Buffer":                      "__Buffer$",
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, env),
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	indirectRequires := newStringSet()
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
		Outdir:            "/esbuild",
		Plugins: []api.Plugin{
			{
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
								if options.target == "deno" {
									_, yes := denoStdNodeModules[resolvePath]
									if yes {
										pathname := fmt.Sprintf("https://deno.land/std@%s/node/%s.ts", denoStdNodeVersion, resolvePath)
										if esm {
											resolvePath = pathname
										} else {
											peerModulesForCommonjs.Set(resolvePath, pathname)
										}
										return api.OnResolveResult{Path: resolvePath, External: true, Namespace: "http"}, nil
									}
								}

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
									_, err := embedFS.Open(fmt.Sprintf("polyfills/node_%s.js", resolvePath))
									if err == nil {
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
		if !strings.HasPrefix(w.Text, `Indirect calls to "require" will not be bundled`) {
			log.Warn(w.Text)
		}
	}
	if len(result.Errors) > 0 {
		extraExternals := []string{}
		for _, e := range result.Errors {
			if strings.HasPrefix(e.Text, `Could not resolve "`) {
				missingModule := strings.Split(e.Text, `"`)[1]
				if missingModule != "" {
					if !indirectRequires.Has(missingModule) {
						indirectRequires.Add(missingModule)
						extraExternals = append(extraExternals, missingModule)
					}
				}
			} else {
				err = errors.New("esbuild: " + e.Text)
				return
			}
		}
		if len(extraExternals) > 0 {
			externals = append(externals, extraExternals...)
			goto esbuild // rebuild
		}
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

	hasCSS := []byte{0}
	for _, file := range result.OutputFiles {
		outputContent := file.Contents
		if strings.HasSuffix(file.Path, ".js") {
			// add nodejs/deno compatibility
			if bytes.Contains(outputContent, []byte("__process$")) {
				if options.target == "deno" {
					fmt.Fprintf(jsContentBuf, `import __process$ from "https://deno.land/std@%s/node/process.ts";%s`, denoStdNodeVersion, eol)
				} else {
					fmt.Fprintf(jsContentBuf, `import __process$ from "/v%d/_node_process.js";%s__process$.env.NODE_ENV="%s";%s`, buildVersion, eol, env, eol)
				}
			}
			if bytes.Contains(outputContent, []byte("__Buffer$")) {
				if options.target == "deno" {
					fmt.Fprintf(jsContentBuf, `import { Buffer as __Buffer$ } from "https://deno.land/std@%s/node/buffer.ts";%s`, denoStdNodeVersion, eol)
				} else {
					fmt.Fprintf(jsContentBuf, `import { Buffer as __Buffer$ } from "/v%d/_node_buffer.js";%s`, buildVersion, eol)
				}
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

			if bytes.Contains(outputContent, []byte("__global$")) {
				fmt.Fprintf(jsContentBuf, `if (typeof __global$ === "undefined") var __global$ = window;%s`, eol)
			}

			// esbuild output
			jsContentBuf.Write(outputContent)
		} else if strings.HasSuffix(file.Path, ".css") {
			saveFilePath := path.Join(storageDir, "builds", ret.buildID+".css")
			ensureDir(path.Dir(saveFilePath))
			file, e := os.Create(saveFilePath)
			if e != nil {
				err = e
				return
			}
			defer file.Close()

			_, err = io.Copy(file, bytes.NewReader(outputContent))
			if err != nil {
				return
			}
			hasCSS = []byte{1}
		}
	}

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
			"css":        hasCSS,
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
	var types string
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
		} else {
			types = "index.d.ts"
		}
	}
	return fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(path.Join("/", types), ".d.ts"))
}
