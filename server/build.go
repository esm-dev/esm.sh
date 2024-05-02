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
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type BuildTask struct {
	pkg        Pkg
	npm        NpmPackageInfo
	args       BuildArgs
	target     string
	dev        bool
	bundle     bool
	noBundle   bool
	id         string
	deprecated string
	stage      string
	wd         string
	resolveDir string
	packageDir string
	imports    [][2]string
	requires   [][2]string
	smOffset   int
	subBuilds  *StringSet
	subTasks   []chan struct{}
}

type BuildResult struct {
	Deps             []string `json:"p,omitempty"`
	Dts              string   `json:"t,omitempty"`
	FromCJS          bool     `json:"c,omitempty"`
	HasExportDefault bool     `json:"d,omitempty"`
	NamedExports     []string `json:"-"`
	PackageCSS       bool     `json:"s,omitempty"`
	TypesOnly        bool     `json:"o,omitempty"`
}

func (task *BuildTask) queryBuild() (*BuildResult, bool) {
	id := task.ID()
	value, err := db.Get(id)
	if err == nil && value != nil {
		var b BuildResult
		err = json.Unmarshal(value, &b)
		if err == nil {
			if !b.TypesOnly {
				_, err = fs.Stat(path.Join("builds", id))
			}
			if err == nil || os.IsExist(err) {
				return &b, true
			}
		}
		// delete the invalid db entry
		db.Delete(id)
	}
	return nil, false
}

func (task *BuildTask) Build() (ret *BuildResult, err error) {
	task.wd = path.Join(cfg.WorkDir, fmt.Sprintf("npm/%s", task.pkg.VersionName()))
	err = ensureDir(task.wd)
	if err != nil {
		return
	}

	// create `.npmrc` file
	var npmrc bytes.Buffer
	npmrc.WriteString("@jsr:registry=https://npm.jsr.io\n")
	if cfg.NpmRegistryScope != "" && cfg.NpmRegistry != "" {
		npmrc.WriteString(fmt.Sprintf("%s:registry=%s\n", cfg.NpmRegistryScope, cfg.NpmRegistry))
	} else if cfg.NpmRegistryScope == "" && cfg.NpmRegistry != "" {
		npmrc.WriteString(fmt.Sprintf("registry=%s\n", cfg.NpmRegistry))
	}
	if cfg.NpmRegistry != "" && cfg.NpmToken != "" {
		var tokenReg string
		tokenReg, err = removeHttpPrefix(cfg.NpmRegistry)
		if err != nil {
			log.Errorf("Invalid npm registry in config: %v", err)
			return
		}
		npmrc.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN}\n", tokenReg))
	}
	if cfg.NpmRegistry != "" && cfg.NpmUser != "" && cfg.NpmPassword != "" {
		var tokenReg string
		tokenReg, err = removeHttpPrefix(cfg.NpmRegistry)
		if err != nil {
			log.Errorf("Invalid npm registry in config: %v", err)
			return
		}
		npmrc.WriteString(fmt.Sprintf("%s:username=${ESM_NPM_USER}\n", tokenReg))
		npmrc.WriteString(fmt.Sprintf("%s:_password=${ESM_NPM_PASSWORD}\n", tokenReg))
	}
	err = os.WriteFile(path.Join(task.wd, ".npmrc"), npmrc.Bytes(), 0644)
	if err != nil {
		log.Errorf("Failed to create .npmrc file: %v", err)
		return
	}

	// check if the package is deprecated
	if !task.pkg.FromGithub && !strings.HasPrefix(task.pkg.Name, "@jsr/") {
		var info NpmPackageInfo
		info, err = fetchPackageInfo(task.pkg.Name, task.pkg.Version)
		if err != nil {
			return
		}
		task.deprecated = info.Deprecated
	}

	// install the package
	task.stage = "install"
	err = installPackage(task.wd, task.pkg)
	if err != nil {
		return
	}

	// resolve the package real install path
	if l, e := filepath.EvalSymlinks(path.Join(task.wd, "node_modules", task.pkg.Name)); e == nil {
		task.packageDir = l
		if task.pkg.FromGithub || strings.HasPrefix(task.pkg.Name, "@") {
			task.resolveDir = path.Join(l, "../../..")
		} else {
			task.resolveDir = path.Join(l, "../..")
		}
	} else {
		task.packageDir = path.Join(task.wd, "node_modules", task.pkg.Name)
		task.resolveDir = task.wd
	}

	// build the module
	task.subBuilds = newStringSet()
	task.stage = "build"
	ret, err = task.build()
	if err != nil {
		return
	}
	if e := db.Put(task.ID(), mustEncodeJSON(ret)); e != nil {
		log.Errorf("db: %v", e)
	}
	return
}

