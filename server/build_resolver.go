package server

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

// BuildEntry represents the build entrypoints of a module
type BuildEntry struct {
	main   string
	module bool
	types  string
}

func (entry *BuildEntry) isEmpty() bool {
	return entry.main == "" && entry.types == ""
}

func (entry *BuildEntry) update(main string, module bool) {
	entry.main = main
	entry.module = module
}

func (ctx *BuildContext) Path() string {
	if ctx.path != "" {
		return ctx.path
	}

	asteriskPrefix := ""
	if ctx.externalAll {
		asteriskPrefix = "*"
	}

	esmPath := ctx.esmPath
	if ctx.target == "types" {
		if strings.HasSuffix(esmPath.SubPath, ".d.ts") {
			ctx.path = fmt.Sprintf(
				"/%s%s/%s%s",
				asteriskPrefix,
				esmPath.Name(),
				ctx.getBuildArgsPrefix(true),
				esmPath.SubPath,
			)
		} else {
			ctx.path = "/" + esmPath.Specifier()
		}
		return ctx.path
	}

	name := strings.TrimSuffix(path.Base(esmPath.PkgName), ".js")
	if esmPath.SubModuleName != "" {
		if esmPath.SubModuleName == name {
			// if the sub-module name is same as the package name
			name = "__" + esmPath.SubModuleName
		} else {
			name = esmPath.SubModuleName
		}
		// workaround for es5-ext "../#/.." path
		if esmPath.PkgName == "es5-ext" {
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
		esmPath.Name(),
		ctx.getBuildArgsPrefix(ctx.target == "types"),
		ctx.target,
		name,
	)
	return ctx.path
}

func (ctx *BuildContext) getImportPath(esmPath EsmPath, buildArgsPrefix string, externalAll bool) string {
	asteriskPrefix := ""
	if externalAll {
		asteriskPrefix = "*"
	}
	name := strings.TrimSuffix(path.Base(esmPath.PkgName), ".js")
	if esmPath.SubModuleName != "" {
		if esmPath.SubModuleName == name {
			// if the sub-module name is same as the package name
			name = "__" + esmPath.SubModuleName
		} else {
			name = esmPath.SubModuleName
		}
		// workaround for es5-ext "../#/.." path
		if esmPath.PkgName == "es5-ext" {
			name = strings.ReplaceAll(name, "/#/", "/%23/")
		}
	}
	if ctx.dev {
		name += ".development"
	}
	return fmt.Sprintf(
		"/%s%s/%s%s/%s.mjs",
		asteriskPrefix,
		esmPath.Name(),
		buildArgsPrefix,
		ctx.target,
		name,
	)
}

func (ctx *BuildContext) getSavepath() string {
	return normalizeSavePath(ctx.zoneId, path.Join("esm", ctx.Path()))
}

func (ctx *BuildContext) getBuildArgsPrefix(isDts bool) string {
	if a := encodeBuildArgs(ctx.args, isDts); a != "" {
		return "X-" + a + "/"
	}
	return ""
}

func (ctx *BuildContext) getNodeEnv() string {
	if ctx.dev {
		return "development"
	}
	return "production"
}

func (ctx *BuildContext) isDenoTarget() bool {
	return ctx.target == "deno" || ctx.target == "denonext"
}

func (ctx *BuildContext) isBrowserTarget() bool {
	return strings.HasPrefix(ctx.target, "es")
}

func (ctx *BuildContext) existsPkgFile(fp ...string) bool {
	args := make([]string, 1+len(fp))
	args[0] = ctx.pkgDir
	copy(args[1:], fp)
	return existsFile(path.Join(args...))
}

func (ctx *BuildContext) lookupDep(specifier string, isDts bool) (esm EsmPath, packageJson *PackageJSON, err error) {
	pkgName, version, subpath, _ := splitEsmPath(specifier)
lookup:
	if v, ok := ctx.args.deps[pkgName]; ok {
		packageJson, err = ctx.npmrc.getPackageInfo(pkgName, v)
		if err == nil {
			esm = EsmPath{
				PkgName:       pkgName,
				PkgVersion:    packageJson.Version,
				SubPath:       subpath,
				SubModuleName: stripEntryModuleExt(subpath),
			}
		}
		return
	}

	var raw PackageJSONRaw
	pkgJsonPath := path.Join(ctx.wd, "node_modules", pkgName, "package.json")
	if utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
		esm = EsmPath{
			PkgName:       pkgName,
			PkgVersion:    raw.Version,
			SubPath:       subpath,
			SubModuleName: stripEntryModuleExt(subpath),
		}
		packageJson = raw.ToNpmPackage()
		return
	}

	if version == "" {
		if v, ok := ctx.packageJson.Dependencies[pkgName]; ok {
			if strings.HasPrefix(v, "npm:") {
				pkgName, version, _, _ = splitEsmPath(v[4:])
			} else {
				version = v
			}
		} else if v, ok = ctx.packageJson.PeerDependencies[pkgName]; ok {
			if strings.HasPrefix(v, "npm:") {
				pkgName, version, _, _ = splitEsmPath(v[4:])
			} else {
				version = v
			}
		} else {
			version = "latest"
		}
	}

	packageJson, err = ctx.npmrc.getPackageInfo(pkgName, version)
	if err == nil {
		esm = EsmPath{
			PkgName:       pkgName,
			PkgVersion:    packageJson.Version,
			SubPath:       subpath,
			SubModuleName: stripEntryModuleExt(subpath),
		}
	}
	if err != nil && strings.HasSuffix(err.Error(), " not found") && isDts && !strings.HasPrefix(pkgName, "@types/") {
		pkgName = toTypesPackageName(pkgName)
		goto lookup
	}
	return
}

