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

	"github.com/esm-dev/esm.sh/server/npm_replacements"
	"github.com/esm-dev/esm.sh/server/storage"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
)

type BundleMode uint8

const (
	BundleDefault BundleMode = iota
	BundleAll
	BundleFalse
)

type BuildContext struct {
	esm         EsmPath
	npmrc       *NpmRC
	args        BuildArgs
	bundleMode  BundleMode
	externalAll bool
	target      string
	pinedTarget bool
	dev         bool
	wd          string
	pkgJson     *PackageJSON
	path        string
	rawPath     string
	status      string
	splitting   *set.ReadOnlySet[string]
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

type Ref struct {
	entries   *set.Set[string]
	importers *set.Set[string]
}

var (
	regexpESMInternalIdent = regexp.MustCompile(`__[a-zA-Z]+\$`)
	regexpVarDecl          = regexp.MustCompile(`var ([\w$]+)\s*=\s*[\w$]+$`)
)

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

func (ctx *BuildContext) Path() string {
	if ctx.path != "" {
		return ctx.path
	}

	ctx.buildPath()
	return ctx.path
}

func (ctx *BuildContext) Exists() (meta *BuildMeta, ok bool, err error) {
	key := ctx.getSavepath() + ".meta"
	meta, err = withLRUCache(key, func() (*BuildMeta, error) {
		r, _, err := buildStorage.Get(key)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		var meta BuildMeta
		if json.NewDecoder(r).Decode(&meta) == nil {
			return &meta, nil
		}

		// delete the invalid meta file in the storage
		buildStorage.Delete(key)
		return nil, storage.ErrNotFound
	})
	if err != nil {
		if err == storage.ErrNotFound {
			err = nil
		}
		return nil, false, err
	}
	ok = true
	return
}

func (ctx *BuildContext) Build() (meta *BuildMeta, err error) {
	if ctx.target == "types" {
		return ctx.buildTypes()
	}

	// check previous build
	meta, ok, err := ctx.Exists()
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
	meta, ok, err = ctx.Exists()
	if err != nil || ok {
		return
	}

	// analyze splitting modules
	ctx.status = "analyze"
	err = ctx.analyzeSplitting()
	if err != nil {
		return
	}

	// build the module
	ctx.status = "build"
	meta, _, err = ctx.buildModule(false)
	if err != nil {
		return
	}

	// save the build result into db
	key := ctx.getSavepath() + ".meta"
	buf, recycle := NewBuffer()
	defer recycle()
	err = json.NewEncoder(buf).Encode(meta)
	if err != nil {
		return
	}
	if e := buildStorage.Put(key, buf); e != nil {
		log.Errorf("db: %v", e)
	}
	return
}

func (ctx *BuildContext) buildPath() {
	asteriskPrefix := ""
	if ctx.externalAll {
		asteriskPrefix = "*"
	}

	esm := ctx.esm
	if ctx.target == "types" {
		if strings.HasSuffix(esm.SubPath, ".d.ts") {
			ctx.path = fmt.Sprintf(
				"/%s%s/%s%s",
				asteriskPrefix,
				esm.Name(),
				ctx.getBuildArgsPrefix(true),
				esm.SubPath,
			)
		} else {
			ctx.path = "/" + esm.Specifier()
		}
		return
	}

	name := strings.TrimSuffix(path.Base(esm.PkgName), ".js")
	if esm.SubModuleName != "" {
		if esm.SubModuleName == name {
			// if the sub-module name is same as the package name
			name = "__" + esm.SubModuleName
		} else {
			name = esm.SubModuleName
		}
		// workaround for es5-ext "../#/.." path
		if esm.PkgName == "es5-ext" {
			name = strings.ReplaceAll(name, "/#/", "/%23/")
		}
	}

	if ctx.dev {
		name += ".development"
	}
	if ctx.bundleMode == BundleAll {
		name += ".bundle"
	} else if ctx.bundleMode == BundleFalse {
		name += ".nobundle"
	}
	ctx.path = fmt.Sprintf(
		"/%s%s/%s%s/%s.mjs",
		asteriskPrefix,
		esm.Name(),
		ctx.getBuildArgsPrefix(ctx.target == "types"),
		ctx.target,
		name,
	)
}

func (ctx *BuildContext) buildModule(analyzeMode bool) (meta *BuildMeta, includes [][2]string, err error) {
	entry := ctx.resolveEntry(ctx.esm)
	if entry.isEmpty() {
		err = errors.New("could not resolve build entry")
		return
	}

	if DEBUG && !analyzeMode {
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

	// json module
	if strings.HasSuffix(entry.main, ".json") {
		if analyzeMode {
			return
		}
		jsonPath := path.Join(ctx.wd, "node_modules", ctx.esm.PkgName, entry.main)
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

	browserExclude := map[string]*set.Set[string]{}
	implicitExternal := set.New[string]()
	pkgSideEffects := esbuild.SideEffectsTrue
	if ctx.pkgJson.SideEffectsFalse {
		pkgSideEffects = esbuild.SideEffectsFalse
	}
	noBundle := ctx.bundleMode == BundleFalse || ctx.pkgJson.SideEffects.Len() > 0
	if ctx.pkgJson.Esmsh != nil {
		if v, ok := ctx.pkgJson.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				noBundle = true
			}
		}
	}
	esmifyPlugin := esbuild.Plugin{
		Name: "esmify",
		Setup: func(build esbuild.PluginBuild) {
			// resovler
			build.OnResolve(
				esbuild.OnResolveOptions{Filter: ".*"},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					// entry point
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

					// ban file: imports
					if strings.HasPrefix(args.Path, "file:") {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(args.Path, "file:"), ctx.esm.Specifier()),
							External: true,
						}, nil
					}

					// skip data: and http: imports
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

					pkgJson := ctx.pkgJson
					specifier := normalizeImportSpecifier(args.Path)
					withTypeJSON := len(args.With) > 0 && args.With["type"] == "json"

					// it's implicit external
					if implicitExternal.Has(args.Path) {
						externalPath, err := ctx.resolveExternalModule(args.Path, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:     externalPath,
							External: true,
						}, nil
					}

					// check `?alias` option
					if len(ctx.args.alias) > 0 && !isRelPathSpecifier(specifier) {
						pkgName, _, subpath, _ := splitEsmPath(specifier)
						if name, ok := ctx.args.alias[pkgName]; ok {
							specifier = name
							if subpath != "" {
								specifier += "/" + subpath
							}
						}
					}

					// resolve specifier using the `imports` field of package.json
					if len(pkgJson.Imports) > 0 {
						if v, ok := pkgJson.Imports[specifier]; ok {
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

					// resolve specifier using the `browser` field of package.json
					if !isRelPathSpecifier(specifier) && len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
						if name, ok := pkgJson.Browser[specifier]; ok {
							if name == "" {
								return esbuild.OnResolveResult{
									Path:      args.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							specifier = name
						}
					}

					// nodejs builtin module
					if isNodeBuiltInModule(specifier) {
						externalPath, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:     externalPath,
							External: true,
						}, nil
					}

					var filename string
					if strings.HasPrefix(specifier, "/") {
						filename = specifier
					} else if isRelPathSpecifier(specifier) && args.ResolveDir != "" {
						filename = path.Join(args.ResolveDir, specifier)
					} else {
						filename = path.Join(ctx.wd, "node_modules", specifier)
					}

					// node native modules do not work via http import
					if strings.HasSuffix(filename, ".node") && existsFile(filename) {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), ctx.esm.Specifier()),
							External: true,
						}, nil
					}

					// externalize top-level module
					// e.g. "react/jsx-runtime" imports "react"
					if ctx.esm.SubModuleName != "" && specifier == ctx.esm.PkgName && ctx.bundleMode != BundleAll {
						externalPath, err := ctx.resolveExternalModule(ctx.esm.PkgName, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							External:    true,
							SideEffects: pkgSideEffects,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peerDependencies and `?external` flag
					if ctx.bundleMode == BundleAll && !ctx.args.external.Has(toPackageName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := toPackageName(specifier)
						_, ok := pkgJson.PeerDependencies[pkgName]
						if !ok {
							return esbuild.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) && pkgJson.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/")) {
						return esbuild.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelPathSpecifier(specifier) {
						pkgName := ctx.esm.PkgName
						specifier = strings.TrimPrefix(filename, path.Join(ctx.wd, "node_modules")+"/")
						isPkgModule := strings.HasPrefix(specifier, pkgName+"/")
						if !isPkgModule && pkgJson.PkgName != "" {
							// github packages may have different package name with the repository name
							pkgName = pkgJson.PkgName
							isPkgModule = strings.HasPrefix(specifier, pkgName+"/")
						}
						if isPkgModule {
							// if meets scenarios of "./index.mjs" importing "./index.c?js"
							// let esbuild to handle it
							if stripModuleExt(filename) == stripModuleExt(args.Importer) {
								return esbuild.OnResolveResult{}, nil
							}

							modulePath := "." + strings.TrimPrefix(specifier, pkgName)

							if path.Ext(filename) == "" || !existsFile(filename) {
								subPath := utils.NormalizePathname(modulePath)[1:]
								entry := ctx.resolveEntry(EsmPath{
									PkgName:       ctx.esm.PkgName,
									PkgVersion:    ctx.esm.PkgVersion,
									SubModuleName: stripEntryModuleExt(subPath),
									SubPath:       subPath,
								})
								if entry.main != "" {
									modulePath = entry.main
								}
							}

							// resolve specifier using the `browser` field
							if len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
								if path, ok := pkgJson.Browser[modulePath]; ok {
									if path == "" {
										return esbuild.OnResolveResult{
											Path:      args.Path,
											Namespace: "browser-exclude",
										}, nil
									}
									if !isRelPathSpecifier(path) {
										externalPath, err := ctx.resolveExternalModule(path, args.Kind, withTypeJSON, analyzeMode)
										if err != nil {
											return esbuild.OnResolveResult{}, err
										}
										return esbuild.OnResolveResult{
											Path:     externalPath,
											External: true,
										}, nil
									}
									modulePath = path
								}
							}

							var asExport string

							// split modules based on the `exports` field of package.json
							if exports := pkgJson.Exports; exports.Len() > 0 {
								for _, exportName := range exports.keys {
									v := exports.values[exportName]
									if exportName == "." || (strings.HasPrefix(exportName, "./") && !strings.ContainsRune(exportName, '*')) {
										match := false
										if s, ok := v.(string); ok && stripModuleExt(s) == stripModuleExt(modulePath) {
											// exports: "./foo": "./foo.js"
											match = true
										} else if m, ok := v.(JSONObject); ok {
											// exports: "./foo": { "import": "./foo.js" }
											// exports: "./foo": { "import": { default: "./foo.js" } }
											// ...
											paths := getExportConditionPaths(m)
											for _, path := range paths {
												if stripModuleExt(path) == stripModuleExt(modulePath) {
													match = true
													break
												}
											}
										}
										if match {
											asExport = path.Join(pkgJson.Name, stripModuleExt(exportName))
											if asExport != entrySpecifier && asExport != entrySpecifier+"/index" {
												externalPath, err := ctx.resolveExternalModule(asExport, args.Kind, withTypeJSON, analyzeMode)
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

							if len(args.With) > 0 && args.With["type"] == "css" {
								return esbuild.OnResolveResult{
									Path:        "/" + ctx.esm.Name() + utils.NormalizePathname(modulePath),
									External:    true,
									SideEffects: esbuild.SideEffectsFalse,
								}, nil
							}

							moduleFilename := path.Join(ctx.wd, "node_modules", ctx.esm.PkgName, modulePath)

							// split the module that is an alias of a dependency
							// means this file just include a single line(js): `export * from "dep"`
							if entry.module && len(pkgJson.Dependencies)+len(pkgJson.PeerDependencies) > 0 {
								fi, err := os.Lstat(moduleFilename)
								if err == nil && fi.Size() < 256 {
									data, err := os.ReadFile(moduleFilename)
									if err == nil {
										var exportFrom string
										for _, line := range bytes.Split(data, []byte{'\n'}) {
											line = bytes.TrimSpace(line)
											if strings.HasPrefix(string(line), "export * from") || strings.HasPrefix(string(line), "export*from") {
												a := bytes.Split(line, []byte{'"'})
												if len(a) != 3 {
													a = bytes.Split(line, []byte{'\''})
												}
												if len(a) == 3 {
													exportFrom = string(a[1])
													break
												}
											} else if !(bytes.HasPrefix(line, []byte("//")) || (bytes.HasPrefix(line, []byte("/*")) && bytes.HasSuffix(line, []byte("*/")))) {
												exportFrom = ""
												break
											}
										}
										if exportFrom != "" && !isRelPathSpecifier(exportFrom) {
											externalPath, err := ctx.resolveExternalModule(exportFrom, args.Kind, withTypeJSON, analyzeMode)
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

							// bundle the sub module if:
							// - it's the entry point
							// - it's not a dynamic import and the `?bundle=false` flag is not present
							// - it's not in the `splitting` list
							if modulePath == entry.main || (asExport != "" && asExport == entrySpecifier) || (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) {
								if existsFile(moduleFilename) {
									pkgDir := path.Join(ctx.wd, "node_modules", ctx.esm.PkgName)
									short := strings.TrimPrefix(moduleFilename, pkgDir)[1:]
									if analyzeMode && moduleFilename != entryModuleFilename && strings.HasPrefix(args.Importer, pkgDir) {
										includes = append(includes, [2]string{short, strings.TrimPrefix(args.Importer, pkgDir)[1:]})
									}
									if !analyzeMode && ctx.splitting != nil && ctx.splitting.Has(short) {
										asExport = pkgJson.Name + utils.NormalizePathname(stripEntryModuleExt(short))
										if asExport != entrySpecifier && asExport != entrySpecifier+"/index" {
											externalPath, err := ctx.resolveExternalModule(asExport, args.Kind, withTypeJSON, false)
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

					// use `npm:` specifier for `denonext` target if the specifier is in the `forceNpmSpecifiers` list
					if forceNpmSpecifiers[specifier] && ctx.target == "denonext" {
						version := ""
						pkgName, _, subPath, _ := splitEsmPath(specifier)
						if pkgName == ctx.esm.PkgName {
							version = ctx.esm.PkgVersion
						} else if v, ok := pkgJson.Dependencies[pkgName]; ok && isExactVersion(v) {
							version = v
						} else if v, ok := pkgJson.PeerDependencies[pkgName]; ok && isExactVersion(v) {
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

					// replace some npm modules with browser native APIs
					var replacement npm_replacements.NpmReplacement
					var ok bool
					query := "browser"
					if ctx.isDenoTarget() {
						query = "deno"
					} else if ctx.target == "node" {
						query = "node"
					}
					if ctx.dev {
						replacement, ok = npm_replacements.Get(specifier + "_" + query + "_dev")
						if !ok {
							replacement, ok = npm_replacements.Get(specifier + "_dev")
						}
					}
					if !ok {
						replacement, ok = npm_replacements.Get(specifier + "_" + query)
					}
					if !ok {
						replacement, ok = npm_replacements.Get(specifier)
					}
					if ok {
						if args.Kind == esbuild.ResolveJSRequireCall || args.Kind == esbuild.ResolveJSRequireResolve {
							ctx.cjsRequires = append(ctx.cjsRequires, [3]string{
								"npm:" + specifier,
								string(replacement.IIFE),
								"",
							})
							return esbuild.OnResolveResult{
								Path:     "npm:" + specifier,
								External: true,
							}, nil
						}
						return esbuild.OnResolveResult{
							Path:       specifier,
							PluginData: replacement.ESM,
							Namespace:  "npm-replacement",
						}, nil
					}

					// dynamic external
					sideEffects := esbuild.SideEffectsTrue
					if specifier == pkgJson.Name || specifier == pkgJson.PkgName || strings.HasPrefix(specifier, pkgJson.Name+"/") || strings.HasPrefix(specifier, pkgJson.Name+"/") {
						sideEffects = pkgSideEffects
					}
					externalPath, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
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
					if !isExactVersion(svelteVersion) {
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
					if !isExactVersion(vueVersion) {
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

	nodeFilename := ctx.Path()
	nodeDirname, _ := utils.SplitByLastByte(nodeFilename, '/')
	nodeEnv := ctx.getNodeEnv()
	define := map[string]string{
		"__filename":           fmt.Sprintf(`"%s"`, nodeFilename),
		"__dirname":            fmt.Sprintf(`"%s"`, nodeDirname),
		"Buffer":               "__Buffer$",
		"process":              "__Process$",
		"setImmediate":         "__setImmediate$",
		"clearImmediate":       "__clearImmediate$",
		"require.resolve":      "__rResolve$",
		"process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
	}
	// support features that can not be polyfilled
	supported := map[string]bool{
		"bigint":          true,
		"top-level-await": true,
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
		if ctx.isDenoTarget() {
			// deno 2 has removed the `window` global object, let's replace it with `globalThis`
			define["window"] = "globalThis"
		}
		for k, v := range define {
			define["global."+k] = v
			define["globalThis."+k] = v
		}
		define["global"] = "globalThis"
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
		Supported:         supported,
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
		Plugins:           []esbuild.Plugin{esmifyPlugin},
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

	esbCtx, ctxErr := esbuild.Context(options)
	if ctxErr != nil {
		err = errors.New("esbuild: " + ctxErr.Error())
		return
	}
	defer esbCtx.Dispose()

REBUILD:
	res := esbCtx.Rebuild()
	if len(res.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := res.Errors[0].Text
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
				goto REBUILD
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
						exports = set.New[string]()
						browserExclude[path] = exports
					}
					if !exports.Has(exportName) {
						exports.Add(exportName)
						goto REBUILD
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

	for _, w := range res.Warnings {
		log.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
	}

	imports := set.New[string]()

	for _, file := range res.OutputFiles {
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
				ids := set.New[string]()
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
				if ids.Has("__setImmediate$") {
					header.WriteString(`var __setImmediate$ = (cb, ...args) => ( { $t: setTimeout(cb, 0, ...args), [Symbol.dispose](){ clearTimeout(this.t) } });`)
					header.WriteByte('\n')
				}
				if ids.Has("__clearImmediate$") {
					header.WriteString(`var __clearImmediate$ = i => clearTimeout(i.$t);`)
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
				set := set.New[string]()
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
								}
								err = b.install()
								if err == nil {
									entry := b.resolveEntry(dep)
									if !entry.module {
										ret, cjsNamedExports, _, e := b.lexer(&entry)
										if e == nil && ret.CJS && stringInSlice(cjsNamedExports, "__esModule") {
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

	for _, file := range res.OutputFiles {
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
		p, err := ctx.npmrc.installPackage(ctx.esm.Package())
		if err != nil {
			return err
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
					} else if obj, ok := v.(JSONObject); ok {
						// exports: { ".": { "require": "./cjs/index.js", "import": "./esm/index.js" } }
						// exports: { ".": { "node": { "require": "./cjs/index.js", "import": "./esm/index.js" } } }
						// ...
						paths := getExportConditionPaths(obj)
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
				ctx.rawPath = ctx.path
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
