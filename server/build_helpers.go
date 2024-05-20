package server

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	esbConfig "github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/gox/utils"
)

func (ctx *BuildContext) Path() string {
	if ctx.path != "" {
		return ctx.path
	}

	pkg := ctx.pkg
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"

	if pkg.SubModule != "" {
		name = pkg.SubModule
		extname = ".js"
		// workaround for es5-ext weird "/#/" path
		if pkg.Name == "es5-ext" {
			name = strings.ReplaceAll(name, "/#/", "/%23/")
		}
	}
	if ctx.target == "types" {
		ctx.path = fmt.Sprintf(
			"/%s%s@%s/%s%s",
			pkg.ghPrefix(),
			pkg.Name,
			pkg.Version,
			ctx.getBuildArgsAsPathSegment(ctx.pkg, ctx.target == "types"),
			name,
		)
		return ctx.path
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
		"/%s%s@%s/%s%s/%s%s",
		pkg.ghPrefix(),
		pkg.Name,
		pkg.Version,
		ctx.getBuildArgsAsPathSegment(ctx.pkg, ctx.target == "types"),
		ctx.target,
		name,
		extname,
	)
	return ctx.path
}

func (ctx *BuildContext) getImportPath(pkg Pkg, buildArgsPrefix string) string {
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"
	if pkg.SubModule != "" {
		name = pkg.SubModule
		extname = ".js"
		// workaround for es5-ext weird "/#/" path
		if pkg.Name == "es5-ext" {
			name = strings.ReplaceAll(name, "/#/", "/%23/")
		}
	}
	if ctx.dev {
		name += ".development"
	}
	ghPrefix := ""
	if pkg.FromGithub {
		ghPrefix = "/gh"
	}
	return fmt.Sprintf(
		"%s/%s@%s/%s%s/%s%s",
		ghPrefix,
		pkg.Name,
		pkg.Version,
		buildArgsPrefix,
		ctx.target,
		name,
		extname,
	)
}

func (ctx *BuildContext) getSavepath() string {
	return normalizeSavePath(ctx.zoneId, path.Join("builds", ctx.Path()))
}

