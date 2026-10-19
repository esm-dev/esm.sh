package server

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/npm_replacements"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
)

type BundleMode uint8

const (
	BundleDefault BundleMode = iota
	BundleDeps
	BundleFalse
)

type BuildContext struct {
	ctx         context.Context
	npmrc       *NpmRC
	logger      *log.Logger
	metaDB      *BuildMetaDB
	storage     storage.Storage
	esmPath     EsmPath
	args        BuildArgs
	bundleMode  BundleMode
	externalAll bool
	target      string
	dev         bool
	wd          string
	pkgJson     *npm.PackageJSON
	path        string
	status      atomic.Value
	splitting   *set.ReadOnlySet[string]
	importsLock sync.Mutex
	esmImports  [][2]string
	cjsRequires [][3]string
	smOffset    int
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
	key := ctx.Path()
	meta, err = withLRUCache(key, func() (*BuildMeta, error) {
		metadata, err := ctx.metaDB.Get(key)
		if err != nil {
			return nil, err
		}
		meta, err := decodeBuildMeta(metadata)
		if err != nil {
			// delete the invalid metadata
			ctx.metaDB.Delete(key)
			return nil, storage.ErrNotFound
		}
		return meta, nil
	})
	if err != nil {
		if err == storage.ErrNotFound {
			err = nil
		}
		return
	}
	ok = true
	return
}

func (ctx *BuildContext) Context() context.Context {
	if ctx.ctx != nil {
		return ctx.ctx
	}
	return context.Background()
}

func (ctx *BuildContext) checkCanceled() error {
	return ctx.Context().Err()
}

func (ctx *BuildContext) Build(buildCtx context.Context) (meta *BuildMeta, err error) {
	if buildCtx == nil {
		buildCtx = context.Background()
	}
	ctx.ctx = buildCtx
	if err = ctx.checkCanceled(); err != nil {
		return
	}
	if message := ctx.npmrc.getCachedPackageNotFound(ctx.esmPath.Package()); message != "" {
		return nil, errors.New(message)
	}
	if ctx.esmPath.PkgVersion != "" && (ctx.esmPath.GhPrefix || ctx.esmPath.PrPrefix || npm.IsExactVersion(ctx.esmPath.PkgVersion)) {
		key := "404-path:" + ctx.esmPath.PackageId() + "/build:" + ctx.Path()
		if message := negativeCache.get(key); message != "" {
			return nil, errors.New(message)
		}
		defer func() {
			// Missing dependencies and transient install failures can recover.
			if err == nil || ctx.pkgJson == nil {
				return
			}
			entry := ctx.resolveEntry(ctx.esmPath)
			missingEntry := err.Error() == "could not resolve build entry" && entry.isEmpty()
			missingTypes := err.Error() == "types not found" && (ctx.target == "types" || entry.isTypesOnly())
			if missingEntry || missingTypes {
				negativeCache.put(key, err.Error(), 0)
			}
		}()
	}
	if ctx.target == "types" {
		return ctx.buildTypes()
	}

	// check previous build
	meta, ok, err := ctx.Exists()
	if err != nil || ok {
		return
	}
	if err = ctx.checkCanceled(); err != nil {
		return
	}

	// install the package
	ctx.status.Store("install")
	err = ctx.install()
	if err != nil {
		return
	}

	// check previous build again after installation (in case the sub-module path has been changed by the `install` function)
	meta, ok, err = ctx.Exists()
	if err != nil || ok {
		return
	}
	if err = ctx.checkCanceled(); err != nil {
		return
	}

	// analyze splitting modules if bundling
	if ctx.pkgJson.Exports.Len() > 1 && (ctx.shouldBundle() || ctx.shouldBundleInternalModules()) {
		ctx.status.Store("analyze")
		ctx.analyzeSplitting()
	}

	// build the module
	ctx.status.Store("build")
	meta, _, err = ctx.buildModule(false)
	if err != nil {
		return
	}
	if err = ctx.checkCanceled(); err != nil {
		return
	}

	// save the build result to the storage
	key := ctx.Path()
	err = ctx.metaDB.Put(key, encodeBuildMeta(meta))
	if err != nil {
		ctx.logger.Errorf("db.put(%s): %v", key, err)
		err = errors.New("db: " + err.Error())
	}
	return
}

func (ctx *BuildContext) buildPath() {
	asteriskPrefix := ""
	if ctx.externalAll {
		asteriskPrefix = "*"
	}

	esm := ctx.esmPath
	if ctx.target == "types" {
		ctx.path = fmt.Sprintf(
			"/%s%s/%s%s",
			asteriskPrefix,
			esm.PackageId(),
			ctx.getBuildArgsPrefix(true),
			esm.SubPath,
		)
		return
	}

	name := strings.TrimSuffix(path.Base(esm.PkgName), ".js")
	if esm.SubPath != "" {
		if esm.SubPath == name {
			// if the sub-module name is same as the package name
			name = "__" + esm.SubPath
		} else {
			name = esm.SubPath
		}
		// workaround for es5-ext "../#/.." path
		if esm.PkgName == "es5-ext" {
			name = strings.ReplaceAll(name, "/#/", "/%23/")
		}
	}

	if ctx.dev {
		name += ".development"
	}
	switch ctx.bundleMode {
	case BundleDeps:
		name += ".bundle"
	case BundleFalse:
		name += ".nobundle"
	}
	ctx.path = fmt.Sprintf(
		"/%s%s/%s%s/%s.mjs",
		asteriskPrefix,
		esm.PackageId(),
		ctx.getBuildArgsPrefix(ctx.target == "types"),
		ctx.target,
		name,
	)
}

