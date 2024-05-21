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

type BundleMode uint8

const (
	BundleDefault BundleMode = iota
	BundleAll
	BundleFalse
)

type BuildContext struct {
	zoneId        string
	npmrc         *NpmRC
	pkg           Pkg
	pkgJson       PackageJSON
	pkgDeprecated string
	args          BuildArgs
	target        string
	bundleMode    BundleMode
	dev           bool
	sourceMap     bool
	wd            string
	path          string
	stage         string
	imports       [][2]string
	requires      [][2]string
	smOffset      int
	subBuilds     *StringSet
	wg            sync.WaitGroup
}

type BuildResult struct {
	Deps             []string `json:"p,omitempty"`
	Dts              string   `json:"t,omitempty"`
	FromCJS          bool     `json:"c,omitempty"`
	HasDefaultExport bool     `json:"d,omitempty"`
	NamedExports     []string `json:"-"`
	PackageCSS       bool     `json:"s,omitempty"`
	TypesOnly        bool     `json:"o,omitempty"`
}

type PackageEntry struct {
	esm string
	cjs string
	dts string
}

func NewBuildContext(zoneId string, npmrc *NpmRC, pkg Pkg, args BuildArgs, target string, bundleMode BundleMode, dev bool, sourceMap bool) *BuildContext {
	return &BuildContext{
		zoneId:     zoneId,
		npmrc:      npmrc,
		pkg:        pkg,
		args:       args,
		target:     target,
		dev:        dev,
		sourceMap:  sourceMap,
		bundleMode: bundleMode,
	}
}

func (ctx *BuildContext) Query() (BuildResult, bool) {
	key := ctx.Path()
	if ctx.zoneId != "" {
		key = ctx.zoneId + key
	}
	value, err := db.Get(key)
	if err == nil && value != nil {
		var b BuildResult
		err = json.Unmarshal(value, &b)
		if err == nil {
			if !b.TypesOnly {
				_, err = fs.Stat(ctx.getSavepath())
			} else {
				_, err = fs.Stat(normalizeSavePath(ctx.zoneId, path.Join("types", b.Dts)))
			}
			// ensure the build files exist
			if err == nil || os.IsExist(err) {
				return b, true
			}
		}
		// delete the invalid db entry
		db.Delete(key)
	}
	return BuildResult{}, false
}

func (ctx *BuildContext) Build() (ret BuildResult, err error) {
	ret, ok := ctx.Query()
	if ok {
		return
	}

	// install the package
	if ctx.wd == "" {
		ctx.wd = path.Join(ctx.npmrc.Dir(), ctx.pkg.FullName())
		ctx.stage = "install"
		err = ctx.npmrc.installPackage(ctx.pkg)
		if err != nil {
			return
		}
		var pkgJson PackageJSON
		err = parseJSONFile(path.Join(ctx.wd, "node_modules", ctx.pkg.Name, "package.json"), &pkgJson)
		if err != nil {
			return
		}
		ctx.pkgJson = ctx.normalizePackageJSON(pkgJson)
	}

	if ctx.target == "types" {
		var dts string
		if endsWith(ctx.pkg.SubModule, ".d.ts", "d.mts") {
			dts = ctx.pkg.FullName() + "/" + ctx.pkg.SubModule
		} else {
			entry := ctx.getEntry()
			if entry.dts == "" {
				err = errors.New("types not found")
				return
			}
			dts = ctx.pkg.FullName() + utils.CleanPath(entry.dts)
		}
		ctx.stage = "build"
		err = ctx.buildTypes(dts)
		if err == nil {
			ret.Dts = "/" + dts
		}
		return
	}

	// check if the package is deprecated
	if ctx.pkgDeprecated != "" && !ctx.pkg.FromGithub && !strings.HasPrefix(ctx.pkg.Name, "@jsr/") {
		var info PackageJSON
		info, err = ctx.npmrc.fetchPackageInfo(ctx.pkg.Name, ctx.pkg.Version)
		if err != nil {
			return
		}
		ctx.pkgDeprecated = info.Deprecated
	}

	if ctx.subBuilds == nil {
		ctx.subBuilds = NewStringSet()
	}

	// build the module
	ctx.stage = "build"
	ret, err = ctx.build()
	if err != nil {
		return
	}

	// save the build result into db
	key := ctx.Path()
	if ctx.zoneId != "" {
		key = ctx.zoneId + key
	}
	if e := db.Put(key, mustEncodeJSON(ret)); e != nil {
		log.Errorf("db: %v", e)
	}
	return
}