func (ctx *BuildContext) getBuildArgsAsPathSegment(pkg Pkg, isDts bool) string {
	if a := encodeBuildArgs(ctx.args, pkg, isDts); a != "" {
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

func (ctx *BuildContext) isServerTarget() bool {
	return ctx.isDenoTarget() || ctx.target == "node"
}

func (ctx *BuildContext) lookupDep(specifier string) (pkg Pkg, p PackageJSON, installed bool, err error) {
	pkgName, version, subpath, _ := splitPkgPath(specifier)
	pkgJsonPath := path.Join(ctx.wd, "node_modules", ".pnpm", "node_modules", pkgName, "package.json")
	if !existsFile(pkgJsonPath) {
		pkgJsonPath = path.Join(ctx.wd, "node_modules", pkgName, "package.json")
	}
	if parseJSONFile(pkgJsonPath, &p) == nil {
		pkg = Pkg{
			Name:      p.Name,
			Version:   p.Version,
			SubPath:   subpath,
			SubModule: toModuleBareName(subpath, true),
		}
		installed = true
		return
	}
	if version == "" {
		if pkg, ok := ctx.args.deps.Get(pkgName); ok {
			version = pkg.Version
		} else if v, ok := ctx.pkgJson.Dependencies[pkgName]; ok {
			if strings.HasPrefix(v, "npm:") {
				pkgName, version, _, _ = splitPkgPath(v[4:])
			} else {
				version = v
			}
		} else if v, ok = ctx.pkgJson.PeerDependencies[pkgName]; ok {
			version = v
		} else {
			version = "latest"
		}
	}
	p, err = ctx.npmrc.getPackageInfo(pkgName, version)
	if err == nil {
		pkg = Pkg{
			Name:      p.Name,
			Version:   p.Version,
			SubPath:   subpath,
			SubModule: toModuleBareName(subpath, true),
		}
	}
	return
}

func (ctx *BuildContext) init(forceCjsOnly bool) (ret BuildResult, entry PackageEntry, reexport string, err error) {
	// npmrc := ctx.npmrc
	pkgJson := ctx.pkgJson
	typesOnly := strings.HasPrefix(pkgJson.Name, "@types/") || (pkgJson.Main == "" && pkgJson.Module == "" && pkgJson.Types != "")

	entry = ctx.getEntry()

	if typesOnly {
		ret.TypesOnly = true
		return
	}

	if entry.esm != "" && !forceCjsOnly {
		isESM, namedExports, erro := ctx.esmLexer(entry.esm)
		if erro != nil {
			err = erro
			return
		}

		if isESM {
			ret.NamedExports = namedExports
			ret.HasDefaultExport = includes(namedExports, "default")
			return
		}

		var cjs cjsLexerResult
		cjs, err = ctx.cjsLexer(entry.esm)
		if err != nil {
			return
		}
		ret.HasDefaultExport = cjs.HasDefaultExport
		ret.NamedExports = cjs.NamedExports
		ret.FromCJS = true
		reexport = cjs.ReExport
		entry.cjs = entry.esm
		entry.esm = ""
		log.Warnf("fake ES module '%s' of '%s'", entry.cjs, pkgJson.Name)
		return
	}

	if entry.cjs != "" {
		var cjs cjsLexerResult
		cjs, err = ctx.cjsLexer(entry.cjs)
		if err != nil {
			return
		}
		ret.HasDefaultExport = cjs.HasDefaultExport
		ret.NamedExports = cjs.NamedExports
		ret.FromCJS = true
		reexport = cjs.ReExport
	}
	return
}

func (ctx *BuildContext) getEntry() (entry PackageEntry) {
	pkg := ctx.pkg
	pkgJson := ctx.pkgJson
	if pkg.SubModule != "" {
		if endsWith(pkg.SubModule, ".d.ts", ".d.mts") {
			entry.dts = pkg.SubModule
			return
		}
		if endsWith(pkg.SubModule, ".jsx", ".ts", ".tsx") {
			entry.esm = pkg.SubModule
			return
		}
		subModuleDir := path.Join(ctx.wd, "node_modules", pkg.Name, pkg.SubModule)
		subModulePkgJson := path.Join(subModuleDir, "package.json")
		var p PackageJSON
		if existsDir(subModuleDir) && existsFile(subModulePkgJson) && parseJSONFile(subModulePkgJson, &p) == nil {
			if p.Module != "" {
				entry.esm = path.Join(pkg.SubModule, p.Module)
			}
			if p.Main != "" {
				entry.cjs = path.Join(pkg.SubModule, p.Main)
			}
			if p.Types != "" {
				entry.dts = path.Join(pkg.SubModule, p.Types)
			} else if p.Typings != "" {
				entry.dts = path.Join(pkg.SubModule, p.Typings)
			}
			// reslove sub-module using `exports` conditions if exists
		} else if pkgJson.Exports != nil {
			sp := PackageJSON{}
			if om, ok := pkgJson.Exports.(*OrderedMap); ok {
				for e := om.l.Front(); e != nil; e = e.Next() {
					name, exports := om.Entry(e)
					if name == "./"+pkg.SubModule || name == "./"+pkg.SubModule+".js" || name == "./"+pkg.SubModule+".mjs" {
						/**
						exports: {
							"./lib/core": {
								"require": "./lib/core.js",
								"import": "./esm/core.js"
							},
						"./lib/core.js": {
								"require": "./lib/core.js",
								"import": "./esm/core.js"
							}
						}
						*/
						ctx.resolveConditions(&sp, exports, pkgJson.Type)
						break
					} else if strings.HasSuffix(name, "*") && strings.HasPrefix("./"+pkg.SubModule, strings.TrimSuffix(name, "*")) {
						/**
						exports: {
							"./lib/languages/*": {
								"require": "./lib/languages/*.js",
								"import": "./esm/languages/*.js"
							},
						}
						*/
						suffix := strings.TrimPrefix("./"+pkg.SubModule, strings.TrimSuffix(name, "*"))
						hitExports := false
						if om, ok := exports.(*OrderedMap); ok {
							newExports := newOrderedMap()
							for e := om.l.Front(); e != nil; e = e.Next() {
								key, value := om.Entry(e)
								if s, ok := value.(string); ok && s != name {
									newExports.Set(key, strings.Replace(s, "*", suffix, -1))
									hitExports = true
								}
								/**
								exports: {
									"./*": {
										"types": "./*.d.ts",
										"import": {
											"types": "./esm/*.d.mts",
											"default": "./esm/*.mjs"
										},
										"default": "./*.js"
									}
								}
								*/
								if s, ok := value.(map[string]interface{}); ok {
									subNewDefinies := newOrderedMap()
									for subKey, subValue := range s {
										if s1, ok := subValue.(string); ok && s1 != name {
											subNewDefinies.Set(subKey, strings.Replace(s1, "*", suffix, -1))
											hitExports = true
										}
									}
									newExports.Set(key, subNewDefinies)
								}
							}
							exports = newExports
						} else if s, ok := exports.(string); ok {
							exports = strings.Replace(s, "*", suffix, -1)
							hitExports = true
						}
						if hitExports {
							ctx.resolveConditions(&sp, exports, pkgJson.Type)
							break
						}
					}
				}
			}
			entry.esm = sp.Module
			entry.cjs = sp.Main
			if sp.Types != "" {
				entry.dts = sp.Types
			} else if sp.Typings != "" {
				entry.dts = sp.Typings
			}
		}

		if entry.esm == "" {
			if existsFile(path.Join(subModuleDir, "index.mjs")) {
				entry.esm = pkg.SubModule + "/index.mjs"
			} else if existsFile(subModuleDir + ".mjs") {
				entry.esm = pkg.SubModule + ".mjs"
			} else if pkgJson.Type == "module" {
				if existsFile(path.Join(subModuleDir, "index.js")) {
					entry.esm = pkg.SubModule + "/index.js"
				} else if existsFile(subModuleDir + ".js") {
					entry.esm = pkg.SubModule + ".js"
				}
			}
		}

		if entry.cjs == "" {
			if existsFile(path.Join(subModuleDir, "index.cjs")) {
				entry.cjs = pkg.SubModule + "/index.cjs"
			} else if existsFile(subModuleDir + ".cjs") {
				entry.cjs = pkg.SubModule + ".cjs"
			} else if pkgJson.Type != "module" {
				if existsFile(path.Join(subModuleDir, "index.js")) {
					entry.cjs = pkg.SubModule + "/index.js"
				} else if existsFile(subModuleDir + ".js") {
					entry.cjs = pkg.SubModule + ".js"
				}
				// check if the cjs entry is ESM
				if entry.cjs != "" {
					isESM, _, _ := validateJS(path.Join(ctx.wd, "node_modules", pkg.Name, entry.cjs))
					if isESM {
						entry.esm = entry.cjs
						entry.cjs = ""
					}
				}
			}
		}

		if entry.dts == "" {
			if existsFile(path.Join(subModuleDir, "index.d.ts")) {
				entry.dts = pkg.SubModule + "/index.d.ts"
			} else if existsFile(path.Join(subModuleDir, "index.d.mts")) {
				entry.dts = pkg.SubModule + "/index.d.mts"
			} else if existsFile(subModuleDir + ".d.ts") {
				entry.dts = pkg.SubModule + ".d.ts"
			} else if existsFile(subModuleDir + ".d.mts") {
				entry.dts = pkg.SubModule + ".d.mts"
			}
		}
	} else {
		if exports := pkgJson.Exports; exports != nil {
			if om, ok := exports.(*OrderedMap); ok {
				v, ok := om.m["."]
				if ok {
					/*
						exports: {
							".": {
								"require": "./cjs/index.js",
								"import": "./esm/index.js"
							}
						}
						exports: {
							".": "./esm/index.js"
						}
					*/
					ctx.resolveConditions(&pkgJson, v, pkgJson.Type)
				} else {
					/*
						exports: {
							"require": "./cjs/index.js",
							"import": "./esm/index.js"
						}
					*/
					ctx.resolveConditions(&pkgJson, om, pkgJson.Type)
				}
			} else if s, ok := exports.(string); ok {
				/*
					exports: "./esm/index.js"
				*/
				ctx.resolveConditions(&pkgJson, s, pkgJson.Type)
			}
		}

		nmDir := path.Join(ctx.wd, "node_modules")
		if pkgJson.Main == "" && pkgJson.Module == "" {
			if existsFile(path.Join(nmDir, pkgJson.Name, "index.mjs")) {
				pkgJson.Module = "./index.mjs"
			} else if existsFile(path.Join(nmDir, pkgJson.Name, "index.js")) {
				if pkgJson.Type == "module" {
					pkgJson.Module = "./index.js"
				} else {
					pkgJson.Main = "./index.js"
				}
			} else if existsFile(path.Join(nmDir, pkgJson.Name, "index.cjs")) {
				pkgJson.Main = "./index.cjs"
			}
		}

		// check `browser` field
		if !ctx.isServerTarget() {
			var browserModule string
			var browserMain string
			if pkgJson.Module != "" {
				m, ok := pkgJson.Browser[pkgJson.Module]
				if ok {
					browserModule = m
				}
			} else if pkgJson.Main != "" {
				m, ok := pkgJson.Browser[pkgJson.Main]
				if ok {
					browserMain = m
				}
			}
			if browserModule == "" && browserMain == "" {
				if m := pkgJson.Browser["."]; m != "" && existsFile(path.Join(nmDir, pkgJson.Name, m)) {
					isEsm, _, _ := validateJS(path.Join(nmDir, pkgJson.Name, m))
					if isEsm {
						browserModule = m
					} else {
						browserMain = m
					}
				}
			}
			if browserModule != "" {
				pkgJson.Module = browserModule
			} else if browserMain != "" {
				pkgJson.Main = browserMain
			}
		}

		if pkgJson.Types == "" && pkgJson.Typings != "" {
			pkgJson.Types = pkgJson.Typings
		}
		if pkgJson.Types == "" && pkgJson.Module != "" {
			name, _ := utils.SplitByLastByte(pkgJson.Module, '.')
			maybeTypesPath := name + ".d.ts"
			if existsFile(path.Join(nmDir, pkgJson.Name, maybeTypesPath)) {
				pkgJson.Types = maybeTypesPath
			} else {
				dir, _ := utils.SplitByLastByte(pkgJson.Module, '/')
				maybeTypesPath := dir + "/index.d.ts"
				if existsFile(path.Join(nmDir, pkgJson.Name, maybeTypesPath)) {
					pkgJson.Types = maybeTypesPath
				}
			}
		}
		if pkgJson.Types == "" && pkgJson.Main != "" {
			if strings.HasSuffix(pkgJson.Main, ".d.ts") {
				pkgJson.Types = pkgJson.Main
				pkgJson.Main = ""
			} else {
				name, _ := utils.SplitByLastByte(pkgJson.Main, '.')
				maybeTypesPath := name + ".d.ts"
				if existsFile(path.Join(nmDir, pkgJson.Name, maybeTypesPath)) {
					pkgJson.Types = maybeTypesPath
				} else {
					dir, _ := utils.SplitByLastByte(pkgJson.Main, '/')
					maybeTypesPath := dir + "/index.d.ts"
					if existsFile(path.Join(nmDir, pkgJson.Name, maybeTypesPath)) {
						pkgJson.Types = maybeTypesPath
					}
				}
			}
		}

		entry = PackageEntry{
			esm: pkgJson.Module,
			cjs: pkgJson.Main,
			dts: pkgJson.Types,
		}
	}
	if entry.esm != "" && !strings.HasPrefix(entry.esm, "./") {
		entry.esm = "." + utils.CleanPath(entry.esm)
	}
	if entry.cjs != "" && !strings.HasPrefix(entry.cjs, "./") {
		entry.cjs = "." + utils.CleanPath(entry.cjs)
	}
	if entry.dts != "" && !strings.HasPrefix(entry.dts, "./") {
		entry.dts = "." + utils.CleanPath(entry.dts)
	}

	// fix types path
	// see https://www.typescriptlang.org/docs/handbook/declaration-files/publishing.html#version-selection-with-typesversions
	if typesVersions := pkgJson.TypesVersions; len(typesVersions) > 0 {
		conditions := make(sort.StringSlice, len(typesVersions))
		i := 0
		for c := range typesVersions {
			if strings.HasPrefix(c, ">") {
				conditions[i] = c
				i++
			}
		}
		conditions = conditions[:i]
		search := []string{"*"}
		if conditions.Len() > 0 {
			conditions.Sort()
			search = []string{conditions[conditions.Len()-1], "*"}
		}
		for _, c := range search {
			if e, ok := typesVersions[c]; ok {
				if m, ok := e.(map[string]interface{}); ok {
					d, ok := m["*"]
					if !ok {
						d, ok = m["."]
					}
					if ok {
						if a, ok := d.([]interface{}); ok && len(a) > 0 {
							if t, ok := a[0].(string); ok {
								if strings.HasSuffix(t, "*") {
									f := entry.dts
									if f == "" {
										f = "index.d.ts"
									}
									t = path.Join(t[:len(t)-1], f)
								}
								entry.dts = t
							}
						}
					}
				}
			}
		}
	}
	return
}

func (ctx *BuildContext) normalizePackageJSON(p PackageJSON) PackageJSON {
	pkg := ctx.pkg

	if pkg.FromGithub {
		// if the name in package.json is not the same as the repository name
		if p.Name != pkg.Name {
			p.PkgName = p.Name
			p.Name = pkg.Name
		}
		p.Version = pkg.Version
	} else {
		p.Version = strings.TrimPrefix(p.Version, "v")
	}

	if ctx.target == "types" && endsWith(pkg.SubPath, ".d.ts", ".d.mts") {
		return p
	}

	if p.Module == "" {
		if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
			p.Main = ""
		} else if p.ES2015 != "" {
			p.Module = p.ES2015
		} else if p.JsNextMain != "" {
			p.Module = p.JsNextMain
		}
	}

	// Check if the `SubPath` is the same as the `main` or `module` field of the package.json
	// See https://github.com/esm-dev/esm.sh/issues/578
	if pkg.SubModule != "" && ((p.Module != "" && pkg.SubModule == utils.CleanPath(stripModuleExt(p.Module))[1:]) ||
		(p.Main != "" && pkg.SubModule == utils.CleanPath(stripModuleExt(p.Main))[1:])) {
		ctx.pkg.SubModule = ""
		ctx.pkg.SubPath = ""
		ctx.path = ""
	}

	return p
}

// see https://nodejs.org/api/packages.html
func (ctx *BuildContext) resolveConditions(p *PackageJSON, exports interface{}, pType string) {
	s, ok := exports.(string)
	if ok {
		if pType == "module" {
			p.Module = s
		} else {
			p.Main = s
		}
		return
	}

	om, ok := exports.(*OrderedMap)
	if !ok {
		return
	}

	for e := om.l.Front(); e != nil; e = e.Next() {
		key := e.Value.(string)
		value := om.m[key]
		switch key {
		case "types":
			if s, ok := value.(string); ok {
				p.Types = s
			} else if m, ok := value.(map[string]interface{}); ok {
				if s, ok := m["default"].(string); ok && s != "" {
					p.Types = s
				}
			}
		case "typings":
			if s, ok := value.(string); ok {
				p.Typings = s
			} else if m, ok := value.(map[string]interface{}); ok {
				if s, ok := m["default"].(string); ok && s != "" {
					p.Typings = s
				}
			}
		}
	}

	targetConditions := []string{"browser"}
	conditions := []string{"module", "import", "es2015"}
	_, hasRequireCondition := om.m["require"]
	_, hasNodeCondition := om.m["node"]
	if pType == "module" || hasRequireCondition || hasNodeCondition {
		conditions = append(conditions, "default")
	}
	switch ctx.target {
	case "deno", "denonext":
		targetConditions = []string{"deno", "worker"}
		conditions = append(conditions, "browser")
		// priority use `node` condition for solid.js (< 1.5.6) in deno
		if (p.Name == "solid-js" || strings.HasPrefix(p.Name, "solid-js/")) && semverLessThan(p.Version, "1.5.6") {
			targetConditions = []string{"node"}
		}
	case "node":
		targetConditions = []string{"node"}
	}
	if ctx.dev {
		targetConditions = append(targetConditions, "development")
	}
	if ctx.args.conditions.Len() > 0 {
		targetConditions = append(ctx.args.conditions.Values(), targetConditions...)
	}
	for _, condition := range append(targetConditions, conditions...) {
		v, ok := om.m[condition]
		if ok {
			ctx.resolveConditions(p, v, "module")
			return
		}
	}
	for _, condition := range append(targetConditions, "require", "node", "default") {
		v, ok := om.m[condition]
		if ok {
			ctx.resolveConditions(p, v, "commonjs")
			break
		}
	}
}

func normalizeSavePath(zoneId string, pathname string) string {
	segs := strings.Split(pathname, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "X-") && len(seg) > 42 {
			h := sha1.New()
			h.Write([]byte(seg))
			segs[i] = "X-" + hex.EncodeToString(h.Sum(nil))
		}
	}
	if zoneId != "" {
		return zoneId + "/" + strings.Join(segs, "/")
	}
	return strings.Join(segs, "/")
}

