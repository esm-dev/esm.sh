package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type BundleMode uint8

const (
	BundleDefault BundleMode = iota
	BundleAll
	BundleFalse
)

type BuildContext struct {
	esm         Esm
	npmrc       *NpmRC
	args        BuildArgs
	bundleMode  BundleMode
	externalAll bool
	target      string
	pinedTarget bool
	dev         bool
	zoneId      string
	wd          string
	pkgJson     *PackageJSON
	path        string
	status      string
	splitting   *Set
	esmImports  [][2]string
	cjsRequires [][3]string
	smOffset    int
}

type BuildMeta struct {
	CJS           bool     `json:"cjs,omitempty"`
	HasCSS        bool     `json:"hasCSS,omitempty"`
	TypesOnly     bool     `json:"typesOnly,omitempty"`
	ExportDefault bool     `json:"exportDefault,omitempty"`
	Imports       []string `json:"imports,omitempty"`
	Dts           string   `json:"dts,omitempty"`
}

var loaders = map[string]esbuild.Loader{
	".js":     esbuild.LoaderJS,
	".mjs":    esbuild.LoaderJS,
	".cjs":    esbuild.LoaderJS,
	".jsx":    esbuild.LoaderJSX,
	".ts":     esbuild.LoaderTS,
	".mts":    esbuild.LoaderTS,
	".cts":    esbuild.LoaderTS,
	".tsx":    esbuild.LoaderTSX,
	".vue":    esbuild.LoaderJS,
	".svelte": esbuild.LoaderJS,
	".css":    esbuild.LoaderCSS,
	".json":   esbuild.LoaderJSON,
	".txt":    esbuild.LoaderText,
	".html":   esbuild.LoaderText,
	".md":     esbuild.LoaderText,
	".svg":    esbuild.LoaderDataURL,
	".png":    esbuild.LoaderDataURL,
	".webp":   esbuild.LoaderDataURL,
	".gif":    esbuild.LoaderDataURL,
	".ttf":    esbuild.LoaderDataURL,
	".eot":    esbuild.LoaderDataURL,
	".woff":   esbuild.LoaderDataURL,
	".woff2":  esbuild.LoaderDataURL,
}