func (ctx *BuildContext) buildModule(analyzeMode bool) (meta *BuildMeta, includes [][2]string, err error) {
	var includesMu sync.Mutex
	if err = ctx.checkCanceled(); err != nil {
		return
	}
	entry := ctx.resolveEntry(ctx.esmPath)
	if entry.isEmpty() {
		err = errors.New("could not resolve build entry")
		return
	}

	if DEBUG && !analyzeMode {
		ctx.logger.Debugf(`build(%s): Entry{main: "%s", module: %v, types: "%s"}`, ctx.esmPath.String(), entry.main, entry.module, entry.types)
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
			Dts:       "/" + ctx.esmPath.PackageId() + "/" + ctx.getBuildArgsPrefix(true) + strings.TrimPrefix(entry.types, "./"),
		}
		return
	}

	// css entry
	if strings.HasSuffix(entry.main, ".css") {
		if analyzeMode {
			return
		}
		meta = &BuildMeta{CSSEntry: entry.main}
		return
	}
	if analyzeMode && strings.HasSuffix(entry.main, ".map") {
		return
	}

	// json module
	if strings.HasSuffix(entry.main, ".json") {
		if analyzeMode {
			return
		}
		var jsonData []byte
		pkgDir := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName)
		jsonPath := path.Join(pkgDir, entry.main)
		if !strings.HasPrefix(jsonPath, pkgDir+"/") {
			err = fmt.Errorf("could not resolve module %s", entry.main)
			return
		}
		jsonData, err = os.ReadFile(jsonPath)
		if err != nil {
			return
		}
		buffer := &bytes.Buffer{}
		buffer.WriteString("export default ")
		buffer.Write(jsonData)
		err = ctx.storage.Put(ctx.getSavePath(), buffer)
		if err != nil {
			ctx.logger.Errorf("storage.put(%s): %v", ctx.getSavePath(), err)
			err = errors.New("storage(put): " + err.Error())
			return
		}
		meta = &BuildMeta{ExportDefault: true}
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
		dep, _, e := ctx.resolveDependency(cjsReexport, false)
		if e != nil {
			err = e
			return
		}
		b := &BuildContext{
			npmrc:       ctx.npmrc,
			logger:      ctx.logger,
			metaDB:      ctx.metaDB,
			storage:     ctx.storage,
			esmPath:     dep,
			args:        ctx.args,
			externalAll: ctx.externalAll,
			target:      ctx.target,
			dev:         ctx.dev,
			ctx:         ctx.ctx,
		}
		err = b.install()
		if err != nil {
			return
		}
		depEntry := b.resolveEntry(dep)
		meta, _, _, err = b.lexer(&depEntry)
		if err != nil {
			return
		}
		importUrl := ctx.getImportPath(dep, ctx.getBuildArgsPrefix(false), ctx.externalAll)
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `export * from "%s";`, importUrl)
		if meta.ExportDefault {
			fmt.Fprintf(buf, `export { default } from "%s";`, importUrl)
		}
		err = ctx.storage.Put(ctx.getSavePath(), buf)
		if err != nil {
			ctx.logger.Errorf("storage.put(%s): %v", ctx.getSavePath(), err)
			err = errors.New("storage(put): " + err.Error())
			return
		}
		meta.Dts, err = ctx.resolveDTS(entry)
		if err == nil && meta.Dts == "" && ctx.pkgJson.Types == "" {
			meta.Dts, err = b.resolveDTS(depEntry)
		}
		return
	}

	entryModuleFilename := ctx.getPkgFullPath(entry.main)
	entrySpecifier := ctx.esmPath.PkgName
	if ctx.esmPath.SubPath != "" {
		entrySpecifier += "/" + ctx.esmPath.SubPath
	}

	var (
		entryPoint string
		stdin      esbuild.StdinOptions
	)

	if entry.module {
		entryPoint = entryModuleFilename
	} else {
		buf := &bytes.Buffer{}
		fmt.Fprintf(buf, `import * as cjsm from "%s";`, entrySpecifier)
		if len(cjsExports) > 0 {
			fmt.Fprintf(buf, `export { %s } from "%s";`, strings.Join(cjsExports, ","), entrySpecifier)
		}
		buf.WriteString("export default cjsm.default ?? cjsm;")
		stdin = esbuild.StdinOptions{
			Sourcefile: "endpoint.js",
			Contents:   buf.String(),
		}
	}

	var dependencyRootVersions sync.Map
	var rootVersion *semver.Version
	if ctx.bundleMode == BundleDeps && !ctx.externalAll && ctx.args.Alias[ctx.pkgJson.Name] == "" {
		rootVersion, _ = semver.NewVersion(ctx.pkgJson.Version)
	}
	browserExclude := map[string]*set.Set[string]{}
	implicitExternal := set.New[string]()
	bundledModules := set.New[string]()
	dynamicImports := set.New[string]()
	bundledDynamicImports := set.New[string]()
	solidJSXRuntimes := set.New[string]()
	noBundle := !ctx.shouldBundle()
	nativeResolve := new(bool)
	probeResolve := new(bool)
	var exportStarExternal sync.Map
	esmifyPlugin := esbuild.Plugin{
		Name: "esmify",
		Setup: func(build esbuild.PluginBuild) {
			// reports whether `export *` in the module reaches an external module, directly or through bundled modules
			var hasExternalExportStar func(filename string, visiting []string) (external bool, cyclic bool)
			hasExternalExportStar = func(filename string, visiting []string) (external bool, cyclic bool) {
				if v, ok := exportStarExternal.Load(filename); ok {
					return v.(bool), false
				}
				if slices.Contains(visiting, filename) {
					return false, true
				}
				if !slices.Contains(moduleExts, path.Ext(filename)) {
					return false, false
				}
				data, err := os.ReadFile(filename)
				if err != nil || !bytes.Contains(data, []byte("export")) || !bytes.ContainsRune(data, '*') {
					return false, false
				}
				ast, err := parseJS(filename, data)
				if err != nil {
					return false, false
				}
				visiting = append(visiting, filename)
				for _, i := range ast.ExportStarImportRecords {
					r := build.Resolve(ast.ImportRecords[i].Path.Text, esbuild.ResolveOptions{
						Importer: filename, ResolveDir: path.Dir(filename), Kind: esbuild.ResolveJSImportStatement, PluginData: probeResolve,
					})
					// unresolved sources become implicit externals on rebuild
					if len(r.Errors) > 0 || r.External {
						external = true
						break
					}
					ext, cyc := hasExternalExportStar(r.Path, visiting)
					cyclic = cyclic || cyc
					if ext {
						external = true
						break
					}
				}
				// warning: a negative result inside a cycle depends on modules still being checked
				if external || !cyclic {
					exportStarExternal.Store(filename, external)
				}
				return
			}

			// resovler
			build.OnResolve(
				esbuild.OnResolveOptions{Filter: ".*"},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if args.PluginData == nativeResolve {
						return esbuild.OnResolveResult{}, nil
					}

					// entry point
					if rootVersion != nil && (args.Path == ctx.pkgJson.Name || strings.HasPrefix(args.Path, ctx.pkgJson.Name+"/")) && !isExternalModule(args.Path, ctx.args.External) {
						importer, ok := strings.CutPrefix(args.Importer, path.Join(ctx.wd, "node_modules")+"/")
						importerName := toPackageName(importer)
						if ok && importerName != ctx.pkgJson.Name {
							cached, ok := dependencyRootVersions.Load(importerName)
							if !ok {
								var raw npm.PackageJSONRaw
								if err := utils.ParseJSONFile(path.Join(ctx.wd, "node_modules", importerName, "package.json"), &raw); err != nil {
									return esbuild.OnResolveResult{}, err
								}
								pkg := raw.ToNpmPackage()
								version := pkg.Dependencies[ctx.pkgJson.Name]
								if version == "" {
									version = pkg.PeerDependencies[ctx.pkgJson.Name]
								}
								if override := ctx.args.Deps[ctx.pkgJson.Name]; override != "" {
									version = override
								}
								if constraint, err := semver.NewConstraint(version); err != nil || constraint.Check(rootVersion) {
									version = ""
								}
								cached, _ = dependencyRootVersions.LoadOrStore(importerName, version)
							}
							if version := cached.(string); version != "" {
								specifier := ctx.pkgJson.Name + "@" + version + strings.TrimPrefix(args.Path, ctx.pkgJson.Name)
								externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, args.With["type"] == "json", analyzeMode)
								return esbuild.OnResolveResult{Path: externalPath, SideEffects: sideEffects, External: true}, err
							}
						}
					}

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

					// ban `file:` imports
					if after, ok := strings.CutPrefix(args.Path, "file:"); ok {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-file-dependency&name=%s&importer=%s", after, ctx.esmPath.String()),
							External: true,
						}, nil
					}

					// skip `data:` and `http:` imports
					if strings.HasPrefix(args.Path, "data:") || strings.HasPrefix(args.Path, "https:") || strings.HasPrefix(args.Path, "http:") {
						return esbuild.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// if `?external-require` present, ignore specifier that is a require call
					if ctx.args.ExternalRequire && args.Kind == esbuild.ResolveJSRequireCall && entry.module {
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

					originalSpecifier := specifier
					if len(ctx.args.Alias) > 0 && !isRelPathSpecifier(specifier) {
						pkgName, _, subPath := splitEsmPath(specifier)
						if name, ok := ctx.args.Alias[pkgName]; ok {
							specifier = name
							if len(subPath) > 0 {
								specifier += "/" + subPath
							}
						}
					}

					if implicitExternal.Has(args.Path) || implicitExternal.Has(specifier) {
						externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							SideEffects: sideEffects,
							External:    true,
						}, nil
					}

					// resolve specifier using the `imports` field of package.json/deno.json
					if pkgJson.Imports.Len() > 0 {
						var v any
						var ok bool
						v, ok = pkgJson.Imports.Get(specifier)
						if !ok && !isRelPathSpecifier(specifier) {
							// check tailing slash
							pkgName, _, subPath := splitEsmPath(specifier)
							v, ok = pkgJson.Imports.Get(pkgName)
							if !ok {
								v, ok = pkgJson.Imports.Get(pkgName + "/")
							}
							if ok && len(subPath) > 0 {
								if s, ok := v.(string); ok {
									v = strings.TrimSuffix(s, "/") + "/" + subPath
								}
							}
						}
						if ok {
							var target string
							if s, ok := v.(string); ok {
								target = s
							} else if obj, ok := v.(npm.JSONObject); ok {
								target = ctx.resolveConditionExportEntry(obj, pkgJson.Type).main
							}
							if target != "" {
								specifier = normalizeImportSpecifier(target)
								if isRelPathSpecifier(specifier) {
									specifier = ctx.esmPath.PkgName + "/" + strings.TrimPrefix(specifier, "./")
								}
							}
						}
					}

					// resolve specifier using the `browser` field of package.json
					if !isRelPathSpecifier(specifier) && len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
						name, ok := pkgJson.Browser[specifier]
						if !ok && strings.HasPrefix(specifier, "node:") {
							name, ok = pkgJson.Browser[specifier[5:]]
						}
						if ok {
							if name == "" {
								return esbuild.OnResolveResult{
									Path:      args.Path,
									Namespace: "browser-exclude",
								}, nil
							}
							specifier = name
						}
					}

					if args.Kind == esbuild.ResolveJSRequireCall && !ctx.externalAll && pkgJson.OptionalPeers.Len() > 0 && strings.HasPrefix(args.Importer, ctx.getPkgFullPath("")+"/") {
						pkgName := toPackageName(specifier)
						if pkgJson.OptionalPeers.Has(pkgName) && pkgJson.Dependencies[pkgName] == "" && ctx.args.Deps[pkgName] == "" && !isExternalModule(specifier, ctx.args.External) {
							return esbuild.OnResolveResult{Path: specifier, Namespace: "optional-peer"}, nil
						}
					}

					// nodejs builtin module
					if isNodeBuiltinSpecifier(specifier) {
						externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							SideEffects: sideEffects,
							External:    true,
						}, nil
					}

					var filename string
					if strings.HasPrefix(specifier, "/") {
						filename = specifier
						specifier = strings.TrimPrefix(filename, path.Join(ctx.wd, "node_modules")+"/")
					} else if isRelPathSpecifier(specifier) && args.ResolveDir != "" {
						filename = path.Join(args.ResolveDir, specifier)
						specifier = strings.TrimPrefix(filename, path.Join(ctx.wd, "node_modules")+"/")
					} else {
						filename = path.Join(ctx.wd, "node_modules", specifier)
						if !ctx.externalAll && !isExternalModule(specifier, ctx.args.External) {
							pkgDir := ctx.getPkgFullPath("")
							for dir := args.ResolveDir; dir == pkgDir || strings.HasPrefix(dir, pkgDir+"/"); dir = path.Dir(dir) {
								if path.Base(dir) != "node_modules" && existsDir(path.Join(dir, "node_modules")) {
									resolved := build.Resolve(specifier, esbuild.ResolveOptions{
										Importer: args.Importer, Namespace: args.Namespace, ResolveDir: args.ResolveDir,
										Kind: args.Kind, With: args.With, PluginData: nativeResolve,
									})
									if len(resolved.Errors) == 0 && strings.HasPrefix(resolved.Path, pkgDir+"/") {
										filename = resolved.Path
										specifier = strings.TrimPrefix(filename, path.Join(ctx.wd, "node_modules")+"/")
									}
									break
								}
							}
						}
					}

					// check if the filename is within the working directory
					if !strings.HasPrefix(filename, ctx.wd+string(os.PathSeparator)) {
						return esbuild.OnResolveResult{}, fmt.Errorf("could not resolve module %s", specifier)
					}

					// node native modules do not work via http import
					if strings.HasSuffix(filename, ".node") && existsFile(filename) {
						return esbuild.OnResolveResult{
							Path:     fmt.Sprintf("/error.js?type=unsupported-node-native-module&name=%s&importer=%s", path.Base(args.Path), ctx.esmPath.String()),
							External: true,
						}, nil
					}

					// externalize top-level module
					// e.g. "react/jsx-runtime" imports "react"
					// Also handles scoped fork self-reference: "@scope/three" importing "three"
					isSelfRef := specifier == ctx.esmPath.PkgName
					if !isSelfRef && strings.HasPrefix(ctx.esmPath.PkgName, "@") {
						_, baseName := utils.SplitByFirstByte(ctx.esmPath.PkgName[1:], '/')
						if specifier == baseName {
							_, inDeps := pkgJson.Dependencies[baseName]
							_, inPeerDeps := pkgJson.PeerDependencies[baseName]
							if !inDeps && !inPeerDeps {
								isSelfRef = true
							}
						}
					}
					if ctx.esmPath.SubPath != "" && isSelfRef && ctx.bundleMode != BundleDeps {
						externalPath, sideEffects, err := ctx.resolveExternalModule(ctx.esmPath.PkgName, args.Kind, withTypeJSON, analyzeMode)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						if withTypeJSON {
							externalPath += "?module"
						}
						return esbuild.OnResolveResult{
							Path:        externalPath,
							SideEffects: sideEffects,
							External:    true,
						}, nil
					}

					// bundles all dependencies in `bundle` mode, apart from peerDependencies and `?external` flag
					if !isRelPathSpecifier(specifier) {
						pkgName := toPackageName(specifier)
						if ctx.bundleMode == BundleDeps && !isExternalModule(specifier, ctx.args.External) && !implicitExternal.Has(specifier) {
							_, ok := pkgJson.PeerDependencies[pkgName]
							_, originalPeer := pkgJson.PeerDependencies[toPackageName(originalSpecifier)]
							if !ok && !originalPeer {
								return esbuild.OnResolveResult{}, nil
							}
						}
					}

					// bundle "@babel/runtime/*"
					if (args.Kind != esbuild.ResolveJSDynamicImport && !noBundle) && pkgJson.Name != "@babel/runtime" && pkgJson.Name != "@swc/helpers" && (strings.HasPrefix(specifier, "@babel/runtime/") || strings.Contains(args.Importer, "/@babel/runtime/") || strings.HasPrefix(specifier, "@swc/helpers/") || strings.Contains(args.Importer, "/@swc/helpers/")) {
						return esbuild.OnResolveResult{}, nil
					}

					pkgName := ctx.esmPath.PkgName
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
								PkgName:    ctx.esmPath.PkgName,
								PkgVersion: ctx.esmPath.PkgVersion,
								SubPath:    stripEntryModuleExt(subPath),
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
									externalPath, sideEffects, err := ctx.resolveExternalModule(path, args.Kind, withTypeJSON, analyzeMode)
									if err != nil {
										return esbuild.OnResolveResult{}, err
									}
									return esbuild.OnResolveResult{
										Path:        externalPath,
										SideEffects: sideEffects,
										External:    true,
									}, nil
								}
								modulePath = path
							}
						}

						var exportAs string

						// split modules based on the `exports` field of package.json
						if exports := pkgJson.Exports; exports.Len() > 0 {
							for _, exportName := range exports.Keys() {
								v, _ := exports.Get(exportName)
								if exportName == "." || (strings.HasPrefix(exportName, "./") && !strings.ContainsRune(exportName, '*')) {
									match := false
									if s, ok := v.(string); ok && stripModuleExt(s) == stripModuleExt(modulePath) {
										// exports: "./foo": "./foo.js"
										match = true
									} else if m, ok := v.(npm.JSONObject); ok {
										exportEntry := ctx.resolveConditionExportEntry(m, pkgJson.Type)
										match = stripModuleExt(exportEntry.main) == stripModuleExt(modulePath)
										if !match && path.Join(pkgJson.Name, stripModuleExt(exportName)) == entrySpecifier {
											for _, p := range getExportConditionPaths(m) {
												if stripModuleExt(p) == stripModuleExt(modulePath) {
													match = true
													break
												}
											}
										}
									}
									if match {
										exportAs = path.Join(pkgJson.Name, stripModuleExt(exportName))
										if exportAs != entrySpecifier && exportAs != entrySpecifier+"/index" {
											externalPath, sideEffects, err := ctx.resolveExternalModule(path.Join(pkgJson.Name, exportName), args.Kind, withTypeJSON, analyzeMode)
											if err != nil {
												return esbuild.OnResolveResult{}, err
											}
											return esbuild.OnResolveResult{
												Path:        externalPath,
												SideEffects: sideEffects,
												External:    true,
											}, nil
										}
									}
								}
							}
						}

						if len(args.With) > 0 && args.With["type"] == "css" {
							return esbuild.OnResolveResult{
								Path:        "/" + ctx.esmPath.PackageId() + utils.NormalizePathname(modulePath) + "?module",
								External:    true,
								SideEffects: esbuild.SideEffectsFalse,
							}, nil
						}

						resolvedFilename := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, modulePath)
						// check if the filename is within the working directory
						if !strings.HasPrefix(resolvedFilename, ctx.wd+string(os.PathSeparator)) {
							return esbuild.OnResolveResult{}, fmt.Errorf("could not resolve module %s", specifier)
						}

						// externalize sideEffects modules in subpath builds so they can be shared
						// across entries (e.g. ol/proj and ol/proj/proj4 share proj.js)
						if ctx.esmPath.SubPath != "" && pkgJson.SideEffects.Len() > 0 {
							shortPath := strings.TrimPrefix(modulePath, "./")
							if pkgJson.SideEffects.Has(modulePath) || pkgJson.SideEffects.Has(shortPath) {
								externalPath, sideEffects, err := ctx.resolveExternalModule(path.Join(pkgJson.Name, stripModuleExt(shortPath)), args.Kind, withTypeJSON, analyzeMode)
								if err != nil {
									return esbuild.OnResolveResult{}, err
								}
								return esbuild.OnResolveResult{
									Path:        externalPath,
									SideEffects: sideEffects,
									External:    true,
								}, nil
							}
						}

						// bundled export stars lose names when their source is external
						if entry.module && exportAs == "" && args.Kind == esbuild.ResolveJSImportStatement && args.PluginData != probeResolve {
							if external, _ := hasExternalExportStar(resolvedFilename, nil); external {
								externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
								if err != nil {
									return esbuild.OnResolveResult{}, err
								}
								return esbuild.OnResolveResult{Path: externalPath, SideEffects: sideEffects, External: true}, nil
							}
						}

						// bundle the sub module if:
						// - it's the entry point
						// - it's not a conditional export
						// - it's not a dynamic import and the `?bundle=false` flag is not present
						// - it's not in the `splitting` list
						isDynamicImport := args.Kind == esbuild.ResolveJSDynamicImport && !bundledDynamicImports.Has(resolvedFilename)
						bundleInternalModule := exportAs == "" && !isDynamicImport && ctx.shouldBundleInternalModules()
						if modulePath == entry.main || exportAs == entrySpecifier || (!isDynamicImport && !noBundle) || bundleInternalModule {
							suffix := ""
							if strings.HasSuffix(resolvedFilename, ".wasm?module") {
								resolvedFilename = strings.TrimSuffix(resolvedFilename, "?module")
								suffix = "?module"
							}
							if existsFile(resolvedFilename) {
								if strings.HasSuffix(resolvedFilename, ".css") {
									return esbuild.OnResolveResult{Path: resolvedFilename}, nil
								}
								pkgDir := path.Join(ctx.wd, "node_modules", pkgName)
								short := strings.TrimPrefix(resolvedFilename, pkgDir)[1:]
								probe := args.PluginData == probeResolve
								if analyzeMode && !probe && resolvedFilename != entryModuleFilename && strings.HasPrefix(args.Importer, pkgDir) {
									includesMu.Lock()
									includes = append(includes, [2]string{short, strings.TrimPrefix(args.Importer, pkgDir)[1:]})
									includesMu.Unlock()
								}
								if !analyzeMode && ctx.splitting != nil && ctx.splitting.Has(short) {
									specifier = pkgJson.Name + utils.NormalizePathname(stripEntryModuleExt(short))
									externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, false)
									if err != nil {
										return esbuild.OnResolveResult{}, err
									}
									return esbuild.OnResolveResult{
										Path:        externalPath,
										SideEffects: sideEffects,
										External:    true,
									}, nil
								}
								if !probe {
									bundledModules.Add(resolvedFilename + suffix)
								}
								if strings.HasSuffix(resolvedFilename, ".wasm") {
									return esbuild.OnResolveResult{
										Path:      resolvedFilename,
										Namespace: "wasm",
										Suffix:    suffix,
									}, nil
								}
								// transfrom svelte component
								if strings.HasSuffix(resolvedFilename, ".svelte") {
									return esbuild.OnResolveResult{
										Path:      resolvedFilename,
										Namespace: "svelte",
									}, nil
								}
								// transfrom Vue SFC
								if strings.HasSuffix(resolvedFilename, ".vue") {
									return esbuild.OnResolveResult{
										Path:      resolvedFilename,
										Namespace: "vue",
									}, nil
								}
								return esbuild.OnResolveResult{Path: resolvedFilename}, nil
							}
							// otherwise, let esbuild to handle it
							return esbuild.OnResolveResult{}, nil
						}
						if isDynamicImport {
							dynamicImports.Add(resolvedFilename)
						}
					}

					// use `npm:` specifier for `denonext` target if the specifier is in the `forceNpmSpecifiers` list
					if forceNpmSpecifiers[specifier] && ctx.target == "denonext" {
						version := ""
						pkgName, _, subPath := splitEsmPath(specifier)
						if pkgName == ctx.esmPath.PkgName {
							version = ctx.esmPath.PkgVersion
						} else if v, ok := pkgJson.Dependencies[pkgName]; ok && npm.IsExactVersion(v) {
							version = v
						} else if v, ok := pkgJson.PeerDependencies[pkgName]; ok && npm.IsExactVersion(v) {
							version = v
						}
						p := pkgName
						if version != "" {
							p += "@" + version
						}
						if len(subPath) > 0 {
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
							ctx.importsLock.Lock()
							ctx.cjsRequires = append(ctx.cjsRequires, [3]string{
								"npm:" + specifier,
								string(replacement.IIFE),
								"",
							})
							ctx.importsLock.Unlock()
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

					// check if the specifier is in the `imports` field and is a http module
					if ctx.externalAll && isHttpSpecifier(specifier) {
						return esbuild.OnResolveResult{
							Path:     args.Path,
							External: true,
						}, nil
					}

					// dynamic external
					externalPath, sideEffects, err := ctx.resolveExternalModule(specifier, args.Kind, withTypeJSON, analyzeMode)
					if err != nil {
						return esbuild.OnResolveResult{}, err
					}
					if (specifier == "solid-js/jsx-runtime" || specifier == "solid-js/jsx-dev-runtime") && toPackageName(externalPath) == "solid-js" {
						solidJSXRuntimes.Add(externalPath)
					}
					return esbuild.OnResolveResult{
						Path:        externalPath,
						External:    true,
						SideEffects: sideEffects,
					}, nil
				},
			)

			build.OnLoad(
				esbuild.OnLoadOptions{Filter: `/node_modules/benchmark/benchmark\.js$`, Namespace: "file"},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					code, err := os.ReadFile(args.Path)
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					contents := strings.Replace(string(code),
						"var freeRequire = typeof require == 'function' && require;",
						"var freeRequire = id => id === 'lodash' ? require('lodash') : id === 'platform' ? require('platform') : undefined;", 1)
					if err := checkSourceComplexity(contents); err != nil {
						return esbuild.OnLoadResult{}, err
					}
					return esbuild.OnLoadResult{Contents: &contents, ResolveDir: path.Dir(args.Path), Loader: esbuild.LoaderJS}, nil
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

			build.OnLoad(
				esbuild.OnLoadOptions{Filter: ".*", Namespace: "optional-peer"},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					contents := fmt.Sprintf(`throw Object.assign(new Error(%q), { code: "MODULE_NOT_FOUND" });`, "Cannot find module '"+args.Path+"'")
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
					code := fmt.Sprintf("Uint8Array.from(atob('%s'), c => c.charCodeAt(0))", wasm64)
					if args.Suffix == "?module" {
						code = "new WebAssembly.Module(" + code + ")"
					}
					code = "export default " + code
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
					if version, ok := ctx.args.Deps["svelte"]; ok {
						svelteVersion = version
					} else if version, ok := ctx.pkgJson.Dependencies["svelte"]; ok {
						svelteVersion = version
					} else if version, ok := ctx.pkgJson.PeerDependencies["svelte"]; ok {
						svelteVersion = version
					}
					if !npm.IsExactVersion(svelteVersion) {
						info, err := ctx.npmrc.getPackageInfoContext(ctx.Context(), "svelte", svelteVersion)
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to get svelte package info")
						}
						svelteVersion = info.Version
					}
					if semverLessThan(svelteVersion, "4.0.0") {
						return esbuild.OnLoadResult{}, errors.New("svelte version must be greater than 4.0.0")
					}
					out, err := transformSvelte(ctx.Context(), ctx.npmrc, svelteVersion, ctx.esmPath.String(), string(code))
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					if err := checkSourceComplexity(out.Code); err != nil {
						return esbuild.OnLoadResult{}, err
					}
					return esbuild.OnLoadResult{
						Contents:   &out.Code,
						ResolveDir: path.Dir(args.Path),
						Loader:     esbuild.LoaderJS,
					}, nil
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
					if version, ok := ctx.args.Deps["vue"]; ok {
						vueVersion = version
					} else if version, ok := ctx.pkgJson.Dependencies["vue"]; ok {
						vueVersion = version
					} else if version, ok := ctx.pkgJson.PeerDependencies["vue"]; ok {
						vueVersion = version
					}
					if !npm.IsExactVersion(vueVersion) {
						info, err := ctx.npmrc.getPackageInfoContext(ctx.Context(), "vue", vueVersion)
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to get vue package info")
						}
						vueVersion = info.Version
					}
					if semverLessThan(vueVersion, "3.0.0") {
						return esbuild.OnLoadResult{}, errors.New("vue version must be greater than 3.0.0")
					}
					out, err := transformVue(ctx.Context(), ctx.npmrc, vueVersion, ctx.esmPath.String(), string(code))
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}
					if err := checkSourceComplexity(out.Code); err != nil {
						return esbuild.OnLoadResult{}, err
					}
					if out.Lang == "ts" {
						return esbuild.OnLoadResult{
							Contents:   &out.Code,
							ResolveDir: path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName),
							Loader:     esbuild.LoaderTS,
						}, nil
					}
					return esbuild.OnLoadResult{
						Contents:   &out.Code,
						ResolveDir: path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName),
						Loader:     esbuild.LoaderJS,
					}, nil
				},
			)
		},
	}

	if err = ctx.checkCanceled(); err != nil {
		return
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
		if ctx.isBrowserTarget() && safeReserveProcessPackages[ctx.esmPath.PkgName] {
			delete(define, "process")
		}
		for k, v := range define {
			if k == "process" {
				continue
			}
			define["global."+k] = v
			define["globalThis."+k] = v
		}
		define["global"] = "globalThis"
	}
	conditions := ctx.args.Conditions
	if ctx.dev {
		conditions = append(conditions, "development")
	}
	if ctx.isDenoTarget() {
		conditions = append(conditions, "deno")
	} else if ctx.target == "node" {
		conditions = append(conditions, "node")
	}
	minify := config.Minify
	if ctx.dev {
		// disable minification for development build
		minify = false
	}
	keepNames := ctx.args.KeepNames
	ignoreAnnotations := ctx.args.IgnoreAnnotations
	if esmsh := ctx.pkgJson.Esmsh; esmsh != nil {
		if v, ok := esmsh["keepNames"]; ok {
			if b, ok := v.(bool); ok {
				keepNames = b
			}
		}
		if v, ok := esmsh["ignoreAnnotations"]; ok {
			if b, ok := v.(bool); ok {
				ignoreAnnotations = b
			}
		}
	}
	options := esbuild.BuildOptions{
		AbsWorkingDir:     ctx.wd,
		Alias:             ctx.args.Alias,
		PreserveSymlinks:  true,
		Format:            esbuild.FormatESModule,
		Target:            targets[ctx.target],
		Platform:          esbuild.PlatformBrowser,
		Define:            define,
		Supported:         supported,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   "react",
		Bundle:            true,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		KeepNames:         keepNames,
		IgnoreAnnotations: ignoreAnnotations,
		Conditions:        conditions,
		Loader:            loaders,
		Plugins:           []esbuild.Plugin{esmifyPlugin, sourceLimitPlugin},
		Outdir:            "/esbuild",
		Write:             false,
	}
	if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
	} else {
		if err = checkSourceComplexity(stdin.Contents); err != nil {
			return
		}
		options.Stdin = &stdin
	}
	if ctx.isDenoTarget() {
		options.Platform = esbuild.PlatformNeutral
		options.MainFields = []string{"module", "main"}
	} else if ctx.target == "node" {
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
			switch pkgName {
			case "mono-jsx":
				options.JSXImportSource += "/dom"
			case "hono":
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
	includes = nil
	ctx.esmImports = nil
	ctx.cjsRequires = nil
	res := esbCtx.Rebuild()
	if len(res.Errors) > 0 {
		retry := false
		for _, e := range res.Errors {
			// mark the missing module as external to exclude it from the bundle
			msg := e.Text
			if strings.HasPrefix(msg, "Could not resolve \"") {
				// current module can not be marked as an external
				if strings.HasPrefix(msg, fmt.Sprintf("Could not resolve \"%s\"", entrySpecifier)) {
					err = fmt.Errorf("could not resolve \"%s\"", entrySpecifier)
					return
				}
				name := strings.Split(msg, "\"")[1]
				// a relative path import can not be externalized, report it as a build error
				if isRelPathSpecifier(name) || strings.HasPrefix(name, "/") {
					if loc := e.Location; loc != nil {
						err = fmt.Errorf("could not resolve \"%s\" (imported by %s)", name, loc.File)
					} else {
						err = fmt.Errorf("could not resolve \"%s\"", name)
					}
					return
				}
				if !implicitExternal.Has(name) {
					ctx.logger.Warnf("build(%s): implicit external '%s'", ctx.Path(), name)
					implicitExternal.Add(name)
					retry = true
				}
			} else if strings.HasPrefix(msg, "No matching export in \"") {
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
							retry = true
						}
					}
				}
			}
		}
		if retry {
			goto REBUILD
		}
		err = errors.New("esbuild: " + res.Errors[0].Text)
		return
	}

	// A separate dynamic build duplicates modules already bundled by static imports.
	rebuild := false
	for _, filename := range dynamicImports.Values() {
		if bundledModules.Has(filename) && !bundledDynamicImports.Has(filename) {
			bundledDynamicImports.Add(filename)
			rebuild = true
		}
	}
	if rebuild {
		goto REBUILD
	}

	if analyzeMode {
		return
	}

	for _, w := range res.Warnings {
		ctx.logger.Warnf("esbuild(%s): %s", ctx.Path(), w.Text)
	}

	imports := set.New[string]()

	for _, file := range res.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			if err = validateSolidJSX(file.Contents, solidJSXRuntimes); err != nil {
				return
			}
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
			if ctx.esmPath.SubPath != "" {
				header.WriteByte('/')
				header.WriteString(ctx.esmPath.SubPath)
			}
			header.WriteString(" */\n")

			jsContent := file.Contents

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
					if ctx.args.External.Has("node:process") {
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
							if excluded {
								header.WriteString(`const __Process$ = globalThis.process;`)
								header.WriteByte('\n')
							} else {
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
					if ctx.args.External.Has("node:buffer") {
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
						if excluded {
							header.WriteString(`const __Buffer$ = globalThis.Buffer;`)
							header.WriteByte('\n')
						} else {
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

			// check cjs requires
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
					if bytes.Contains(jsContent, fmt.Appendf(nil, `("%s").default`, specifier)) {
						isEsModule[i] = true
						continue
					}
					// `var mod = require("module");...;mod()` -> cjs
					// `var mod = require("module");...;mod.default` -> es module
					if j := bytes.Index(jsContent, fmt.Appendf(nil, `("%s")`, specifier)); j >= 0 {
						if ret := regexpVarDecl.FindSubmatch(jsContent[:j]); len(ret) == 2 {
							if ret := regexp.MustCompile(`[^\w$]` + regexp.QuoteMeta(string(ret[1])) + `(\(|\.default[^\w$=])`).FindSubmatch(jsContent); len(ret) == 2 {
								isEsModule[i] = string(ret[1]) != "("
								continue
							}
						}
					}
					if !isRelPathSpecifier(specifier) && !isNodeBuiltinSpecifier(specifier) && !strings.HasPrefix(specifier, "npm:") && !isJsonModuleSpecifier(specifier) {
						dep, pkgJson, err := ctx.resolveDependency(specifier, false)
						if err == nil {
							if pkgJson.Type == "module" || pkgJson.Module != "" {
								isEsModule[i] = true
							} else {
								b := &BuildContext{
									npmrc:       ctx.npmrc,
									logger:      ctx.logger,
									metaDB:      ctx.metaDB,
									storage:     ctx.storage,
									esmPath:     dep,
									args:        ctx.args,
									externalAll: ctx.externalAll,
									target:      ctx.target,
									dev:         ctx.dev,
									ctx:         ctx.ctx,
								}
								err = b.install()
								if err == nil {
									entry := b.resolveEntry(dep)
									if !entry.module {
										ret, cjsNamedExports, _, e := b.lexer(&entry)
										if e == nil && ret.CJS && slices.Contains(cjsNamedExports, "__esModule") {
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

			// check esm imports
			for _, a := range ctx.esmImports {
				resolvedPathFull, resolvedPath := a[0], a[1]
				if bytes.Contains(jsContent, fmt.Appendf(nil, `"%s"`, resolvedPath)) {
					imports.Add(resolvedPathFull)
				}
			}

			// to fix the source map
			ctx.smOffset += strings.Count(header.String(), "\n")

			// apply rewrites
			jsContent, dropSourceMap := ctx.rewriteJS(jsContent)

			finalJS := &bytes.Buffer{}
			io.Copy(finalJS, header)
			finalJS.Write(jsContent)

			if ctx.bundleMode == BundleDeps && !ctx.externalAll && ctx.isBrowserTarget() && bytes.Contains(finalJS.Bytes(), []byte(`"/node/`)) {
				banner, _, _ := bytes.Cut(finalJS.Bytes(), []byte{'\n'})
				options := esbuild.BuildOptions{
					Bundle: true, Format: esbuild.FormatESModule, Target: targets[ctx.target],
					MinifyWhitespace: minify, MinifyIdentifiers: minify, MinifySyntax: minify, KeepNames: keepNames,
					Supported: supported,
					Outfile:   file.Path,
					Banner:    map[string]string{"js": string(banner)},
					Plugins: []esbuild.Plugin{{Name: "bundle-node-runtime", Setup: func(build esbuild.PluginBuild) {
						build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
							specifier := args.Path
							if args.Namespace == "node-runtime" && isRelPathSpecifier(specifier) {
								specifier = path.Join(path.Dir(args.Importer), specifier)
							}
							if strings.HasPrefix(specifier, "/node/") {
								return esbuild.OnResolveResult{Path: specifier, Namespace: "node-runtime"}, nil
							}
							return esbuild.OnResolveResult{Path: specifier, External: true}, nil
						})
						build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "node-runtime"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
							js, ok := getNodeRuntimeJS(strings.TrimPrefix(args.Path, "/node/"))
							if !ok {
								return esbuild.OnLoadResult{}, fmt.Errorf("node runtime module not found: %s", args.Path)
							}
							code := string(js)
							return esbuild.OnLoadResult{Contents: &code, Loader: esbuild.LoaderJS}, nil
						})
					}}},
				}
				sourceMapIndex := -1
				if config.SourceMap && !dropSourceMap {
					for i, output := range res.OutputFiles {
						if output.Path == file.Path+".map" {
							var sourceMap map[string]any
							if err = json.Unmarshal(output.Contents, &sourceMap); err != nil {
								return
							}
							if mappings, ok := sourceMap["mappings"].(string); ok {
								sourceMap["mappings"] = strings.Repeat(";", ctx.smOffset) + mappings
							}
							data, e := json.Marshal(sourceMap)
							if e != nil {
								err = e
								return
							}
							finalJS.WriteString("\n//# sourceMappingURL=data:application/json;base64,")
							finalJS.WriteString(base64.StdEncoding.EncodeToString(data))
							options.Sourcemap = esbuild.SourceMapExternal
							sourceMapIndex = i
							break
						}
					}
				}
				options.Stdin = &esbuild.StdinOptions{Contents: finalJS.String(), Sourcefile: file.Path, Loader: esbuild.LoaderJS}
				bundled := esbuild.Build(options)
				if len(bundled.Errors) > 0 {
					err = errors.New("esbuild: " + bundled.Errors[0].Text)
					return
				}
				for _, output := range bundled.OutputFiles {
					if output.Path == file.Path {
						finalJS.Reset()
						finalJS.Write(output.Contents)
					} else if sourceMapIndex >= 0 && output.Path == file.Path+".map" {
						res.OutputFiles[sourceMapIndex].Contents = output.Contents
					}
				}
				ctx.smOffset = 0
				for _, specifier := range imports.Values() {
					if strings.HasPrefix(specifier, "/node/") {
						imports.Remove(specifier)
					}
				}
			}

			// check if the package is deprecated
			if !ctx.esmPath.GhPrefix && !ctx.esmPath.PrPrefix {
				deprecated, _ := ctx.npmrc.isDeprecated(ctx.pkgJson.Name, ctx.pkgJson.Version)
				if deprecated != "" {
					fmt.Fprintf(finalJS, `console.warn("%%c[esm.sh]%%c %%cdeprecated%%c %s@%s: " + %s, "color:grey", "", "color:red", "");%s`, ctx.esmPath.PkgName, ctx.esmPath.PkgVersion, utils.MustEncodeJSON(deprecated), "\n")
				}
			}

			// add sourcemap Url
			if config.SourceMap && !dropSourceMap {
				finalJS.WriteString("//# sourceMappingURL=")
				finalJS.WriteString(path.Base(ctx.Path()))
				finalJS.WriteString(".map")
			}

			var stat storage.Stat
			stat, err = ctx.storage.Stat(ctx.getSavePath())
			if err == nil && stat.Size() > 0 {
				ctx.logger.Infof("build(%s): file already exists in the storage, skip it", ctx.Path())
				continue
			}
			if err != nil && err != storage.ErrNotFound {
				ctx.logger.Errorf("storage.stat(%s): %v", ctx.getSavePath(), err)
				err = errors.New("storage(stat): " + err.Error())
				return
			}
			sha := sha512.New384()
			err = ctx.storage.Put(ctx.getSavePath(), storage.TeeReader(finalJS, sha))
			if err != nil {
				ctx.logger.Errorf("storage.put(%s): %v", ctx.getSavePath(), err)
				err = errors.New("storage(put): " + err.Error())
				return
			}
			meta.Integrity = "sha384-" + base64.StdEncoding.EncodeToString(sha.Sum(nil))
		}
	}

	for _, file := range res.OutputFiles {
		if strings.HasSuffix(file.Path, ".css") {
			savePath := ctx.getSavePath()
			savePath = strings.TrimSuffix(savePath, path.Ext(savePath)) + ".css"
			err = ctx.storage.Put(savePath, bytes.NewReader(file.Contents))
			if err != nil {
				ctx.logger.Errorf("storage.put(%s): %v", savePath, err)
				err = errors.New("storage(put): " + err.Error())
				return
			}
			meta.CSSInJS = true
		} else if config.SourceMap && strings.HasSuffix(file.Path, ".js.map") {
			var sourceMap map[string]any
			if json.Unmarshal(file.Contents, &sourceMap) == nil {
				if mapping, ok := sourceMap["mappings"].(string); ok {
					fixedMapping := make([]byte, ctx.smOffset+len(mapping))
					for i := 0; i < ctx.smOffset; i++ {
						fixedMapping[i] = ';'
					}
					copy(fixedMapping[ctx.smOffset:], mapping)
					sourceMap["mappings"] = string(fixedMapping)
				}
				buf := &bytes.Buffer{}
				if json.NewEncoder(buf).Encode(sourceMap) == nil {
					err = ctx.storage.Put(ctx.getSavePath()+".map", buf)
					if err != nil {
						ctx.logger.Errorf("storage.put(%s): %v", ctx.getSavePath()+".map", err)
						err = errors.New("storage(put): " + err.Error())
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
	meta.Dts, err = ctx.resolveDTS(entry)
	return
}

func (ctx *BuildContext) buildTypes() (ret *BuildMeta, err error) {
	// install the package
	ctx.status.Store("install")
	err = ctx.install()
	if err != nil {
		return
	}
	if err = ctx.checkCanceled(); err != nil {
		return
	}

	var dts string
	if endsWith(ctx.esmPath.SubPath, ".ts", ".mts", ".tsx", ".cts") {
		dts = "./" + ctx.esmPath.SubPath
	} else {
		entry := ctx.resolveEntry(ctx.esmPath)
		if entry.types == "" {
			err = errors.New("types not found")
			return
		}
		dts = entry.types
	}

	ctx.status.Store("build")
	err = ctx.transformDTS(dts)
	if err != nil {
		return
	}

	ret = &BuildMeta{Dts: "/" + ctx.esmPath.PackageId() + "/" + ctx.getBuildArgsPrefix(true) + strings.TrimPrefix(dts, "./")}
	return
}

func (ctx *BuildContext) install() (err error) {
	if ctx.wd == "" || ctx.pkgJson == nil {
		p, err := ctx.npmrc.installPackageContext(ctx.Context(), ctx.esmPath.Package())
		if err != nil {
			return err
		}

		if ctx.esmPath.GhPrefix || ctx.esmPath.PrPrefix {
			// if the name in package.json is not the same as the repository name
			if p.Name != ctx.esmPath.PkgName {
				p.PkgName = p.Name
				p.Name = ctx.esmPath.PkgName
			}
			p.Version = ctx.esmPath.PkgVersion
		} else {
			p.Version = strings.TrimPrefix(p.Version, "v")
		}

		// Check if `SubPath` resolves to the package root
		if subPath := ctx.esmPath.SubPath; subPath != "" && ctx.target != "types" {
			isMainModule := false
			check := func(s string) bool {
				return isMainModule || (s != "" && subPath == utils.NormalizePathname(stripModuleExt(s))[1:])
			}
			if p.Exports.Len() > 0 {
				if v, ok := p.Exports.Get("."); ok {
					if s, ok := v.(string); ok {
						// exports: { ".": "./index.js" }
						isMainModule = check(s)
					} else if obj, ok := v.(npm.JSONObject); ok {
						// exports: { ".": { "require": "./cjs/index.js", "import": "./esm/index.js" } }
						// exports: { ".": { "node": { "require": "./cjs/index.js", "import": "./esm/index.js" } } }
						// ...
						paths := getExportConditionPaths(obj)
						isMainModule = slices.ContainsFunc(paths, check)
					}
				} else if !strings.HasPrefix(p.Exports.Keys()[0], ".") {
					isMainModule = slices.ContainsFunc(getExportConditionPaths(p.Exports), check)
				}
			} else {
				isMainModule = (p.Module != "" && check(p.Module)) || (p.Main != "" && check(p.Main))
			}
			if isMainModule {
				ctx.esmPath.SubPath = ""
				ctx.path = ""
			}
		}

		ctx.wd = path.Join(ctx.npmrc.StoreDir(), ctx.esmPath.PackageId())
		if len(ctx.args.Deps) > 0 {
			pkgDir := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName)
			ctx.wd = path.Join(ctx.wd, fmt.Sprintf("deps-%x", sha512.Sum512_256([]byte(encodeBuildArgs(BuildArgs{Deps: ctx.args.Deps}, false)))))
			linkDir := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName)
			if err := ensureDir(path.Dir(linkDir)); err != nil {
				return err
			}
			if err := os.Symlink(pkgDir, linkDir); err != nil && !os.IsExist(err) {
				return err
			}
		}
		ctx.pkgJson = p
	}

	// - install dependencies in `BundleDeps` mode
	// - install '@babel/runtime' and '@swc/helpers' if they are present in the dependencies in `BundleDefault` mode
	switch ctx.bundleMode {
	case BundleDeps:
		err = ctx.npmrc.installDependenciesContext(ctx.Context(), ctx.wd, ctx.pkgJson, false, ctx.args.Deps)
	case BundleDefault:
		if v, ok := ctx.pkgJson.Dependencies["@babel/runtime"]; ok {
			err = ctx.npmrc.installDependenciesContext(ctx.Context(), ctx.wd, &npm.PackageJSON{Dependencies: map[string]string{"@babel/runtime": v}}, false, ctx.args.Deps)
		}
		if err == nil {
			if v, ok := ctx.pkgJson.Dependencies["@swc/helpers"]; ok {
				err = ctx.npmrc.installDependenciesContext(ctx.Context(), ctx.wd, &npm.PackageJSON{Dependencies: map[string]string{"@swc/helpers": v}}, false, ctx.args.Deps)
			}
		}
	}
	if err != nil {
		return
	}
	return
}

// internal modules of an ESM package are bundled into every entry point even when `shouldBundle`
// is false, so the analysis must run for them, or module-level state is duplicated per entry.
func (ctx *BuildContext) shouldBundleInternalModules() bool {
	return ctx.bundleMode != BundleFalse && ctx.pkgJson.Type == "module"
}

func (ctx *BuildContext) shouldBundle() bool {
	if ctx.bundleMode == BundleFalse || ctx.pkgJson.SideEffects.Len() > 0 {
		return false
	}
	if ctx.pkgJson.Esmsh != nil {
		if v, ok := ctx.pkgJson.Esmsh["bundle"]; ok {
			if b, ok := v.(bool); ok && !b {
				return false
			}
		}
	}
	return true
}