func (ctx *BuildContext) resolveEntry(esmPath EsmPath) (entry BuildEntry) {
	pkgJson := ctx.packageJson

	// apply the `browser` field if it's a browser target
	if len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
		if entry.main != "" {
			if path, ok := pkgJson.Browser[entry.main]; ok && ctx.existsPkgFile(path) {
				entry.main = normalizeEntryPath(path)
			}
		}
		if esmPath.SubModuleName == "" {
			if path, ok := pkgJson.Browser["."]; ok && ctx.existsPkgFile(path) {
				entry.update(normalizeEntryPath(path), pkgJson.Type == "module" || strings.HasSuffix(path, ".mjs"))
			}
		}
	}

	if esmPath.SubModuleName != "" {
		if endsWith(esmPath.SubPath, ".d.ts", ".d.mts", ".d.cts") {
			entry.types = normalizeEntryPath(esmPath.SubPath)
			return
		}

		if endsWith(esmPath.SubPath, ".jsx", ".ts", ".tsx", ".mts", ".svelte", ".vue") {
			entry.update(normalizeEntryPath(esmPath.SubPath), true)
			return
		}

		subModuleName := esmPath.SubModuleName

		// reslove sub-module using `exports` conditions if exists
		// see https://nodejs.org/api/packages.html#package-entry-points
		if pkgJson.Exports.Len() > 0 {
			exportEntry := BuildEntry{}
			for _, name := range pkgJson.Exports.keys {
				conditions, ok := pkgJson.Exports.values[name]
				if ok {
					if name == "./"+subModuleName || stripEntryModuleExt(name) == "./"+subModuleName {
						if s, ok := conditions.(string); ok {
							/**
							exports: {
								"./lib/foo": "./lib/foo.js"
							}
							*/
							exportEntry.update(s, pkgJson.Type == "module" || strings.HasSuffix(s, ".mjs"))
						} else if om, ok := conditions.(*JsonObject); ok {
							/**
							exports: {
								"./lib/foo": {
									"require": "./lib/foo.js",
									"import": "./esm/foo.js",
									"types": "./types/foo.d.ts"
								}
							}
							*/
							exportEntry = ctx.resolveConditionExportEntry(om, pkgJson.Type)
						}
						break
					} else if diff, ok := matchAsteriskExports(name, subModuleName); ok {
						if s, ok := conditions.(string); ok {
							/**
							exports: {
								"./lib/*": "./dist/lib/*.js",
							}
							*/
							path := strings.ReplaceAll(s, "*", diff)
							if ctx.existsPkgFile(path) {
								exportEntry.update(path, pkgJson.Type == "module" || strings.HasSuffix(path, ".mjs"))
								break
							}
						} else if om, ok := conditions.(*JsonObject); ok {
							/**
							exports: {
								"./lib/*": {
									"require": ".dist/lib/dist/*.js",
									"import": ".dist/lib/esm/*.js",
									"types": ".dist/lib/types/*.d.ts"
								},
							}
							*/
							exportEntry = ctx.resolveConditionExportEntry(resloveAsteriskPathMapping(om, diff), pkgJson.Type)
							ctx.finalizeBuildEntry(&exportEntry)
							if !exportEntry.isEmpty() {
								break
							}
						}
					}
				}
			}
			ctx.finalizeBuildEntry(&exportEntry)
			if exportEntry.main != "" {
				entry.update(exportEntry.main, exportEntry.module)
			}
			if exportEntry.types != "" {
				entry.types = exportEntry.types
			}
		}

		// check if the sub-module is a directory and has a package.json
		var rawInfo PackageJSONRaw
		if utils.ParseJSONFile(path.Join(ctx.pkgDir, subModuleName, "package.json"), &rawInfo) == nil {
			p := rawInfo.ToNpmPackage()
			if entry.main == "" {
				if p.Module != "" {
					entry.update("./"+path.Join(subModuleName, p.Module), true)
				} else if p.Main != "" {
					entry.update("./"+path.Join(subModuleName, p.Main), p.Type == "module")
				}
			}
			if entry.types == "" {
				if p.Types != "" {
					entry.types = "./" + path.Join(subModuleName, p.Types)
				} else if p.Typings != "" {
					entry.types = "./" + path.Join(subModuleName, p.Typings)
				}
			}
		}

		// get rid of invalid main and types, and lookup the entry from file system
		ctx.finalizeBuildEntry(&entry)

		// lookup esm entry from the sub-module directory if it's not defined in `package.json`
		if entry.main == "" {
			if ctx.existsPkgFile(subModuleName + ".mjs") {
				entry.update("./"+subModuleName+".mjs", true)
			} else if ctx.existsPkgFile(subModuleName, "index.mjs") {
				entry.update("./"+subModuleName+"/index.mjs", true)
			} else if pkgJson.Type == "module" {
				if ctx.existsPkgFile(subModuleName + ".js") {
					entry.update("./"+subModuleName+".js", true)
				} else if ctx.existsPkgFile(subModuleName, "index.js") {
					entry.update("./"+subModuleName+"/index.js", true)
				}
			}
		}

		// lookup cjs entry from the sub-module directory if it's not defined in `package.json`
		if entry.main == "" {
			if ctx.existsPkgFile(subModuleName + ".cjs") {
				entry.update("./"+subModuleName+".cjs", false)
			} else if ctx.existsPkgFile(subModuleName, "index.cjs") {
				entry.update("./"+subModuleName+"/index.cjs", false)
			} else if pkgJson.Type != "module" {
				if ctx.existsPkgFile(subModuleName + ".js") {
					entry.update("./"+subModuleName+".js", false)
				} else if ctx.existsPkgFile(subModuleName, "index.js") {
					entry.update("./"+subModuleName+"/index.js", false)
				}
			}
		}

		if entry.types == "" {
			if entry.main != "" && ctx.existsPkgFile(stripModuleExt(entry.main)+".d.mts") {
				entry.types = stripModuleExt(entry.main) + ".d.mts"
			} else if entry.main != "" && ctx.existsPkgFile(stripModuleExt(entry.main)+".d.ts") {
				entry.types = stripModuleExt(entry.main) + ".d.ts"
			} else if ctx.existsPkgFile(subModuleName + ".d.mts") {
				entry.types = "./" + subModuleName + ".d.mts"
			} else if ctx.existsPkgFile(subModuleName + ".d.ts") {
				entry.types = "./" + subModuleName + ".d.ts"
			} else if ctx.existsPkgFile(subModuleName, "index.d.mts") {
				entry.types = "./" + subModuleName + "/index.d.mts"
			} else if ctx.existsPkgFile(subModuleName, "index.d.ts") {
				entry.types = "./" + subModuleName + "/index.d.ts"
			}
		}
	} else {
		entry = BuildEntry{
			main:   pkgJson.Main,
			module: pkgJson.Main != "" && (pkgJson.Type == "module" || strings.HasSuffix(pkgJson.Main, ".mjs")),
			types:  pkgJson.Types,
		}
		if pkgJson.Module != "" && ctx.existsPkgFile(pkgJson.Module) {
			entry.update(pkgJson.Module, true)
		}
		if entry.types == "" && pkgJson.Typings != "" {
			entry.types = pkgJson.Typings
		}

		if exports := pkgJson.Exports; exports.Len() > 0 {
			exportEntry := BuildEntry{}
			v, ok := exports.Get(".")
			if ok {
				if s, ok := v.(string); ok {
					/**
					exports: {
						".": "./index.js"
					}
					*/
					exportEntry.update(s, pkgJson.Type == "module" || strings.HasSuffix(s, ".mjs"))
				} else if om, ok := v.(*JsonObject); ok {
					/**
					exports: {
						".": {
							"require": "./cjs/index.js",
							"import": "./esm/index.js"
						}
					}
					*/
					exportEntry = ctx.resolveConditionExportEntry(om, pkgJson.Type)
				}
			} else {
				/**
				exports: {
					"require": "./cjs/index.js",
					"import": "./esm/index.js"
				}
				*/
				exportEntry = ctx.resolveConditionExportEntry(exports, pkgJson.Type)
			}
			ctx.finalizeBuildEntry(&exportEntry)
			if exportEntry.main != "" {
				entry.update(exportEntry.main, exportEntry.module)
			}
			if exportEntry.types != "" {
				entry.types = exportEntry.types
			}
		}

		// get rid of invalid main and types, and lookup the entry from file system
		ctx.finalizeBuildEntry(&entry)

		// lookup esm entry from the package directory if it's not defined in `package.json`
		if entry.main == "" {
			if ctx.existsPkgFile("index.mjs") {
				entry.update("./index.mjs", true)
			} else if pkgJson.Type == "module" && ctx.existsPkgFile("index.js") {
				entry.update("./index.js", true)
			}
		}

		// lookup cjs entry from the package directory if it's not defined in `package.json`
		if entry.main == "" {
			if ctx.existsPkgFile("index.cjs") {
				entry.update("./index.cjs", false)
			} else if pkgJson.Type != "module" && ctx.existsPkgFile("index.js") {
				entry.update("./index.js", false)
			}
		}

		if entry.types == "" {
			if ctx.existsPkgFile("index.d.mts") {
				entry.types = "./index.d.mts"
			} else if ctx.existsPkgFile("index.d.ts") {
				entry.types = "./index.d.ts"
			}
		}

		if entry.types == "" && entry.main != "" {
			if ctx.existsPkgFile(stripModuleExt(entry.main) + ".d.mts") {
				entry.types = stripModuleExt(entry.main) + ".d.mts"
			} else if ctx.existsPkgFile(stripModuleExt(entry.main) + ".d.ts") {
				entry.types = stripModuleExt(entry.main) + ".d.ts"
			} else if stripModuleExt(path.Base(entry.main)) == "index" {
				dir, _ := utils.SplitByLastByte(entry.main, '/')
				if ctx.existsPkgFile(dir, "index.d.mts") {
					entry.types = dir + "/index.d.mts"
				} else if ctx.existsPkgFile(dir, "index.d.ts") {
					entry.types = dir + "/index.d.ts"
				}
			}
		}
	}

	// resovle dts from `typesVersions` field if it's defined
	// see https://www.typescriptlang.org/docs/handbook/declaration-files/publishing.html#version-selection-with-typesversions
	if typesVersions := pkgJson.TypesVersions; len(typesVersions) > 0 && entry.types != "" {
		versions := make(sort.StringSlice, len(typesVersions))
		i := 0
		for c := range typesVersions {
			if strings.HasPrefix(c, ">") {
				versions[i] = c
				i++
			}
		}
		versions = versions[:i]
		if versions.Len() > 0 {
			versions.Sort()
			latestVersion := typesVersions[versions[versions.Len()-1]]
			if mapping, ok := latestVersion.(map[string]interface{}); ok {
				var paths interface{}
				var matched bool
				var exact bool
				var suffix string
				dts := normalizeEntryPath(entry.types)
				paths, matched = mapping[dts]
				if !matched {
					// try to match the dts wihout leading "./"
					paths, matched = mapping[strings.TrimPrefix(dts, "./")]
				}
				if matched {
					exact = true
				}
				if !matched {
					for key, value := range mapping {
						if strings.HasSuffix(key, "/*") {
							key = normalizeEntryPath(key)
							if strings.HasPrefix(dts, strings.TrimSuffix(key, "/*")) {
								paths = value
								matched = true
								suffix = strings.TrimPrefix(dts, strings.TrimSuffix(key, "*"))
								break
							}
						}
					}
				}
				if !matched {
					paths, matched = mapping["*"]
				}
				if matched {
					if a, ok := paths.([]interface{}); ok && len(a) > 0 {
						if path, ok := a[0].(string); ok {
							path = normalizeEntryPath(path)
							if exact {
								entry.types = path
							} else {
								prefix, _ := utils.SplitByLastByte(path, '*')
								if suffix != "" {
									entry.types = prefix + suffix
								} else if strings.HasPrefix(dts, prefix) {
									diff := strings.TrimPrefix(dts, prefix)
									entry.types = strings.ReplaceAll(path, "*", diff)
								} else {
									entry.types = prefix + dts[2:]
								}
							}
						}
					}
				}
			}
		}
	}

	ctx.finalizeBuildEntry(&entry)
	return
}

