package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type ESMBuild struct {
	NamedExports     []string `json:"-"`
	HasExportDefault bool     `json:"d,omitempty"`
	FromCJS          bool     `json:"c,omitempty"`
	Dts              string   `json:"t,omitempty"`
	TypesOnly        bool     `json:"o,omitempty"`
	PackageCSS       bool     `json:"s,omitempty"`
	Deps             []string `json:"p,omitempty"`
}

type BuildTask struct {
	Args         BuildArgs
	Pkg          Pkg
	CdnOrigin    string
	Target       string
	BuildVersion int
	Dev          bool
	Bundle       bool
	Deprecated   string
	// internal
	lock        sync.Mutex
	id          string
	stage       string
	wd          string
	realWd      string
	installDir  string
	imports     []string
	requires    [][2]string
	headerLines int // to fix the source map
	esm         *ESMBuild
	npm         NpmPackageInfo
}

func (task *BuildTask) Build() (esm *ESMBuild, err error) {
	// check request package
	if !task.Pkg.FromEsmsh && !task.Pkg.FromGithub {
		var p NpmPackageInfo
		p, _, err = getPackageInfo("", task.Pkg.Name, task.Pkg.Version)
		if err != nil {
			return
		}
		task.Deprecated = p.Deprecated
	}

	pkgVersionName := task.Pkg.VersionName()
	if task.wd == "" {
		task.wd = path.Join(cfg.WorkDir, fmt.Sprintf("npm/%s", pkgVersionName))
		err = ensureDir(task.wd)
		if err != nil {
			return
		}

		if cfg.NpmRegistry != "" || cfg.NpmToken != "" || (cfg.NpmUser != "" && cfg.NpmPassword != "") {
			rcFilePath := path.Join(task.wd, ".npmrc")
			if !fileExists(rcFilePath) {
				var output bytes.Buffer

				if cfg.NpmRegistryScope != "" && cfg.NpmRegistry != "" {
					output.WriteString(fmt.Sprintf("%s:registry=%s\n", cfg.NpmRegistryScope, cfg.NpmRegistry))
				} else if cfg.NpmRegistryScope == "" && cfg.NpmRegistry != "" {
					output.WriteString(fmt.Sprintf("registry=%s\n", cfg.NpmRegistry))
				}

				if cfg.NpmRegistry != "" && cfg.NpmToken != "" {
					var tokenReg string
					tokenReg, err = removeHttpPrefix(cfg.NpmRegistry)
					if err != nil {
						log.Errorf("Invalid npm registry in config: %v", err)
						return
					}
					output.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN}\n", tokenReg))
				}

				if cfg.NpmRegistry != "" && cfg.NpmUser != "" && cfg.NpmPassword != "" {
					var tokenReg string
					tokenReg, err = removeHttpPrefix(cfg.NpmRegistry)
					if err != nil {
						log.Errorf("Invalid npm registry in config: %v", err)
						return
					}
					output.WriteString(fmt.Sprintf("%s:username=${ESM_NPM_USER}\n", tokenReg))
					output.WriteString(fmt.Sprintf("%s:_password=${ESM_NPM_PASSWORD}\n", tokenReg))
				}
				err = os.WriteFile(rcFilePath, output.Bytes(), 0644)
				if err != nil {
					log.Errorf("Failed to create .npmrc file: %v", err)
					return
				}
			}
		}
	}

	defer func(dir string, pkgVersionName string) {
		v, loaded := purgeTimers.LoadAndDelete(pkgVersionName)
		if loaded {
			v.(*time.Timer).Stop()
		}
		toPurge(pkgVersionName, dir)
	}(task.wd, pkgVersionName)

	task.stage = "install"

	err = installPackage(task.wd, task.Pkg)
	if err != nil {
		return
	}

	if l, e := filepath.EvalSymlinks(path.Join(task.wd, "node_modules", task.Pkg.Name)); e == nil {
		task.realWd = l
		if task.Pkg.FromGithub || strings.HasPrefix(task.Pkg.Name, "@") {
			task.installDir = path.Join(l, "../../..")
		} else {
			task.installDir = path.Join(l, "../..")
		}
	} else {
		task.realWd = task.wd
		task.installDir = task.wd
	}

	if task.Target == "raw" {
		return
	}

	task.stage = "build"
	err = task.build()
	if err != nil {
		return
	}

	return task.esm, nil
}