func (ctx *BuildContext) cjsLexer(specifier string) (cjs cjsLexerResult, err error) {
	cjs, err = cjsLexer(ctx.npmrc, ctx.pkg.Name, ctx.wd, specifier, ctx.getNodeEnv())
	if err == nil && cjs.Error != "" {
		err = fmt.Errorf("cjsLexer: %s", cjs.Error)
	}
	return
}

func (ctx *BuildContext) esmLexer(specifier string) (isESM bool, namedExports []string, err error) {
	isESM, namedExports, err = validateJS(path.Join(ctx.wd, "node_modules", ctx.pkg.Name, specifier))
	if err != nil {
		err = fmt.Errorf("esmLexer: %v", err)
	}
	return
}

func validateJS(filename string) (isESM bool, namedExports []string, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	parserOpts := js_parser.OptionsFromConfig(&esbConfig.Options{
		JSX: esbConfig.JSXOptions{
			Parse: endsWith(filename, ".jsx", ".tsx"),
		},
		TS: esbConfig.TSOptions{
			Parse: endsWith(filename, ".ts", ".mts", ".cts", ".tsx"),
		},
	})
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       string(data),
		IdentifierName: "stdin",
	}, parserOpts)
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	isESM = ast.ExportsKind == js_ast.ExportsESM
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
}

func minify(code string, target api.Target, loader api.Loader) ([]byte, error) {
	ret := api.Transform(code, api.TransformOptions{
		Target:            target,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsInline,
		Loader:            loader,
	})
	if ret.Errors != nil && len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.Code, nil
}