// normalizes the build entry
func (ctx *BuildContext) finalizeBuildEntry(entry *BuildEntry) {
	if entry.main != "" {
		entry.main = normalizeEntryPath(entry.main)
		preferedExt := ".cjs"
		if entry.module {
			preferedExt = ".mjs"
		}
		if !endsWith(entry.main, preferedExt, ".js") {
			if ctx.existsPkgFile(entry.main + preferedExt) {
				entry.main = entry.main + preferedExt
			} else if ctx.existsPkgFile(entry.main + ".js") {
				entry.main = entry.main + ".js"
			} else if ctx.existsPkgFile(entry.main, "index"+preferedExt) {
				entry.main = entry.main + "/index" + preferedExt
			} else if ctx.existsPkgFile(entry.main, "index.js") {
				entry.main = entry.main + "/index.js"
			}
		}
		if !ctx.existsPkgFile(entry.main) {
			entry.main = ""
		} else if !entry.module && !strings.HasSuffix(entry.main, ".cjs") {
			// check if the cjs entry is an ESM
			isESM, _, err := validateModuleFile(path.Join(ctx.pkgDir, entry.main))
			if err == nil {
				entry.module = isESM
			}
		}
	}

	if entry.types != "" {
		entry.types = normalizeEntryPath(entry.types)
		if endsWith(entry.types, ".js", ".mjs", ".cjs") {
			maybeDts := stripModuleExt(entry.types) + ".d.mts"
			if ctx.existsPkgFile(maybeDts) {
				entry.types = maybeDts
			} else {
				maybeDts = stripModuleExt(entry.types) + ".d.ts"
				if ctx.existsPkgFile(maybeDts) {
					entry.types = maybeDts
				}
			}
		} else if strings.HasPrefix(entry.types, ".d") {
			if ctx.existsPkgFile(entry.types + ".mts") {
				entry.types += ".mts"
			} else if ctx.existsPkgFile(entry.types + ".ts") {
				entry.types += ".ts"
			}
		} else if !endsWith(entry.types, ".d.ts", ".d.mts") {
			if ctx.existsPkgFile(entry.types + ".d.mts") {
				entry.types = entry.types + ".d.mts"
			} else if ctx.existsPkgFile(entry.types + ".d.ts") {
				entry.types = entry.types + ".d.ts"
			} else if ctx.existsPkgFile(entry.types + ".mts") {
				entry.types = entry.types + ".mts"
			} else if ctx.existsPkgFile(entry.types + ".ts") {
				entry.types = entry.types + ".ts"
			} else if ctx.existsPkgFile(entry.types, "index.d.mts") {
				entry.types = entry.types + "/index.d.mts"
			} else if ctx.existsPkgFile(entry.types, "index.d.ts") {
				entry.types = entry.types + "/index.d.ts"
			} else if ctx.existsPkgFile(entry.types, "index.mts") {
				entry.types = entry.types + "/index.mts"
			} else if ctx.existsPkgFile(entry.types, "index.ts") {
				entry.types = entry.types + "/index.ts"
			}
		}
	}
}