func (task *BuildTask) build() (err error) {
	// build json
	if strings.HasSuffix(task.Pkg.Submodule, ".json") {
		nmDir := path.Join(task.wd, "node_modules")
		jsonPath := path.Join(nmDir, task.Pkg.Name, task.Pkg.Submodule)
		if fileExists(jsonPath) {
			json, err := os.ReadFile(jsonPath)
			if err != nil {
				return err
			}
			buffer := bytes.NewBufferString("export default ")
			buffer.Write(json)
			_, err = fs.WriteFile(task.getSavepath(), buffer)
			if err != nil {
				return err
			}
			task.esm = &ESMBuild{
				HasExportDefault: true,
			}
			task.storeToDB()
			return nil
		}
	}

	esm, npm, reexport, err := task.analyze(false)
	if err != nil && !strings.HasPrefix(err.Error(), "cjsLexer: Can't resolve") {
		return
	}
	task.npm = npm
	task.esm = esm

	if task.Target == "types" {
		if npm.Types != "" {
			dts := npm.Name + "@" + npm.Version + path.Join("/", npm.Types)
			task.buildDTS(dts)
		}
		return
	}

	if esm.TypesOnly {
		dts := npm.Name + "@" + npm.Version + path.Join("/", npm.Types)
		esm.Dts = fmt.Sprintf("/v%d%s/%s", task.BuildVersion, task.ghPrefix(), dts)
		task.buildDTS(dts)
		task.storeToDB()
		return
	}

	// cjs reexport
	if reexport != "" {
		pkg, _, formJson, e := task.getPackageInfo(reexport)
		if e != nil {
			err = e
			return
		}
		// Check if the package has default export
		t := &BuildTask{
			Args:   task.Args,
			Pkg:    pkg,
			Target: task.Target,
			Dev:    task.Dev,
			wd:     task.installDir,
		}
		if !formJson {
			err = installPackage(task.wd, t.Pkg)
			if err != nil {
				return
			}
		}
		m, _, _, e := t.analyze(false)
		if e != nil {
			err = e
			return
		}

		buf := bytes.NewBuffer(nil)
		importPath := task.getImportPath(t.Pkg, encodeBuildArgsPrefix(task.Args, task.Pkg, false))
		fmt.Fprintf(buf, `export * from "%s";`, importPath)
		if m.HasExportDefault {
			fmt.Fprintf(buf, "\n")
			fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
		}

		_, err = fs.WriteFile(task.getSavepath(), buf)
		if err != nil {
			return
		}
		task.checkDTS()
		task.storeToDB()
		return
	}

	defer func() {
		if err != nil {
			esm = nil
		}
	}()

	var entryPoint string
	var input *api.StdinOptions

	if npm.Module == "" {
		buf := bytes.NewBuffer(nil)
		importPath := task.Pkg.ImportPath()
		fmt.Fprintf(buf, `import * as __module from "%s";`, importPath)
		if len(esm.NamedExports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = __module;`, strings.Join(esm.NamedExports, ","))
		}
		fmt.Fprintf(buf, "const { default: __default, ...__rest } = __module;")
		fmt.Fprintf(buf, "export default (__default !== undefined ? __default : __rest);")
		// Default reexport all members from original module to prevent missing named exports members
		fmt.Fprintf(buf, `export * from "%s";`, importPath)
		input = &api.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: task.wd,
			Sourcefile: "build.js",
		}
	} else {
		if task.Args.exports.Len() > 0 {
			buf := bytes.NewBuffer(nil)
			importPath := task.Pkg.ImportPath()
			fmt.Fprintf(buf, `export { %s } from "%s";`, strings.Join(task.Args.exports.Values(), ","), importPath)
			input = &api.StdinOptions{
				Contents:   buf.String(),
				ResolveDir: task.wd,
				Sourcefile: "build.js",
			}
		} else {
			entryPoint = path.Join(task.wd, "node_modules", npm.Name, npm.Module)
		}
	}

	nodeEnv := "production"
	if task.Dev {
		nodeEnv = "development"
	}
	define := map[string]string{
		"__filename":                  fmt.Sprintf(`"/_virtual/esm.sh/%s"`, task.ID()),
		"__dirname":                   fmt.Sprintf(`"/_virtual/esm.sh/%s"`, path.Dir(task.ID())),
		"Buffer":                      "__Buffer$",
		"process":                     "__Process$",
		"setImmediate":                "__setImmediate$",
		"clearImmediate":              "clearTimeout",
		"require.resolve":             "__rResolve$",
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, nodeEnv),
		"global":                      "__global$",
		"global.Buffer":               "__Buffer$",
		"global.process":              "__Process$",
		"global.setImmediate":         "__setImmediate$",
		"global.clearImmediate":       "clearTimeout",
		"global.require.resolve":      "__rResolve$",
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
	}
	if task.Target == "node" {
		define = map[string]string{}
	}
	browserExclude := map[string]*stringSet{}
	implicitExternal := newStringSet()

rebuild:
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Format:            api.FormatESModule,
		Target:            targets[task.Target],
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  !task.Dev,
		MinifyIdentifiers: !task.Dev,
		MinifySyntax:      !task.Dev,
		KeepNames:         task.Args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: task.Args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        task.Args.conditions.Values(),
		// prevent features that can not be polyfilled
		Supported: map[string]bool{
			"bigint":          true,
			"top-level-await": true,
		},
		Plugins: []api.Plugin{{
			Name: "esm",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(
					api.OnResolveOptions{Filter: ".*"},
					func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						if strings.HasPrefix(args.Path, "file:") {
							return api.OnResolveResult{
								Path:     fmt.Sprintf("%s/error.js?type=unsupported-file-dependency&name=%s&importer=%s", cfg.CdnBasePath, strings.TrimPrefix(args.Path, "file:"), task.Pkg),
								External: true,
							}, nil
						}

						if strings.HasPrefix(args.Path, "data:") || strings.HasPrefix(args.Path, "https:") || strings.HasPrefix(args.Path, "http:") {
							return api.OnResolveResult{Path: args.Path, External: true}, nil
						}

						// `?ignore-require`
						if task.Args.ignoreRequire && args.Kind == api.ResolveJSRequireCall && npm.Module != "" {
							return api.OnResolveResult{Path: args.Path, External: true}, nil
						}

						if implicitExternal.Has(args.Path) {
							return api.OnResolveResult{Path: task.resolveExternal(args.Path, args.Kind), External: true}, nil
						}

						// externalize yarn PnP API
						if args.Path == "pnpapi" {
							return api.OnResolveResult{Path: args.Path, Namespace: "browser-exclude"}, nil
						}

						// clean up specifier
						specifier := strings.TrimSuffix(args.Path, "/")
						specifier = strings.TrimPrefix(specifier, "node:")
						specifier = strings.TrimPrefix(specifier, "npm:")

						// use `imports` field of package.json
						if v, ok := npm.Imports[specifier]; ok {
							if s, ok := v.(string); ok {
								specifier = s
							} else if m, ok := v.(map[string]interface{}); ok {
								targets := []string{"browser", "default", "node"}
								if task.isServerTarget() {
									targets = []string{"node", "default", "browser"}
								}
								for _, t := range targets {
									if v, ok := m[t]; ok {
										if s, ok := v.(string); ok {
											specifier = s
											break
										}
									}
								}
							}
						}

						// use `browser` field of package.json
						if len(npm.Browser) > 0 && !task.isServerTarget() {
							spec := specifier
							if strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || specifier == ".." {
								fullFilepath := filepath.Join(args.ResolveDir, specifier)
								spec = "." + strings.TrimPrefix(fullFilepath, path.Join(task.installDir, "node_modules", npm.Name))
							}
							if _, ok := npm.Browser[spec]; !ok && path.Ext(spec) == "" {
								spec += ".js"
							}
							if name, ok := npm.Browser[spec]; ok {
								if name == "" {
									// browser exclude
									return api.OnResolveResult{Path: args.Path, Namespace: "browser-exclude"}, nil
								}
								if strings.HasPrefix(name, "./") {
									specifier = path.Join(task.installDir, "node_modules", npm.Name, name)
								} else {
									specifier = name
								}
							}
						}

						// use `?alias` query
						if len(task.Args.alias) > 0 {
							if name, ok := task.Args.alias[specifier]; ok {
								specifier = name
							} else {
								pkgName, _, subpath := splitPkgPath(specifier)
								if subpath != "" {
									if name, ok := task.Args.alias[pkgName]; ok {
										specifier = name + "/" + subpath
									}
								}
							}
						}

						// externalize native node packages like fsevent
						for _, name := range nativeNodePackages {
							if specifier == name || strings.HasPrefix(specifier, name+"/") {
								if task.isDenoTarget() {
									pkgName, _, subPath := splitPkgPath(specifier)
									version := "latest"
									if pkgName == task.Pkg.Name {
										version = task.Pkg.Version
									} else if v, ok := npm.Dependencies[pkgName]; ok {
										version = v
									} else if v, ok := npm.PeerDependencies[pkgName]; ok {
										version = v
									}
									if !regexpFullVersion.MatchString(version) {
										p, _, err := getPackageInfo(task.installDir, pkgName, version)
										if err == nil {
											version = p.Version
										}
									}
									if err == nil {
										pkg := Pkg{
											Name:      pkgName,
											Version:   version,
											Subpath:   subPath,
											Submodule: toModuleName(subPath),
										}
										return api.OnResolveResult{Path: fmt.Sprintf("npm:%s", pkg.String()), External: true}, nil
									}
								}
								return api.OnResolveResult{
									Path:     fmt.Sprintf("%s/error.js?type=unsupported-npm-package&name=%s&importer=%s", cfg.CdnBasePath, specifier, task.Pkg),
									External: true,
								}, nil
							}
						}

						var fullFilepath string
						if isLocalSpecifier(specifier) {
							fullFilepath = filepath.Join(args.ResolveDir, specifier)
						} else {
							fullFilepath = filepath.Join(task.installDir, "node_modules", specifier)
						}

						if strings.HasSuffix(fullFilepath, ".node") && fileExists(fullFilepath) {
							return api.OnResolveResult{
								Path:     fmt.Sprintf("%s/error.js?type=unsupported-node-native-module&name=%s&importer=%s", cfg.CdnBasePath, path.Base(args.Path), task.Pkg),
								External: true,
							}, nil
						}

						if strings.HasSuffix(fullFilepath, ".json") && fileExists(fullFilepath) {
							return api.OnResolveResult{Path: fullFilepath}, nil
						}

						if strings.HasSuffix(fullFilepath, ".wasm") && fileExists(fullFilepath) {
							return api.OnResolveResult{Path: fullFilepath, Namespace: "wasm"}, nil
						}

						// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query
						if task.Bundle && !task.Args.external.Has(getPkgName(specifier)) && !implicitExternal.Has(specifier) {
							if internalNodeModules[specifier] {
								if task.isServerTarget() {
									return api.OnResolveResult{Path: task.resolveExternal(specifier, args.Kind), External: true}, nil
								}
								data, err := embedFS.ReadFile(("server/embed/polyfills/node_" + specifier))
								if err == nil {
									return api.OnResolveResult{
										Path:       "embed:polyfills/node_" + specifier,
										Namespace:  "embed",
										PluginData: data,
									}, nil
								}
							}
							pkgName := getPkgName(specifier)
							if !internalNodeModules[pkgName] {
								_, ok := npm.PeerDependencies[pkgName]
								if !ok {
									return api.OnResolveResult{}, nil
								}
							}
						}

						// resolve github dependencies
						if v, ok := npm.Dependencies[specifier]; ok && (strings.HasPrefix(v, "git+ssh://") || strings.HasPrefix(v, "git+https://") || strings.HasPrefix(v, "git://")) {
							gitUrl, err := url.Parse(v)
							if err == nil && gitUrl.Hostname() == "github.com" {
								repo := strings.TrimSuffix(gitUrl.Path[1:], ".git")
								if gitUrl.Scheme == "git+ssh" {
									repo = gitUrl.Port() + "/" + repo
								}
								path := fmt.Sprintf("/v%d/gh/%s", task.BuildVersion, repo)
								if gitUrl.Fragment != "" {
									path += "@" + url.QueryEscape(gitUrl.Fragment)
								}
								return api.OnResolveResult{
									Path:     path,
									External: true,
								}, nil
							}
						}

						// externalize the _parent_ module
						// e.g. "react/jsx-runtime" imports "react"
						if task.Pkg.Submodule != "" && task.Pkg.Name == specifier {
							return api.OnResolveResult{Path: task.resolveExternal(specifier, args.Kind), External: true}, nil
						}

						// bundle the module it self and the entrypoint
						if specifier == entryPoint || specifier == task.Pkg.ImportPath() || specifier == path.Join(npm.Name, npm.Module) || specifier == path.Join(npm.Name, npm.Main) {
							return api.OnResolveResult{}, nil
						}

						if isLocalSpecifier(specifier) {
							// is sub-module of current package and non-dynamic import
							if strings.HasPrefix(fullFilepath, task.realWd) && args.Kind != api.ResolveJSDynamicImport {
								relPath := "." + strings.TrimPrefix(fullFilepath, path.Join(task.installDir, "node_modules", npm.Name))
								// split modules based on the `exports` defines in package.json,
								// see https://nodejs.org/api/packages.html
								if om, ok := npm.PkgExports.(*orderedMap); ok {
									modName := relPath
									if strings.HasSuffix(modName, ".mjs") {
										modName = strings.TrimSuffix(specifier, ".mjs")
									} else {
										modName = strings.TrimSuffix(modName, ".js")
									}
									for e := om.l.Front(); e != nil; e = e.Next() {
										name, paths := om.Entry(e)
										m, ok := paths.(*orderedMap)
										if ok && name != "." {
											for e := m.l.Front(); e != nil; e = e.Next() {
												_, value := om.Entry(e)
												s, ok := value.(string)
												if ok && s != "" {
													match := modName == s || modName+".js" == s || modName+".mjs" == s
													if !match {
														if a := strings.Split(s, "*"); len(a) == 2 {
															prefix, suffix := a[0], a[1]
															if strings.HasPrefix(modName, prefix) &&
																(strings.HasSuffix(modName, suffix) ||
																	strings.HasSuffix(modName+".js", suffix) ||
																	strings.HasSuffix(modName+".mjs", suffix)) {
																matchName := strings.TrimPrefix(strings.TrimSuffix(modName, suffix), prefix)
																name = strings.Replace(name, "*", matchName, -1)
																match = true
															}
														}
													}
													if match {
														url := path.Join(npm.Name, name)
														if i := task.Pkg.ImportPath(); url != i && url != i+"/index" {
															return api.OnResolveResult{Path: task.resolveExternal(url, args.Kind), External: true}, nil
														}
													}
												}
											}
										}
									}
								}

								// split the sub module that is in the `sideEffects` field(as list)
								if npm.SideEffects != nil {
									if npm.SideEffects.Has(relPath) || npm.SideEffects.Has(strings.TrimPrefix(relPath, "./")) {
										url := path.Join(npm.Name, relPath)
										return api.OnResolveResult{Path: task.resolveExternal(url, args.Kind), External: true}, nil
									}
								}

								// split the module that is an alias of a dependency
								// means this file just include a single line(js): `export * from "dep"`
								fi, ioErr := os.Lstat(fullFilepath)
								if ioErr == nil && fi.Size() < 256 {
									data, ioErr := os.ReadFile(fullFilepath)
									if ioErr == nil {
										out, esbErr := minify(string(data), api.ESNext, api.LoaderJS)
										if esbErr == nil {
											p := bytes.Split(out, []byte("\""))
											if len(p) == 3 && string(p[0]) == "export*from" && string(p[2]) == ";\n" {
												url := string(p[1])
												if !isLocalSpecifier(url) {
													return api.OnResolveResult{Path: task.resolveExternal(url, args.Kind), External: true}, nil
												}
											}
										}
									}
								}

								return api.OnResolveResult{}, nil
							}

							specifier = strings.TrimPrefix(fullFilepath, filepath.Join(task.installDir, "node_modules")+"/")
						}

						// dynamic external
						return api.OnResolveResult{Path: task.resolveExternal(specifier, args.Kind), External: true}, nil
					},
				)

				// for embed module bundle
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "embed"},
					func(args api.OnLoadArgs) (api.OnLoadResult, error) {
						data := args.PluginData.([]byte)
						contents := string(data)
						return api.OnLoadResult{
							Contents: &contents,
							Loader:   api.LoaderJS,
						}, nil
					},
				)

				// for wasm module exclude
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "wasm"},
					func(args api.OnLoadArgs) (ret api.OnLoadResult, err error) {
						wasm, err := os.ReadFile(args.Path)
						if err != nil {
							return
						}
						wasm64 := base64.StdEncoding.EncodeToString(wasm)
						code := fmt.Sprintf("export default new WebAssembly.Module(Uint8Array.from(atob('%s'), c => c.charCodeAt(0)))", wasm64)
						return api.OnLoadResult{Contents: &code, Loader: api.LoaderJS}, nil
					},
				)

				// for browser exclude
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "browser-exclude"},
					func(args api.OnLoadArgs) (ret api.OnLoadResult, err error) {
						contents := "export default {};"
						if exports, ok := browserExclude[args.Path]; ok {
							for _, name := range exports.Values() {
								contents = fmt.Sprintf("%sexport const %s = {};", contents, name)
							}
						}
						return api.OnLoadResult{Contents: &contents, Loader: api.LoaderJS}, nil
					},
				)
			},
		}},
		// for css bundling
		Loader: map[string]api.Loader{
			".svg":   api.LoaderDataURL,
			".png":   api.LoaderDataURL,
			".webp":  api.LoaderDataURL,
			".gif":   api.LoaderDataURL,
			".ttf":   api.LoaderDataURL,
			".eot":   api.LoaderDataURL,
			".woff":  api.LoaderDataURL,
			".woff2": api.LoaderDataURL,
		},
		SourceRoot: "/",
		Sourcemap:  api.SourceMapExternal,
	}
	if task.Target == "node" {
		options.Platform = api.PlatformNode
	} else {
		options.Define = define
	}
	if input != nil {
		options.Stdin = input
	} else if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
	}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := result.Errors[0].Text
		if strings.HasPrefix(msg, "Could not resolve \"") {
			// current package/module can not be marked as external
			if strings.Contains(msg, fmt.Sprintf("Could not resolve \"%s\"", task.Pkg.ImportPath())) {
				err = fmt.Errorf("could not resolve \"%s\"", task.Pkg.ImportPath())
				return
			}
			name := strings.Split(msg, "\"")[1]
			if !implicitExternal.Has(name) {
				log.Warnf("build(%s): implicit external '%s'", task.ID(), name)
				implicitExternal.Add(name)
				goto rebuild
			}
		}
		if strings.HasPrefix(msg, "No matching export in \"") {
			a := strings.Split(msg, "\"")
			if len(a) > 4 {
				path, exportName := a[1], a[3]
				if strings.HasPrefix(path, "browser-exclude:") && exportName != "default" {
					path = strings.TrimPrefix(path, "browser-exclude:")
					exports, ok := browserExclude[path]
					if !ok {
						exports = newStringSet()
						browserExclude[path] = exports
					}
					if !exports.Has(exportName) {
						exports.Add(exportName)
						goto rebuild
					}
				}
			}
		}
		err = errors.New("esbuild: " + msg)
		return
	}

	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Text, "Could not resolve \"") {
			log.Warnf("esbuild(%s): %s", task.ID(), w.Text)
		}
	}

	for _, file := range result.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			jsContent := file.Contents
			header := bytes.NewBufferString(fmt.Sprintf(
				"/* esm.sh - esbuild bundle(%s) %s %s */\n",
				task.Pkg.String(),
				strings.ToLower(task.Target),
				nodeEnv,
			))

			// remove shebang
			if bytes.HasPrefix(jsContent, []byte("#!/")) {
				jsContent = jsContent[bytes.IndexByte(jsContent, '\n')+1:]
				task.headerLines--
			}

			// add nodejs compatibility
			if task.Target != "node" {
				ids := newStringSet()
				for _, r := range regexpGlobalIdent.FindAll(jsContent, -1) {
					ids.Add(string(r))
				}
				if ids.Has("__Process$") {
					if task.Target == "denonext" {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if task.Target == "deno" {
						fmt.Fprintf(header, `import __Process$ from "https://deno.land/std@%s/node/process.ts";%s`, task.Args.denoStdVersion, EOL)
					} else if task.Bundle {
						var js []byte
						js, err = bundleNodePolyfill("process", "__Process$", "default", targets[task.Target])
						if err != nil {
							return
						}
						fmt.Fprintf(header, "%s", js)
					} else {
						fmt.Fprintf(header, `import __Process$ from "%s/v%d/node_process.js";%s`, cfg.CdnBasePath, task.BuildVersion, EOL)
					}
				}
				if ids.Has("__Buffer$") {
					if task.Target == "denonext" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if task.Target == "deno" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "https://deno.land/std@%s/node/buffer.ts";%s`, task.Args.denoStdVersion, EOL)
					} else if task.Bundle {
						var js []byte
						js, err = bundleNodePolyfill("buffer", "__Buffer$", "Buffer", targets[task.Target])
						if err != nil {
							return
						}
						fmt.Fprintf(header, "%s", js)
					} else {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "%s/v%d/buffer@6.0.3/%s/buffer.mjs";%s`, cfg.CdnBasePath, task.BuildVersion, task.Target, EOL)
					}
				}
				if ids.Has("__global$") {
					fmt.Fprintf(header, `var __global$ = globalThis || (typeof window !== "undefined" ? window : self);%s`, EOL)
				}
				if ids.Has("__setImmediate$") {
					fmt.Fprintf(header, `var __setImmediate$ = (cb, ...args) => setTimeout(cb, 0, ...args);%s`, EOL)
				}
				if ids.Has("__rResolve$") {
					fmt.Fprintf(header, `var __rResolve$ = p => p;%s`, EOL)
				}
			}

			if len(task.requires) > 0 {
				isEsModule := make([]bool, len(task.requires))
				for i, d := range task.requires {
					specifier := d[0]
					fmt.Fprintf(header, `import * as __%x$ from "%s";%s`, i, d[1], EOL)
					if bytes.Contains(jsContent, []byte(fmt.Sprintf(`("%s").default`, specifier))) {
						// if `require("module").default` found
						isEsModule[i] = true
						continue
					}
					if !isLocalSpecifier(specifier) && !internalNodeModules[specifier] {
						if a := bytes.SplitN(jsContent, []byte(fmt.Sprintf(`("%s")`, specifier)), 2); len(a) == 2 {
							p1 := a[0]
							ret := regexpVarEqual.FindSubmatch(p1)
							if len(ret) > 0 {
								r, e := regexp.Compile(fmt.Sprintf(`[^a-zA-Z0-9_$]%s\(`, string(ret[len(ret)-1])))
								if e == nil && r.Match(a[1]) {
									// if `var a = require("module");a()` found
									continue
								}
							}
						}
						pkg, p, formJson, e := task.getPackageInfo(specifier)
						if e == nil {
							// if the dep is a esm only package
							// or the dep(cjs) exports `__esModule`
							if p.Type == "module" {
								isEsModule[i] = true
							} else {
								t := &BuildTask{
									Args:   task.Args,
									Pkg:    pkg,
									Target: task.Target,
									Dev:    task.Dev,
									wd:     task.installDir,
								}
								if !formJson {
									e = installPackage(task.wd, t.Pkg)
								}
								if e == nil {
									m, _, _, e := t.analyze(true)
									if e == nil && includes(m.NamedExports, "__esModule") {
										isEsModule[i] = true
									}
								}
							}
						}
					}
				}
				fmt.Fprint(header, `var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({},m);switch(n){`)
				record := newStringSet()
				for i, d := range task.requires {
					specifier := d[0]
					if record.Has(specifier) {
						continue
					}
					record.Add(specifier)
					esModule := isEsModule[i]
					if esModule {
						fmt.Fprintf(header, `case"%s":return c(__%x$);`, specifier, i)
					} else {
						fmt.Fprintf(header, `case"%s":return e(__%x$);`, specifier, i)
					}
				}
				fmt.Fprintf(header, `default:throw new Error("module \""+n+"\" not found");}};%s`, EOL)
			}

			// to fix the source map
			task.headerLines += strings.Count(header.String(), EOL)

			finalContent := bytes.NewBuffer(nil)
			finalContent.Write(header.Bytes())
			finalContent.Write(rewriteJS(task, jsContent))

			// check if package is deprecated
			if task.Deprecated != "" {
				fmt.Fprintf(finalContent, `console.warn("[npm] %%cdeprecated%%c %s@%s: %s", "color:red", "");%s`, task.Pkg.Name, task.Pkg.Version, strings.ReplaceAll(task.Deprecated, "\"", "\\\""), "\n")
			}

			// add sourcemap Url
			finalContent.WriteString("//# sourceMappingURL=")
			finalContent.WriteString(filepath.Base(task.ID()))
			finalContent.WriteString(".map")

			_, err = fs.WriteFile(task.getSavepath(), finalContent)
			if err != nil {
				return
			}
		}
	}

	for _, file := range result.OutputFiles {
		if strings.HasSuffix(file.Path, ".css") {
			savePath := task.getSavepath()
			_, err = fs.WriteFile(strings.TrimSuffix(savePath, path.Ext(savePath))+".css", bytes.NewReader(file.Contents))
			if err != nil {
				return
			}
			esm.PackageCSS = true
		} else if strings.HasSuffix(file.Path, ".js.map") {
			var sourceMap map[string]interface{}
			if json.Unmarshal(file.Contents, &sourceMap) == nil {
				if mapping, ok := sourceMap["mappings"].(string); ok {
					fixedMapping := make([]byte, task.headerLines+len(mapping))
					for i := 0; i < task.headerLines; i++ {
						fixedMapping[i] = ';'
					}
					copy(fixedMapping[task.headerLines:], mapping)
					sourceMap["mappings"] = string(fixedMapping)
				}
				buf := bytes.NewBuffer(nil)
				if json.NewEncoder(buf).Encode(sourceMap) == nil {
					_, err = fs.WriteFile(task.getSavepath()+".map", buf)
					if err != nil {
						return
					}
				}
			}
		}
	}

	record := newStringSet()
	esm.Deps = filter(task.imports, func(dep string) bool {
		if record.Has(dep) {
			return false
		}
		record.Add(dep)
		return strings.HasPrefix(dep, "/") || strings.HasPrefix(dep, "http:") || strings.HasPrefix(dep, "https:")
	})

	task.checkDTS()
	task.storeToDB()
	return
}

func (task *BuildTask) resolveExternal(specifier string, kind api.ResolveKind) (resolvedPath string) {
	// node builtin module
	if internalNodeModules[specifier] && !task.Args.external.Has(getPkgName(specifier)) {
		if task.Target == "node" {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if task.Target == "denonext" && !denoNextUnspportedNodeModules[specifier] {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if task.Target == "deno" {
			resolvedPath = fmt.Sprintf("https://deno.land/std@%s/node/%s.ts", task.Args.denoStdVersion, specifier)
		} else {
			polyfill, ok := polyfilledInternalNodeModules[specifier]
			if ok {
				p, _, e := validatePkgPath(polyfill)
				if e == nil {
					importPath := task.getImportPath(p, "")
					extname := filepath.Ext(importPath)
					resolvedPath = strings.TrimSuffix(importPath, extname) + extname
				} else {
					resolvedPath = specifier
				}
			} else {
				_, err := embedFS.ReadFile(fmt.Sprintf("server/embed/polyfills/node_%s.js", specifier))
				if err == nil {
					resolvedPath = fmt.Sprintf("%s/v%d/node_%s.js", cfg.CdnBasePath, task.BuildVersion, specifier)
				} else {
					resolvedPath = fmt.Sprintf(
						"%s/error.js?type=unsupported-node-builtin-module&name=%s&importer=%s",
						cfg.CdnBasePath,
						specifier,
						task.Pkg,
					)
				}
			}
		}
	}
	// check `?external`
	if resolvedPath == "" && (task.Args.external.Has("*") || task.Args.external.Has(getPkgName(specifier))) {
		resolvedPath = specifier
	}
	// is sub-module of current package
	if resolvedPath == "" && strings.HasPrefix(specifier, task.Pkg.Name+"/") {
		subPath := strings.TrimPrefix(specifier, task.Pkg.Name+"/")
		subPkg := Pkg{
			Name:      task.Pkg.Name,
			Version:   task.Pkg.Version,
			Subpath:   subPath,
			Submodule: toModuleName(subPath),
		}
		resolvedPath = task.getImportPath(subPkg, encodeBuildArgsPrefix(task.Args, subPkg, false))
	}
	// replace some polyfills with native APIs
	if resolvedPath == "" {
		switch specifier {
		case "object-assign":
			resolvedPath = jsDataUrl(`export default Object.assign`)
		case "array-flatten":
			resolvedPath = jsDataUrl(`export const flatten=(a,d)=>a.flat(typeof d<"u"?d:Infinity);export default flatten`)
		case "array-includes":
			resolvedPath = jsDataUrl(`export default (a,p,i)=>a.includes(p,i)`)
		case "abort-controller":
			resolvedPath = jsDataUrl(`export const AbortSignal=globalThis.AbortSignal;export const AbortController=globalThis.AbortController;export default AbortController`)
		case "node-fetch":
			if task.Target != "node" {
				resolvedPath = fmt.Sprintf("%s/v%d/node_fetch.js", cfg.CdnBasePath, task.BuildVersion)
			}
		}
	}
	// common npm dependency
	if resolvedPath == "" {
		pkgName, _, subpath := splitPkgPath(specifier)
		version := "latest"
		if pkgName == task.Pkg.Name {
			version = task.Pkg.Version
		} else if pkg, ok := task.Args.deps.Get(pkgName); ok {
			version = pkg.Version
		} else if v, ok := task.npm.Dependencies[pkgName]; ok {
			version = v
		} else if v, ok := task.npm.PeerDependencies[pkgName]; ok {
			version = v
		}
		if !regexpFullVersion.MatchString(version) {
			p, _, err := getPackageInfo(task.installDir, pkgName, version)
			if err == nil {
				version = p.Version
			}
		}
		pkg := Pkg{
			Name:      pkgName,
			Version:   version,
			Subpath:   subpath,
			Submodule: toModuleName(subpath),
		}
		args := BuildArgs{
			alias:      cloneMap(task.Args.alias),
			conditions: newStringSet(task.Args.conditions.Values()...),
			external:   newStringSet(task.Args.external.Values()...),
			exports:    newStringSet(),
		}
		// force the dependency version of `react` equals to react-dom
		if task.Pkg.Name == "react-dom" && specifier == "react" {
			pkg.Version = task.Pkg.Version
		}
		// use version defined in `?deps` query
		for _, dep := range task.Args.deps {
			if specifier == dep.Name || strings.HasPrefix(specifier, dep.Name+"/") {
				pkg.Version = dep.Version
				break
			}
		}
		resolvedPath = task.getImportPath(pkg, encodeBuildArgsPrefix(args, pkg, false))
	}

	if kind != api.ResolveJSDynamicImport {
		task.lock.Lock()
		task.imports = append(task.imports, resolvedPath)
		task.lock.Unlock()
	}

	// `require("module")`
	if kind == api.ResolveJSRequireCall {
		task.lock.Lock()
		task.requires = append(task.requires, [2]string{specifier, resolvedPath})
		task.lock.Unlock()
		resolvedPath = specifier
	}

	return
}

func (task *BuildTask) storeToDB() {
	err := db.Put(task.ID(), utils.MustEncodeJSON(task.esm))
	if err != nil {
		log.Errorf("db: %v", err)
	}
}

func (task *BuildTask) checkDTS() {
	name := task.Pkg.Name
	submodule := task.Pkg.Submodule
	var dts string
	if task.npm.Types != "" {
		dts = task.toTypesPath(task.wd, task.npm, "", encodeBuildArgsPrefix(task.Args, task.Pkg, true), submodule)
	} else if !strings.HasPrefix(name, "@types/") {
		versions := []string{"latest"}
		versionParts := strings.Split(task.Pkg.Version, ".")
		if len(versionParts) > 2 {
			versions = []string{
				"~" + strings.Join(versionParts[:2], "."), // minor
				"^" + versionParts[0],                     // major
				"latest",
			}
		}
		typesPkgName := toTypesPackageName(name)
		pkg, ok := task.Args.deps.Get(typesPkgName)
		if ok {
			// use the version of the `?deps` query if it exists
			versions = append([]string{pkg.Version}, versions...)
		}
		for _, version := range versions {
			p, _, err := getPackageInfo(task.installDir, typesPkgName, version)
			if err == nil {
				prefix := encodeBuildArgsPrefix(task.Args, Pkg{Name: p.Name}, true)
				dts = task.toTypesPath(task.wd, p, version, prefix, submodule)
				break
			}
		}
	}
	if dts != "" {
		bv := task.BuildVersion
		if stableBuild[task.Pkg.Name] {
			bv = STABLE_VERSION
		}
		task.esm.Dts = fmt.Sprintf("/v%d%s/%s", bv, task.ghPrefix(), dts)
	}
}

func (task *BuildTask) buildDTS(dts string) {
	start := time.Now()
	task.stage = "transform-dts"
	n, err := task.TransformDTS(dts)
	if err != nil && os.IsExist(err) {
		log.Errorf("TransformDTS(%s): %v", dts, err)
		return
	}
	log.Debugf("transform dts '%s'(%d related dts files) in %v", dts, n, time.Since(start))
}