func (task *BuildTask) build() (result *BuildResult, err error) {
	// build json
	if strings.HasSuffix(task.pkg.SubModule, ".json") {
		nmDir := path.Join(task.wd, "node_modules")
		jsonPath := path.Join(nmDir, task.pkg.Name, task.pkg.SubModule)
		if existsFile(jsonPath) {
			json, err := os.ReadFile(jsonPath)
			if err != nil {
				return nil, err
			}
			buffer := bytes.NewBufferString("export default ")
			buffer.Write(json)
			_, err = fs.WriteFile(task.getSavepath(), buffer)
			if err != nil {
				return nil, err
			}
			result := &BuildResult{
				HasExportDefault: true,
			}
			return result, nil
		}
	}

	var npm NpmPackageInfo
	var reexport string
	result, npm, reexport, err = task.analyze(false)
	if err != nil && !strings.HasPrefix(err.Error(), "cjsLexer: Can't resolve") {
		return nil, err
	}
	task.npm = npm

	if task.target == "types" {
		if npm.Types != "" {
			dts := npm.Name + "@" + npm.Version + path.Join("/", npm.Types)
			task.buildDTS(dts)
		}
		return
	}

	if result.TypesOnly {
		dts := npm.Name + "@" + npm.Version + path.Join("/", npm.Types)
		result.Dts = fmt.Sprintf("%s%s", task._ghPrefix(), dts)
		task.buildDTS(dts)
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
			args:   task.args,
			pkg:    pkg,
			target: task.target,
			dev:    task.dev,
			wd:     task.resolveDir,
		}
		if !formJson {
			err = installPackage(task.wd, t.pkg)
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
		importPath := task.getImportPath(t.pkg, encodeBuildArgsPrefix(task.args, task.pkg, false))
		fmt.Fprintf(buf, `export * from "%s";`, importPath)
		if m.HasExportDefault {
			fmt.Fprintf(buf, "\n")
			fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
		}

		_, err = fs.WriteFile(task.getSavepath(), buf)
		if err != nil {
			return
		}
		result.Dts = task.checkDTS()
		return
	}

	defer func() {
		if err != nil {
			result = nil
		}
	}()

	var entryPoint string
	var input *api.StdinOptions

	moduleName := npm.Name
	if task.pkg.SubModule != "" {
		moduleName += "/" + task.pkg.SubModule
	}

	if npm.Module == "" {
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `import * as __module from "%s";`, moduleName)
		if len(result.NamedExports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = __module;`, strings.Join(result.NamedExports, ","))
		}
		fmt.Fprintf(buf, "const { default: __default, ...__rest } = __module;")
		fmt.Fprintf(buf, "export default (__default !== undefined ? __default : __rest);")
		// Default reexport all members from original module to prevent missing named exports members
		fmt.Fprintf(buf, `export * from "%s";`, moduleName)
		input = &api.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: task.wd,
			Sourcefile: "build.js",
		}
	} else {
		if task.args.exports.Len() > 0 {
			buf := bytes.NewBuffer(nil)
			fmt.Fprintf(buf, `export { %s } from "%s";`, strings.Join(task.args.exports.Values(), ","), moduleName)
			input = &api.StdinOptions{
				Contents:   buf.String(),
				ResolveDir: task.wd,
				Sourcefile: "build.js",
			}
		} else {
			entryPoint = path.Join(task.wd, "node_modules", npm.Name, npm.Module)
		}
	}

	pkgSideEffects := api.SideEffectsTrue
	if npm.SideEffectsFalse {
		pkgSideEffects = api.SideEffectsFalse
	}

	noBundle := task.noBundle || (npm.SideEffects != nil && npm.SideEffects.Len() > 0)
	if npm.Esmsh != nil {
		if v, ok := npm.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				noBundle = true
			}
		}
	}

	nodeEnv := "production"
	if task.dev {
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
	if task.target == "node" {
		define = map[string]string{}
	}
	imports := []string{}
	browserExclude := map[string]*StringSet{}
	implicitExternal := newStringSet()

	esmPlugin := api.Plugin{
		Name: "esm",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					// ban file urls
					if strings.HasPrefix(args.Path, "file:") {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(args.Path, "file:"), task.pkg),
							External: true,
						}, nil
					}

					// skip http modules
					if strings.HasPrefix(args.Path, "data:") || strings.HasPrefix(args.Path, "https:") || strings.HasPrefix(args.Path, "http:") {
						return api.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// if `?ignore-require` present, ignore specifier that is a require call
					if task.args.ignoreRequire && args.Kind == api.ResolveJSRequireCall && npm.Module != "" {
						return api.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// ignore yarn PnP API
					if args.Path == "pnpapi" {
						return api.OnResolveResult{
							Path:      args.Path,
							Namespace: "browser-exclude",
						}, nil
					}

					// it's implicit external
					if implicitExternal.Has(args.Path) {
						return api.OnResolveResult{
							Path:     task.resolveExternalModule(args.Path, args.Kind),
							External: true,
						}, nil
					}

					// normalize specifier
					specifier := strings.TrimPrefix(args.Path, "node:")
					specifier = strings.TrimPrefix(specifier, "npm:")

					// resolve specifier with package `imports` field
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

					// resolve specifier with package `browser` field
					if len(npm.Browser) > 0 && !task.isServerTarget() {
						spec := specifier
						if isRelativeSpecifier(specifier) {
							fullFilepath := filepath.Join(args.ResolveDir, specifier)
							spec = "." + strings.TrimPrefix(fullFilepath, path.Join(task.resolveDir, "node_modules", npm.Name))
						}
						if _, ok := npm.Browser[spec]; !ok && path.Ext(spec) == "" {
							spec += ".js"
						}
						if name, ok := npm.Browser[spec]; ok {
							if name == "" {
								// browser exclude
								return api.OnResolveResult{
									Path:      args.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							if strings.HasPrefix(name, "./") {
								specifier = path.Join(task.resolveDir, "node_modules", npm.Name, name)
							} else {
								specifier = name
							}
						}
					}

					// resolve specifier by checking `?alias` query
					if len(task.args.alias) > 0 {
						if name, ok := task.args.alias[specifier]; ok {
							specifier = name
						} else {
							pkgName, _, subpath := splitPkgPath(specifier)
							if subpath != "" {
								if name, ok := task.args.alias[pkgName]; ok {
									specifier = name + "/" + subpath
								}
							}
						}
					}

					// force to use `npm:` specifier for `denonext` target
					if forceNpmSpecifiers[specifier] && task.target == "denonext" {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("npm:%s", specifier),
							External: true,
						}, nil
					}

					// ignore native node packages like 'fsevent'
					for _, name := range nativeNodePackages {
						if specifier == name || strings.HasPrefix(specifier, name+"/") {
							if task.target == "denonext" {
								pkgName, _, subPath := splitPkgPath(specifier)
								version := "latest"
								if pkgName == task.pkg.Name {
									version = task.pkg.Version
								} else if v, ok := npm.Dependencies[pkgName]; ok {
									version = v
								} else if v, ok := npm.PeerDependencies[pkgName]; ok {
									version = v
								}
								if !regexpFullVersion.MatchString(version) {
									p, _, err := getPackageInfo(task.resolveDir, pkgName, version)
									if err == nil {
										version = p.Version
									}
								}
								if err == nil {
									pkg := Pkg{
										Name:      pkgName,
										Version:   version,
										SubModule: toModuleBareName(subPath, true),
										SubPath:   subPath,
									}
									return api.OnResolveResult{
										Path:     fmt.Sprintf("npm:%s", pkg.String()),
										External: true,
									}, nil
								}
							}
							if specifier == "fsevents" {
								return api.OnResolveResult{
									Path:     fmt.Sprintf("%s/npm_fsevents.js", cfg.CdnBasePath),
									External: true,
								}, nil
							}
							return api.OnResolveResult{
								Path:     fmt.Sprintf("/error.js?type=unsupported-npm-package&name=%s&importer=%s", specifier, task.pkg),
								External: true,
							}, nil
						}
					}

					var fullFilepath string
					if strings.HasPrefix(specifier, "/") {
						fullFilepath = specifier
					} else if isRelativeSpecifier(specifier) {
						fullFilepath = filepath.Join(args.ResolveDir, specifier)
					} else {
						fullFilepath = filepath.Join(task.resolveDir, "node_modules", specifier)
					}

					// native node modules do not work via http import
					if strings.HasSuffix(fullFilepath, ".node") && existsFile(fullFilepath) {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), task.pkg),
							External: true,
						}, nil
					}

					// bundles json module
					if strings.HasSuffix(fullFilepath, ".json") && existsFile(fullFilepath) {
						return api.OnResolveResult{}, nil
					}

					// embed wasm as WebAssembly.Module
					if strings.HasSuffix(fullFilepath, ".wasm") && existsFile(fullFilepath) {
						return api.OnResolveResult{
							Path:      fullFilepath,
							Namespace: "wasm",
						}, nil
					}

					// externalize the _parent_ module
					// e.g. "react/jsx-runtime" imports "react"
					if task.pkg.SubModule != "" && task.pkg.Name == specifier && !task.bundle {
						return api.OnResolveResult{
							Path:        task.resolveExternalModule(specifier, args.Kind),
							External:    true,
							SideEffects: pkgSideEffects,
						}, nil
					}

					// it's the entry point
					if specifier == entryPoint || specifier == moduleName || specifier == path.Join(npm.Name, npm.Module) || specifier == path.Join(npm.Name, npm.Main) {
						return api.OnResolveResult{}, nil
					}

					// it's nodejs internal module
					if nodejsInternalModules[specifier] {
						return api.OnResolveResult{
							Path:     task.resolveExternalModule(specifier, args.Kind),
							External: true,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query
					if task.bundle && !task.args.external.Has(getPkgName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := getPkgName(specifier)
						_, ok := npm.PeerDependencies[pkgName]
						if !ok {
							return api.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind == api.ResolveJSRequireCall || !noBundle) && task.npm.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/")) {
						return api.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelativeSpecifier(specifier) {
						specifier = strings.TrimPrefix(fullFilepath, filepath.Join(task.resolveDir, "node_modules")+"/")
						if strings.HasPrefix(specifier, ".pnpm") {
							a := strings.Split(specifier, "/node_modules/")
							if len(a) > 1 {
								specifier = a[1]
							}
						}
						pkgName := npm.Name
						isSubModuleOfCurrentPkg := strings.HasPrefix(specifier, pkgName+"/")
						if !isSubModuleOfCurrentPkg && npm.PkgName != "" {
							pkgName = npm.PkgName
							isSubModuleOfCurrentPkg = strings.HasPrefix(specifier, pkgName+"/")
						}
						if isSubModuleOfCurrentPkg {
							modulePath := "." + strings.TrimPrefix(specifier, pkgName)
							bareName := stripModuleExt(modulePath)

							// if meets scenarios in "lib/index.mjs" imports "lib/index.cjs"
							// let esbuild to handle it
							if bareName == "./"+task.pkg.SubModule {
								return api.OnResolveResult{}, nil
							}

							// split modules based on the `exports` defines in package.json,
							// see https://nodejs.org/api/packages.html
							if om, ok := npm.Exports.(*OrderedMap); ok {
								for e := om.l.Front(); e != nil; e = e.Next() {
									name, paths := om.Entry(e)
									if !(name == "." || strings.HasPrefix(name, "./")) {
										continue
									}
									if strings.ContainsRune(name, '*') {
										var match bool
										var prefix string
										var suffix string
										if s, ok := paths.(string); ok {
											// exports: "./*": "./dist/*.js"
											prefix, suffix = utils.SplitByLastByte(s, '*')
											match = strings.HasPrefix(bareName, prefix) && (suffix == "" || strings.HasSuffix(modulePath, suffix))
										} else if m, ok := paths.(*OrderedMap); ok {
											// exports: "./*": { "import": "./dist/*.js" }
											for e := m.l.Front(); e != nil; e = e.Next() {
												_, value := m.Entry(e)
												if s, ok := value.(string); ok {
													prefix, suffix = utils.SplitByLastByte(s, '*')
													match = strings.HasPrefix(bareName, prefix) && (suffix == "" || strings.HasSuffix(modulePath, suffix))
													if match {
														break
													}
												}
											}
										}
										if match {
											exportPrefix, _ := utils.SplitByLastByte(name, '*')
											url := path.Join(npm.Name, exportPrefix+strings.TrimPrefix(bareName, prefix))
											if i := moduleName; url != i && url != i+"/index" {
												return api.OnResolveResult{
													Path:        task.resolveExternalModule(url, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									} else {
										match := false
										if s, ok := paths.(string); ok && stripModuleExt(s) == bareName {
											// exports: "./foo": "./foo.js"
											match = true
										} else if m, ok := paths.(*OrderedMap); ok {
										Loop:
											for e := m.l.Front(); e != nil; e = e.Next() {
												_, value := m.Entry(e)
												if s, ok := value.(string); ok {
													// exports: "./foo": { "import": "./foo.js" }
													if stripModuleExt(s) == bareName {
														match = true
														break
													}
												} else if m, ok := value.(*OrderedMap); ok {
													// exports: "./foo": { "import": { default: "./foo.js" } }
													for e := m.l.Front(); e != nil; e = e.Next() {
														_, value := m.Entry(e)
														if s, ok := value.(string); ok {
															if stripModuleExt(s) == bareName {
																match = true
																break Loop
															}
														}
													}
												}
											}
										}
										if match {
											url := path.Join(npm.Name, stripModuleExt(name))
											if i := moduleName; url != i && url != i+"/index" {
												return api.OnResolveResult{
													Path:        task.resolveExternalModule(url, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// split the module that is an alias of a dependency
							// means this file just include a single line(js): `export * from "dep"`
							fi, ioErr := os.Lstat(fullFilepath)
							if ioErr == nil && fi.Size() < 128 {
								data, ioErr := os.ReadFile(fullFilepath)
								if ioErr == nil {
									out, esbErr := minify(string(data), api.ESNext, api.LoaderJS)
									if esbErr == nil {
										p := bytes.Split(out, []byte("\""))
										if len(p) == 3 && string(p[0]) == "export*from" && string(p[2]) == ";\n" {
											url := string(p[1])
											if !isRelativeSpecifier(url) {
												return api.OnResolveResult{
													Path:        task.resolveExternalModule(url, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// bundle the module
							if args.Kind != api.ResolveJSDynamicImport && !noBundle {
								return api.OnResolveResult{}, nil
							}
						}
					}

					// dynamic external
					sideEffects := api.SideEffectsFalse
					if specifier == npm.Name || specifier == npm.PkgName || strings.HasPrefix(specifier, npm.Name+"/") || strings.HasPrefix(specifier, npm.Name+"/") {
						sideEffects = pkgSideEffects
					}
					return api.OnResolveResult{
						Path:        task.resolveExternalModule(specifier, args.Kind),
						External:    true,
						SideEffects: sideEffects,
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
	}

	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Format:            api.FormatESModule,
		Target:            targets[task.target],
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  !task.dev,
		MinifyIdentifiers: !task.dev,
		MinifySyntax:      !task.dev,
		KeepNames:         task.args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: task.args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        task.args.conditions.Values(),
		Plugins:           []api.Plugin{esmPlugin},
		SourceRoot:        "/",
		Sourcemap:         api.SourceMapExternal,
	}
	// ignore features that can not be polyfilled
	options.Supported = map[string]bool{
		"bigint":          true,
		"top-level-await": true,
	}
	// bundling image/font assets
	options.Loader = map[string]api.Loader{
		".svg":   api.LoaderDataURL,
		".png":   api.LoaderDataURL,
		".webp":  api.LoaderDataURL,
		".gif":   api.LoaderDataURL,
		".ttf":   api.LoaderDataURL,
		".eot":   api.LoaderDataURL,
		".woff":  api.LoaderDataURL,
		".woff2": api.LoaderDataURL,
	}
	if task.target == "node" {
		options.Platform = api.PlatformNode
	} else {
		options.Define = define
	}
	if !task.isDenoTarget() {
		options.JSX = api.JSXAutomatic
		if task.args.jsxRuntime != nil {
			if task.args.external.Has(task.args.jsxRuntime.Name) || task.args.external.Has("*") {
				options.JSXImportSource = task.args.jsxRuntime.Name
			} else {
				options.JSXImportSource = "https://esm.sh/" + task.args.jsxRuntime.String()
			}
		} else if task.args.external.Has("react") {
			options.JSXImportSource = "react"
		} else if task.args.external.Has("preact") {
			options.JSXImportSource = "preact"
		} else if task.args.external.Has("*") {
			options.JSXImportSource = "react"
		} else if pkg, ok := task.args.deps.Get("react"); ok {
			options.JSXImportSource = "https://esm.sh/react@" + pkg.Version
		} else if pkg, ok := task.args.deps.Get("preact"); ok {
			options.JSXImportSource = "https://esm.sh/preact@" + pkg.Version
		} else {
			options.JSXImportSource = "https://esm.sh/react"
		}
	}
	if input != nil {
		options.Stdin = input
	} else if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
	}

rebuild:
	ret := api.Build(options)
	if len(ret.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := ret.Errors[0].Text
		if strings.HasPrefix(msg, "Could not resolve \"") {
			// current package/module can not be marked as external
			if strings.Contains(msg, fmt.Sprintf("Could not resolve \"%s\"", moduleName)) {
				err = fmt.Errorf("could not resolve \"%s\"", moduleName)
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

	for _, w := range ret.Warnings {
		if strings.HasPrefix(w.Text, "Could not resolve \"") {
			log.Warnf("esbuild(%s): %s", task.ID(), w.Text)
		}
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			jsContent := file.Contents
			extraBanner := ""
			if nodeEnv == "" {
				extraBanner = " development"
			}
			if task.bundle {
				extraBanner = " standalone"
			}
			header := bytes.NewBufferString(fmt.Sprintf(
				"/* esm.sh(v%d) - %s %s%s */\n",
				VERSION,
				task.pkg.String(),
				strings.ToLower(task.target),
				extraBanner,
			))

			// filter tree-shaking imports
			imports = make([]string, len(task.imports))
			i := 0
			for _, a := range task.imports {
				fullpath, path := a[0], a[1]
				if bytes.Contains(jsContent, []byte(fmt.Sprintf(`"%s"`, path))) {
					imports[i] = fullpath
					i++
				}
			}
			imports = imports[:i]

			// remove shebang
			if bytes.HasPrefix(jsContent, []byte("#!/")) {
				jsContent = jsContent[bytes.IndexByte(jsContent, '\n')+1:]
				task.smOffset--
			}

			// add nodejs compatibility
			if task.target != "node" {
				ids := newStringSet()
				for _, r := range regexpGlobalIdent.FindAll(jsContent, -1) {
					ids.Add(string(r))
				}
				if ids.Has("__Process$") {
					if task.args.external.Has("node:process") || task.args.external.Has("*") {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if task.target == "denonext" {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if task.target == "deno" {
						fmt.Fprintf(header, `import __Process$ from "https://deno.land/std@%s/node/process.ts";%s`, task.args.denoStdVersion, EOL)
					} else {
						var browserExclude bool
						if len(npm.Browser) > 0 {
							if name, ok := npm.Browser["process"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import __Process$ from "%s/node/process.js";%s`, cfg.CdnBasePath, EOL)
						}
					}
				}
				if ids.Has("__Buffer$") {
					if task.args.external.Has("node:buffer") || task.args.external.Has("*") {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if task.target == "denonext" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if task.target == "deno" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "https://deno.land/std@%s/node/buffer.ts";%s`, task.args.denoStdVersion, EOL)
					} else {
						var browserExclude bool
						if len(npm.Browser) > 0 {
							if name, ok := npm.Browser["buffer"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "%s/node/buffer.js";%s`, cfg.CdnBasePath, EOL)
						}
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
					if !isRelativeSpecifier(specifier) && !nodejsInternalModules[specifier] {
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
									args:   task.args,
									pkg:    pkg,
									target: task.target,
									dev:    task.dev,
									wd:     task.resolveDir,
								}
								if !formJson {
									e = installPackage(task.wd, t.pkg)
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
				fmt.Fprint(header, `var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({__esModule:true},m);switch(n){`)
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
			task.smOffset += strings.Count(header.String(), EOL)

			ret, dropSourceMap := task.rewriteJS(jsContent)
			if ret != nil {
				jsContent = ret
			}

			finalContent := bytes.NewBuffer(nil)
			finalContent.Write(header.Bytes())
			finalContent.Write(jsContent)

			if task.deprecated != "" {
				fmt.Fprintf(finalContent, `console.warn("[npm] %%cdeprecated%%c %s@%s: %s", "color:red", "");%s`, task.pkg.Name, task.pkg.Version, strings.ReplaceAll(task.deprecated, "\"", "\\\""), "\n")
			}

			// add sourcemap Url
			if !dropSourceMap {
				finalContent.WriteString("//# sourceMappingURL=")
				finalContent.WriteString(filepath.Base(task.ID()))
				finalContent.WriteString(".map")
			}

			_, err = fs.WriteFile(task.getSavepath(), finalContent)
			if err != nil {
				return
			}
		}
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".css") {
			savePath := task.getSavepath()
			_, err = fs.WriteFile(strings.TrimSuffix(savePath, path.Ext(savePath))+".css", bytes.NewReader(file.Contents))
			if err != nil {
				return
			}
			result.PackageCSS = true
		} else if strings.HasSuffix(file.Path, ".js.map") {
			var sourceMap map[string]interface{}
			if json.Unmarshal(file.Contents, &sourceMap) == nil {
				if mapping, ok := sourceMap["mappings"].(string); ok {
					fixedMapping := make([]byte, task.smOffset+len(mapping))
					for i := 0; i < task.smOffset; i++ {
						fixedMapping[i] = ';'
					}
					copy(fixedMapping[task.smOffset:], mapping)
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

	// wait for sub-builds
	for _, ch := range task.subTasks {
		<-ch
	}

	record := newStringSet()
	result.Deps = filter(imports, func(dep string) bool {
		if record.Has(dep) {
			return false
		}
		record.Add(dep)
		return strings.HasPrefix(dep, "/") || strings.HasPrefix(dep, "http:") || strings.HasPrefix(dep, "https:")
	})
	result.Dts = task.checkDTS()
	return
}

func (task *BuildTask) resolveExternalModule(specifier string, kind api.ResolveKind) (resolvedPath string) {
	defer func() {
		fullResolvedPath := resolvedPath
		// use relative path for sub-module of current package
		if strings.HasPrefix(specifier, task.npm.Name+"/") {
			rel, err := filepath.Rel(filepath.Dir("/"+task.ID()), resolvedPath)
			if err == nil {
				if !(strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../")) {
					rel = "./" + rel
				}
				resolvedPath = rel
			}
		}
		// mark the resolved path for _preload_
		if kind != api.ResolveJSDynamicImport {
			task.imports = append(task.imports, [2]string{fullResolvedPath, resolvedPath})
		}
		// if it's `require("module")` call
		if kind == api.ResolveJSRequireCall {
			task.requires = append(task.requires, [2]string{specifier, resolvedPath})
			resolvedPath = specifier
		}
	}()

	// it's current package from github
	if npm := task.npm; task.pkg.FromGithub && (specifier == npm.Name || specifier == npm.PkgName) {
		pkg := Pkg{
			Name:       npm.Name,
			Version:    npm.Version,
			FromGithub: true,
		}
		resolvedPath = task.getImportPath(pkg, encodeBuildArgsPrefix(task.args, pkg, false))
		return
	}

	// node builtin module
	if nodejsInternalModules[specifier] {
		if task.args.external.Has("node:"+specifier) || task.args.external.Has("*") {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if task.target == "node" {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if task.target == "denonext" && !denoNextUnspportedNodeModules[specifier] {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if task.target == "deno" {
			resolvedPath = fmt.Sprintf("https://deno.land/std@%s/node/%s.ts", task.args.denoStdVersion, specifier)
		} else {
			resolvedPath = fmt.Sprintf("%s/node/%s.js", cfg.CdnBasePath, specifier)
		}
		return
	}

	// check `?external`
	if task.args.external.Has("*") || task.args.external.Has(getPkgName(specifier)) {
		resolvedPath = specifier
		return
	}

	// it's sub-module of current package
	if strings.HasPrefix(specifier, task.npm.Name+"/") {
		subPath := strings.TrimPrefix(specifier, task.npm.Name+"/")
		subPkg := Pkg{
			Name:       task.pkg.Name,
			Version:    task.pkg.Version,
			SubPath:    subPath,
			SubModule:  toModuleBareName(subPath, false),
			FromGithub: task.pkg.FromGithub,
		}
		if task.subBuilds != nil {
			subBuild := &BuildTask{
				args:       task.args,
				pkg:        subPkg,
				target:     task.target,
				dev:        task.dev,
				bundle:     task.bundle,
				noBundle:   task.noBundle,
				wd:         task.wd,
				deprecated: task.deprecated,
				resolveDir: task.resolveDir,
				packageDir: task.packageDir,
				subBuilds:  task.subBuilds,
			}
			id := subBuild.ID()
			if !task.subBuilds.Has(id) {
				task.subBuilds.Add(id)
				ch := make(chan struct{})
				task.subTasks = append(task.subTasks, ch)
				go func() {
					subBuild.build()
					ch <- struct{}{}
				}()
			}
		}
		resolvedPath = task.getImportPath(subPkg, encodeBuildArgsPrefix(task.args, subPkg, false))
		if task.noBundle {
			n, e := utils.SplitByLastByte(resolvedPath, '.')
			resolvedPath = n + ".nobundle." + e
		}
		return
	}

	// replace some npm polyfills with native APIs
	if specifier == "node-fetch" && task.target != "node" {
		resolvedPath = fmt.Sprintf("%s/npm_node-fetch.js", cfg.CdnBasePath)
		return
	}
	data, err := embedFS.ReadFile(("server/embed/polyfills/npm_" + specifier + ".js"))
	if err == nil {
		resolvedPath = fmt.Sprintf("data:application/javascript;base64,%s", base64.StdEncoding.EncodeToString(data))
		return
	}

	// common npm dependency
	pkgName, version, subpath := splitPkgPath(specifier)
	if version == "" {
		if pkgName == task.pkg.Name {
			version = task.pkg.Version
		} else if pkg, ok := task.args.deps.Get(pkgName); ok {
			version = pkg.Version
		} else if v, ok := task.npm.Dependencies[pkgName]; ok {
			version = v
		} else if v, ok := task.npm.PeerDependencies[pkgName]; ok {
			version = v
		} else {
			version = "latest"
		}
	}
	// force the version of 'react' (as dependency) equals to 'react-dom'
	if task.pkg.Name == "react-dom" && pkgName == "react" {
		version = task.pkg.Version
	}

	pkg := Pkg{
		Name:      pkgName,
		Version:   version,
		SubPath:   subpath,
		SubModule: toModuleBareName(subpath, true),
	}
	caretVersion := false

	// resolve alias in dependencies
	// follow https://docs.npmjs.com/cli/v10/configuring-npm/package-json#git-urls-as-dependencies
	// e.g. "@mark/html": "npm:@jsr/mark__html@^1.0.0"
	// e.g. "tslib": "git+https://github.com/microsoft/tslib.git#v2.3.0"
	// e.g. "react": "github:facebook/react#v18.2.0"
	{
		// ban file specifier
		if strings.HasPrefix(version, "file:") {
			resolvedPath = fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", pkgName, task.pkg)
			return
		}
		if strings.HasPrefix(version, "npm:") {
			pkg.Name, pkg.Version, _ = splitPkgPath(version[4:])
		} else if strings.HasPrefix(version, "git+ssh://") || strings.HasPrefix(version, "git+https://") || strings.HasPrefix(version, "git://") {
			gitUrl, err := url.Parse(version)
			if err != nil || gitUrl.Hostname() != "github.com" {
				resolvedPath = fmt.Sprintf("/error.js?type=unsupported-git-dependency&name=%s&importer=%s", pkgName, task.pkg)
				return
			}
			repo := strings.TrimSuffix(gitUrl.Path[1:], ".git")
			if gitUrl.Scheme == "git+ssh" {
				repo = gitUrl.Port() + "/" + repo
			}
			pkg.FromGithub = true
			pkg.Name = repo
			pkg.Version = strings.TrimPrefix(url.QueryEscape(gitUrl.Fragment), "semver:")
		} else if strings.HasPrefix(version, "github:") || (!strings.HasPrefix(version, "@") && strings.ContainsRune(version, '/')) {
			repo, fragment := utils.SplitByLastByte(strings.TrimPrefix(version, "github:"), '#')
			pkg.FromGithub = true
			pkg.Name = repo
			pkg.Version = strings.TrimPrefix(url.QueryEscape(fragment), "semver:")
		}
	}

	// fetch the latest version of the package based on the semver range
	if !pkg.FromGithub {
		if strings.HasPrefix(version, "^") && regexpFullVersion.MatchString(version[1:]) {
			caretVersion = true
			pkg.Version = version[1:]
		} else if !regexpFullVersion.MatchString(version) {
			p, _, err := getPackageInfo(task.resolveDir, pkgName, version)
			if err == nil {
				pkg.Version = p.Version
			}
		}
	} else if pkg.Version == "" {
		refs, err := listRepoRefs(fmt.Sprintf("https://github.com/%s", pkg.Name))
		if err == nil {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					pkg.Version = ref.Sha[:16]
					break
				}
			}
		}
	}

	args := BuildArgs{
		alias:      task.args.alias,
		conditions: task.args.conditions,
		deps:       task.args.deps,
		external:   task.args.external,
		exports:    newStringSet(),
	}
	fixBuildArgs(&args, pkg)
	if caretVersion {
		resolvedPath = cfg.CdnBasePath + "/" + pkg.Name + "@^" + pkg.Version
		if pkg.SubModule != "" {
			resolvedPath += "/" + pkg.SubModule
		}
		// workaround for es5-ext weird "/#/" path
		if pkg.Name == "es5-ext" {
			resolvedPath = strings.ReplaceAll(resolvedPath, "/#/", "/%23/")
		}
		params := []string{"target=" + task.target}
		if len(args.alias) > 0 {
			var alias []string
			for k, v := range args.alias {
				alias = append(alias, fmt.Sprintf("%s:%s", k, v))
			}
			params = append(params, "alias="+strings.Join(alias, ","))
		}
		if args.conditions.Len() > 0 {
			params = append(params, "conditions="+strings.Join(args.conditions.Values(), ","))
		}
		if args.deps.Len() > 0 {
			var deps []string
			for _, v := range args.deps {
				deps = append(deps, v.String())
			}
			params = append(params, "deps="+strings.Join(deps, ","))
		}
		if args.external.Len() > 0 {
			params = append(params, "external="+strings.Join(args.external.Values(), ","))
		}
		if task.dev {
			params = append(params, "dev")
		}
		if task.isDenoTarget() {
			params = append(params, "no-dts")
		}
		resolvedPath += "?" + strings.Join(params, "&")
	} else {
		resolvedPath = task.getImportPath(pkg, encodeBuildArgsPrefix(args, pkg, false))
	}
	return
}

func (task *BuildTask) checkDTS() string {
	name := task.pkg.Name
	submodule := task.pkg.SubModule
	var dts string
	if task.npm.Types != "" {
		dts = task.toTypesPath(task.wd, task.npm, "", encodeBuildArgsPrefix(task.args, task.pkg, true), submodule)
	} else if !strings.HasPrefix(name, "@types/") {
		versions := []string{"latest"}
		versionParts := strings.Split(task.pkg.Version, ".")
		if len(versionParts) > 2 {
			versions = []string{
				"~" + strings.Join(versionParts[:2], "."), // minor
				"~" + versionParts[0],                     // major
				"latest",
			}
		}
		typesPkgName := toTypesPackageName(name)
		pkg, ok := task.args.deps.Get(typesPkgName)
		if ok {
			// use the version of the `?deps` query if it exists
			versions = append([]string{pkg.Version}, versions...)
		}
		for _, version := range versions {
			p, _, err := getPackageInfo(task.resolveDir, typesPkgName, version)
			if err == nil {
				prefix := encodeBuildArgsPrefix(task.args, Pkg{Name: p.Name}, true)
				dts = task.toTypesPath(task.wd, p, version, prefix, submodule)
				break
			}
		}
	}
	if dts != "" {
		return fmt.Sprintf("%s%s", task._ghPrefix(), dts)
	}
	return ""
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