// see https://nodejs.org/api/packages.html#nested-conditions
func (ctx *BuildContext) resolveConditionExportEntry(conditions *JsonObject, preferedModuleType string) (entry BuildEntry) {
	if preferedModuleType == "types" {
		for _, conditionName := range []string{"module", "import", "es2015", "default", "require"} {
			condition, ok := conditions.Get(conditionName)
			if ok {
				if s, ok := condition.(string); ok {
					entry.types = s
				} else if om, ok := condition.(*JsonObject); ok {
					entry = ctx.resolveConditionExportEntry(om, "types")
				}
				break
			}
		}
		return
	}

	if ctx.isBrowserTarget() {
		condition, ok := conditions.Get("browser")
		if ok {
			if s, ok := condition.(string); ok {
				entry.update(s, preferedModuleType == "module")
			} else if om, ok := condition.(*JsonObject); ok {
				entry = ctx.resolveConditionExportEntry(om, preferedModuleType)
			}
		}
	} else if ctx.isDenoTarget() {
		conditionName := "deno"
		// [workaround] to support ssr in Deno, use `node` condition for solid-js < 1.6.0
		if ctx.esmPath.PkgName == "solid-js" && semverLessThan(ctx.esmPath.PkgVersion, "1.6.0") {
			conditionName = "node"
		}
		condition, ok := conditions.Get(conditionName)
		if ok {
			if s, ok := condition.(string); ok {
				entry.update(s, preferedModuleType == "module")
			} else if om, ok := condition.(*JsonObject); ok {
				entry = ctx.resolveConditionExportEntry(om, preferedModuleType)
			}
		}
	} else if ctx.target == "node" {
		condition, ok := conditions.Get("node")
		if ok {
			if s, ok := condition.(string); ok {
				entry.update(s, preferedModuleType == "module")
			} else if om, ok := condition.(*JsonObject); ok {
				entry = ctx.resolveConditionExportEntry(om, preferedModuleType)
			}
		}
	}

	if ctx.dev {
		condition, ok := conditions.Get("development")
		if ok {
			if s, ok := condition.(string); ok {
				entry.update(s, preferedModuleType == "module")
			} else if om, ok := condition.(*JsonObject); ok {
				entry = ctx.resolveConditionExportEntry(om, preferedModuleType)
			}
		}
	}

	if len(ctx.args.conditions) > 0 {
		for _, conditionName := range ctx.args.conditions {
			condition, ok := conditions.Get(conditionName)
			if ok {
				if s, ok := condition.(string); ok {
					entry.update(s, preferedModuleType == "module")
				} else if om, ok := condition.(*JsonObject); ok {
					entry = ctx.resolveConditionExportEntry(om, preferedModuleType)
				}
				break
			}
		}
	}

LOOP:
	for _, conditionName := range conditions.keys {
		condition := conditions.values[conditionName]
		module := false
		prefered := preferedModuleType
		switch conditionName {
		case "module", "import", "es2015":
			module = true
			prefered = "module"
		case "require":
			module = false
			prefered = "commonjs"
		case "default":
			module = preferedModuleType == "module"
			prefered = preferedModuleType
		case "types", "typings":
			if entry.types == "" {
				if s, ok := condition.(string); ok {
					entry.types = s
				} else if om, ok := condition.(*JsonObject); ok {
					e := ctx.resolveConditionExportEntry(om, "types")
					if e.types != "" {
						entry.types = e.types
					}
				}
			}
			continue LOOP
		default:
			// skip unknown condition
			continue LOOP
		}
		if entry.main == "" || (!entry.module && module) {
			if s, ok := condition.(string); ok {
				entry.update(s, module)
			} else if om, ok := condition.(*JsonObject); ok {
				e := ctx.resolveConditionExportEntry(om, prefered)
				if e.main != "" {
					entry.update(e.main, e.module)
				}
				if e.types != "" {
					entry.types = e.types
				}
			}
		}
	}

	return
}