func (ctx *BuildContext) build() (result BuildResult, err error) {
	// build json
	if strings.HasSuffix(ctx.pkg.SubModule, ".json") {
		nmDir := path.Join(ctx.wd, "node_modules")
		jsonPath := path.Join(nmDir, ctx.pkg.Name, ctx.pkg.SubModule)
		if existsFile(jsonPath) {
			var jsonData []byte
			jsonData, err = os.ReadFile(jsonPath)
			if err != nil {
				return
			}
			buffer := bytes.NewBufferString("export default ")
			buffer.Write(jsonData)
			_, err = fs.WriteFile(ctx.getSavepath(), buffer)
			if err != nil {
				return
			}
			result = BuildResult{
				HasDefaultExport: true,
			}
			return
		}
	}

	result, entry, reexport, err := ctx.init(false)
	if err != nil && !strings.HasPrefix(err.Error(), "cjsLexer: Can't resolve") {
		return
	}

	if result.TypesOnly {
		dts := ctx.pkgJson.Name + "@" + ctx.pkgJson.Version + path.Join("/", entry.dts)
		result.Dts = fmt.Sprintf("/%s%s", ctx.pkg.ghPrefix(), dts)
		ctx.buildTypes(dts)
		return
	}

	// cjs reexport
	if reexport != "" {
		pkg, _, installed, e := ctx.lookupDep(reexport)
		if e != nil {
			err = e
			return
		}
		// create a new build context to check if the reexported module has default export
		ctx := NewBuildContext(ctx.zoneId, ctx.npmrc, pkg, ctx.args, ctx.target, BundleFalse, ctx.dev, false)
		if installed {
			ctx.wd = path.Join(ctx.wd, "node_modules", ".pnpm")
		} else {
			ctx.wd = path.Join(ctx.npmrc.Dir(), pkg.FullName())
			err = ctx.npmrc.installPackage(pkg)
			if err != nil {
				return
			}
		}
		r, _, _, e := ctx.init(false)
		if err = e; err != nil {
			return
		}
		buf := bytes.NewBuffer(nil)
		importPath := ctx.getImportPath(pkg, ctx.getBuildArgsAsPathSegment(pkg, false))
		fmt.Fprintf(buf, `export * from "%s";`, importPath)
		if r.HasDefaultExport {
			fmt.Fprintf(buf, "\n")
			fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
		}
		_, err = fs.WriteFile(ctx.getSavepath(), buf)
		if err != nil {
			return
		}
		result.Dts = ctx.checkTypes(entry)
		return
	}

	var entryPoint string
	var input *api.StdinOptions

	moduleName := ctx.pkg.Name
	if ctx.pkg.SubModule != "" {
		moduleName += "/" + ctx.pkg.SubModule
	}

	if entry.esm == "" {
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
			ResolveDir: ctx.wd,
			Sourcefile: "build.js",
		}
	} else {
		if ctx.args.exports.Len() > 0 {
			buf := bytes.NewBuffer(nil)
			fmt.Fprintf(buf, `export { %s } from "%s";`, strings.Join(ctx.args.exports.Values(), ","), moduleName)
			input = &api.StdinOptions{
				Contents:   buf.String(),
				ResolveDir: ctx.wd,
				Sourcefile: "build.js",
			}
		} else {
			entryPoint = path.Join(ctx.wd, "node_modules", ctx.pkg.Name, entry.esm)
		}
	}

	pkgSideEffects := api.SideEffectsTrue
	if ctx.pkgJson.SideEffectsFalse {
		pkgSideEffects = api.SideEffectsFalse
	}

	noBundle := ctx.bundleMode == BundleFalse || (ctx.pkgJson.SideEffects != nil && ctx.pkgJson.SideEffects.Len() > 0)
	if ctx.pkgJson.Esmsh != nil {
		if v, ok := ctx.pkgJson.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				noBundle = true
			}
		}
	}

	nodeEnv := ctx.getNodeEnv()
	define := map[string]string{
		"__filename":                  fmt.Sprintf(`"/_virtual/esm.sh%s"`, ctx.Path()),
		"__dirname":                   fmt.Sprintf(`"/_virtual/esm.sh%s"`, path.Dir(ctx.Path())),
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
	if ctx.target == "node" {
		define = map[string]string{}
	}
	imports := []string{}
	browserExclude := map[string]*StringSet{}
	implicitExternal := NewStringSet()

	esmPlugin := api.Plugin{
		Name: "esm",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(res api.OnResolveArgs) (api.OnResolveResult, error) {
					// ban file urls
					if strings.HasPrefix(res.Path, "file:") {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(res.Path, "file:"), ctx.pkg),
							External: true,
						}, nil
					}

					// skip http modules
					if strings.HasPrefix(res.Path, "data:") || strings.HasPrefix(res.Path, "https:") || strings.HasPrefix(res.Path, "http:") {
						return api.OnResolveResult{
							Path:     res.Path,
							External: true,
						}, nil
					}

					// if `?ignore-require` present, ignore specifier that is a require call
					if ctx.args.externalRequire && res.Kind == api.ResolveJSRequireCall && entry.esm != "" {
						return api.OnResolveResult{
							Path:     res.Path,
							External: true,
						}, nil
					}

					// ignore yarn PnP API
					if res.Path == "pnpapi" {
						return api.OnResolveResult{
							Path:      res.Path,
							Namespace: "browser-exclude",
						}, nil
					}

					// it's implicit external
					if implicitExternal.Has(res.Path) {
						return api.OnResolveResult{
							Path:     ctx.resolveExternalModule(res.Path, res.Kind),
							External: true,
						}, nil
					}

					// normalize specifier
					specifier := strings.TrimPrefix(res.Path, "node:")
					specifier = strings.TrimPrefix(specifier, "npm:")
					npm := ctx.pkgJson

					// resolve specifier with package `imports` field
					if v, ok := npm.Imports[specifier]; ok {
						if s, ok := v.(string); ok {
							specifier = s
						} else if m, ok := v.(map[string]interface{}); ok {
							targets := []string{"browser", "module", "import", "default"}
							if ctx.isServerTarget() {
								targets = []string{"module", "import", "default", "browser"}
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
					if len(npm.Browser) > 0 && !ctx.isServerTarget() {
						if name, ok := npm.Browser[specifier]; ok {
							if name == "" {
								// browser exclude
								return api.OnResolveResult{
									Path:      res.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							specifier = name
						}
					}

					// resolve specifier by checking `?alias` query
					if len(ctx.args.alias) > 0 {
						if name, ok := ctx.args.alias[specifier]; ok {
							specifier = name
						} else {
							pkgName, _, subpath, _ := splitPkgPath(specifier)
							if subpath != "" {
								if name, ok := ctx.args.alias[pkgName]; ok {
									specifier = name + "/" + subpath
								}
							}
						}
					}

					// force to use `npm:` specifier for `denonext` target
					if forceNpmSpecifiers[specifier] && ctx.target == "denonext" {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("npm:%s", specifier),
							External: true,
						}, nil
					}

					// ignore native node packages like 'fsevent'
					for _, name := range nativeNodePackages {
						if specifier == name || strings.HasPrefix(specifier, name+"/") {
							if ctx.target == "denonext" {
								pkgName, _, subPath, _ := splitPkgPath(specifier)
								version := ""
								if pkgName == ctx.pkg.Name {
									version = ctx.pkg.Version
								} else if v, ok := npm.Dependencies[pkgName]; ok {
									version = v
								} else if v, ok := npm.PeerDependencies[pkgName]; ok {
									version = v
								}
								if err == nil {
									res := fmt.Sprintf("npm:%s", pkgName)
									if version != "" {
										res += "@" + version
									}
									if subPath != "" {
										res += "/" + subPath
									}
									return api.OnResolveResult{
										Path:     res,
										External: true,
									}, nil
								}
							}
							// use polyfilled 'fsevents' module for browser
							if specifier == "fsevents" {
								return api.OnResolveResult{
									Path:     "npm_fsevents.js",
									External: true,
								}, nil
							}
							return api.OnResolveResult{
								Path:     fmt.Sprintf("/error.js?type=unsupported-npm-package&name=%s&importer=%s", specifier, ctx.pkg),
								External: true,
							}, nil
						}
					}

					var fullFilepath string
					if strings.HasPrefix(specifier, "/") {
						fullFilepath = specifier
					} else if isRelativeSpecifier(specifier) {
						fullFilepath = filepath.Join(res.ResolveDir, specifier)
					} else {
						fullFilepath = filepath.Join(ctx.wd, "node_modules", ".pnpm", "node_modules", specifier)
					}

					// native node modules do not work via http import
					if strings.HasSuffix(fullFilepath, ".node") && existsFile(fullFilepath) {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(res.Path), ctx.pkg),
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
					if ctx.pkg.SubModule != "" && ctx.pkg.Name == specifier && ctx.bundleMode != BundleAll {
						return api.OnResolveResult{
							Path:        ctx.resolveExternalModule(specifier, res.Kind),
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
							Path:     ctx.resolveExternalModule(specifier, res.Kind),
							External: true,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query
					if ctx.bundleMode == BundleAll && !ctx.args.external.Has(getPkgName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := getPkgName(specifier)
						_, ok := npm.PeerDependencies[pkgName]
						if !ok {
							return api.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (res.Kind == api.ResolveJSRequireCall || !noBundle) && ctx.pkgJson.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(res.Importer, "/@babel/runtime/")) {
						return api.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelativeSpecifier(specifier) {
						specifier = strings.TrimPrefix(fullFilepath, filepath.Join(ctx.wd, "node_modules")+"/")
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
							if bareName == "./"+ctx.pkg.SubModule {
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
													Path:        ctx.resolveExternalModule(url, res.Kind),
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
													Path:        ctx.resolveExternalModule(url, res.Kind),
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
													Path:        ctx.resolveExternalModule(url, res.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// bundle the module
							if res.Kind != api.ResolveJSDynamicImport && !noBundle {
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
						Path:        ctx.resolveExternalModule(specifier, res.Kind),
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
		Target:            targets[ctx.target],
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  !ctx.dev,
		MinifyIdentifiers: !ctx.dev,
		MinifySyntax:      !ctx.dev,
		KeepNames:         ctx.args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: ctx.args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        ctx.args.conditions.Values(),
		Plugins:           []api.Plugin{esmPlugin},
		SourceRoot:        "/",
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
	if ctx.target == "node" {
		options.Platform = api.PlatformNode
	} else {
		options.Define = define
	}
	if ctx.sourceMap {
		options.Sourcemap = api.SourceMapExternal
	}
	if !ctx.isDenoTarget() {
		options.JSX = api.JSXAutomatic
		if ctx.args.jsxRuntime != nil {
			if ctx.args.external.Has(ctx.args.jsxRuntime.Name) || ctx.args.external.Has("*") {
				options.JSXImportSource = ctx.args.jsxRuntime.Name
			} else {
				options.JSXImportSource = "/" + ctx.args.jsxRuntime.String()
			}
		} else if ctx.args.external.Has("react") {
			options.JSXImportSource = "react"
		} else if ctx.args.external.Has("preact") {
			options.JSXImportSource = "preact"
		} else if ctx.args.external.Has("*") {
			options.JSXImportSource = "react"
		} else if pkg, ok := ctx.args.deps.Get("react"); ok {
			options.JSXImportSource = "/react@" + pkg.Version
		} else if pkg, ok := ctx.args.deps.Get("preact"); ok {
			options.JSXImportSource = "/preact@" + pkg.Version
		} else {
			options.JSXImportSource = "/react"
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
				log.Warnf("build(%s): implicit external '%s'", ctx.Path(), name)
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
						exports = NewStringSet()
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
			log.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
		}
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			jsContent := file.Contents
			extraBanner := ""
			if nodeEnv == "" {
				extraBanner = " development"
			}
			if ctx.bundleMode == BundleAll {
				extraBanner = " bundle-all"
			}
			header := bytes.NewBufferString(fmt.Sprintf(
				"/* esm.sh(v%d) - %s %s%s */\n",
				VERSION,
				ctx.pkg.String(),
				strings.ToLower(ctx.target),
				extraBanner,
			))

			// filter tree-shaking imports
			imports = make([]string, len(ctx.imports))
			i := 0
			for _, a := range ctx.imports {
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
				ctx.smOffset--
			}

			// add nodejs compatibility
			if ctx.target != "node" {
				ids := NewStringSet()
				for _, r := range regexpGlobalIdent.FindAll(jsContent, -1) {
					ids.Add(string(r))
				}
				if ids.Has("__Process$") {
					if ctx.args.external.Has("node:process") || ctx.args.external.Has("*") {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if ctx.target == "denonext" {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if ctx.target == "deno" {
						fmt.Fprintf(header, `import __Process$ from "https://deno.land/std@0.177.1/node/process.ts";%s`, EOL)
					} else {
						var browserExclude bool
						if len(ctx.pkgJson.Browser) > 0 {
							if name, ok := ctx.pkgJson.Browser["process"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import __Process$ from "/node/process.js";%s`, EOL)
						}
					}
				}
				if ids.Has("__Buffer$") {
					if ctx.args.external.Has("node:buffer") || ctx.args.external.Has("*") {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if ctx.target == "denonext" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if ctx.target == "deno" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "https://deno.land/std@0.177.1/node/buffer.ts";%s`, EOL)
					} else {
						var browserExclude bool
						if len(ctx.pkgJson.Browser) > 0 {
							if name, ok := ctx.pkgJson.Browser["buffer"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "/node/buffer.js";%s`, EOL)
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

			if len(ctx.requires) > 0 {
				isEsModule := make([]bool, len(ctx.requires))
				for i, d := range ctx.requires {
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
						pkg, p, installed, e := ctx.lookupDep(specifier)
						if e == nil {
							if p.Type == "module" {
								isEsModule[i] = true
							} else {
								ctx := NewBuildContext(ctx.zoneId, ctx.npmrc, pkg, ctx.args, ctx.target, BundleFalse, ctx.dev, false)
								if installed {
									ctx.wd = path.Join(ctx.wd, "node_modules", ".pnpm")
								} else {
									ctx.wd = path.Join(ctx.npmrc.Dir(), pkg.FullName())
									ctx.npmrc.installPackage(pkg)
								}
								m, _, _, e := ctx.init(true)
								if e == nil && includes(m.NamedExports, "__esModule") {
									isEsModule[i] = true
								}
							}
						}
					}
				}
				fmt.Fprint(header, `var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({__esModule:true},m);switch(n){`)
				record := NewStringSet()
				for i, d := range ctx.requires {
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
			ctx.smOffset += strings.Count(header.String(), EOL)

			ret, dropSourceMap := ctx.rewriteJS(jsContent)
			if ret != nil {
				jsContent = ret
			}

			finalContent := bytes.NewBuffer(nil)
			finalContent.Write(header.Bytes())
			finalContent.Write(jsContent)

			if ctx.pkgDeprecated != "" {
				fmt.Fprintf(finalContent, `console.warn("[npm] %%cdeprecated%%c %s@%s: %s", "color:red", "");%s`, ctx.pkg.Name, ctx.pkg.Version, strings.ReplaceAll(ctx.pkgDeprecated, "\"", "\\\""), "\n")
			}

			// add sourcemap Url
			if ctx.sourceMap && !dropSourceMap {
				finalContent.WriteString("//# sourceMappingURL=")
				finalContent.WriteString(filepath.Base(ctx.Path()))
				finalContent.WriteString(".map")
			}

			_, err = fs.WriteFile(ctx.getSavepath(), finalContent)
			if err != nil {
				return
			}
		}
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".css") {
			savePath := ctx.getSavepath()
			_, err = fs.WriteFile(strings.TrimSuffix(savePath, path.Ext(savePath))+".css", bytes.NewReader(file.Contents))
			if err != nil {
				return
			}
			result.PackageCSS = true
		} else if ctx.sourceMap && strings.HasSuffix(file.Path, ".js.map") {
			var sourceMap map[string]interface{}
			if json.Unmarshal(file.Contents, &sourceMap) == nil {
				if mapping, ok := sourceMap["mappings"].(string); ok {
					fixedMapping := make([]byte, ctx.smOffset+len(mapping))
					for i := 0; i < ctx.smOffset; i++ {
						fixedMapping[i] = ';'
					}
					copy(fixedMapping[ctx.smOffset:], mapping)
					sourceMap["mappings"] = string(fixedMapping)
				}
				buf := bytes.NewBuffer(nil)
				if json.NewEncoder(buf).Encode(sourceMap) == nil {
					_, err = fs.WriteFile(ctx.getSavepath()+".map", buf)
					if err != nil {
						return
					}
				}
			}
		}
	}

	// wait for sub-builds
	ctx.wg.Wait()

	record := NewStringSet()
	result.Deps = filter(imports, func(dep string) bool {
		if record.Has(dep) {
			return false
		}
		record.Add(dep)
		return strings.HasPrefix(dep, "/") || isHttpSepcifier(dep)
	})
	result.Dts = ctx.checkTypes(entry)
	return
}

func (ctx *BuildContext) buildTypes(types string) (err error) {
	start := time.Now()
	buildArgsPrefix := ctx.getBuildArgsAsPathSegment(ctx.pkg, true)
	n, err := transformDTS(ctx, types, buildArgsPrefix, nil)
	if err != nil {
		return
	}
	log.Debugf("transform dts '%s'(%d related dts files) in %v", types, n, time.Since(start))
	return
}

func (ctx *BuildContext) resolveExternalModule(specifier string, kind api.ResolveKind) (resolvedPath string) {
	defer func() {
		fullResolvedPath := resolvedPath
		// use relative path for sub-module of current package
		if strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") {
			rel, err := filepath.Rel(filepath.Dir(ctx.Path()), resolvedPath)
			if err == nil {
				if !(strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../")) {
					rel = "./" + rel
				}
				resolvedPath = rel
			}
		}
		// mark the resolved path for _preload_
		if kind != api.ResolveJSDynamicImport {
			ctx.imports = append(ctx.imports, [2]string{fullResolvedPath, resolvedPath})
		}
		// if it's `require("module")` call
		if kind == api.ResolveJSRequireCall {
			ctx.requires = append(ctx.requires, [2]string{specifier, resolvedPath})
			resolvedPath = specifier
		}
	}()

	// it's current package from github
	if npm := ctx.pkgJson; ctx.pkg.FromGithub && (specifier == npm.Name || specifier == npm.PkgName) {
		pkg := Pkg{
			Name:       npm.Name,
			Version:    npm.Version,
			FromGithub: true,
		}
		resolvedPath = ctx.getImportPath(pkg, ctx.getBuildArgsAsPathSegment(pkg, false))
		return
	}

	// node builtin module
	if nodejsInternalModules[specifier] {
		if ctx.args.external.Has("node:"+specifier) || ctx.args.external.Has("*") {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if ctx.target == "node" {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if ctx.target == "denonext" && !denoNextUnspportedNodeModules[specifier] {
			resolvedPath = fmt.Sprintf("node:%s", specifier)
		} else if ctx.target == "deno" {
			resolvedPath = fmt.Sprintf("https://deno.land/std@0.177.1/node/%s.ts", specifier)
		} else {
			resolvedPath = fmt.Sprintf("/node/%s.js", specifier)
		}
		return
	}

	// check `?external`
	if ctx.args.external.Has("*") || ctx.args.external.Has(getPkgName(specifier)) {
		resolvedPath = specifier
		return
	}

	// it's sub-module of current package
	if strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") {
		subPath := strings.TrimPrefix(specifier, ctx.pkgJson.Name+"/")
		subPkg := Pkg{
			Name:       ctx.pkg.Name,
			Version:    ctx.pkg.Version,
			SubPath:    subPath,
			SubModule:  toModuleBareName(subPath, false),
			FromGithub: ctx.pkg.FromGithub,
		}
		if ctx.subBuilds != nil {
			buildCtx := &BuildContext{
				zoneId:        ctx.zoneId,
				npmrc:         ctx.npmrc,
				pkg:           subPkg,
				pkgJson:       ctx.pkgJson,
				pkgDeprecated: ctx.pkgDeprecated,
				args:          ctx.args,
				target:        ctx.target,
				dev:           ctx.dev,
				sourceMap:     ctx.sourceMap,
				wd:            ctx.wd,
				subBuilds:     ctx.subBuilds,
			}
			if ctx.bundleMode == BundleFalse {
				buildCtx.bundleMode = BundleFalse
			}
			id := buildCtx.Path()
			if !ctx.subBuilds.Has(id) {
				ctx.subBuilds.Add(id)
				ctx.wg.Add(1)
				go func() {
					defer ctx.wg.Done()
					buildCtx.Build()
				}()
			}
		}
		resolvedPath = ctx.getImportPath(subPkg, ctx.getBuildArgsAsPathSegment(subPkg, false))
		if ctx.bundleMode == BundleFalse {
			n, e := utils.SplitByLastByte(resolvedPath, '.')
			resolvedPath = n + ".nobundle." + e
		}
		return
	}

	// replace some npm polyfills with native APIs
	if specifier == "node-fetch" && ctx.target != "node" {
		resolvedPath = "npm_node-fetch.js"
		return
	}
	data, err := embedFS.ReadFile(("server/embed/polyfills/npm_" + specifier + ".js"))
	if err == nil {
		resolvedPath = fmt.Sprintf("data:application/javascript;base64,%s", base64.StdEncoding.EncodeToString(data))
		return
	}

	// common npm dependency
	pkgName, version, subpath, _ := splitPkgPath(specifier)
	if version == "" {
		if pkgName == ctx.pkg.Name {
			version = ctx.pkg.Version
		} else if pkg, ok := ctx.args.deps.Get(pkgName); ok {
			version = pkg.Version
		} else if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok {
			version = v
		} else if v, ok := ctx.pkgJson.PeerDependencies[pkgName]; ok {
			version = v
		} else {
			version = "latest"
		}
	}
	// force the version of 'react' (as dependency) equals to 'react-dom'
	if ctx.pkg.Name == "react-dom" && pkgName == "react" {
		version = ctx.pkg.Version
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
			resolvedPath = fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", pkgName, ctx.pkg)
			return
		}
		if strings.HasPrefix(version, "npm:") {
			pkg.Name, pkg.Version, _, _ = splitPkgPath(version[4:])
		} else if strings.HasPrefix(version, "git+ssh://") || strings.HasPrefix(version, "git+https://") || strings.HasPrefix(version, "git://") {
			gitUrl, err := url.Parse(version)
			if err != nil || gitUrl.Hostname() != "github.com" {
				resolvedPath = fmt.Sprintf("/error.js?type=unsupported-git-dependency&name=%s&importer=%s", pkgName, ctx.pkg)
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
			_, p, _, err := ctx.lookupDep(pkgName + "@" + version)
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
		alias:      ctx.args.alias,
		conditions: ctx.args.conditions,
		deps:       ctx.args.deps,
		external:   ctx.args.external,
		exports:    NewStringSet(),
	}
	fixBuildArgs(ctx.npmrc, &args, pkg)
	if caretVersion {
		resolvedPath = "/" + pkg.Name + "@^" + pkg.Version
		if pkg.SubModule != "" {
			resolvedPath += "/" + pkg.SubModule
		}
		// workaround for es5-ext weird "/#/" path
		if pkg.Name == "es5-ext" {
			resolvedPath = strings.ReplaceAll(resolvedPath, "/#/", "/%23/")
		}
		params := []string{"target=" + ctx.target}
		if len(args.alias) > 0 {
			var alias []string
			for k, v := range args.alias {
				alias = append(alias, fmt.Sprintf("%s:%s", k, v))
			}
			params = append(params, "alias="+strings.Join(alias, ","))
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
		if args.conditions.Len() > 0 {
			params = append(params, "conditions="+strings.Join(args.conditions.Values(), ","))
		}
		if ctx.dev {
			params = append(params, "dev")
		}
		if ctx.isDenoTarget() {
			params = append(params, "no-dts")
		}
		resolvedPath += "?" + strings.Join(params, "&")
	} else {
		buildArgsPrefix := ""
		if a := encodeBuildArgs(args, pkg, false); a != "" {
			buildArgsPrefix = "X-" + a + "/"
		}
		resolvedPath = ctx.getImportPath(pkg, buildArgsPrefix)
	}
	return
}

func (ctx *BuildContext) checkTypes(entry PackageEntry) string {
	if entry.dts != "" {
		if !existsFile(path.Join(ctx.wd, "node_modules", ctx.pkg.Name, entry.dts)) {
			return ""
		}
		return fmt.Sprintf(
			"/%s%s@%s/%s%s",
			ctx.pkg.ghPrefix(),
			ctx.pkg.Name,
			ctx.pkgJson.Version,
			ctx.getBuildArgsAsPathSegment(ctx.pkg, true),
			utils.CleanPath(path.Join("/", entry.dts))[1:],
		)
	}

	// use types from package "@types/[task.npm.Name]" if it exists
	if ctx.pkgJson.Types == "" && !strings.HasPrefix(ctx.pkgJson.Name, "@types/") {
		versionParts := strings.Split(ctx.pkgJson.Version, ".")
		versions := []string{
			versionParts[0] + "." + versionParts[1], // major.minor
			versionParts[0],                         // major
		}
		typesPkgName := toTypesPkgName(ctx.pkgJson.Name)
		pkg, ok := ctx.args.deps.Get(typesPkgName)
		if ok {
			// use the version of the `?deps` query if it exists
			versions = append([]string{pkg.Version}, versions...)
		}
		for _, version := range versions {
			p, err := ctx.npmrc.getPackageInfo(typesPkgName, version)
			if err == nil {
				typesPkg := Pkg{
					Name:      typesPkgName,
					Version:   p.Version,
					SubModule: ctx.pkg.SubModule,
					SubPath:   ctx.pkg.SubPath,
				}
				buildCtx := NewBuildContext(ctx.zoneId, ctx.npmrc, typesPkg, ctx.args, "types", BundleFalse, false, false)
				ret, err := buildCtx.Build()
				if err == nil {
					// use _caret_ semver range instead of the exact version
					return strings.ReplaceAll(ret.Dts, fmt.Sprintf("%s@%s", typesPkgName, p.Version), fmt.Sprintf("%s@^%s", typesPkgName, p.Version))
				}
				break
			}
		}
	}

	return ""
}
