package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/server/storage"
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
	pkgDir        string
	pnpmPkgDir    string
	path          string
	stage         string
	imports       [][2]string
	requires      [][3]string
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

var loaders = map[string]api.Loader{
	".js":    api.LoaderJS,
	".mjs":   api.LoaderJS,
	".cjs":   api.LoaderJS,
	".jsx":   api.LoaderJSX,
	".ts":    api.LoaderTS,
	".tsx":   api.LoaderTSX,
	".mts":   api.LoaderTS,
	".css":   api.LoaderCSS,
	".json":  api.LoaderJSON,
	".txt":   api.LoaderText,
	".html":  api.LoaderText,
	".md":    api.LoaderText,
	".svg":   api.LoaderDataURL,
	".png":   api.LoaderDataURL,
	".webp":  api.LoaderDataURL,
	".gif":   api.LoaderDataURL,
	".ttf":   api.LoaderDataURL,
	".eot":   api.LoaderDataURL,
	".woff":  api.LoaderDataURL,
	".woff2": api.LoaderDataURL,
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
		subBuilds:  NewStringSet(),
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
			// ensure the build file exists
			if err == nil || err != storage.ErrNotFound {
				return b, true
			}
		}
		// delete the invalid db entry
		db.Delete(key)
	}
	return BuildResult{}, false
}