func (ctx *BuildContext) resolveExternalModule(specifier string, kind api.ResolveKind) (resolvedPath string, err error) {
	defer func() {
		if err == nil {
			resolvedPathFull := resolvedPath
			// use relative path for sub-module of current package
			if strings.HasPrefix(specifier, ctx.packageJson.Name+"/") {
				rp, err := relPath(path.Dir(ctx.Path()), resolvedPath)
				if err == nil {
					resolvedPath = rp
				}
			}
			// mark the resolved path for _preload_
			if kind != api.ResolveJSDynamicImport {
				ctx.esmImports = append(ctx.esmImports, [2]string{resolvedPathFull, resolvedPath})
			}
			// if it's `require("module")` call
			if kind == api.ResolveJSRequireCall {
				ctx.cjsRequires = append(ctx.cjsRequires, [3]string{specifier, resolvedPathFull, resolvedPath})
				resolvedPath = specifier
			}
		}
	}()

	// it's the entry of current package from GitHub
	if npm := ctx.packageJson; ctx.esmPath.GhPrefix && (specifier == npm.Name || specifier == npm.PkgName) {
		resolvedPath = ctx.getImportPath(EsmPath{
			PkgName:    npm.Name,
			PkgVersion: npm.Version,
			GhPrefix:   true,
		}, ctx.getBuildArgsPrefix(false), ctx.externalAll)
		return
	}

	// node builtin module
	if isNodeBuiltInModule(specifier) {
		if ctx.externalAll || ctx.target == "node" || ctx.target == "denonext" || ctx.args.external.Has(specifier) {
			resolvedPath = specifier
		} else if ctx.target == "deno" {
			resolvedPath = fmt.Sprintf("https://deno.land/std@0.177.1/node/%s.ts", specifier[5:])
		} else {
			resolvedPath = fmt.Sprintf("/node/%s.mjs", specifier[5:])
		}
		return
	}

	// check `?external`
	if ctx.externalAll || ctx.args.external.Has(toPackageName(specifier)) {
		resolvedPath = specifier
		return
	}

	// it's a sub-module of current package
	if strings.HasPrefix(specifier, ctx.packageJson.Name+"/") {
		subPath := strings.TrimPrefix(specifier, ctx.packageJson.Name+"/")
		subModule := EsmPath{
			GhPrefix:      ctx.esmPath.GhPrefix,
			PrPrefix:      ctx.esmPath.PrPrefix,
			PkgName:       ctx.esmPath.PkgName,
			PkgVersion:    ctx.esmPath.PkgVersion,
			SubPath:       subPath,
			SubModuleName: stripEntryModuleExt(subPath),
		}
		resolvedPath = ctx.getImportPath(subModule, ctx.getBuildArgsPrefix(false), ctx.externalAll)
		if ctx.bundleMode == BundleFalse {
			n, e := utils.SplitByLastByte(resolvedPath, '.')
			resolvedPath = n + ".nobundle." + e
		}
		return
	}

	// common npm dependency
	pkgName, version, subpath, _ := splitEsmPath(specifier)
	if version == "" {
		if pkgName == ctx.esmPath.PkgName {
			version = ctx.esmPath.PkgVersion
		} else if pkgVerson, ok := ctx.args.deps[pkgName]; ok {
			version = pkgVerson
		} else if v, ok := ctx.packageJson.Dependencies[pkgName]; ok {
			version = strings.TrimSpace(v)
		} else if v, ok := ctx.packageJson.PeerDependencies[pkgName]; ok {
			version = strings.TrimSpace(v)
		} else {
			version = "latest"
		}
	}

	// force the version of 'react' (as dependency) equals to 'react-dom'
	if ctx.esmPath.PkgName == "react-dom" && pkgName == "react" {
		version = ctx.esmPath.PkgVersion
	}

	dep := EsmPath{
		PkgName:       pkgName,
		PkgVersion:    version,
		SubPath:       subpath,
		SubModuleName: stripEntryModuleExt(subpath),
	}

	// resolve alias in dependencies
	// e.g. "@mark/html": "npm:@jsr/mark__html@^1.0.0"
	// e.g. "tslib": "git+https://github.com/microsoft/tslib.git#v2.3.0"
	// e.g. "react": "github:facebook/react#v18.2.0"
	p, err := resolveDependencyVersion(version)
	if err != nil {
		resolvedPath = fmt.Sprintf("/error.js?type=%s&name=%s&importer=%s", strings.ReplaceAll(err.Error(), " ", "-"), pkgName, ctx.esmPath.Specifier())
		return
	}
	if p.Name != "" {
		dep.GhPrefix = p.Github
		dep.PrPrefix = p.PkgPrNew
		dep.PkgName = p.Name
		dep.PkgVersion = p.Version
	}

	// fetch the latest tag as the version of the repository
	if dep.GhPrefix && dep.PkgVersion == "" {
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", dep.PkgName))
		if err != nil {
			return
		}
		for _, ref := range refs {
			if ref.Ref == "HEAD" {
				dep.PkgVersion = ref.Sha[:16]
				break
			}
		}
	}

	var isFixedVersion bool
	if dep.GhPrefix {
		isFixedVersion = isCommitish(dep.PkgVersion) || regexpVersionStrict.MatchString(strings.TrimPrefix(dep.PkgVersion, "v"))
	} else if dep.PrPrefix {
		isFixedVersion = true
	} else {
		isFixedVersion = regexpVersionStrict.MatchString(dep.PkgVersion)
	}
	args := BuildArgs{
		alias:      ctx.args.alias,
		deps:       ctx.args.deps,
		external:   ctx.args.external,
		conditions: ctx.args.conditions,
	}

	err = resolveBuildArgs(ctx.npmrc, ctx.wd, &args, dep)
	if err != nil {
		return
	}

	if isFixedVersion {
		buildArgsPrefix := ""
		if a := encodeBuildArgs(args, false); a != "" {
			buildArgsPrefix = "X-" + a + "/"
		}
		resolvedPath = ctx.getImportPath(dep, buildArgsPrefix, false)
		return
	}

	if strings.ContainsRune(dep.PkgVersion, '|') || strings.ContainsRune(dep.PkgVersion, ' ') {
		// fetch the latest version of the package based on the semver range
		var p *PackageJSON
		_, p, err = ctx.lookupDep(pkgName+"@"+version, false)
		if err != nil {
			return
		}
		dep.PkgVersion = "^" + p.Version
	}

	resolvedPath = "/" + dep.Specifier()
	// workaround for es5-ext "../#/.." path
	if dep.PkgName == "es5-ext" {
		resolvedPath = strings.ReplaceAll(resolvedPath, "/#/", "/%23/")
	}
	params := []string{}
	if len(args.alias) > 0 {
		var alias []string
		for k, v := range args.alias {
			alias = append(alias, fmt.Sprintf("%s:%s", k, v))
		}
		params = append(params, "alias="+strings.Join(alias, ","))
	}
	if len(args.deps) > 0 {
		var deps sort.StringSlice
		for n, v := range args.deps {
			deps = append(deps, n+"@"+v)
		}
		deps.Sort()
		params = append(params, "deps="+strings.Join(deps, ","))
	}
	if args.external.Len() > 0 {
		external := make(sort.StringSlice, args.external.Len())
		for i, e := range args.external.Values() {
			external[i] = e
		}
		external.Sort()
		params = append(params, "external="+strings.Join(external, ","))
	}
	if len(args.conditions) > 0 {
		conditions := make(sort.StringSlice, len(args.conditions))
		copy(conditions, args.conditions)
		conditions.Sort()
		params = append(params, "conditions="+strings.Join(conditions, ","))
	}
	if dep.SubModuleName != "" && strings.HasSuffix(dep.SubModuleName, ".json") {
		params = append(params, "module")
	} else {
		params = append(params, "target="+ctx.target)
	}
	if ctx.dev {
		params = append(params, "dev")
	}
	resolvedPath += "?" + strings.Join(params, "&")
	return
}

