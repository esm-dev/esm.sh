package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

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
	zoneId      string
	npmrc       *NpmRC
	packageJson *PackageJSON
	esmPath     ESMPath
	args        BuildArgs
	bundleMode  BundleMode
	target      string
	pinedTarget bool
	dev         bool
	wd          string
	pkgDir      string
	pnpmPkgDir  string
	path        string
	stage       string
	deprecated  string
	importMap   [][2]string
	cjsRequires [][3]string
	smOffset    int
	subBuilds   *StringSet
	wg          sync.WaitGroup
}

type BuildMeta struct {
	CSS              bool     `json:"css,omitempty"`
	FromCJS          bool     `json:"fromCJS,omitempty"`
	TypesOnly        bool     `json:"typesOnly,omitempty"`
	Dts              string   `json:"dts,omitempty"`
	Deps             []string `json:"deps,omitempty"`
	HasDefaultExport bool     `json:"hasDefaultExport,omitempty"`
	NamedExports     []string `json:"-"`
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

func NewBuildContext(zoneId string, npmrc *NpmRC, esm ESMPath, args BuildArgs, target string, pinedTarget bool, bundleMode BundleMode, dev bool) *BuildContext {
	return &BuildContext{
		zoneId:      zoneId,
		npmrc:       npmrc,
		esmPath:     esm,
		args:        args,
		target:      target,
		pinedTarget: pinedTarget,
		dev:         dev,
		bundleMode:  bundleMode,
		subBuilds:   NewStringSet(),
	}
}

func (ctx *BuildContext) Query() (*BuildMeta, error) {
	key := ctx.getSavepath() + ".meta"
	r, _, err := buildStorage.Get(key)
	if err != nil && err != storage.ErrNotFound {
		return nil, err
	}
	if err == nil {
		var b BuildMeta
		err = json.NewDecoder(r).Decode(&b)
		r.Close()
		if err == nil {
			return &b, nil
		}
		// delete the invalid build meta
		buildStorage.Delete(key)
	}
	return nil, nil
}

func (ctx *BuildContext) Build() (ret *BuildMeta, err error) {
	if ctx.target == "types" {
		return ctx.buildTypes()
	}

	// query previous build
	ret, err = ctx.Query()
	if err != nil || ret != nil {
		return
	}

	// check if the package is deprecated
	if ctx.deprecated == "" && !ctx.esmPath.GhPrefix && !ctx.esmPath.PrPrefix && !strings.HasPrefix(ctx.esmPath.PkgName, "@jsr/") {
		var info *PackageJSON
		info, err = ctx.npmrc.fetchPackageInfo(ctx.esmPath.PkgName, ctx.esmPath.PkgVersion)
		if err != nil {
			return
		}
		ctx.deprecated = info.Deprecated
	}

	// install the package
	ctx.stage = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	// query again after installation (in case the sub-module path has been changed by the `normalizePackageJSON` function)
	ret, err = ctx.Query()
	if err != nil || ret != nil {
		return
	}

	// build the module
	ctx.stage = "build"
	ret, err = ctx.buildModule()
	if err != nil {
		return
	}

	// save the build result into db
	key := ctx.getSavepath() + ".meta"
	buf := bytes.NewBuffer(nil)
	err = json.NewEncoder(buf).Encode(ret)
	if err != nil {
		return
	}
	if e := buildStorage.Put(key, buf); e != nil {
		log.Errorf("db: %v", e)
	}
	return
}

func (ctx *BuildContext) install() (err error) {
	if ctx.wd == "" || ctx.packageJson == nil {
		ctx.packageJson, err = ctx.npmrc.installPackage(ctx.esmPath)
		if err != nil {
			return
		}
		ctx.normalizePackageJSON(ctx.packageJson)
		ctx.wd = path.Join(ctx.npmrc.StoreDir(), ctx.esmPath.PackageName())
		ctx.pkgDir = path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName)
		if rp, e := os.Readlink(ctx.pkgDir); e == nil {
			ctx.pnpmPkgDir = path.Join(path.Dir(ctx.pkgDir), rp)
		} else {
			ctx.pnpmPkgDir = ctx.pkgDir
		}
	}
	return
}