func (ctx *BuildContext) Exists() (*BuildMeta, bool, error) {
	key := ctx.getSavepath() + ".meta"
	meta, err := withLRUCache(key, func() (*BuildMeta, error) {
		r, _, err := buildStorage.Get(key)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		var meta BuildMeta
		if json.NewDecoder(r).Decode(&meta) == nil {
			return &meta, nil
		}

		// delete the invalid meta file
		buildStorage.Delete(key)
		return nil, storage.ErrNotFound
	})
	if err != nil {
		if err == storage.ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return meta, true, nil
}

func (ctx *BuildContext) Build() (ret *BuildMeta, err error) {
	if ctx.target == "types" {
		return ctx.buildTypes()
	}

	// check previous build
	ret, ok, err := ctx.Exists()
	if err != nil || ok {
		return
	}

	// install the package
	ctx.status = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	// check previous build again after installation (in case the sub-module path has been changed by the `install` function)
	ret, ok, err = ctx.Exists()
	if err != nil || ok {
		return
	}

	// if ctx.pkgJson.Exports.Len() > 0 && ctx.bundleMode == BundleDefault {
	// 	var exportNames []string
	// 	var exportAll bool
	// 	for _, exportName := range ctx.pkgJson.Exports.keys {
	// 		exportName := stripModuleExt(exportName)
	// 		if (exportName == "." || strings.HasPrefix(exportName, "./")) && !endsWith(exportName, ".json", ".css") {
	// 			if exportName == "./*" {
	// 				exportAll = true
	// 				break
	// 			}
	// 			if !strings.ContainsRune(exportName, '*') {
	// 				v := ctx.pkgJson.Exports.values[exportName]
	// 				if obj, ok := v.(*JSONObject); ok {
	// 					// ignore types exports
	// 					if len(obj.keys) == 1 && obj.keys[0] == "types" {
	// 						continue
	// 					}
	// 				}
	// 				if exportName == "." {
	// 					exportNames = append(exportNames, "")
	// 				} else if strings.HasPrefix(exportName, "./") {
	// 					exportNames = append(exportNames, exportName[2:])
	// 				}
	// 			}
	// 		}
	// 	}
	// 	if !exportAll {
	// 		refs := map[string]uint16{}
	// 		for _, exportName := range exportNames {
	// 			esm := ctx.esm
	// 			esm.SubPath = exportName
	// 			esm.SubModuleName = stripEntryModuleExt(exportName)
	// 			b := &BuildContext{
	// 				esm:         esm,
	// 				npmrc:       ctx.npmrc,
	// 				args:        ctx.args,
	// 				externalAll: ctx.externalAll,
	// 				target:      ctx.target,
	// 				pinedTarget: ctx.pinedTarget,
	// 				dev:         ctx.dev,
	// 				zoneId:      ctx.zoneId,
	// 				wd:          ctx.wd,
	// 				pkgJson:     ctx.pkgJson,
	// 			}
	// 			_, depTree, err := b.buildModule(true)
	// 			if err == nil {
	// 				for _, dep := range depTree.Values() {
	// 					refs[dep]++
	// 				}
	// 			}
	// 		}
	// 		for p, n := range refs {
	// 			if n > 1 {
	// 				if ctx.splitting == nil {
	// 					ctx.splitting = NewSet()
	// 				}
	// 				ctx.splitting.Add(p)
	// 			}
	// 		}
	// 	}
	// }

	// build the module
	ctx.status = "build"
	ret, _, err = ctx.buildModule(false)
	if err != nil {
		return
	}

	// save the build result into db
	key := ctx.getSavepath() + ".meta"
	buf, recycle := NewBuffer()
	defer recycle()
	err = json.NewEncoder(buf).Encode(ret)
	if err != nil {
		return
	}
	if e := buildStorage.Put(key, buf); e != nil {
		log.Errorf("db: %v", e)
	}
	return
}

func (ctx *BuildContext) buildModule(analyzeMode bool) (meta *BuildMeta, depTree *Set, err error) {
	if analyzeMode {
		depTree = NewSet()
	}

	// json module
	if strings.HasSuffix(ctx.esm.SubModuleName, ".json") {
		if analyzeMode {
			return
		}
		nmDir := path.Join(ctx.wd, "node_modules")
		jsonPath := path.Join(nmDir, ctx.esm.PkgName, ctx.esm.SubModuleName)
		if existsFile(jsonPath) {
			var jsonData []byte
			jsonData, err = os.ReadFile(jsonPath)
			if err != nil {
				return
			}
			buffer, recycle := NewBuffer()
			defer recycle()
			buffer.WriteString("export default ")
			buffer.Write(jsonData)
			err = buildStorage.Put(ctx.getSavepath(), buffer)
			if err != nil {
				return
			}
			meta = &BuildMeta{
				ExportDefault: true,
			}
			return
		}
	}

	entry := ctx.resolveEntry(ctx.esm)
	if entry.isEmpty() {
		err = fmt.Errorf("could not resolve build entry")
		return
	}
	if debug && !analyzeMode {
		log.Debugf(`build(%s): Entry{main: "%s", module: %v, types: "%s"}`, ctx.esm.Specifier(), entry.main, entry.module, entry.types)
	}

	isTypesOnly := strings.HasPrefix(ctx.pkgJson.Name, "@types/") || entry.isTypesOnly()
	if isTypesOnly {
		if analyzeMode {
			return
		}
		err = ctx.transformDTS(entry.types)
		if err != nil {
			return
		}
		meta = &BuildMeta{
			TypesOnly: true,
			Dts:       "/" + ctx.esm.Name() + entry.types[1:],
		}
		return
	}

	var (
		cjsReexport string
		cjsExports  []string
	)

	if !analyzeMode {
		meta, cjsExports, cjsReexport, err = ctx.lexer(&entry)
		if err != nil {
			return
		}
	}

	// cjs reexport
	if cjsReexport != "" {
		dep, _, e := ctx.lookupDep(cjsReexport, false)
		if e != nil {
			err = e
			return
		}
		b := &BuildContext{
			esm:         dep,
			npmrc:       ctx.npmrc,
			args:        ctx.args,
			externalAll: ctx.externalAll,
			target:      ctx.target,
			pinedTarget: ctx.pinedTarget,
			dev:         ctx.dev,
			zoneId:      ctx.zoneId,
		}
		err = b.install()
		if err != nil {
			return
		}
		entry = b.resolveEntry(dep)
		meta, _, _, err = b.lexer(&entry)
		if err != nil {
			return
		}
		importUrl := ctx.getImportPath(dep, ctx.getBuildArgsPrefix(false), ctx.externalAll)
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `export * from "%s";`, importUrl)
		if meta.ExportDefault {
			fmt.Fprintf(buf, `export { default } from "%s";`, importUrl)
		}
		err = buildStorage.Put(ctx.getSavepath(), buf)
		if err != nil {
			return
		}
		meta.Dts, err = ctx.resloveDTS(entry)
		return
	}

	entryModuleFilename := path.Join(ctx.wd, "node_modules", ctx.esm.PkgName, entry.main)
	entrySpecifier := ctx.esm.PkgName
	if ctx.esm.SubModuleName != "" {
		entrySpecifier += "/" + ctx.esm.SubModuleName
	}

	var (
		entryPoint string
		stdin      esbuild.StdinOptions
	)

	if entry.module {
		entryPoint = entryModuleFilename
	} else {
		buf, recycle := NewBuffer()
		defer recycle()
		fmt.Fprintf(buf, `import * as cjsm from "%s";`, entrySpecifier)
		if len(cjsExports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = cjsm;`, strings.Join(cjsExports, ","))
		}
		buf.WriteString("export default cjsm.default ?? cjsm;")
		stdin = esbuild.StdinOptions{
			Sourcefile: "endpoint.js",
			Contents:   buf.String(),
		}
	}

	browserExclude := map[string]*Set{}
	implicitExternal := NewSet()
	pkgSideEffects := esbuild.SideEffectsTrue
	if ctx.pkgJson.SideEffectsFalse {
		pkgSideEffects = esbuild.SideEffectsFalse
	}
	noBundle := ctx.bundleMode == BundleFalse || (ctx.pkgJson.SideEffects != nil && ctx.pkgJson.SideEffects.Len() > 0)
	if ctx.pkgJson.Esmsh != nil {
		if v, ok := ctx.pkgJson.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				noBundle = true
			}
		}
	}
	corePlugin := esbuild.Plugin{
		Name: "core",
		Setup: func(build esbuild.PluginBuild) {
			// resovler
			build.OnResolve(
				esbuild.OnResolveOptions{Filter: ".*"},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					// if it's the entry module
					if args.Path == entryPoint || args.Path == entrySpecifier {
						path := args.Path
						if path == entrySpecifier {
							path = entryModuleFilename
						}
						if strings.HasSuffix(path, ".svelte") {
							return esbuild.OnResolveResult{Path: path, Namespace: "svelte"}, nil
						}
						if strings.HasSuffix(path, ".vue") {
							return esbuild.OnResolveResult{Path: path, Namespace: "vue"}, nil
						}
						return esbuild.OnResolveResult{Path: path}, nil
					}

					// ban file urls
					if strings.HasPrefix(args.Path, "file:") {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(args.Path, "file:"), ctx.esm.Specifier()),
							External: true,
						}, nil
					}

					// skip dataurl/http modules
					if strings.HasPrefix(args.Path, "data:") || strings.HasPrefix(args.Path, "https:") || strings.HasPrefix(args.Path, "http:") {
						return esbuild.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// if `?external-require` present, ignore specifier that is a require call
					if ctx.args.externalRequire && args.Kind == esbuild.ResolveJSRequireCall && entry.module {
						return esbuild.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// ignore yarn PnP API
					if args.Path == "pnpapi" {
						return esbuild.OnResolveResult{
							Path:      args.Path,
							Namespace: "browser-exclude",
						}, nil
					}

					// it's implicit external
					if implicitExternal.Has(args.Path) {
						externalPath, err := ctx.resolveExternalModule(args.Path, args.Kind, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:     externalPath,
							External: true,
						}, nil
					}

					// normalize specifier
					specifier := normalizeImportSpecifier(args.Path)

					// resolve specifier by checking `?alias` query
					if len(ctx.args.alias) > 0 && !isRelPathSpecifier(specifier) {
						pkgName, _, subpath, _ := splitEsmPath(specifier)
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
					if !isRelPathSpecifier(specifier) && len(ctx.pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
						if name, ok := ctx.pkgJson.Browser[specifier]; ok {
							if name == "" {
								return esbuild.OnResolveResult{
									Path:      args.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							specifier = name
						}
					}

					// force to use `npm:` specifier for `denonext` target
					if forceNpmSpecifiers[specifier] && ctx.target == "denonext" {
						version := ""
						pkgName, _, subPath, _ := splitEsmPath(specifier)
						if pkgName == ctx.esm.PkgName {
							version = ctx.esm.PkgVersion
						} else if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok && regexpVersionStrict.MatchString(v) {
							version = v
						} else if v, ok := ctx.pkgJson.PeerDependencies[pkgName]; ok && regexpVersionStrict.MatchString(v) {
							version = v
						}
						p := pkgName
						if version != "" {
							p += "@" + version
						}
						if subPath != "" {
							p += "/" + subPath
						}
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("npm:%s", p),
							External: true,
						}, nil
					}

					var fullFilepath string
					if strings.HasPrefix(specifier, "/") {
						fullFilepath = specifier
					} else if isRelPathSpecifier(specifier) {
						fullFilepath = path.Join(args.ResolveDir, specifier)
					} else {
						fullFilepath = path.Join(ctx.wd, "node_modules", specifier)
					}

					// it's nodejs builtin module
					if isNodeBuiltInModule(specifier) {
						externalPath, err := ctx.resolveExternalModule(specifier, args.Kind, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:     externalPath,
							External: true,
						}, nil
					}

					// node native modules do not work via http import
					if strings.HasSuffix(fullFilepath, ".node") && existsFile(fullFilepath) {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), ctx.esm.Specifier()),
							External: true,
						}, nil
					}

					// externalize parent module
					// e.g. "react/jsx-runtime" imports "react"
					if ctx.esm.SubModuleName != "" && specifier == ctx.esm.PkgName && ctx.bundleMode != BundleAll {
						externalPath, err := ctx.resolveExternalModule(ctx.esm.PkgName, args.Kind, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							External:    true,
							SideEffects: pkgSideEffects,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query]
					if ctx.bundleMode == BundleAll && !ctx.args.external.Has(toPackageName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := toPackageName(specifier)
						_, ok := ctx.pkgJson.PeerDependencies[pkgName]
						if !ok {
							return esbuild.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) && ctx.pkgJson.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/")) {
						return esbuild.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelPathSpecifier(specifier) {
						specifier = strings.TrimPrefix(fullFilepath, path.Join(ctx.wd, "node_modules")+"/")
						pkgName := ctx.pkgJson.Name
						isPkgModule := strings.HasPrefix(specifier, pkgName+"/")
						if !isPkgModule && ctx.pkgJson.PkgName != "" {
							// github packages may have different package name with the repository name
							pkgName = ctx.pkgJson.PkgName
							isPkgModule = strings.HasPrefix(specifier, pkgName+"/")
						}
						if isPkgModule {
							// if meets scenarios of "./index.mjs" importing "./index.c?js"
							// let esbuild to handle it
							if stripModuleExt(fullFilepath) == stripModuleExt(args.Importer) {
								return esbuild.OnResolveResult{}, nil
							}

							moduleSpecifier := "." + strings.TrimPrefix(specifier, pkgName)

							if path.Ext(fullFilepath) == "" || !existsFile(fullFilepath) {
								subPath := utils.NormalizePathname(moduleSpecifier)[1:]
								entry := ctx.resolveEntry(Esm{
									PkgName:       ctx.esm.PkgName,
									PkgVersion:    ctx.esm.PkgVersion,
									SubModuleName: stripEntryModuleExt(subPath),
									SubPath:       subPath,
								})
								if entry.main != "" {
									moduleSpecifier = entry.main
								}
							}

							// resolve specifier with package `browser` field
							if len(ctx.pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
								if path, ok := ctx.pkgJson.Browser[moduleSpecifier]; ok {
									if path == "" {
										return esbuild.OnResolveResult{
											Path:      args.Path,
											Namespace: "browser-exclude",
										}, nil
									}
									if !isRelPathSpecifier(path) {
										externalPath, err := ctx.resolveExternalModule(path, args.Kind, analyzeMode)
										if err != nil {
											return esbuild.OnResolveResult{}, err
										}
										return esbuild.OnResolveResult{
											Path:     externalPath,
											External: true,
										}, nil
									}
									moduleSpecifier = path
								}
							}

							var exportSpecifier string

							// split modules based on the `exports` field of package.json
							if exports := ctx.pkgJson.Exports; exports.Len() > 0 {
								for _, exportName := range exports.keys {
									v := exports.values[exportName]
									if exportName != "." && !strings.HasPrefix(exportName, "./") {
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
											match = strings.HasPrefix(stripModuleExt(moduleSpecifier), prefix) && (suffix == "" || strings.HasSuffix(moduleSpecifier, suffix))
										} else if m, ok := v.(*JSONObject); ok {
											// exports: "./*": { "import": "./dist/*.js" }
											// exports: "./*": { "import": { default: "./dist/*.js" } }
											// ...
											paths := getAllExportsPaths(m)
											for _, path := range paths {
												prefix, suffix = utils.SplitByLastByte(path, '*')
												match = strings.HasPrefix(stripModuleExt(moduleSpecifier), prefix) && (suffix == "" || strings.HasSuffix(moduleSpecifier, suffix))
												if match {
													break
												}
											}
										}
										if match {
											exportPrefix, _ := utils.SplitByLastByte(exportName, '*')
											exportSpecifier = path.Join(ctx.pkgJson.Name, exportPrefix+strings.TrimPrefix(stripModuleExt(moduleSpecifier), prefix))
											if exportSpecifier != entrySpecifier && exportSpecifier != entrySpecifier+"/index" {
												externalPath, err := ctx.resolveExternalModule(exportSpecifier, args.Kind, analyzeMode)
												if err != nil {
													return esbuild.OnResolveResult{}, err
												}
												return esbuild.OnResolveResult{
													Path:        externalPath,
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									} else {
										match := false
										if s, ok := v.(string); ok && stripModuleExt(s) == stripModuleExt(moduleSpecifier) {
											// exports: "./foo": "./foo.js"
											match = true
										} else if m, ok := v.(*JSONObject); ok {
											// exports: "./foo": { "import": "./foo.js" }
											// exports: "./foo": { "import": { default: "./foo.js" } }
											// ...
											paths := getAllExportsPaths(m)
											for _, path := range paths {
												if stripModuleExt(path) == stripModuleExt(moduleSpecifier) {
													match = true
													break
												}
											}
										}
										if match {
											exportSpecifier = path.Join(ctx.pkgJson.Name, stripModuleExt(exportName))
											if exportSpecifier != entrySpecifier && exportSpecifier != entrySpecifier+"/index" {
												externalPath, err := ctx.resolveExternalModule(exportSpecifier, args.Kind, analyzeMode)
												if err != nil {
													return esbuild.OnResolveResult{}, err
												}
												return esbuild.OnResolveResult{
													Path:        externalPath,
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// module file path
							moduleFilename := path.Join(ctx.wd, "node_modules", ctx.esm.PkgName, moduleSpecifier)

							// split the module that is an alias of a dependency
							// means this file just include a single line(js): `export * from "dep"`
							fi, ioErr := os.Lstat(moduleFilename)
							if ioErr == nil && fi.Size() < 128 {
								data, ioErr := os.ReadFile(moduleFilename)
								if ioErr == nil {
									out, esbErr := minify(string(data), esbuild.LoaderJS, esbuild.ESNext)
									if esbErr == nil {
										p := bytes.Split(out, []byte("\""))
										if len(p) == 3 && string(p[0]) == "export*from" && string(p[2]) == ";\n" {
											url := string(p[1])
											if !isRelPathSpecifier(url) {
												externalPath, err := ctx.resolveExternalModule(url, args.Kind, analyzeMode)
												if err != nil {
													return esbuild.OnResolveResult{}, err
												}
												return esbuild.OnResolveResult{
													Path:        externalPath,
													External:    true,
													SideEffects: pkgSideEffects,
												}, nil
											}
										}
									}
								}
							}

							// bundle the package module if
							// - it's the entry module
							// - it's not a dynamic import and `?bundle=false` query is not present
							if moduleSpecifier == entry.main || (exportSpecifier != "" && exportSpecifier == entrySpecifier) || (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) {
								if existsFile(moduleFilename) {
									if analyzeMode && moduleFilename != entryModuleFilename {
										depTree.Add(moduleFilename)
									}
									if !analyzeMode && ctx.splitting != nil && ctx.splitting.Has(moduleFilename) && !ctx.splitting.Has(entryModuleFilename) {
										fmt.Println("splitting", moduleFilename)
									}
									// embed wasm as WebAssembly.Module
									if strings.HasSuffix(moduleFilename, ".wasm") {
										return esbuild.OnResolveResult{
											Path:      moduleFilename,
											Namespace: "wasm",
										}, nil
									}
									// transfrom svelte component
									if strings.HasSuffix(moduleFilename, ".svelte") {
										return esbuild.OnResolveResult{
											Path:      moduleFilename,
											Namespace: "svelte",
										}, nil
									}
									// transfrom Vue SFC
									if strings.HasSuffix(moduleFilename, ".vue") {
										return esbuild.OnResolveResult{
											Path:      moduleFilename,
											Namespace: "vue",
										}, nil
									}
									return esbuild.OnResolveResult{Path: moduleFilename}, nil
								}
								// otherwise, let esbuild to handle it
								return esbuild.OnResolveResult{}, nil
							}
						}
					}

					// replace some npm modules with browser native APIs
					if specifier != "fsevents" || ctx.isBrowserTarget() {
						replacement, ok := npmReplacements[specifier+"_"+ctx.target]
						if !ok {
							replacement, ok = npmReplacements[specifier]
						}
						if ok {
							if args.Kind == esbuild.ResolveJSRequireCall || args.Kind == esbuild.ResolveJSRequireResolve {
								ctx.cjsRequires = append(ctx.cjsRequires, [3]string{
									"npm:" + specifier,
									string(replacement.iife),
									"",
								})
								return esbuild.OnResolveResult{
									Path:     "npm:" + specifier,
									External: true,
								}, nil
							}
							return esbuild.OnResolveResult{
								Path:       specifier,
								PluginData: replacement.esm,
								Namespace:  "npm-replacement",
							}, nil
						}
					}

					// dynamic external
					sideEffects := esbuild.SideEffectsFalse
					if specifier == ctx.pkgJson.Name || specifier == ctx.pkgJson.PkgName || strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") || strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") {
						sideEffects = pkgSideEffects
					}
					externalPath, err := ctx.resolveExternalModule(specifier, args.Kind, analyzeMode)
					if err != nil {
						return esbuild.OnResolveResult{}, err
					}
					return esbuild.OnResolveResult{
						Path:        externalPath,
						External:    true,
						SideEffects: sideEffects,
					}, nil
				},
			)

			// npm replacement loader
			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "npm-replacement"},
				func(args esbuild.OnLoadArgs) (ret esbuild.OnLoadResult, err error) {
					contents := string(args.PluginData.([]byte))
					return esbuild.OnLoadResult{Contents: &contents, Loader: esbuild.LoaderJS}, nil
				},
			)

			// browser exclude loader
			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "browser-exclude"},
				func(args esbuild.OnLoadArgs) (ret esbuild.OnLoadResult, err error) {
					contents := "export default {};"
					if exports, ok := browserExclude[args.Path]; ok {
						for _, name := range exports.Values() {
							contents = fmt.Sprintf("%sexport const %s = {};", contents, name)
						}
					}
					return esbuild.OnLoadResult{Contents: &contents, Loader: esbuild.LoaderJS}, nil
				},
			)

			// wasm module exclude loader
			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "wasm"},
				func(args esbuild.OnLoadArgs) (ret esbuild.OnLoadResult, err error) {
					wasm, err := os.ReadFile(args.Path)
					if err != nil {
						return
					}
					wasm64 := base64.StdEncoding.EncodeToString(wasm)
					code := fmt.Sprintf("export default Uint8Array.from(atob('%s'), c => c.charCodeAt(0))", wasm64)
					return esbuild.OnLoadResult{Contents: &code, Loader: esbuild.LoaderJS}, nil
				},
			)

			// svelte SFC loader
			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "svelte"},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					code, err := os.ReadFile(args.Path)
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					svelteVersion := "5"
					if version, ok := ctx.args.deps["svelte"]; ok {
						svelteVersion = version
					} else if version, ok := ctx.pkgJson.Dependencies["svelte"]; ok {
						svelteVersion = version
					} else if version, ok := ctx.pkgJson.PeerDependencies["svelte"]; ok {
						svelteVersion = version
					}
					if !regexpVersionStrict.MatchString(svelteVersion) {
						info, err := ctx.npmrc.getPackageInfo("svelte", svelteVersion)
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to get svelte package info")
						}
						svelteVersion = info.Version
					}
					if semverLessThan(svelteVersion, "4.0.0") {
						return esbuild.OnLoadResult{}, errors.New("svelte version must be greater than 4.0.0")
					}
					out, err := transformSvelte(ctx.npmrc, svelteVersion, ctx.esm.Specifier(), string(code))
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					return esbuild.OnLoadResult{Contents: &out.Code, Loader: esbuild.LoaderJS}, nil
				},
			)

			// vue SFC loader
			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "vue"},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					code, err := os.ReadFile(args.Path)
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					vueVersion := "3"
					if version, ok := ctx.args.deps["vue"]; ok {
						vueVersion = version
					} else if version, ok := ctx.pkgJson.Dependencies["vue"]; ok {
						vueVersion = version
					} else if version, ok := ctx.pkgJson.PeerDependencies["vue"]; ok {
						vueVersion = version
					}
					if !regexpVersionStrict.MatchString(vueVersion) {
						info, err := ctx.npmrc.getPackageInfo("vue", vueVersion)
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to get vue package info")
						}
						vueVersion = info.Version
					}
					if semverLessThan(vueVersion, "3.0.0") {
						return esbuild.OnLoadResult{}, errors.New("vue version must be greater than 3.0.0")
					}
					out, err := transformVue(ctx.npmrc, vueVersion, ctx.esm.Specifier(), string(code))
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					if out.Lang == "ts" {
						return esbuild.OnLoadResult{Contents: &out.Code, Loader: esbuild.LoaderTS}, nil
					}
					return esbuild.OnLoadResult{Contents: &out.Code, Loader: esbuild.LoaderJS}, nil
				},
			)
		},
	}

	filename := ctx.Path()
	dirname, _ := utils.SplitByLastByte(filename, '/')
	nodeEnv := ctx.getNodeEnv()
	define := map[string]string{
		"__filename":           fmt.Sprintf(`"%s"`, filename),
		"__dirname":            fmt.Sprintf(`"%s"`, dirname),
		"Buffer":               "__Buffer$",
		"process":              "__Process$",
		"setImmediate":         "__setImmediate$",
		"clearImmediate":       "clearTimeout",
		"require.resolve":      "__rResolve$",
		"process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
	}
	if ctx.target == "node" {
		define = map[string]string{
			"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, nodeEnv),
			"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
		}
	} else {
		if ctx.isBrowserTarget() {
			switch ctx.esm.PkgName {
			case "react", "react-dom", "typescript":
				// safe to reserve `process` for these packages
				delete(define, "process")
			}
		}
		for k, v := range define {
			define["global."+k] = v
		}
		define["global"] = "__global$"
	}
	conditions := ctx.args.conditions
	if ctx.dev {
		conditions = append(conditions, "development")
	}
	if ctx.isDenoTarget() {
		conditions = append(conditions, "deno")
	} else if ctx.target == "node" {
		conditions = append(conditions, "node")
	}
	options := esbuild.BuildOptions{
		AbsWorkingDir:     ctx.wd,
		PreserveSymlinks:  true,
		Format:            esbuild.FormatESModule,
		Target:            targets[ctx.target],
		Platform:          esbuild.PlatformBrowser,
		Define:            define,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   "react",
		Bundle:            true,
		MinifyWhitespace:  config.Minify,
		MinifyIdentifiers: config.Minify,
		MinifySyntax:      config.Minify,
		KeepNames:         ctx.args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: ctx.args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        conditions,
		Loader:            loaders,
		Plugins:           []esbuild.Plugin{corePlugin},
		Outdir:            "/esbuild",
		Write:             false,
	}
	if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
	} else {
		options.Stdin = &stdin
	}
	if ctx.target == "node" {
		options.Platform = esbuild.PlatformNode
	}
	// support features that can not be polyfilled
	options.Supported = map[string]bool{
		"bigint":          true,
		"top-level-await": true,
	}
	if config.SourceMap {
		options.Sourcemap = esbuild.SourceMapExternal
	}
	for _, pkgName := range []string{"preact", "react", "solid-js", "mono-jsx", "vue", "hono"} {
		_, ok1 := ctx.pkgJson.Dependencies[pkgName]
		_, ok2 := ctx.pkgJson.PeerDependencies[pkgName]
		if ok1 || ok2 {
			options.JSXImportSource = pkgName
			if pkgName == "hono" {
				options.JSXImportSource += "/jsx"
			}
			break
		}
	}

rebuild:
	ret := esbuild.Build(options)
	if len(ret.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := ret.Errors[0].Text
		if strings.HasPrefix(msg, "Could not resolve \"") {
			// current module can not be marked as an external
			if strings.HasPrefix(msg, fmt.Sprintf("Could not resolve \"%s\"", entrySpecifier)) {
				err = fmt.Errorf("could not resolve \"%s\"", entrySpecifier)
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
						exports = NewSet()
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

	if analyzeMode {
		return
	}

	for _, w := range ret.Warnings {
		log.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
	}

	imports := NewSet()

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			jsContent := file.Contents
			header, recycle := NewBuffer()
			defer recycle()
			header.WriteString("/* esm.sh - ")
			if ctx.esm.GhPrefix {
				header.WriteString("github:")
			} else if ctx.esm.PrPrefix {
				header.WriteString("pkg.pr.new/")
			}
			header.WriteString(ctx.esm.PkgName)
			if ctx.esm.GhPrefix {
				header.WriteByte('#')
			} else {
				header.WriteByte('@')
			}
			header.WriteString(ctx.esm.PkgVersion)
			if ctx.esm.SubModuleName != "" {
				header.WriteByte('/')
				header.WriteString(ctx.esm.SubModuleName)
			}
			header.WriteString(" */\n")

			// remove shebang
			if bytes.HasPrefix(jsContent, []byte("#!/")) {
				jsContent = jsContent[bytes.IndexByte(jsContent, '\n')+1:]
				ctx.smOffset--
			}

			// add nodejs compatibility
			if ctx.target != "node" {
				ids := NewSet()
				for _, r := range regexpESMInternalIdent.FindAll(jsContent, -1) {
					ids.Add(string(r))
				}
				if ids.Has("__Process$") {
					if ctx.args.external.Has("node:process") {
						header.WriteString(`import __Process$ from "node:process";`)
						header.WriteByte('\n')
					} else if ctx.isBrowserTarget() {
						if len(ctx.pkgJson.Browser) > 0 {
							var excluded bool
							if name, ok := ctx.pkgJson.Browser["process"]; ok {
								excluded = name == ""
							} else if name, ok := ctx.pkgJson.Browser["node:process"]; ok {
								excluded = name == ""
							}
							if !excluded {
								header.WriteString(`import __Process$ from "/node/process.mjs";`)
								header.WriteByte('\n')
								imports.Add("/node/process.mjs")
							}
						} else {
							header.WriteString(`import __Process$ from "/node/process.mjs";`)
							header.WriteByte('\n')
							imports.Add("/node/process.mjs")
						}
					} else if ctx.target == "denonext" {
						header.WriteString(`import __Process$ from "node:process";`)
						header.WriteByte('\n')
					} else if ctx.target == "deno" {
						header.WriteString(`import __Process$ from "https://deno.land/std@0.177.1/node/process.ts";`)
						header.WriteByte('\n')
					}
				}
				if ids.Has("__Buffer$") {
					if ctx.args.external.Has("node:buffer") {
						header.WriteString(`import { Buffer as __Buffer$ } from "node:buffer";`)
						header.WriteByte('\n')
					} else if ctx.isBrowserTarget() {
						var excluded bool
						if len(ctx.pkgJson.Browser) > 0 {
							if name, ok := ctx.pkgJson.Browser["buffer"]; ok {
								excluded = name == ""
							} else if name, ok := ctx.pkgJson.Browser["node:buffer"]; ok {
								excluded = name == ""
							}
						}
						if !excluded {
							header.WriteString(`import { Buffer as __Buffer$ } from "/node/buffer.mjs";`)
							header.WriteByte('\n')
							imports.Add("/node/buffer.mjs")
						}
					} else if ctx.target == "denonext" {
						header.WriteString(`import { Buffer as __Buffer$ } from "node:buffer";`)
						header.WriteByte('\n')
					} else if ctx.target == "deno" {
						header.WriteString(`import { Buffer as __Buffer$ } from "https://deno.land/std@0.177.1/node/buffer.ts";`)
						header.WriteByte('\n')
					}
				}
				if ids.Has("__global$") {
					header.WriteString(`var __global$ = globalThis || (typeof window !== "undefined" ? window : self);`)
					header.WriteByte('\n')
				}
				if ids.Has("__setImmediate$") {
					header.WriteString(`var __setImmediate$ = (cb, ...args) => setTimeout(cb, 0, ...args);`)
					header.WriteByte('\n')
				}
				if ids.Has("__rResolve$") {
					header.WriteString(`var __rResolve$ = p => p;`)
					header.WriteByte('\n')
				}
			}

			// apply cjs requires
			if len(ctx.cjsRequires) > 0 {
				requires := make([][3]string, 0, len(ctx.cjsRequires))
				set := NewSet()
				for _, r := range ctx.cjsRequires {
					specifier := r[0]
					if !set.Has(specifier) {
						set.Add(specifier)
						requires = append(requires, r)
					}
				}
				isEsModule := make([]bool, len(requires))
				for i, r := range requires {
					specifier := r[0]
					importUrl := r[2]
					if strings.HasPrefix(specifier, "npm:") {
						// npm replacements
						fmt.Fprintf(header, `var __%x$=%s;`, i, r[1])
					} else if isJsonModuleSpecifier(specifier) {
						fmt.Fprintf(header, `import __%x$ from"%s";`, i, importUrl)
						imports.Add(r[1])
					} else {
						fmt.Fprintf(header, `import*as __%x$ from"%s";`, i, importUrl)
						imports.Add(r[1])
					}
					// if `require("module").default` found
					if bytes.Contains(jsContent, []byte(fmt.Sprintf(`("%s").default`, specifier))) {
						isEsModule[i] = true
						continue
					}
					// `var mod = require("module");...;mod()` -> cjs
					// `var mod = require("module");...;mod.default` -> es module
					if a := bytes.SplitN(jsContent, []byte(fmt.Sprintf(`("%s")`, specifier)), 2); len(a) >= 2 {
						ret := regexpVarDecl.FindSubmatch(a[0])
						if len(ret) == 2 {
							r, e := regexp.Compile(fmt.Sprintf(`[^\w$]%s(\(|\.default[^\w$=])`, string(ret[1])))
							if e == nil {
								ret := r.FindSubmatch(jsContent)
								if len(ret) == 2 {
									isEsModule[i] = string(ret[1]) != "("
									continue
								}
							}
						}
					}
					if !isRelPathSpecifier(specifier) && !isNodeBuiltInModule(specifier) && !strings.HasPrefix(specifier, "npm:") && !isJsonModuleSpecifier(specifier) {
						dep, pkgJson, err := ctx.lookupDep(specifier, false)
						if err == nil {
							if pkgJson.Type == "module" || pkgJson.Module != "" {
								isEsModule[i] = true
							} else {
								b := &BuildContext{
									esm:         dep,
									npmrc:       ctx.npmrc,
									args:        ctx.args,
									externalAll: ctx.externalAll,
									target:      ctx.target,
									pinedTarget: ctx.pinedTarget,
									dev:         ctx.dev,
									zoneId:      ctx.zoneId,
								}
								err = b.install()
								if err == nil {
									entry := b.resolveEntry(dep)
									if !entry.module {
										ret, cjsNamedExports, _, e := b.lexer(&entry)
										if e == nil && ret.CJS && contains(cjsNamedExports, "__esModule") {
											isEsModule[i] = true
										}
									}
								}
							}
						}
					}
				}
				header.WriteString(`var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({__esModule:true},m);switch(n){`)
				for i, r := range requires {
					specifier := r[0]
					if isEsModule[i] {
						fmt.Fprintf(header, `case"%s":return c(__%x$);`, specifier, i)
					} else {
						fmt.Fprintf(header, `case"%s":return e(__%x$);`, specifier, i)
					}
				}
				header.WriteString(`default:console.error('module "'+n+'" not found');return null;}};`)
				header.WriteByte('\n')
			}

			// apply esm imports
			for _, a := range ctx.esmImports {
				resolvedPathFull, resolvedPath := a[0], a[1]
				if bytes.Contains(jsContent, []byte(fmt.Sprintf(`"%s"`, resolvedPath))) {
					imports.Add(resolvedPathFull)
				}
			}

			// to fix the source map
			ctx.smOffset += strings.Count(header.String(), "\n")

			// apply rewrites
			jsContent, dropSourceMap := ctx.rewriteJS(jsContent)

			finalJS, recycle := NewBuffer()
			defer recycle()

			io.Copy(finalJS, header)
			finalJS.Write(jsContent)

			// check if the package is deprecated
			if !ctx.esm.GhPrefix && !ctx.esm.PrPrefix {
				deprecated, _ := ctx.npmrc.isDeprecated(ctx.pkgJson.Name, ctx.pkgJson.Version)
				if deprecated != "" {
					fmt.Fprintf(finalJS, `console.warn("%%c[esm.sh]%%c %%cdeprecated%%c %s@%s: " + %s, "color:grey", "", "color:red", "");%s`, ctx.esm.PkgName, ctx.esm.PkgVersion, utils.MustEncodeJSON(deprecated), "\n")
				}
			}

			// add sourcemap Url
			if config.SourceMap && !dropSourceMap {
				finalJS.WriteString("//# sourceMappingURL=")
				finalJS.WriteString(path.Base(ctx.Path()))
				finalJS.WriteString(".map")
			}

			err = buildStorage.Put(ctx.getSavepath(), finalJS)
			if err != nil {
				return
			}
		}
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".css") {
			savePath := ctx.getSavepath()
			err = buildStorage.Put(strings.TrimSuffix(savePath, path.Ext(savePath))+".css", bytes.NewReader(file.Contents))
			if err != nil {
				return
			}
			meta.HasCSS = true
		} else if config.SourceMap && strings.HasSuffix(file.Path, ".js.map") {
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
				buf, recycle := NewBuffer()
				defer recycle()
				if json.NewEncoder(buf).Encode(sourceMap) == nil {
					err = buildStorage.Put(ctx.getSavepath()+".map", buf)
					if err != nil {
						return
					}
				}
			}
		}
	}

	// sort imports
	for _, path := range imports.Values() {
		if strings.HasPrefix(path, "/") {
			meta.Imports = append(meta.Imports, path)
		}
	}
	sort.Strings(meta.Imports)

	// resolve types(dts)
	meta.Dts, err = ctx.resloveDTS(entry)
	return
}

func (ctx *BuildContext) buildTypes() (ret *BuildMeta, err error) {
	// install the package
	ctx.status = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	var dts string
	if endsWith(ctx.esm.SubPath, ".d.ts", "d.mts") {
		dts = "./" + ctx.esm.SubPath
	} else {
		entry := ctx.resolveEntry(ctx.esm)
		if entry.types == "" {
			err = errors.New("types not found")
			return
		}
		dts = entry.types
	}

	ctx.status = "build"
	err = ctx.transformDTS(dts)
	if err != nil {
		return
	}

	ret = &BuildMeta{Dts: "/" + ctx.esm.Name() + dts[1:]}
	return
}

func (ctx *BuildContext) install() (err error) {
	if ctx.wd == "" || ctx.pkgJson == nil {
		var p *PackageJSON
		p, err = ctx.npmrc.installPackage(ctx.esm.Package())
		if err != nil {
			return
		}

		if ctx.esm.GhPrefix || ctx.esm.PrPrefix {
			// if the name in package.json is not the same as the repository name
			if p.Name != ctx.esm.PkgName {
				p.PkgName = p.Name
				p.Name = ctx.esm.PkgName
			}
			p.Version = ctx.esm.PkgVersion
		} else {
			p.Version = strings.TrimPrefix(p.Version, "v")
		}

		// Check if the `SubPath` is the same as the `main` or `module` field of the package.json
		if subModule := ctx.esm.SubModuleName; subModule != "" && ctx.target != "types" {
			isMainModule := false
			check := func(s string) bool {
				return isMainModule || (s != "" && subModule == utils.NormalizePathname(stripModuleExt(s))[1:])
			}
			if p.Exports.Len() > 0 {
				if v, ok := p.Exports.Get("."); ok {
					if s, ok := v.(string); ok {
						// exports: { ".": "./index.js" }
						isMainModule = check(s)
					} else if obj, ok := v.(*JSONObject); ok {
						// exports: { ".": { "require": "./cjs/index.js", "import": "./esm/index.js" } }
						// exports: { ".": { "node": { "require": "./cjs/index.js", "import": "./esm/index.js" } } }
						// ...
						paths := getAllExportsPaths(obj)
						for _, path := range paths {
							if check(path) {
								isMainModule = true
								break
							}
						}
					}
				}
			}
			if !isMainModule {
				isMainModule = (p.Module != "" && check(p.Module)) || (p.Main != "" && check(p.Main))
			}
			if isMainModule {
				ctx.esm.SubModuleName = ""
				ctx.esm.SubPath = ""
				ctx.path = ""
			}
		}

		ctx.wd = path.Join(ctx.npmrc.StoreDir(), ctx.esm.Name())
		ctx.pkgJson = p
	}

	// install dependencies in bundle mode
	if ctx.bundleMode == BundleAll {
		ctx.npmrc.installDependencies(ctx.wd, ctx.pkgJson, false, nil)
	} else if v, ok := ctx.pkgJson.Dependencies["@babel/runtime"]; ok {
		// we bundle @babel/runtime modules even not in bundle mode
		// install it if it's in the dependencies
		ctx.npmrc.installDependencies(ctx.wd, &PackageJSON{Dependencies: map[string]string{"@babel/runtime": v}}, false, nil)
	}
	return
}