func (ctx *BuildContext) resloveDTS(entry BuildEntry) (string, error) {
	if entry.types != "" {
		if !ctx.existsPkgFile(entry.types) {
			return "", nil
		}
		return fmt.Sprintf(
			"/%s/%s%s",
			ctx.esmPath.Name(),
			ctx.getBuildArgsPrefix(true),
			strings.TrimPrefix(entry.types, "./"),
		), nil
	}

	if ctx.esmPath.SubPath != "" && (ctx.packageJson.Types != "" || ctx.packageJson.Typings != "") {
		return "", nil
	}

	// lookup types in @types scope
	if packageJson := ctx.packageJson; packageJson.Types == "" && !strings.HasPrefix(packageJson.Name, "@types/") && regexpVersionStrict.MatchString(packageJson.Version) {
		versionParts := strings.Split(packageJson.Version, ".")
		versions := []string{
			versionParts[0] + "." + versionParts[1], // major.minor
			versionParts[0],                         // major
		}
		typesPkgName := toTypesPackageName(packageJson.Name)
		pkgVersion, ok := ctx.args.deps[typesPkgName]
		if ok {
			// use the version of the `?deps` query if it exists
			versions = append([]string{pkgVersion}, versions...)
		}
		for _, version := range versions {
			p, err := ctx.npmrc.getPackageInfo(typesPkgName, version)
			if err == nil {
				dtsModule := EsmPath{
					PkgName:       typesPkgName,
					PkgVersion:    p.Version,
					SubPath:       ctx.esmPath.SubPath,
					SubModuleName: ctx.esmPath.SubModuleName,
				}
				b := NewBuildContext(ctx.zoneId, ctx.npmrc, dtsModule, ctx.args, ctx.externalAll, "types", false, BundleFalse, false)
				err := b.install()
				if err != nil {
					if strings.Contains(err.Error(), " not found") {
						return "", nil
					}
					return "", err
				}
				dts, err := b.resloveDTS(b.resolveEntry(dtsModule))
				if err != nil {
					return "", err
				}
				if dts != "" {
					// use tilde semver range instead of the exact version
					return strings.ReplaceAll(dts, fmt.Sprintf("%s@%s", typesPkgName, p.Version), fmt.Sprintf("%s@~%s", typesPkgName, p.Version)), nil
				}
			}
		}
	}

	return "", nil
}