func (ctx *BuildContext) buildModule() (result *BuildMeta, err error) {
	// build json
	if strings.HasSuffix(ctx.esmPath.SubBareName, ".json") {
		nmDir := path.Join(ctx.wd, "node_modules")
		jsonPath := path.Join(nmDir, ctx.esmPath.PkgName, ctx.esmPath.SubBareName)
		if existsFile(jsonPath) {
			var jsonData []byte
			jsonData, err = os.ReadFile(jsonPath)
			if err != nil {
				return
			}
			buffer := bytes.NewBufferString("export default ")
			buffer.Write(jsonData)
			err = buildStorage.Put(ctx.getSavepath(), buffer)
			if err != nil {
				return
			}
			result = &BuildMeta{
				HasDefaultExport: true,
			}
			return
		}
	}

	entry := ctx.resolveEntry(ctx.esmPath)
	if entry.isEmpty() {
		err = fmt.Errorf("could not resolve the build entry")
		return
	}
	log.Debugf("build(%s): Entry%+v", ctx.esmPath, entry)

	typesOnly := strings.HasPrefix(ctx.packageJson.Name, "@types/") || (entry.esm == "" && entry.cjs == "" && entry.dts != "")
	if typesOnly {
		result = &BuildMeta{
			TypesOnly: true,
			Dts:       "/" + ctx.esmPath.PackageName() + entry.dts[1:],
		}
		ctx.transformDTS(entry.dts)
		return
	}

	result, reexport, err := ctx.lexer(&entry, false)
	if err != nil && !strings.HasPrefix(err.Error(), "cjsLexer: Can't resolve") {
		return
	}

	// cjs reexport
	if reexport != "" {
		mod, _, e := ctx.lookupDep(reexport, false)
		if e != nil {
			err = e
			return
		}
		// create a new build context to check if the reexported module has default export
		b := NewBuildContext(ctx.zoneId, ctx.npmrc, mod, ctx.args, ctx.target, ctx.pinedTarget, BundleFalse, ctx.dev)
		err = b.install()
		if err != nil {
			return
		}
		entry := b.resolveEntry(mod)
		result, _, err = b.lexer(&entry, false)
		if err != nil {
			return
		}
		importUrl := ctx.getImportPath(mod, ctx.getBuildArgsPrefix(false))
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `export * from "%s";`, importUrl)
		if result.HasDefaultExport {
			fmt.Fprintf(buf, "\n")
			fmt.Fprintf(buf, `export { default } from "%s";`, importUrl)
		}
		err = buildStorage.Put(ctx.getSavepath(), buf)
		if err != nil {
			return
		}
		result.Dts, err = ctx.resloveDTS(entry)
		return
	}

	var entryPoint string
	var input *esbuild.StdinOptions

	entryModuleSpecifier := ctx.esmPath.PkgName
	if ctx.esmPath.SubBareName != "" {
		entryModuleSpecifier += "/" + ctx.esmPath.SubBareName
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
		input = &esbuild.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: ctx.wd,
			Sourcefile: "entry.js",
		}
	} else {
		if ctx.args.exports.Len() > 0 {
			input = &esbuild.StdinOptions{
				Contents:   fmt.Sprintf(`export { %s } from "%s";`, strings.Join(ctx.args.exports.Values(), ","), entryModuleSpecifier),
				ResolveDir: ctx.wd,
				Sourcefile: "entry.js",
			}
		} else {
			entryPoint = path.Join(ctx.pkgDir, entry.esm)
		}
	}

	pkgSideEffects := esbuild.SideEffectsTrue
	if ctx.packageJson.SideEffectsFalse {
		pkgSideEffects = esbuild.SideEffectsFalse
	}

	noBundle := ctx.bundleMode == BundleFalse || (ctx.packageJson.SideEffects != nil && ctx.packageJson.SideEffects.Len() > 0)
	if ctx.packageJson.Esmsh != nil {
		if v, ok := ctx.packageJson.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				noBundle = true
			}
		}
	}

	browserExclude := map[string]*StringSet{}
	implicitExternal := NewStringSet()
	deps := NewStringSet()

	nodeEnv := ctx.getNodeEnv()
	filename := ctx.Path()
	dirname, _ := utils.SplitByLastByte(filename, '/')
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
	}
	options := esbuild.BuildOptions{
		Bundle:            true,
		Format:            esbuild.FormatESModule,
		Target:            targets[ctx.target],
		Platform:          esbuild.PlatformBrowser,
		Define:            define,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   "react",
		MinifyWhitespace:  config.Minify,
		MinifyIdentifiers: config.Minify,
		MinifySyntax:      config.Minify,
		KeepNames:         ctx.args.keepNames,         // prevent class/function names erasing
		IgnoreAnnotations: ctx.args.ignoreAnnotations, // some libs maybe use wrong side-effect annotations
		Conditions:        conditions,
		Loader:            loaders,
		Outdir:            "/esbuild",
		Write:             false,
	}
	if input != nil {
		options.Stdin = input
	} else if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
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
		_, ok1 := ctx.packageJson.Dependencies[pkgName]
		_, ok2 := ctx.packageJson.PeerDependencies[pkgName]
		if ok1 || ok2 {
			options.JSXImportSource = pkgName
			if pkgName == "hono" {
				options.JSXImportSource += "/jsx"
			}
			break
		}
	}
	options.Plugins = []esbuild.Plugin{{
		Name: "esm",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(
				esbuild.OnResolveOptions{Filter: ".*"},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					// if it's the entry module
					if args.Path == entryPoint || args.Path == entryModuleSpecifier {
						path := args.Path
						if path == entryModuleSpecifier {
							if entry.esm != "" {
								path = filepath.Join(ctx.pnpmPkgDir, entry.esm)
							} else if entry.cjs != "" {
								path = filepath.Join(ctx.pnpmPkgDir, entry.cjs)
							}
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
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", strings.TrimPrefix(args.Path, "file:"), ctx.esmPath),
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

					// if `?ignore-require` present, ignore specifier that is a require call
					if ctx.args.externalRequire && args.Kind == esbuild.ResolveJSRequireCall && entry.esm != "" {
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
						externalPath, err := ctx.resolveExternalModule(args.Path, args.Kind)
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
						pkgName, _, subpath, _ := splitESMPath(specifier)
						if name, ok := ctx.args.alias[pkgName]; ok {
							specifier = name
							if subpath != "" {
								specifier += "/" + subpath
							}
						}
					}

					// resolve specifier with package `imports` field
					if len(ctx.packageJson.Imports) > 0 {
						if v, ok := ctx.packageJson.Imports[specifier]; ok {
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
					if !isRelPathSpecifier(specifier) && len(ctx.packageJson.Browser) > 0 && ctx.isBrowserTarget() {
						if name, ok := ctx.packageJson.Browser[specifier]; ok {
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
						pkgName, _, subPath, _ := splitESMPath(specifier)
						if pkgName == ctx.esmPath.PkgName {
							version = ctx.esmPath.PkgVersion
						} else if v, ok := ctx.packageJson.Dependencies[pkgName]; ok && regexpVersionStrict.MatchString(v) {
							version = v
						} else if v, ok := ctx.packageJson.PeerDependencies[pkgName]; ok && regexpVersionStrict.MatchString(v) {
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
						fullFilepath = path.Join(ctx.wd, "node_modules", ".pnpm", "node_modules", specifier)
					}

					// node native modules do not work via http import
					if strings.HasSuffix(fullFilepath, ".node") && existsFile(fullFilepath) {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), ctx.esmPath),
							External: true,
						}, nil
					}

					// bundles json module
					if strings.HasSuffix(fullFilepath, ".json") {
						return esbuild.OnResolveResult{}, nil
					}

					// embed wasm as WebAssembly.Module
					if strings.HasSuffix(fullFilepath, ".wasm") {
						return esbuild.OnResolveResult{
							Path:      fullFilepath,
							Namespace: "wasm",
						}, nil
					}

					// transfrom svelte component
					if strings.HasSuffix(fullFilepath, ".svelte") {
						return esbuild.OnResolveResult{
							Path:      fullFilepath,
							Namespace: "svelte",
						}, nil
					}

					// transfrom Vue SFC
					if strings.HasSuffix(fullFilepath, ".vue") {
						return esbuild.OnResolveResult{
							Path:      fullFilepath,
							Namespace: "vue",
						}, nil
					}

					// externalize parent module
					// e.g. "react/jsx-runtime" imports "react"
					if ctx.esmPath.SubBareName != "" && specifier == ctx.esmPath.PkgName && ctx.bundleMode != BundleAll {
						externalPath, err := ctx.resolveExternalModule(ctx.esmPath.PkgName, args.Kind)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							External:    true,
							SideEffects: pkgSideEffects,
						}, nil
					}

					// it's nodejs builtin module
					if isNodeBuiltInModule(specifier) {
						externalPath, err := ctx.resolveExternalModule(specifier, args.Kind)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:     externalPath,
							External: true,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies and `?external` query]
					if ctx.bundleMode == BundleAll && !ctx.args.external.Has(toPackageName(specifier)) && !implicitExternal.Has(specifier) {
						pkgName := toPackageName(specifier)
						_, ok := ctx.packageJson.PeerDependencies[pkgName]
						if !ok {
							return esbuild.OnResolveResult{}, nil
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) && ctx.packageJson.Name != "@babel/runtime" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/")) {
						return esbuild.OnResolveResult{}, nil
					}

					if strings.HasPrefix(specifier, "/") || isRelPathSpecifier(specifier) {
						specifier = strings.TrimPrefix(fullFilepath, path.Join(ctx.wd, "node_modules")+"/")
						if strings.HasPrefix(specifier, ".pnpm") {
							a := strings.Split(specifier, "/node_modules/")
							if len(a) > 1 {
								specifier = a[1]
							}
						}
						pkgName := ctx.packageJson.Name
						isInternalModule := strings.HasPrefix(specifier, pkgName+"/")
						if !isInternalModule && ctx.packageJson.PkgName != "" {
							// github packages may have different package name with the repository name
							pkgName = ctx.packageJson.PkgName
							isInternalModule = strings.HasPrefix(specifier, pkgName+"/")
						}
						if isInternalModule {
							// if meets scenarios of "./index.mjs" importing "./index.c?js"
							// let esbuild to handle it
							if stripModuleExt(fullFilepath) == stripModuleExt(args.Importer) {
								return esbuild.OnResolveResult{}, nil
							}

							moduleSpecifier := "." + strings.TrimPrefix(specifier, pkgName)

							if path.Ext(fullFilepath) == "" || !existsFile(fullFilepath) {
								subPath := utils.NormalizePathname(moduleSpecifier)[1:]
								entry := ctx.resolveEntry(ESMPath{
									PkgName:     ctx.esmPath.PkgName,
									PkgVersion:  ctx.esmPath.PkgVersion,
									SubBareName: toModuleBareName(subPath, true),
									SubPath:     subPath,
								})
								if args.Kind == esbuild.ResolveJSImportStatement || args.Kind == esbuild.ResolveJSDynamicImport {
									if entry.esm != "" {
										moduleSpecifier = entry.esm
									} else if entry.cjs != "" {
										moduleSpecifier = entry.cjs
									}
								} else if args.Kind == esbuild.ResolveJSRequireCall || args.Kind == esbuild.ResolveJSRequireResolve {
									if entry.cjs != "" {
										moduleSpecifier = entry.cjs
									} else if entry.esm != "" {
										moduleSpecifier = entry.esm
									}
								}
							}

							// resolve specifier with package `browser` field
							if len(ctx.packageJson.Browser) > 0 && ctx.isBrowserTarget() {
								if path, ok := ctx.packageJson.Browser[moduleSpecifier]; ok {
									if path == "" {
										return esbuild.OnResolveResult{
											Path:      args.Path,
											Namespace: "browser-exclude",
										}, nil
									}
									if !isRelPathSpecifier(path) {
										externalPath, err := ctx.resolveExternalModule(path, args.Kind)
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

							bareName := stripModuleExt(moduleSpecifier)

							// split modules based on the `exports` field of package.json
							if om, ok := ctx.packageJson.Exports.(*OrderedMap); ok {
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
											exportModuleName := path.Join(ctx.packageJson.Name, exportPrefix+strings.TrimPrefix(bareName, prefix))
											if exportModuleName != entryModuleSpecifier && exportModuleName != entryModuleSpecifier+"/index" {
												externalPath, err := ctx.resolveExternalModule(exportModuleName, args.Kind)
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
											exportModuleName := path.Join(ctx.packageJson.Name, stripModuleExt(exportName))
											if exportModuleName != entryModuleSpecifier && exportModuleName != entryModuleSpecifier+"/index" {
												externalPath, err := ctx.resolveExternalModule(exportModuleName, args.Kind)
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
							moduleFilepath := path.Join(ctx.pnpmPkgDir, moduleSpecifier)

							// if it's the entry module
							if moduleSpecifier == entry.cjs || moduleSpecifier == entry.esm {
								return esbuild.OnResolveResult{Path: moduleFilepath}, nil
							}

							// split the module that is an alias of a dependency
							// means this file just include a single line(js): `export * from "dep"`
							fi, ioErr := os.Lstat(moduleFilepath)
							if ioErr == nil && fi.Size() < 128 {
								data, ioErr := os.ReadFile(moduleFilepath)
								if ioErr == nil {
									out, esbErr := minify(string(data), esbuild.LoaderJS, esbuild.ESNext)
									if esbErr == nil {
										p := bytes.Split(out, []byte("\""))
										if len(p) == 3 && string(p[0]) == "export*from" && string(p[2]) == ";\n" {
											url := string(p[1])
											if !isRelPathSpecifier(url) {
												externalPath, err := ctx.resolveExternalModule(url, args.Kind)
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

							// bundle the internal module if it's not a dynamic import or `?bundle=false` query present
							if args.Kind != esbuild.ResolveJSDynamicImport && !noBundle {
								if existsFile(moduleFilepath) {
									return esbuild.OnResolveResult{Path: moduleFilepath}, nil
								}
								// let esbuild to handle it
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
									string(replacement.cjs),
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
					if specifier == ctx.packageJson.Name || specifier == ctx.packageJson.PkgName || strings.HasPrefix(specifier, ctx.packageJson.Name+"/") || strings.HasPrefix(specifier, ctx.packageJson.Name+"/") {
						sideEffects = pkgSideEffects
					}
					externalPath, err := ctx.resolveExternalModule(specifier, args.Kind)
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

			// svelte component loader
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
					} else if version, ok := ctx.packageJson.Dependencies["svelte"]; ok {
						svelteVersion = version
					} else if version, ok := ctx.packageJson.PeerDependencies["svelte"]; ok {
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
					out, err := transformSvelte(ctx.npmrc, svelteVersion, []string{ctx.esmPath.String(), string(code)})
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					return esbuild.OnLoadResult{Contents: &out.Code, Loader: esbuild.LoaderJS}, nil
				},
			)

			// vue component loader
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
					} else if version, ok := ctx.packageJson.Dependencies["vue"]; ok {
						vueVersion = version
					} else if version, ok := ctx.packageJson.PeerDependencies["vue"]; ok {
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
					out, err := transformVue(ctx.npmrc, vueVersion, []string{ctx.esmPath.String(), string(code)})
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
	}}

rebuild:
	ret := esbuild.Build(options)
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
		return
	}

	for _, w := range ret.Warnings {
		log.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
	}

	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			jsContent := file.Contents
			header := bytes.NewBufferString("/* esm.sh - ")
			if ctx.esmPath.GhPrefix {
				header.WriteString("github:")
			} else if ctx.esmPath.PrPrefix {
				header.WriteString("pkg.pr.new/")
			}
			header.WriteString(ctx.esmPath.PkgName)
			if ctx.esmPath.GhPrefix {
				header.WriteByte('#')
			} else {
				header.WriteByte('@')
			}
			header.WriteString(ctx.esmPath.PkgVersion)
			if ctx.esmPath.SubBareName != "" {
				header.WriteByte('/')
				header.WriteString(ctx.esmPath.SubBareName)
			}
			header.WriteString(" */\n")

			// remove shebang
			if bytes.HasPrefix(jsContent, []byte("#!/")) {
				jsContent = jsContent[bytes.IndexByte(jsContent, '\n')+1:]
				ctx.smOffset--
			}

			// add nodejs compatibility
			if ctx.target != "node" {
				ids := NewStringSet()
				for _, r := range regexpESMInternalIdent.FindAll(jsContent, -1) {
					ids.Add(string(r))
				}
				if ids.Has("__Process$") {
					if ctx.args.external.Has("node:process") || ctx.args.externalAll {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if ctx.isBrowserTarget() {
						if len(ctx.packageJson.Browser) > 0 {
							var browserExclude bool
							if name, ok := ctx.packageJson.Browser["process"]; ok {
								browserExclude = name == ""
							} else if name, ok := ctx.packageJson.Browser["node:process"]; ok {
								browserExclude = name == ""
							}
							if !browserExclude {
								fmt.Fprintf(header, `const __Process$ = {env:{}};%s`, EOL)
							}
						} else {
							fmt.Fprintf(header, `import __Process$ from "/node/process.mjs";%s`, EOL)
							deps.Add("/node/process.mjs")
						}
					} else if ctx.target == "denonext" {
						fmt.Fprintf(header, `import __Process$ from "node:process";%s`, EOL)
					} else if ctx.target == "deno" {
						fmt.Fprintf(header, `import __Process$ from "https://deno.land/std@0.177.1/node/process.ts";%s`, EOL)
					}
				}
				if ids.Has("__Buffer$") {
					if ctx.args.external.Has("node:buffer") || ctx.args.externalAll {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if ctx.isBrowserTarget() {
						var browserExclude bool
						if len(ctx.packageJson.Browser) > 0 {
							if name, ok := ctx.packageJson.Browser["buffer"]; ok {
								browserExclude = name == ""
							} else if name, ok := ctx.packageJson.Browser["node:buffer"]; ok {
								browserExclude = name == ""
							}
						}
						if !browserExclude {
							fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "/node/buffer.mjs";%s`, EOL)
							deps.Add("/node/buffer.mjs")
						}
					} else if ctx.target == "denonext" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "node:buffer";%s`, EOL)
					} else if ctx.target == "deno" {
						fmt.Fprintf(header, `import { Buffer as __Buffer$ } from "https://deno.land/std@0.177.1/node/buffer.ts";%s`, EOL)
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

			if len(ctx.cjsRequires) > 0 {
				requires := make([][3]string, 0, len(ctx.cjsRequires))
				mark := NewStringSet()
				for _, r := range ctx.cjsRequires {
					specifier := r[0]
					if mark.Has(specifier) {
						continue
					}
					mark.Add(specifier)
					requires = append(requires, r)
				}
				isEsModule := make([]bool, len(requires))
				for i, r := range requires {
					specifier := r[0]
					if strings.HasPrefix(specifier, "npm:") {
						fmt.Fprintf(header, `var __%x$=(()=>{%s})();`, i, r[1])
					} else {
						fmt.Fprintf(header, `import*as __%x$ from"%s";`, i, r[2])
						deps.Add(r[1])
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
					if !isRelPathSpecifier(specifier) && !isNodeBuiltInModule(specifier) && !strings.HasPrefix(specifier, "npm:") {
						esm, pkgJson, err := ctx.lookupDep(specifier, false)
						if err == nil {
							ctx.normalizePackageJSON(pkgJson)
							if pkgJson.Type == "module" || pkgJson.Module != "" {
								isEsModule[i] = true
							} else {
								b := NewBuildContext(ctx.zoneId, ctx.npmrc, esm, ctx.args, ctx.target, ctx.pinedTarget, BundleFalse, ctx.dev)
								err = b.install()
								if err == nil {
									entry := b.resolveEntry(esm)
									if entry.esm == "" {
										ret, _, e := b.lexer(&entry, true)
										if e == nil && contains(ret.NamedExports, "__esModule") {
											isEsModule[i] = true
										}
									}
								}
							}
						}
					}
				}
				fmt.Fprint(header, `var require=n=>{const e=m=>typeof m.default<"u"?m.default:m,c=m=>Object.assign({__esModule:true},m);switch(n){`)
				for i, r := range requires {
					specifier := r[0]
					if isEsModule[i] {
						fmt.Fprintf(header, `case"%s":return c(__%x$);`, specifier, i)
					} else {
						fmt.Fprintf(header, `case"%s":return e(__%x$);`, specifier, i)
					}
				}
				fmt.Fprintf(header, `default:console.error('module "'+n+'" not found');return null;}};%s`, EOL)
			}

			// check imports
			for _, a := range ctx.importMap {
				resolvedPathFull, resolvedPath := a[0], a[1]
				if bytes.Contains(jsContent, []byte(fmt.Sprintf(`"%s"`, resolvedPath))) {
					deps.Add(resolvedPathFull)
				}
			}

			// to fix the source map
			ctx.smOffset += strings.Count(header.String(), EOL)

			jsContent, dropSourceMap := ctx.rewriteJS(jsContent)
			finalContent := bytes.NewBuffer(header.Bytes())
			finalContent.Write(jsContent)

			if ctx.deprecated != "" {
				fmt.Fprintf(finalContent, `console.warn("%%c[esm.sh]%%c %%cdeprecated%%c %s@%s: %s", "color:grey", "", "color:red", "");%s`, ctx.esmPath.PkgName, ctx.esmPath.PkgVersion, strings.ReplaceAll(ctx.deprecated, "\"", "\\\""), "\n")
			}

			// add sourcemap Url
			if config.SourceMap && !dropSourceMap {
				finalContent.WriteString("//# sourceMappingURL=")
				finalContent.WriteString(path.Base(ctx.Path()))
				finalContent.WriteString(".map")
			}

			err = buildStorage.Put(ctx.getSavepath(), finalContent)
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
			result.CSS = true
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
				buf := bytes.NewBuffer(nil)
				if json.NewEncoder(buf).Encode(sourceMap) == nil {
					err = buildStorage.Put(ctx.getSavepath()+".map", buf)
					if err != nil {
						return
					}
				}
			}
		}
	}

	// wait for sub-builds
	ctx.wg.Wait()

	// filter and sort dependencies
	sortedDeps := sort.StringSlice{}
	for _, url := range deps.Values() {
		if strings.HasPrefix(url, "/") {
			sortedDeps = append(sortedDeps, url)
		}
	}
	sortedDeps.Sort()
	result.Deps = sortedDeps

	// resolve types(dts)
	result.Dts, err = ctx.resloveDTS(entry)
	return
}

func (ctx *BuildContext) buildTypes() (ret *BuildMeta, err error) {
	// install the package
	ctx.stage = "install"
	err = ctx.install()
	if err != nil {
		return
	}

	var dts string
	if endsWith(ctx.esmPath.SubPath, ".d.ts", "d.mts") {
		dts = "./" + ctx.esmPath.SubPath
	} else {
		entry := ctx.resolveEntry(ctx.esmPath)
		if entry.dts == "" {
			err = errors.New("types not found")
			return
		}
		dts = entry.dts
	}

	ctx.stage = "build"
	err = ctx.transformDTS(dts)
	if err == nil {
		ret = &BuildMeta{
			Dts: "/" + ctx.esmPath.PackageName() + dts[1:],
		}

	}
	return
}

func (ctx *BuildContext) transformDTS(types string) (err error) {
	start := time.Now()
	buildArgsPrefix := ctx.getBuildArgsPrefix(true)
	n, err := transformDTS(ctx, types, buildArgsPrefix, nil)
	if err != nil {
		return
	}
	log.Debugf("transform dts '%s'(%d related dts files) in %v", types, n, time.Since(start))
	return
}
