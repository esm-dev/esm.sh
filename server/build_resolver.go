package server

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/esm-dev/esm.sh/internal/npm"
	esbuild "github.com/ije/esbuild-internal/api"
	esbuild_config "github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/gox/utils"
)

var moduleExts = []string{".js", ".ts", ".mjs", ".mts", ".jsx", ".tsx", ".cjs", ".cts"}

// BuildEntry represents the build entrypoints of a module
type BuildEntry struct {
	main   string
	module bool
	types  string
}

func (entry *BuildEntry) isEmpty() bool {
	return entry.main == "" && entry.types == ""
}

func (entry *BuildEntry) isTypesOnly() bool {
	return entry.main == "" && entry.types != ""
}

func (entry *BuildEntry) update(main string, module bool) {
	entry.main = main
	entry.module = module || strings.HasSuffix(main, ".mjs")
}

func (ctx *BuildContext) resolveEntry(esm EsmPath) (entry BuildEntry) {
	pkgJson := ctx.pkgJson

	if subPath := esm.SubPath; subPath != "" {
		if endsWith(subPath, ".d.ts", ".d.mts", ".d.cts") {
			entry.types = normalizeEntryPath(subPath)
			return
		}

		switch ext := path.Ext(subPath); ext {
		case ".mts", ".ts", ".tsx", ".cts":
			entry.update(subPath, true)
			// entry.types = strings.TrimSuffix(subPath, ext) + ".d" + strings.TrimSuffix(ext,"x")
			// lookup jsr built dts
			if strings.HasPrefix(esm.PkgName, "@jsr/") {
				for _, v := range pkgJson.Exports.Values() {
					if obj, ok := v.(npm.JSONObject); ok {
						if v, ok := obj.Get("default"); ok {
							if s, ok := v.(string); ok && s == "./"+stripModuleExt(subPath)+".js" {
								if v, ok := obj.Get("types"); ok {
									if s, ok := v.(string); ok {
										entry.types = normalizeEntryPath(s)
									}
								}
								break
							}
						}
					}
				}
			}
			return
		case ".json", ".jsx", ".svelte", ".vue":
			entry.update(subPath, true)
			return
		default:
			// continue
		}
	}

	if subModuleName := esm.SubModuleName; subModuleName != "" {
		// reslove sub-module using `exports` conditions if exists
		// see https://nodejs.org/api/packages.html#package-entry-points
		if pkgJson.Exports.Len() > 0 {
			var exportEntry BuildEntry
			conditions, ok := pkgJson.Exports.Get("./" + subModuleName)
			if ok {
				if s, ok := conditions.(string); ok {
					/**
					exports: {
						"./lib/foo": "./lib/foo.js"
					}
					*/
					exportEntry.update(s, pkgJson.Type == "module")
				} else if obj, ok := conditions.(npm.JSONObject); ok {
					/**
					exports: {
						"./lib/foo": {
							"require": "./lib/foo.js",
							"import": "./esm/foo.js",
							"types": "./types/foo.d.ts"
						}
					}
					*/
					exportEntry = ctx.resolveConditionExportEntry(obj, pkgJson.Type)
				}
			} else {
				for _, name := range pkgJson.Exports.Keys() {
					conditions, _ := pkgJson.Exports.Get(name)
					if stripEntryModuleExt(name) == "./"+subModuleName {
						if s, ok := conditions.(string); ok {
							/**
							exports: {
								"./lib/foo.js": "./lib/foo.js"
							}
							*/
							exportEntry.update(s, pkgJson.Type == "module")
						} else if obj, ok := conditions.(npm.JSONObject); ok {
							/**
							exports: {
								"./lib/foo.js": {
									"require": "./lib/foo.js",
									"import": "./esm/foo.js",
									"types": "./types/foo.d.ts"
								}
							}
							*/
							exportEntry = ctx.resolveConditionExportEntry(obj, pkgJson.Type)
						}
						break
					} else if diff, ok := matchAsteriskExport(name, subModuleName); ok {
						if s, ok := conditions.(string); ok {
							/**
							exports: {
								"./lib/*": "./dist/lib/*.js",
							}
							*/
							path := strings.ReplaceAll(s, "*", diff)
							if endsWith(path, ".mjs", ".js", ".cjs") {
								if ctx.existsPkgFile(path) {
									exportEntry.update(path, pkgJson.Type == "module")
									break
								}
							} else if p := path + ".mjs"; ctx.existsPkgFile(p) {
								exportEntry.update(p, true)
								break
							} else if p := path + ".js"; ctx.existsPkgFile(p) {
								exportEntry.update(p, pkgJson.Type == "module")
								break
							} else if p := path + ".cjs"; ctx.existsPkgFile(p) {
								exportEntry.update(p, false)
								break
							} else if p := path + "/index.mjs"; ctx.existsPkgFile(p) {
								exportEntry.update(p, true)
								break
							} else if p := path + "/index.js"; ctx.existsPkgFile(p) {
								exportEntry.update(p, pkgJson.Type == "module")
								break
							} else if p := path + "/index.cjs"; ctx.existsPkgFile(p) {
								exportEntry.update(p, false)
								break
							}
						} else if obj, ok := conditions.(npm.JSONObject); ok {
							/**
							exports: {
								"./lib/*": {
									"require": ".dist/lib/dist/*.js",
									"import": ".dist/lib/esm/*.js",
									"types": ".dist/lib/types/*.d.ts"
								},
							}
							*/
							exportEntry = ctx.resolveConditionExportEntry(resloveAsteriskPathMapping(obj, diff), pkgJson.Type)
							ctx.finalizeBuildEntry(&exportEntry)
							if !exportEntry.isEmpty() {
								break
							}
						}
					}
				}
			}
			if exportEntry.main != "" && ctx.existsPkgFile(exportEntry.main) {
				entry.update(exportEntry.main, exportEntry.module)
			}
			if exportEntry.types != "" && ctx.existsPkgFile(exportEntry.types) {
				entry.types = exportEntry.types
			}
		}

		// check if the sub-module is a directory and has a package.json
		var rawInfo npm.PackageJSONRaw
		if utils.ParseJSONFile(path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, subModuleName, "package.json"), &rawInfo) == nil {
			p := rawInfo.ToNpmPackage()
			if entry.main == "" {
				if p.Module != "" && ctx.existsPkgFile(subModuleName, p.Module) {
					entry.update("./"+path.Join(subModuleName, p.Module), true)
				} else if p.Main != "" && ctx.existsPkgFile(subModuleName, p.Main) {
					entry.update("./"+path.Join(subModuleName, p.Main), p.Type == "module")
				}
			}
			if entry.types == "" {
				if p.Types != "" && ctx.existsPkgFile(subModuleName, p.Types) {
					entry.types = "./" + path.Join(subModuleName, p.Types)
				} else if p.Typings != "" && ctx.existsPkgFile(subModuleName, p.Typings) {
					entry.types = "./" + path.Join(subModuleName, p.Typings)
				}
			}
		}

		// lookup entry from the sub-module directory if it's not defined in `package.json`
		if entry.main == "" {
			for _, ext := range []string{"mjs", "js", "cjs", "mts", "ts", "tsx", "cts"} {
				isModule := ext == "mjs" || ext == "mts" || ext == "ts" || (ext == "js" && pkgJson.Type == "module")
				if filename := "./" + subModuleName + "." + ext; ctx.existsPkgFile(filename) {
					entry.update(filename, isModule)
					break
				} else if filename := "./" + subModuleName + "/index." + ext; ctx.existsPkgFile(filename) {
					entry.update(filename, isModule)
					break
				}
			}
		}

		if entry.main == "" && len(ctx.pkgJson.Imports) > 0 {
			if v, ok := ctx.pkgJson.Imports[ctx.pkgJson.PkgName+"/*"]; ok {
				if s, ok := v.(string); ok && strings.HasSuffix(s, "/*") {
					for _, ext := range []string{"mjs", "js", "cjs", "mts", "ts", "tsx", "cts"} {
						isModule := ext == "mjs" || ext == "mts" || ext == "ts" || (ext == "js" && pkgJson.Type == "module")
						if filename := strings.TrimSuffix(s, "*") + subModuleName + "." + ext; ctx.existsPkgFile(filename) {
							entry.update(filename, isModule)
							break
						} else if filename := strings.TrimSuffix(s, "*") + subModuleName + "/index." + ext; ctx.existsPkgFile(filename) {
							entry.update(filename, isModule)
							break
						}
					}
				}
			}
		}

		// lookup entry main from `src` directory
		if entry.main == "" && esm.GhPrefix {
			for _, ext := range []string{"mts", "ts", "mjs", "js", "tsx", "cts", "cjs"} {
				isModule := ext != "cjs" && ext != "cts"
				if filename := "./src/" + subModuleName + "/index." + ext; ctx.existsPkgFile(filename) {
					entry.update(filename, isModule)
					break
				} else if filename := "./" + subModuleName + "/src/index." + ext; ctx.existsPkgFile(filename) {
					entry.update(filename, isModule)
					break
				}
			}
		}

		if entry.types == "" {
			if entry.main != "" && ctx.existsPkgFile(stripModuleExt(entry.main)+".d.mts") {
				entry.types = stripModuleExt(entry.main) + ".d.mts"
			} else if entry.main != "" && ctx.existsPkgFile(stripModuleExt(entry.main)+".d.ts") {
				entry.types = stripModuleExt(entry.main) + ".d.ts"
			} else if entry.main != "" && ctx.existsPkgFile(stripModuleExt(entry.main)+".d.cts") {
				entry.types = stripModuleExt(entry.main) + ".d.cts"
			} else if ctx.existsPkgFile(subModuleName + ".d.mts") {
				entry.types = "./" + subModuleName + ".d.mts"
			} else if ctx.existsPkgFile(subModuleName + ".d.ts") {
				entry.types = "./" + subModuleName + ".d.ts"
			} else if ctx.existsPkgFile(subModuleName + ".d.cts") {
				entry.types = "./" + subModuleName + ".d.cts"
			} else if ctx.existsPkgFile(subModuleName, "index.d.mts") {
				entry.types = "./" + subModuleName + "/index.d.mts"
			} else if ctx.existsPkgFile(subModuleName, "index.d.ts") {
				entry.types = "./" + subModuleName + "/index.d.ts"
			} else if ctx.existsPkgFile(subModuleName, "index.d.cts") {
				entry.types = "./" + subModuleName + "/index.d.cts"
			}
		}
	} else {
		if pkgJson.Module != "" && ctx.existsPkgFile(pkgJson.Module) {
			entry.update(pkgJson.Module, true)
		} else if pkgJson.Main != "" {
			entry.update(pkgJson.Main, pkgJson.Type == "module")
		}
		if pkgJson.Types != "" {
			entry.types = normalizeEntryPath(pkgJson.Types)
		} else if pkgJson.Typings != "" {
			entry.types = normalizeEntryPath(pkgJson.Typings)
		}
		if len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
			if path, ok := pkgJson.Browser["."]; ok && ctx.existsPkgFile(path) {
				entry.update(path, pkgJson.Type == "module")
			}
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
					exportEntry.update(s, pkgJson.Type == "module")
				} else if obj, ok := v.(npm.JSONObject); ok {
					/**
					exports: {
						".": {
							"require": "./cjs/index.js",
							"import": "./esm/index.js"
						}
					}
					*/
					exportEntry = ctx.resolveConditionExportEntry(obj, pkgJson.Type)
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
			if exportEntry.main != "" && ctx.existsPkgFile(exportEntry.main) {
				entry.update(exportEntry.main, exportEntry.module)
			}
			if exportEntry.types != "" && ctx.existsPkgFile(exportEntry.types) {
				entry.types = exportEntry.types
			}
		}

		// lookup entry from the package directory if it's not defined in `package.json`
		if entry.main == "" {
			if ctx.existsPkgFile("index.mjs") {
				entry.update("./index.mjs", true)
			} else if ctx.existsPkgFile("index.js") {
				entry.update("./index.js", pkgJson.Type == "module")
			} else if ctx.existsPkgFile("index.cjs") {
				entry.update("./index.cjs", false)
			}
		}

		// lookup entry main from `src` directory
		if entry.main == "" && esm.GhPrefix {
			for _, ext := range []string{"mts", "ts", "mjs", "js", "tsx", "cts", "cjs"} {
				filename := "./src/index." + ext
				if ctx.existsPkgFile(filename) {
					entry.update(filename, ext != "cjs" && ext != "cts")
					break
				}
			}
		}

		if entry.types == "" {
			if ctx.existsPkgFile("index.d.mts") {
				entry.types = "./index.d.mts"
			} else if ctx.existsPkgFile("index.d.ts") {
				entry.types = "./index.d.ts"
			} else if ctx.existsPkgFile("index.d.cts") {
				entry.types = "./index.d.cts"
			}
		}

		if entry.types == "" && entry.main != "" {
			if ctx.existsPkgFile(stripModuleExt(entry.main) + ".d.mts") {
				entry.types = stripModuleExt(entry.main) + ".d.mts"
			} else if ctx.existsPkgFile(stripModuleExt(entry.main) + ".d.ts") {
				entry.types = stripModuleExt(entry.main) + ".d.ts"
			} else if ctx.existsPkgFile(stripModuleExt(entry.main) + ".d.cts") {
				entry.types = stripModuleExt(entry.main) + ".d.cts"
			} else if stripModuleExt(path.Base(entry.main)) == "index" {
				dir, _ := utils.SplitByLastByte(entry.main, '/')
				if ctx.existsPkgFile(dir, "index.d.mts") {
					entry.types = dir + "/index.d.mts"
				} else if ctx.existsPkgFile(dir, "index.d.ts") {
					entry.types = dir + "/index.d.ts"
				} else if ctx.existsPkgFile(dir, "index.d.cts") {
					entry.types = dir + "/index.d.cts"
				}
			}
		}
	}

	// resolve entry main from `browser` field if it's defined
	if len(pkgJson.Browser) > 0 && ctx.isBrowserTarget() {
		if entry.main != "" {
			if path, ok := pkgJson.Browser[entry.main]; ok && ctx.existsPkgFile(path) {
				entry.update(path, pkgJson.Type == "module")
			}
		}
	}

	// resolve types from `typesVersions` field if it's defined
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
			if mapping, ok := latestVersion.(map[string]any); ok {
				var paths any
				var matched bool
				var exact bool
				var suffix string
				types := entry.types
				paths, matched = mapping[entry.types]
				if !matched {
					// try to match the dts wihout leading "./"
					paths, matched = mapping[strings.TrimPrefix(types, "./")]
				}
				if matched {
					exact = true
				}
				if !matched {
					for key, value := range mapping {
						if strings.HasSuffix(key, "/*") {
							key = normalizeEntryPath(key)
							if strings.HasPrefix(types, strings.TrimSuffix(key, "/*")) {
								paths = value
								matched = true
								suffix = strings.TrimPrefix(types, strings.TrimSuffix(key, "*"))
								break
							}
						}
					}
				}
				if !matched {
					paths, matched = mapping["*"]
				}
				if matched {
					if a, ok := paths.([]any); ok && len(a) > 0 {
						if path, ok := a[0].(string); ok {
							path = normalizeEntryPath(path)
							if exact {
								entry.types = path
							} else {
								prefix, _ := utils.SplitByLastByte(path, '*')
								if suffix != "" {
									entry.types = prefix + suffix
								} else if strings.HasPrefix(types, prefix) {
									diff := strings.TrimPrefix(types, prefix)
									entry.types = strings.ReplaceAll(path, "*", diff)
								} else {
									entry.types = prefix + types[2:]
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
		if !ctx.existsPkgFile(entry.main) {
			preferedExt := ".cjs"
			if entry.module {
				preferedExt = ".mjs"
			}
			if ctx.existsPkgFile(entry.main + preferedExt) {
				entry.main = entry.main + preferedExt
			} else if ctx.existsPkgFile(entry.main + ".js") {
				entry.main = entry.main + ".js"
			} else if ctx.existsPkgFile(entry.main, "index"+preferedExt) {
				entry.main = entry.main + "/index" + preferedExt
			} else if ctx.existsPkgFile(entry.main, "index.js") {
				entry.main = entry.main + "/index.js"
			} else {
				entry.main = ""
			}
		} else if !entry.module && endsWith(entry.main, ".js", ".ts") {
			// check if the cjs entry is an ESM
			isESM, _, err := validateJSFile(path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, entry.main))
			if err == nil {
				entry.module = isESM
			}
		}
	}

	if entry.types != "" {
		entry.types = normalizeEntryPath(entry.types)
		if endsWith(entry.types, ".js", ".mjs", ".cjs") {
			bearName := stripModuleExt(entry.types)
			if ctx.existsPkgFile(bearName + ".d.mts") {
				entry.types += ".d.mts"
			} else if ctx.existsPkgFile(bearName + ".d.ts") {
				entry.types += ".d.ts"
			} else if ctx.existsPkgFile(bearName + ".d.cts") {
				entry.types += ".d.cts"
			} else {
				entry.types = ""
			}
		} else if strings.HasSuffix(entry.types, ".d") {
			if ctx.existsPkgFile(entry.types + ".mts") {
				entry.types += ".mts"
			} else if ctx.existsPkgFile(entry.types + ".ts") {
				entry.types += ".ts"
			} else if ctx.existsPkgFile(entry.types + ".cts") {
				entry.types += ".cts"
			} else {
				entry.types = ""
			}
		} else if !endsWith(entry.types, ".d.ts", ".d.mts", ".d.cts") {
			if ctx.existsPkgFile(entry.types + ".d.mts") {
				entry.types = entry.types + ".d.mts"
			} else if ctx.existsPkgFile(entry.types + ".d.ts") {
				entry.types = entry.types + ".d.ts"
			} else if ctx.existsPkgFile(entry.types + ".d.cts") {
				entry.types = entry.types + ".d.cts"
			} else if ctx.existsPkgFile(entry.types, "index.d.mts") {
				entry.types = entry.types + "/index.d.mts"
			} else if ctx.existsPkgFile(entry.types, "index.d.ts") {
				entry.types = entry.types + "/index.d.ts"
			} else if ctx.existsPkgFile(entry.types, "index.d.cts") {
				entry.types = entry.types + "/index.d.cts"
			} else {
				entry.types = ""
			}
		} else if !ctx.existsPkgFile(entry.types) {
			entry.types = ""
		}
	} else if ext := path.Ext(entry.main); ext == ".mts" || ext == ".ts" || ext == ".tsx" || ext == ".cts" {
		// entry.types = strings.TrimSuffix(entry.main, ext) + ".d" + strings.TrimSuffix(ext,"x")
	}
}

// see https://nodejs.org/api/packages.html#nested-conditions
func (ctx *BuildContext) resolveConditionExportEntry(conditions npm.JSONObject, preferedModuleType string) (entry BuildEntry) {
	if preferedModuleType == "types" {
		for _, conditionName := range []string{"module", "import", "es2015", "default", "require"} {
			condition, ok := conditions.Get(conditionName)
			if ok {
				if s, ok := condition.(string); ok {
					if entry.types == "" || endsWith(s, ".d.ts", ".d.mts", ".d.cts", ".d") {
						entry.types = s
					}
				} else if obj, ok := condition.(npm.JSONObject); ok {
					entry = ctx.resolveConditionExportEntry(obj, "types")
				}
				break
			}
		}
		return
	}

	applyCondition := func(conditionName string) bool {
		condition, ok := conditions.Get(conditionName)
		if ok {
			if s, ok := condition.(string); ok {
				entry.update(s, preferedModuleType == "module")
				return true
			} else if obj, ok := condition.(npm.JSONObject); ok {
				entry = ctx.resolveConditionExportEntry(obj, preferedModuleType)
				return entry.main != ""
			}
		}
		return false
	}

	var conditionFound bool

	if ctx.isBrowserTarget() {
		conditionName := "browser"
		// [workaround] fix astring entry in browser
		if ctx.esmPath.PkgName == "astring" {
			conditionName = "import"
		}
		conditionFound = applyCondition(conditionName)
	} else if ctx.isDenoTarget() {
		conditionName := "deno"
		// [workaround] to support solid-js/ssr in Deno, use `node` condition for < 1.6.0
		if ctx.esmPath.PkgName == "solid-js" && semverLessThan(ctx.esmPath.PkgVersion, "1.6.0") {
			conditionName = "node"
		}
		conditionFound = applyCondition(conditionName)
	} else if ctx.target == "node" {
		conditionFound = applyCondition("node")
	}

	if len(ctx.args.Conditions) > 0 {
		for _, conditionName := range ctx.args.Conditions {
			conditionFound = applyCondition(conditionName)
			if conditionFound {
				break
			}
		}
	} else if ctx.dev {
		conditionFound = applyCondition("development")
	}

LOOP:
	for _, conditionName := range conditions.Keys() {
		condition, _ := conditions.Get(conditionName)
		module := false
		prefered := ""
		switch conditionName {
		case "import", "module", "es2015":
			module = true
			prefered = "module"
		case "require":
			module = false
			prefered = "commonjs"
		case "default":
			prefered = preferedModuleType
			if prefered != "module" {
				if s, ok := condition.(string); ok && strings.HasSuffix(s, ".mjs") {
					prefered = "module"
				} else if _, ok := conditions.Get("require"); ok {
					prefered = "module"
				}
			}
			module = prefered == "module"
		case "types", "typings":
			if s, ok := condition.(string); ok {
				if entry.types == "" || (!strings.HasSuffix(entry.types, ".d.mts") && strings.HasSuffix(s, ".d.mts")) {
					entry.types = s
				}
			} else if obj, ok := condition.(npm.JSONObject); ok {
				e := ctx.resolveConditionExportEntry(obj, "types")
				if e.types != "" {
					if entry.types == "" || (!strings.HasSuffix(entry.types, ".d.mts") && strings.HasSuffix(e.types, ".d.mts")) {
						entry.types = e.types
					}
				}
			}
			continue LOOP
		default:
			// skip unknown condition
			continue LOOP
		}
		if entry.main == "" || (!entry.module && module && !conditionFound) {
			if s, ok := condition.(string); ok {
				entry.update(s, module)
			} else if obj, ok := condition.(npm.JSONObject); ok {
				e := ctx.resolveConditionExportEntry(obj, prefered)
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

func (ctx *BuildContext) resolveExternalModule(specifier string, kind esbuild.ResolveKind, withTypeJSON bool, analyzeMode bool) (resolvedPath string, err error) {
	// return the specifier directly in analyze mode
	if analyzeMode {
		return specifier, nil
	}

	defer func() {
		if err == nil && !withTypeJSON {
			resolvedPathFull := resolvedPath
			// use relative path for sub-module of current package
			if pkgJson := ctx.pkgJson; specifier == pkgJson.Name || strings.HasPrefix(specifier, pkgJson.Name+"/") {
				rp, err := relPath(path.Dir(ctx.Path()), resolvedPath)
				if err == nil {
					resolvedPath = rp
				}
			}
			if kind == esbuild.ResolveJSRequireCall {
				ctx.cjsRequires = append(ctx.cjsRequires, [3]string{specifier, resolvedPathFull, resolvedPath})
				resolvedPath = specifier
			} else if kind == esbuild.ResolveJSImportStatement && !withTypeJSON {
				ctx.esmImports = append(ctx.esmImports, [2]string{resolvedPathFull, resolvedPath})
			}
		}
	}()

	// check `?external`
	packageName := toPackageName(specifier)
	if ctx.externalAll || ctx.args.External.Has(packageName) || isPackageInExternalNamespace(packageName, ctx.args.External) {
		resolvedPath = specifier
		return
	}

	// if it's a node builtin module
	if isNodeBuiltinSpecifier(specifier) {
		if ctx.externalAll || ctx.target == "node" || ctx.target == "denonext" || ctx.args.External.Has(specifier) {
			resolvedPath = specifier
		} else if ctx.target == "deno" {
			resolvedPath = fmt.Sprintf("https://deno.land/std@0.177.1/node/%s.ts", specifier[5:])
		} else {
			resolvedPath = fmt.Sprintf("/node/%s.mjs", specifier[5:])
		}
		return
	}

	// if it's a http module
	if isHttpSepcifier(specifier) {
		return specifier, nil
	}

	if strings.HasPrefix(specifier, "file:") {
		return "", errors.New("file: protocol is not supported: " + specifier)
	}

	// if it's `main` entry of current package
	if pkgJson := ctx.pkgJson; specifier == pkgJson.Name || specifier == pkgJson.PkgName {
		resolvedPath = ctx.getImportPath(EsmPath{
			PkgName:    pkgJson.Name,
			PkgVersion: pkgJson.Version,
			GhPrefix:   ctx.esmPath.GhPrefix,
			PrPrefix:   ctx.esmPath.PrPrefix,
		}, ctx.getBuildArgsPrefix(false), ctx.externalAll)
		return
	}

	// if it's a sub-module of current package
	if strings.HasPrefix(specifier, ctx.pkgJson.Name+"/") {
		subPath := strings.TrimPrefix(specifier, ctx.pkgJson.Name+"/")
		subModule := EsmPath{
			GhPrefix:      ctx.esmPath.GhPrefix,
			PrPrefix:      ctx.esmPath.PrPrefix,
			PkgName:       ctx.esmPath.PkgName,
			PkgVersion:    ctx.esmPath.PkgVersion,
			SubPath:       subPath,
			SubModuleName: stripEntryModuleExt(subPath),
		}
		if withTypeJSON {
			resolvedPath = "/" + subModule.Specifier()
			if !strings.HasSuffix(subPath, ".json") {
				entry := ctx.resolveEntry(subModule)
				if entry.main != "" {
					resolvedPath = "/" + subModule.Name() + entry.main[1:]
				}
			}
		} else {
			resolvedPath = ctx.getImportPath(subModule, ctx.getBuildArgsPrefix(false), ctx.externalAll)
			if ctx.bundleMode == BundleFalse {
				n, e := utils.SplitByLastByte(resolvedPath, '.')
				resolvedPath = n + ".nobundle." + e
			}
		}
		return
	}

	var pkgName, pkgVersion, subPath string

	// jsr dependency
	if strings.HasPrefix(specifier, "jsr:") {
		pkgName, pkgVersion, subPath, _ = splitEsmPath(specifier[4:])
		if !strings.HasPrefix(pkgName, "@") || !strings.ContainsRune(pkgName, '/') {
			return specifier, errors.New("invalid `jsr:` dependency:" + specifier)
		}
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		pkgName = "@jsr/" + scope[1:] + "__" + name
	} else {
		pkgName, pkgVersion, subPath, _ = splitEsmPath(specifier)
	}

	if pkgVersion == "" {
		if pkgName == ctx.esmPath.PkgName {
			pkgVersion = ctx.esmPath.PkgVersion
		} else if pkgVerson, ok := ctx.args.Deps[pkgName]; ok {
			pkgVersion = pkgVerson
		} else if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok {
			pkgVersion = strings.TrimSpace(v)
		} else if v, ok := ctx.pkgJson.PeerDependencies[pkgName]; ok {
			pkgVersion = strings.TrimSpace(v)
		} else {
			pkgVersion = "latest"
		}
	}

	dep := EsmPath{
		PkgName:       pkgName,
		PkgVersion:    pkgVersion,
		SubPath:       subPath,
		SubModuleName: stripEntryModuleExt(subPath),
	}

	// resolve alias in dependencies
	// e.g. "@mark/html": "npm:@jsr/mark__html@^1.0.0"
	// e.g. "tslib": "git+https://github.com/microsoft/tslib.git#v2.3.0"
	// e.g. "react": "github:facebook/react#v18.2.0"
	p, err := npm.ResolveDependencyVersion(pkgVersion)
	if err != nil {
		resolvedPath = fmt.Sprintf("/error.js?type=%s&name=%s&importer=%s", strings.ReplaceAll(err.Error(), " ", "-"), pkgName, ctx.esmPath.Specifier())
		return
	}
	if p.Url != "" {
		resolvedPath = p.Url
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
		refs, err = listGhRepoRefs(fmt.Sprintf("https://github.com/%s", dep.PkgName))
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

	// [workaround] force the dependency version of `react` equals to react-dom
	if ctx.esmPath.PkgName == "react-dom" && dep.PkgName == "react" {
		dep.PkgVersion = ctx.esmPath.PkgVersion
	}

	if withTypeJSON {
		resolvedPath = "/" + dep.Specifier()
		if subPath == "" || !strings.HasSuffix(subPath, ".json") {
			b := &BuildContext{
				npmrc:   ctx.npmrc,
				logger:  ctx.logger,
				esmPath: dep,
			}
			err = b.install()
			if err != nil {
				return
			}
			entry := b.resolveEntry(dep)
			if entry.main != "" {
				resolvedPath = "/" + dep.Name() + entry.main[1:]
			}
		}
		return
	}

	args := BuildArgs{
		Alias:      ctx.args.Alias,
		Deps:       ctx.args.Deps,
		External:   ctx.args.External,
		Conditions: ctx.args.Conditions,
	}
	err = resolveBuildArgs(ctx.npmrc, ctx.wd, &args, dep)
	if err != nil {
		return
	}

	var exactVersion bool
	if dep.GhPrefix {
		exactVersion = isCommitish(dep.PkgVersion) || npm.IsExactVersion(strings.TrimPrefix(dep.PkgVersion, "v"))
	} else if dep.PrPrefix {
		exactVersion = true
	} else {
		exactVersion = npm.IsExactVersion(dep.PkgVersion)
	}
	if exactVersion {
		buildArgsPrefix := ""
		if a := encodeBuildArgs(args, false); a != "" {
			buildArgsPrefix = "X-" + a + "/"
		}
		resolvedPath = ctx.getImportPath(dep, buildArgsPrefix, false)
		return
	}

	if strings.ContainsRune(dep.PkgVersion, '|') || strings.ContainsRune(dep.PkgVersion, ' ') {
		// fetch the latest version of the package based on the semver range
		var p *npm.PackageJSON
		_, p, err = ctx.lookupDep(pkgName+"@"+pkgVersion, false)
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
	if len(args.Alias) > 0 {
		var alias []string
		for k, v := range args.Alias {
			alias = append(alias, fmt.Sprintf("%s:%s", k, v))
		}
		params = append(params, "alias="+strings.Join(alias, ","))
	}
	if len(args.Deps) > 0 {
		var deps sort.StringSlice
		for n, v := range args.Deps {
			deps = append(deps, n+"@"+v)
		}
		deps.Sort()
		params = append(params, "deps="+strings.Join(deps, ","))
	}
	if args.External.Len() > 0 {
		external := make(sort.StringSlice, args.External.Len())
		for i, e := range args.External.Values() {
			external[i] = e
		}
		external.Sort()
		params = append(params, "external="+strings.Join(external, ","))
	}
	if len(args.Conditions) > 0 {
		conditions := make(sort.StringSlice, len(args.Conditions))
		copy(conditions, args.Conditions)
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

func (ctx *BuildContext) resolveDTS(entry BuildEntry) (string, error) {
	if entry.types != "" {
		return fmt.Sprintf(
			"/%s/%s%s",
			ctx.esmPath.Name(),
			ctx.getBuildArgsPrefix(true),
			strings.TrimPrefix(entry.types, "./"),
		), nil
	}

	if ctx.esmPath.SubPath != "" && (ctx.pkgJson.Types != "" || ctx.pkgJson.Typings != "") {
		return "", nil
	}

	// lookup types in @types scope
	if pkgJson := ctx.pkgJson; pkgJson.Types == "" && !strings.HasPrefix(pkgJson.Name, "@types/") && npm.IsExactVersion(pkgJson.Version) {
		versionParts := strings.Split(pkgJson.Version, ".")
		versions := []string{
			versionParts[0] + "." + versionParts[1], // major.minor
			versionParts[0],                         // major
		}
		typesPkgName := npm.ToTypesPackageName(pkgJson.Name)
		pkgVersion, ok := ctx.args.Deps[typesPkgName]
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
				b := &BuildContext{
					npmrc:       ctx.npmrc,
					logger:      ctx.logger,
					esmPath:     dtsModule,
					args:        ctx.args,
					externalAll: ctx.externalAll,
					target:      "types",
				}
				err := b.install()
				if err != nil {
					if strings.Contains(err.Error(), " not found") {
						return "", nil
					}
					return "", err
				}
				dts, err := b.resolveDTS(b.resolveEntry(dtsModule))
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

func (ctx *BuildContext) getImportPath(esm EsmPath, buildArgsPrefix string, externalAll bool) string {
	if strings.HasSuffix(esm.SubPath, ".json") && ctx.existsPkgFile(esm.SubPath) {
		return "/" + esm.Name() + "/" + esm.SubPath + "?module"
	}
	asteriskPrefix := ""
	if externalAll {
		asteriskPrefix = "*"
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
	return fmt.Sprintf(
		"/%s%s/%s%s/%s.mjs",
		asteriskPrefix,
		esm.Name(),
		buildArgsPrefix,
		ctx.target,
		name,
	)
}

func (ctx *BuildContext) getSavepath() string {
	return normalizeSavePath(ctx.npmrc.zoneId, path.Join("modules", ctx.Path()))
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
	args := make([]string, 3+len(fp))
	args[0] = ctx.wd
	args[1] = "node_modules"
	args[2] = ctx.esmPath.PkgName
	copy(args[3:], fp)
	return existsFile(path.Join(args...))
}

func (ctx *BuildContext) lookupDep(specifier string, isDts bool) (esm EsmPath, packageJson *npm.PackageJSON, err error) {
	pkgName, version, subpath, _ := splitEsmPath(specifier)
lookup:
	if v, ok := ctx.args.Deps[pkgName]; ok {
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

	var raw npm.PackageJSONRaw
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
		if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok {
			if strings.HasPrefix(v, "npm:") {
				pkgName, version, _, _ = splitEsmPath(v[4:])
			} else {
				version = v
			}
		} else if v, ok = ctx.pkgJson.PeerDependencies[pkgName]; ok {
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
		pkgName = npm.ToTypesPackageName(pkgName)
		goto lookup
	}
	return
}

func (ctx *BuildContext) lexer(entry *BuildEntry) (ret *BuildMeta, cjsExports []string, cjsReexport string, err error) {
	if entry.main != "" && entry.module {
		if strings.HasSuffix(entry.main, ".vue") || strings.HasSuffix(entry.main, ".svelte") {
			ret = &BuildMeta{
				ExportDefault: true,
			}
			return
		}

		var isESM bool
		var namedExports []string
		isESM, namedExports, err = validateJSFile(path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, entry.main))
		if err != nil {
			return
		}
		if isESM {
			ret = &BuildMeta{
				ExportDefault: slices.Contains(namedExports, "default"),
			}
			return
		}

		var cjs cjsModuleLexerResult
		cjs, err = cjsModuleLexer(ctx, entry.main)
		if err != nil {
			return
		}

		if DEBUG {
			ctx.logger.Debugf("fake ES module '%s' of '%s'", entry.main, ctx.pkgJson.Name)
		}

		ret = &BuildMeta{
			ExportDefault: true,
			CJS:           true,
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
			ExportDefault: true,
			CJS:           true,
		}
		cjsExports = cjs.Exports
		cjsReexport = cjs.Reexport
		return
	}

	ret = &BuildMeta{}
	return
}

func matchAsteriskExport(exportName string, subModuleName string) (diff string, match bool) {
	if strings.ContainsRune(exportName, '*') {
		prefix, suffix := utils.SplitByLastByte(exportName, '*')
		if strings.HasPrefix("./"+subModuleName, prefix) && strings.HasSuffix(subModuleName, suffix) {
			return strings.TrimPrefix("./"+subModuleName, prefix), true
		}
	}
	return "", false
}

func resloveAsteriskPathMapping(conditions npm.JSONObject, diff string) npm.JSONObject {
	var keys []string
	var values = map[string]any{}
	for _, key := range conditions.Keys() {
		value, ok := conditions.Get(key)
		if ok {
			if s, ok := value.(string); ok {
				keys = append(keys, key)
				values[key] = strings.ReplaceAll(s, "*", diff)
			} else if c, ok := value.(npm.JSONObject); ok {
				keys = append(keys, key)
				values[key] = resloveAsteriskPathMapping(c, diff)
			}
		}
	}
	return npm.NewJSONObject(keys, values)
}

func getExportConditionPaths(condition npm.JSONObject) []string {
	var values []string
	for _, key := range condition.Keys() {
		v, _ := condition.Get(key)
		if s, ok := v.(string); ok {
			values = append(values, s)
		} else if condition, ok := v.(npm.JSONObject); ok {
			values = append(values, getExportConditionPaths(condition)...)
		}
	}
	return values
}

// stripModuleExt strips the module extension from the given string.
func stripModuleExt(s string) string {
	for _, ext := range moduleExts {
		if strings.HasSuffix(s, ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

// stripEntryModuleExt strips the entry module extension from the given string.
func stripEntryModuleExt(s string) string {
	if strings.HasSuffix(s, ".mjs") || strings.HasSuffix(s, ".cjs") {
		return s[:len(s)-4]
	}
	if strings.HasSuffix(s, ".js") {
		return s[:len(s)-3]
	}
	return s
}

func normalizeEntryPath(path string) string {
	return "." + utils.NormalizePathname(path)
}

func normalizeSavePath(zoneId string, pathname string) string {
	if strings.HasPrefix(pathname, "modules/transform/") || strings.HasPrefix(pathname, "modules/x/") {
		if zoneId != "" {
			return zoneId + "/" + pathname
		}
		return pathname
	}
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

// normalizeImportSpecifier normalizes the given specifier.
func normalizeImportSpecifier(specifier string) string {
	switch specifier {
	case ".":
		specifier = "./index"
	case "..":
		specifier = "../index"
	default:
		specifier = strings.TrimPrefix(specifier, "npm:")
	}
	if nodeBuiltinModules[specifier] {
		return "node:" + specifier
	}
	return specifier
}

// validateJSFile validates javascript/typescript module from the given file.
func validateJSFile(filename string) (isESM bool, namedExports []string, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	parserOpts := js_parser.OptionsFromConfig(&esbuild_config.Options{
		JSX: esbuild_config.JSXOptions{
			Parse: endsWith(filename, ".jsx", ".tsx"),
		},
		TS: esbuild_config.TSOptions{
			Parse: endsWith(filename, ".ts", ".mts", ".cts", ".tsx"),
		},
	})
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPaths:    logger.PrettyPaths{Rel: "<stdin>"},
		IdentifierName: "stdin",
		Contents:       string(data),
	}, parserOpts)
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	isESM = ast.ExportsKind == js_ast.ExportsESM || ast.ExportsKind == js_ast.ExportsESMWithDynamicFallback
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
}