func (ctx *BuildContext) lexer(entry *BuildEntry) (ret *BuildMeta, cjsExports []string, cjsReexport string, err error) {
	if entry.main != "" && entry.module {
		if strings.HasSuffix(entry.main, ".vue") || strings.HasSuffix(entry.main, ".svelte") {
			ret = &BuildMeta{
				HasDefaultExport: true,
			}
			return
		}

		var isESM bool
		var namedExports []string
		isESM, namedExports, err = validateModuleFile(path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, entry.main))
		if err != nil {
			return
		}
		if isESM {
			ret = &BuildMeta{
				HasDefaultExport: contains(namedExports, "default"),
			}
			return
		}
		log.Warnf("fake ES module '%s' of '%s'", entry.main, ctx.packageJson.Name)

		var cjs cjsModuleLexerResult
		cjs, err = cjsModuleLexer(ctx, entry.main)
		if err != nil {
			return
		}

		ret = &BuildMeta{
			HasDefaultExport: true,
			CJS:              true,
		}
		cjsExports = cjs.Exports
		cjsReexport = cjs.Reexport
		entry.module = false
		return
	}

	if entry.main != "" && !entry.module {
		var cjs cjsModuleLexerResult
		cjs, err = cjsModuleLexer(ctx, entry.main)
		if err != nil {
			return
		}
		ret = &BuildMeta{
			HasDefaultExport: true,
			CJS:              true,
		}
		cjsExports = cjs.Exports
		cjsReexport = cjs.Reexport
		return
	}

	ret = &BuildMeta{}
	return
}