func (ctx *BuildContext) Build() (ret BuildResult, err error) {
	if ctx.target == "types" {
		return ctx.buildTypes()
	}

	// query the build result from db
	ret, ok := ctx.Query()
	if ok {
		return
	}

	// check if the package is deprecated
	if ctx.pkgDeprecated == "" && !ctx.pkg.FromGithub && !strings.HasPrefix(ctx.pkg.Name, "@jsr/") {
		var info PackageJSON
		info, err = ctx.npmrc.fetchPackageInfo(ctx.pkg.Name, ctx.pkg.Version)
		if err != nil {
			return
		}
		ctx.pkgDeprecated = info.Deprecated
	}

	// install the package
	ctx.stage = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	// query again after installation (in case the `normalizePackageJSON` method has changed the sub-module path)
	ret, ok = ctx.Query()
	if ok {
		return
	}

	// build the module
	ctx.stage = "build"
	ret, err = ctx.buildModule()
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

func (ctx *BuildContext) install() (err error) {
	if ctx.wd == "" || ctx.pkgJson.Name == "" {
		err = ctx.npmrc.installPackage(ctx.pkg)
		if err != nil {
			return
		}
		ctx.wd = path.Join(ctx.npmrc.Dir(), ctx.pkg.Fullname())
		ctx.pkgDir = path.Join(ctx.wd, "node_modules", ctx.pkg.Name)
		if rp, e := os.Readlink(ctx.pkgDir); e == nil {
			ctx.pnpmPkgDir = path.Join(path.Dir(ctx.pkgDir), rp)
		} else {
			ctx.pnpmPkgDir = ctx.pkgDir
		}
		var pkgJson PackageJSON
		err = parseJSONFile(path.Join(ctx.pkgDir, "package.json"), &pkgJson)
		if err != nil {
			return
		}
		ctx.pkgJson = ctx.normalizePackageJSON(pkgJson)
	}
	return
}

func (ctx *BuildContext) buildModule() (result BuildResult, err error) {
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

	entry := ctx.resolveEntry(ctx.pkg)
	if entry.isEmpty() {
		err = fmt.Errorf("could not resolve entry")
		return
	}
	log.Debugf("build(%s): Entry%+v", ctx.pkg, entry)

	typesOnly := strings.HasPrefix(ctx.pkgJson.Name, "@types/") || (entry.esm == "" && entry.cjs == "" && entry.dts != "")
	if typesOnly {
		result.TypesOnly = true
		result.Dts = "/" + ctx.pkg.ghPrefix() + ctx.pkg.Fullname() + entry.dts[1:]
		ctx.transformDTS(entry.dts)
		return
	}

	result, reexport, err := ctx.lexer(&entry, false)
	if err != nil && !strings.HasPrefix(err.Error(), "cjsLexer: Can't resolve") {
		return
	}

	// cjs reexport
	if reexport != "" {
		pkg, _, _, e := ctx.lookupDep(reexport)
		if e != nil {
			err = e
			return
		}
		// create a new build context to check if the reexported module has default export
		b := NewBuildContext(ctx.zoneId, ctx.npmrc, pkg, ctx.args, ctx.target, BundleFalse, ctx.dev, false)
		err = b.install()
		if err != nil {
			return
		}
		var r BuildResult
		entry := b.resolveEntry(pkg)
		r, _, err = b.lexer(&entry, false)
		if err != nil {
			return
		}
		buf := bytes.NewBuffer(nil)
		importPath := ctx.getImportPath(pkg, ctx.getBuildArgsPrefix(pkg, false))
		fmt.Fprintf(buf, `export * from "%s";`, importPath)
		if r.HasDefaultExport {
			fmt.Fprintf(buf, "\n")
			fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
		}
		_, err = fs.WriteFile(ctx.getSavepath(), buf)
		if err != nil {
			return
		}
		result.Dts, err = ctx.resloveDTS(entry)
		return
	}

	var entryPoint string
	var input *api.StdinOptions

	entryModuleSpecifier := ctx.pkg.Name
	if ctx.pkg.SubModule != "" {
		entryModuleSpecifier += "/" + ctx.pkg.SubModule
	}

	if entry.esm == "" {
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `import * as __module from "%s";`, entryModuleSpecifier)
		if len(result.NamedExports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = __module;`, strings.Join(result.NamedExports, ","))
		}
		fmt.Fprintf(buf, "const { default: __default, ...__rest } = __module;")
		fmt.Fprintf(buf, "export default (__default !== undefined ? __default : __rest);")
		// Default reexport all members from original module to prevent missing named exports members
		fmt.Fprintf(buf, `export * from "%s";`, entryModuleSpecifier)
		input = &api.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: ctx.wd,
			Sourcefile: "entry.js",
		}
	} else {
		if ctx.args.exports.Len() > 0 {
			input = &api.StdinOptions{
				Contents:   fmt.Sprintf(`export { %s } from "%s";`, strings.Join(ctx.args.exports.Values(), ","), entryModuleSpecifier),
				ResolveDir: ctx.wd,
				Sourcefile: "entry.js",
			}
		} else {
			entryPoint = path.Join(ctx.pkgDir, entry.esm)
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

	browserExclude := map[string]*StringSet{}
	implicitExternal := NewStringSet()
	imports := NewStringSet()
	tarballs := NewStringSet()
	esmPlugin := api.Plugin{
		Name: "esm",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					// if it's the entry module
					if args.Path == entryPoint || args.Path == entryModuleSpecifier {
						if args.Path == entryModuleSpecifier {
							if entry.esm != "" {
								return api.OnResolveResult{Path: path.Join(ctx.pnpmPkgDir, entry.esm)}, nil
							}
							if entry.cjs != "" {
								return api.OnResolveResult{Path: path.Join(ctx.pnpmPkgDir, entry.cjs)}, nil
							}
						}
						return api.OnResolveResult{}, nil
					}

					// ban file urls
					if strings.HasPrefix(args.Path, "file:") {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(args.Path, "file:"), ctx.pkg),
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
					if ctx.args.externalRequire && args.Kind == api.ResolveJSRequireCall && entry.esm != "" {
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
							Path:     ctx.resolveExternalModule(args.Path, args.Kind),
							External: true,
						}, nil
					}

					// normalize specifier
					specifier := strings.TrimPrefix(args.Path, "node:")
					specifier = strings.TrimPrefix(specifier, "npm:")

					// resolve specifier by checking `?alias` query
					if len(ctx.args.alias) > 0 && !isRelativeSpecifier(specifier) {
						pkgName, _, subpath, _ := splitPkgPath(specifier)
						if name, ok := ctx.args.alias[pkgName]; ok {
							specifier = name
							if subpath != "" {
								specifier += "/" + subpath
							}
						}
					}

					// resolve specifier with package `imports` field
					if len(ctx.pkgJson.Imports) > 0 {
						if v, ok := ctx.pkgJson.Imports[specifier]; ok {
							if s, ok := v.(string); ok {
								specifier = s
							} else if m, ok := v.(map[string]interface{}); ok {
								targets := []string{"browser", "module", "import", "default"}
								if ctx.isDenoTarget() {
									targets = []string{"deno", "module", "import", "default"}
								} else if ctx.target == "node" {
									targets = []string{"node", "module", "import", "default"}
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
					}

					// resolve specifier with package `browser` field
					if !isRelativeSpecifier(specifier) && len(ctx.pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
						if name, ok := ctx.pkgJson.Browser[specifier]; ok {
							if name == "" {
								return api.OnResolveResult{
									Path:      args.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							specifier = name
						}
					}

					// use polyfilled 'fsevents' module for browser
					if specifier == "fsevents" && ctx.isBrowserTarget() {
						return api.OnResolveResult{
							Path:     "npm_fsevents.js",
							External: true,
						}, nil
					}

					// force to use `npm:` specifier for `denonext` target
					if forceNpmSpecifiers[specifier] && ctx.target == "denonext" {
						version := ""
						pkgName, _, subPath, _ := splitPkgPath(specifier)
						if pkgName == ctx.pkg.Name {
							version = ctx.pkg.Version
						} else if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok && regexpFullVersion.MatchString(v) {
							version = v
						} else if v, ok := ctx.pkgJson.PeerDependencies[pkgName]; ok && regexpFullVersion.MatchString(v) {
							version = v
						}
						p := pkgName
						if version != "" {
							p += "@" + version
						}
						if subPath != "" {
							p += "/" + subPath
						}
						return api.OnResolveResult{
							Path:     fmt.Sprintf("npm:%s", p),
							External: true,
						}, nil
					}

					var fullFilepath string
					if strings.HasPrefix(specifier, "/") {
						fullFilepath = specifier
					} else if isRelativeSpecifier(specifier) {
						fullFilepath = path.Join(args.ResolveDir, specifier)
					} else {
						fullFilepath = path.Join(ctx.wd, "node_modules", ".pnpm", "node_modules", specifier)
					}

					// node native modules do not work via http import
					if strings.HasSuffix(fullFilepath, ".node") && existsFile(fullFilepath) {
						return api.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), ctx.pkg),
							External: true,
						}, nil
					}

					// bundles json module
					if strings.HasSuffix(fullFilepath, ".json") {
						return api.OnResolveResult{}, nil
					}

					// embed wasm as WebAssembly.Module
					if strings.HasSuffix(fullFilepath, ".wasm") {
						return api.OnResolveResult{
							Path:      fullFilepath,
							Namespace: "wasm",
						}, nil
					}

					// externalize the _parent_ module
					// e.g. "react/jsx-runtime" imports "react"
					if ctx.pkg.SubModule != "" && specifier == ctx.pkg.Name && ctx.bundleMode != BundleAll {
						return api.OnResolveResult{
							Path:        ctx.resolveExternalModule(ctx.pkg.Name, args.Kind),
							External:    true,
							SideEffects: pkgSideEffects,
						}, nil
					}

					// it's nodejs internal module
					if nodejsInternalModules[specifier] {
						return api.OnResolveResult{
							Path:     ctx.resolveExternalModule(specifier, args.Kind),
							External: true,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query]
					if ctx.bundleMode == BundleAll && !ctx.args.external.Has(getPkgName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := getPkgName(specifier)
						_, ok := ctx.pkgJson.PeerDependencies[pkgName]
						if !ok {
							return api.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind != api.ResolveJSDynamicImport && !noBundle) && ctx.pkgJson.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/")) {
						return api.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelativeSpecifier(specifier) {
						specifier = strings.TrimPrefix(fullFilepath, path.Join(ctx.wd, "node_modules")+"/")
						if strings.HasPrefix(specifier, ".pnpm") {
							a := strings.Split(specifier, "/node_modules/")
							if len(a) > 1 {
								specifier = a[1]
							}
						}
						pkgName := ctx.pkgJson.Name
						isInternalModule := strings.HasPrefix(specifier, pkgName+"/")
						if !isInternalModule && ctx.pkgJson.PkgName != "" {
							// github packages may have different package name with the repository name
							pkgName = ctx.pkgJson.PkgName
							isInternalModule = strings.HasPrefix(specifier, pkgName+"/")
						}
						if isInternalModule {
							// if meets scenarios of "./index.mjs" importing "./index.c?js"
							// let esbuild to handle it
							if stripModuleExt(fullFilepath) == stripModuleExt(args.Importer) {
								return api.OnResolveResult{}, nil
							}

							moduleSpecifier := "." + strings.TrimPrefix(specifier, pkgName)

							if path.Ext(fullFilepath) == "" || !existsFile(fullFilepath) {
								subPath := utils.CleanPath(moduleSpecifier)[1:]
								entry := ctx.resolveEntry(Pkg{
									Name:      ctx.pkg.Name,
									Version:   ctx.pkg.Version,
									SubModule: toModuleBareName(subPath, true),
									SubPath:   subPath,
								})
								if args.Kind == api.ResolveJSImportStatement || args.Kind == api.ResolveJSDynamicImport {
									if entry.esm != "" {
										moduleSpecifier = entry.esm
									} else if entry.cjs != "" {
										moduleSpecifier = entry.cjs
									}
								} else if args.Kind == api.ResolveJSRequireCall || args.Kind == api.ResolveJSRequireResolve {
									if entry.cjs != "" {
										moduleSpecifier = entry.cjs
									} else if entry.esm != "" {
										moduleSpecifier = entry.esm
									}
								}
							}

							// resolve specifier with package `browser` field
							if len(ctx.pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
								if path, ok := ctx.pkgJson.Browser[moduleSpecifier]; ok {
									if path == "" {
										return api.OnResolveResult{
											Path:      args.Path,
											Namespace: "browser-exclude",
										}, nil
									}
									if !isRelativeSpecifier(path) {
										return api.OnResolveResult{
											Path:     ctx.resolveExternalModule(path, args.Kind),
											External: true,
										}, nil
									}
									moduleSpecifier = path
								}
							}

							bareName := stripModuleExt(moduleSpecifier)

							// split modules based on the `exports` field of package.json
							if om, ok := ctx.pkgJson.Exports.(*OrderedMap); ok {
								for _, exportName := range om.keys {
									v := om.Get(exportName)
									if !(exportName == "." || strings.HasPrefix(exportName, "./")) {
										continue
									}
									if strings.ContainsRune(exportName, '*') {
										var (
											match  bool
											prefix string
											suffix string
										)
										if s, ok := v.(string); ok {
											// exports: "./*": "./dist/*.js"
											prefix, suffix = utils.SplitByLastByte(s, '*')
											match = strings.HasPrefix(bareName, prefix) && (suffix == "" || strings.HasSuffix(moduleSpecifier, suffix))
										} else if m, ok := v.(*OrderedMap); ok {
											// exports: "./*": { "import": "./dist/*.js" }
											// exports: "./*": { "import": { default: "./dist/*.js" } }
											// ...
											paths := getAllExportsPaths(m)
											for _, path := range paths {
												prefix, suffix = utils.SplitByLastByte(path, '*')
												match = strings.HasPrefix(bareName, prefix) && (suffix == "" || strings.HasSuffix(moduleSpecifier, suffix))
												if match {
													break
												}
											}
										}
										if match {
											exportPrefix, _ := utils.SplitByLastByte(exportName, '*')
											exportModuleName := path.Join(ctx.pkgJson.Name, exportPrefix+strings.TrimPrefix(bareName, prefix))
											if exportModuleName != entryModuleSpecifier && exportModuleName != entryModuleSpecifier+"/index" {
												return api.OnResolveResult{
													Path:        ctx.resolveExternalModule(exportModuleName, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									} else {
										match := false
										if s, ok := v.(string); ok && stripModuleExt(s) == bareName {
											// exports: "./foo": "./foo.js"
											match = true
										} else if m, ok := v.(*OrderedMap); ok {
											// exports: "./foo": { "import": "./foo.js" }
											// exports: "./foo": { "import": { default: "./foo.js" } }
											// ...
											paths := getAllExportsPaths(m)
											for _, path := range paths {
												if stripModuleExt(path) == bareName {
													match = true
													break
												}
											}
										}
										if match {
											exportModuleName := path.Join(ctx.pkgJson.Name, stripModuleExt(exportName))
											if exportModuleName != entryModuleSpecifier && exportModuleName != entryModuleSpecifier+"/index" {
												return api.OnResolveResult{
													Path:        ctx.resolveExternalModule(exportModuleName, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// module file path
							moduleFilepath := path.Join(ctx.pnpmPkgDir, moduleSpecifier)

							// if it's the entry module
							if moduleSpecifier == entry.cjs || moduleSpecifier == entry.esm {
								return api.OnResolveResult{Path: moduleFilepath}, nil
							}

							// split the module that is an alias of a dependency
							// means this file just include a single line(js): `export * from "dep"`
							fi, ioErr := os.Lstat(moduleFilepath)
							if ioErr == nil && fi.Size() < 128 {
								data, ioErr := os.ReadFile(moduleFilepath)
								if ioErr == nil {
									out, esbErr := minify(string(data), api.ESNext, api.LoaderJS)
									if esbErr == nil {
										p := bytes.Split(out, []byte("\""))
										if len(p) == 3 && string(p[0]) == "export*from" && string(p[2]) == ";\n" {
											url := string(p[1])
											if !isRelativeSpecifier(url) {
												return api.OnResolveResult{
													Path:        ctx.resolveExternalModule(url, args.Kind),
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// bundle the internal module if it's not a dynamic import or `?bundle=false` query present
							if args.Kind != api.ResolveJSDynamicImport && !noBundle {
								if existsFile(moduleFilepath) {
									return api.OnResolveResult{Path: moduleFilepath}, nil
								}
								// let esbuild to handle it
								return api.OnResolveResult{}, nil
							}
						}
					}

					// dynamic external
					sideEffects := api.SideEffectsFalse
					if specifier == ctx.pkgJson.Name || specifier == ctx.pkgJson.PkgName || strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") || strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") {
						sideEffects = pkgSideEffects
					}
					return api.OnResolveResult{
						Path:        ctx.resolveExternalModule(specifier, args.Kind),
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
					code := fmt.Sprintf("export default Uint8Array.from(atob('%s'), c => c.charCodeAt(0))", wasm64)
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

			build.OnLoad(
				api.OnLoadOptions{Filter: "\\.c?js$"},
				func(args api.OnLoadArgs) (ret api.OnLoadResult, err error) {
					data, err := os.ReadFile(args.Path)
					if err != nil {
						return
					}

					if bytes.Contains(data, []byte("__filename")) || bytes.Contains(data, []byte("__dirname")) {
						defines := map[string]string{
							"__filename": "__filename$",
							"__dirname":  "__dirname$",
						}
						r := api.Transform(string(data), api.TransformOptions{
							Loader:    api.LoaderJS,
							Define:    defines,
							Sourcemap: api.SourceMapInline,
						})
						if len(r.Errors) > 0 {
							ret.Errors = r.Errors
							return
						}
						js := string(regexpGlobalIdent.ReplaceAllFunc(r.Code, func(b []byte) []byte {
							id := string(b)
							if id != "__filename$" && id != "__dirname$" {
								return b
							}
							filename := strings.TrimPrefix(args.Path, path.Join(ctx.wd, "node_modules")+"/")
							if strings.HasPrefix(filename, ".pnpm") {
								a := strings.Split(filename, "/node_modules/")
								if len(a) > 1 {
									filename = a[1]
								}
							}
							pkgName, _, subPath, _ := splitPkgPath(filename)
							pkgVersion := ""
							if ctx.pkgJson.Name == pkgName {
								pkgVersion = ctx.pkgJson.Version
							} else {
								_, pkgJson, _, err := ctx.lookupDep(pkgName)
								if err != nil {
									return b
								}
								pkgVersion = pkgJson.Version
							}
							filename = pkgName + "@" + pkgVersion + "/" + subPath
							registry := ctx.npmrc.NpmRegistry.Registry
							if pkgName[0] == '@' {
								scope, _ := utils.SplitByFirstByte(pkgName, '/')
								if reg, ok := ctx.npmrc.Registries[scope]; ok {
									registry = reg.Registry
								}
							}
							if !ctx.isBrowserTarget() {
								tarballs.Add(fmt.Sprintf("%s %s %s", registry, pkgName, pkgVersion))
							}
							if id == "__filename$" {
								if ctx.isBrowserTarget() {
									return []byte(fmt.Sprintf(`"/https/esm.sh/%s"`, filename))
								}
								return []byte(fmt.Sprintf(`__filename$("%s")`, filename))
							} else if id == "__dirname$" {
								dirname, _ := utils.SplitByLastByte(filename, '/')
								if ctx.isBrowserTarget() {
									return []byte(fmt.Sprintf(`"/https/esm.sh/%s"`, dirname))
								}
								return []byte(fmt.Sprintf(`__dirname$("%s")`, dirname))
							}
							return b
						}))
						return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
					}
					js := string(data)
					return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
				},
			)
		},
	}

	nodeEnv := ctx.getNodeEnv()
	define := map[string]string{
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
		define = map[string]string{
			"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, nodeEnv),
			"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
		}
	}
	conditions := ctx.args.conditions
	if ctx.dev {
		conditions = append(conditions, "development")
	}
	if ctx.isDenoTarget() {
		conditions = append(conditions, "deno")
	}
	minify := config.Minify == nil || !bytes.Equal(config.Minify, []byte("false"))
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Define:            define,
		Format:            api.FormatESModule,
		Target:            targets[ctx.target],
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		KeepNames:         ctx.args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: ctx.args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        conditions,
		Loader:            loaders,
		Plugins:           []api.Plugin{esmPlugin},
		SourceRoot:        "/",
	}
	// ignore features that can not be polyfilled
	options.Supported = map[string]bool{
		"bigint":          true,
		"top-level-await": true,
	}
	if ctx.target == "node" {
		options.Platform = api.PlatformNode
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
		} else if pkgVersion, ok := ctx.args.deps["react"]; ok {
			options.JSXImportSource = "/react@" + pkgVersion
		} else if pkgVersion, ok := ctx.args.deps["preact"]; ok {
			options.JSXImportSource = "/preact@" + pkgVersion
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
			// current module can not be marked as an external
			if strings.HasPrefix(msg, fmt.Sprintf("Could not resolve \"%s\"", entryModuleSpecifier)) {
				err = fmt.Errorf("could not resolve \"%s\"", entryModuleSpecifier)
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
		fmt.Println(ret.Errors[0].Location)
		return
	}

	for _, w := range ret.Warnings {
		log.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
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
					} else if ctx.isBrowserTarget() {
						var browserExclude bool
						if len(ctx.pkgJson.Browser) > 0 {
							if name, ok := ctx.pkgJson.Browser["process"]; ok {
								browserExclude = name == ""
							} else if name, ok := ctx.pkgJson.Browser["node:process"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import __Process$ from "/node/process.js";%s`, EOL)
							imports.Add("/node/process.js")
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
					} else if ctx.isBrowserTarget() {
						var browserExclude bool
						if len(ctx.pkgJson.Browser) > 0 {
							if name, ok := ctx.pkgJson.Browser["buffer"]; ok {
								browserExclude = name == ""
							} else if name, ok := ctx.pkgJson.Browser["node:buffer"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "/node/buffer.js";%s`, EOL)
							imports.Add("/node/buffer.js")
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
				if ids.Has("__filename$") {
					fmt.Fprintf(header, `import { __filename$ } from "/node/filename_resolver.js";%s`, EOL)
				}
				if ids.Has("__dirname$") {
					fmt.Fprintf(header, `import { __dirname$ } from "/node/filename_resolver.js";%s`, EOL)
				}
			}

			if tarballs.Len() > 0 {
				fmt.Fprintf(header, `import { __downloadPackageTarball$ } from "/node/filename_resolver.js";%s`, EOL)
				for _, tarball := range tarballs.Values() {
					fmt.Fprintf(header, `await __downloadPackageTarball$("%s");%s`, tarball, EOL)
				}
			}

			if len(ctx.requires) > 0 {
				record := NewStringSet()
				requires := make([][3]string, 0, len(ctx.requires))
				for _, r := range ctx.requires {
					specifier := r[0]
					if record.Has(specifier) {
						continue
					}
					record.Add(specifier)
					requires = append(requires, r)
				}
				isEsModule := make([]bool, len(requires))
				for i, r := range requires {
					specifier := r[0]
					fmt.Fprintf(header, `import * as __%x$ from "%s";%s`, i, r[2], EOL)
					imports.Add(r[1])
					if bytes.Contains(jsContent, []byte(fmt.Sprintf(`("%s").default`, specifier))) {
						// if `require("SPECIFIER").default` found
						isEsModule[i] = true
						continue
					}
					if !isRelativeSpecifier(specifier) && !nodejsInternalModules[specifier] {
						if a := bytes.SplitN(jsContent, []byte(fmt.Sprintf(`("%s")`, specifier)), 2); len(a) >= 2 {
							ret := regexpVarEqual.FindSubmatch(a[0])
							if len(ret) == 2 {
								r, e := regexp.Compile(fmt.Sprintf(`[^\w$]%s(\(|\.default[^\w$])`, string(ret[1])))
								if e == nil {
									ret := r.FindSubmatch(jsContent)
									if len(ret) == 2 {
										// `var mod = require("module");...;mod()` is cjs
										// `var mod = require("module");...;mod.default` is es module
										isEsModule[i] = string(ret[1]) != "("
										continue
									}
								}
							}
						}
						pkg, p, _, e := ctx.lookupDep(specifier)
						if e == nil {
							p = ctx.normalizePackageJSON(p)
							if p.Type == "module" || p.Module != "" {
								isEsModule[i] = true
							} else {
								b := NewBuildContext(ctx.zoneId, ctx.npmrc, pkg, ctx.args, ctx.target, BundleFalse, ctx.dev, false)
								e = b.install()
								if e == nil {
									entry := b.resolveEntry(pkg)
									ret, _, e := b.lexer(&entry, true)
									if e == nil && includes(ret.NamedExports, "__esModule") {
										isEsModule[i] = true
									}
								}
							}
						}
					}
				}
				fmt.Fprint(header, `var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({__esModule:true},m);switch(n){`)
				for i, r := range requires {
					specifier := r[0]
					esModule := isEsModule[i]
					if esModule {
						fmt.Fprintf(header, `case"%s":return c(__%x$);`, specifier, i)
					} else {
						fmt.Fprintf(header, `case"%s":return e(__%x$);`, specifier, i)
					}
				}
				fmt.Fprintf(header, `default:throw new Error("module \""+n+"\" not found");}};%s`, EOL)
			}

			// check imports
			for _, a := range ctx.imports {
				fullpath, path := a[0], a[1]
				if bytes.Contains(jsContent, []byte(fmt.Sprintf(`"%s"`, path))) {
					imports.Add(fullpath)
				}
			}

			// to fix the source map
			ctx.smOffset += strings.Count(header.String(), EOL)

			jsContent, dropSourceMap := ctx.rewriteJS(jsContent)
			finalContent := bytes.NewBuffer(header.Bytes())
			finalContent.Write(jsContent)

			if ctx.pkgDeprecated != "" {
				fmt.Fprintf(finalContent, `console.warn("%%c[esm.sh]%%c %%cdeprecated%%c %s@%s: %s", "color:grey", "", "color:red", "");%s`, ctx.pkg.Name, ctx.pkg.Version, strings.ReplaceAll(ctx.pkgDeprecated, "\"", "\\\""), "\n")
			}

			// add sourcemap Url
			if ctx.sourceMap && !dropSourceMap {
				finalContent.WriteString("//# sourceMappingURL=")
				finalContent.WriteString(path.Base(ctx.Path()))
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

	// sort the imports
	deps := sort.StringSlice{}
	for _, url := range imports.Values() {
		if strings.HasPrefix(url, "/") {
			deps = append(deps, url)
		}
	}
	deps.Sort()

	result.Deps = deps
	result.Dts, err = ctx.resloveDTS(entry)
	return
}

func (ctx *BuildContext) buildTypes() (ret BuildResult, err error) {
	// install the package
	ctx.stage = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	var dts string
	if endsWith(ctx.pkg.SubPath, ".d.ts", "d.mts") {
		dts = "./" + ctx.pkg.SubPath
	} else {
		entry := ctx.resolveEntry(ctx.pkg)
		if entry.dts == "" {
			err = errors.New("types not found")
			return
		}
		dts = entry.dts
	}

	ctx.stage = "build"
	err = ctx.transformDTS(dts)
	if err == nil {
		ret.Dts = "/" + ctx.pkg.ghPrefix() + ctx.pkg.Fullname() + dts[1:]
	}
	return
}

func (ctx *BuildContext) transformDTS(types string) (err error) {
	start := time.Now()
	buildArgsPrefix := ctx.getBuildArgsPrefix(ctx.pkg, true)
	n, err := transformDTS(ctx, types, buildArgsPrefix, nil)
	if err != nil {
		return
	}
	log.Debugf("transform dts '%s'(%d related dts files) in %v", types, n, time.Since(start))
	return
}