func matchAsteriskExports(epxortsKey string, subModuleName string) (diff string, match bool) {
	if strings.ContainsRune(epxortsKey, '*') {
		prefix, _ := utils.SplitByLastByte(epxortsKey, '*')
		if subModule := "./" + subModuleName; strings.HasPrefix(subModule, prefix) {
			return strings.TrimPrefix(subModule, prefix), true
		}
	}
	return "", false
}

func resloveAsteriskPathMapping(om *JsonObject, diff string) *JsonObject {
	reslovedConditions := newJSONObject()
	for _, key := range om.keys {
		value, ok := om.Get(key)
		if ok {
			if s, ok := value.(string); ok {
				reslovedConditions.Set(key, strings.ReplaceAll(s, "*", diff))
			} else if om, ok := value.(*JsonObject); ok {
				reslovedConditions.Set(key, resloveAsteriskPathMapping(om, diff))
			}
		}
	}
	return reslovedConditions
}

func getAllExportsPaths(exports *JsonObject) []string {
	var values []string
	for _, key := range exports.keys {
		v := exports.values[key]
		if s, ok := v.(string); ok {
			values = append(values, s)
		} else if condition, ok := v.(*JsonObject); ok {
			values = append(values, getAllExportsPaths(condition)...)
		}
	}
	return values
}

func normalizeEntryPath(path string) string {
	return "." + utils.NormalizePathname(path)
}

func normalizeSavePath(zoneId string, pathname string) string {
	segs := strings.Split(pathname, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "X-") && len(seg) > 42 {
			h := sha1.New()
			h.Write([]byte(seg))
			segs[i] = "x-" + hex.EncodeToString(h.Sum(nil))
		} else if strings.HasPrefix(seg, "*") {
			segs[i] = seg[1:] + "/ea"
		}
	}
	if zoneId != "" {
		return zoneId + "/" + strings.Join(segs, "/")
	}
	return strings.Join(segs, "/")
}
