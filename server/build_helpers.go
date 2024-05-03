package server

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/gox/utils"
)

func (task *BuildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.pkg
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
	if task.target == "raw" {
		extname = ""
	}
	if task.dev {
		name += ".development"
	}
	if task.bundle {
		name += ".bundle"
	} else if task.noBundle {
		name += ".nobundle"
	}

	task.id = fmt.Sprintf(
		"%s%s@%s/%s%s/%s%s",
		task._ghPrefix(),
		pkg.Name,
		pkg.Version,
		encodeBuildArgsPrefix(task.args, task.pkg, task.target == "types"),
		task.target,
		name,
		extname,
	)
	if task.target == "types" {
		task.id = strings.TrimSuffix(task.id, extname)
	}
	return task.id
}

func (task *BuildTask) _ghPrefix() string {
	if task.pkg.FromGithub {
		return "gh/"
	}
	return ""
}

func (task *BuildTask) getImportPath(pkg Pkg, buildArgsPrefix string) string {
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
	if task.dev {
		name += ".development"
	}
	ghPrefix := ""
	if pkg.FromGithub {
		ghPrefix = "/gh"
	}
	return fmt.Sprintf(
		"%s%s/%s@%s/%s%s/%s%s",
		cfg.CdnBasePath,
		ghPrefix,
		pkg.Name,
		pkg.Version,
		buildArgsPrefix,
		task.target,
		name,
		extname,
	)
}

func (task *BuildTask) getSavepath() string {
	id := task.ID()
	return normalizeSavePath(path.Join("builds", id))
}

func normalizeSavePath(pathname string) string {
	segs := strings.Split(pathname, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "X-") && len(seg) > 42 {
			h := sha1.New()
			h.Write([]byte(seg))
			segs[i] = "X-" + hex.EncodeToString(h.Sum(nil))
		}
	}
	return strings.Join(segs, "/")
}

func (task *BuildTask) getPackageInfo(name string) (pkg Pkg, p NpmPackageInfo, fromPackageJSON bool, err error) {
	pkgName, _, subpath := splitPkgPath(name)
	var version string
	if pkg, ok := task.args.deps.Get(pkgName); ok {
		version = pkg.Version
	} else if v, ok := task.npm.Dependencies[pkgName]; ok {
		version = v
	} else if v, ok = task.npm.PeerDependencies[pkgName]; ok {
		version = v
	} else {
		version = "latest"
	}
	p, fromPackageJSON, err = getPackageInfo(task.resolveDir, pkgName, version)
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

func (task *BuildTask) isServerTarget() bool {
	return task.target == "deno" || task.target == "denonext" || task.target == "node"
}

func (task *BuildTask) isDenoTarget() bool {
	return task.target == "deno" || task.target == "denonext"
}

func (task *BuildTask) analyze(forceCjsOnly bool) (ret *BuildResult, npm NpmPackageInfo, reexport string, err error) {
	wd := task.wd
	pkg := task.pkg

	var p NpmPackageInfo
	err = parseJSONFile(path.Join(wd, "node_modules", pkg.Name, "package.json"), &p)
	if err != nil {
		return
	}

	npm = task.normalizeNpmPackage(p)
	ret = &BuildResult{}

	// Check if the supplied path name is actually a main export.
	// See https://github.com/esm-dev/esm.sh/issues/578
	if pkg.SubPath == path.Clean(npm.Main) || pkg.SubPath == path.Clean(npm.Module) {
		task.pkg.SubModule = ""
		npm = task.normalizeNpmPackage(p)
	}

	defer func() {
		ret.FromCJS = npm.Module == "" && npm.Main != ""
		ret.TypesOnly = isTypesOnlyPackage(npm)
	}()

	if pkg.SubModule != "" {
		if endsWith(pkg.SubModule, ".d.ts", ".d.mts") {
			if strings.HasSuffix(pkg.SubModule, "~.d.ts") {
				subModule := strings.TrimSuffix(pkg.SubModule, "~.d.ts")
				subModulePath := path.Join(wd, "node_modules", npm.Name, subModule)
				if existsFile(path.Join(subModulePath, "index.d.ts")) {
					npm.Types = path.Join(subModule, "index.d.ts")
				} else if existsFile(path.Join(subModulePath + ".d.ts")) {
					npm.Types = subModule + ".d.ts"
				}
			} else {
				npm.Types = pkg.SubModule
			}
		} else {
			subModulePath := path.Join(wd, "node_modules", npm.Name, pkg.SubModule)
			subModulePackageJson := path.Join(subModulePath, "package.json")
			if npm.Exports == nil && existsDir(subModulePath) && existsFile(subModulePackageJson) {
				var p NpmPackageInfo
				err = parseJSONFile(subModulePackageJson, &p)
				if err != nil {
					return
				}
				if p.Version == "" {
					// use parent package version if submodule package.json doesn't have version
					p.Version = npm.Version
				}
				np := task.normalizeNpmPackage(p)
				if np.Module != "" {
					npm.Module = path.Join(pkg.SubModule, np.Module)
				} else {
					npm.Module = ""
				}
				if p.Main != "" {
					npm.Main = path.Join(pkg.SubModule, p.Main)
				} else {
					npm.Main = path.Join(pkg.SubModule, "index.js")
				}
				npm.Types = ""
				if p.Types != "" {
					npm.Types = path.Join(pkg.SubModule, p.Types)
				} else if p.Typings != "" {
					npm.Types = path.Join(pkg.SubModule, p.Typings)
				} else if existsFile(path.Join(subModulePath, "index.d.ts")) {
					npm.Types = path.Join(pkg.SubModule, "index.d.ts")
				} else if existsFile(path.Join(subModulePath + ".d.ts")) {
					npm.Types = pkg.SubModule + ".d.ts"
				}
			} else {
				isTsx := endsWith(subModulePath, ".jsx", ".ts", ".tsx")
				if npm.Type == "module" || npm.Module != "" || isTsx || existsFile(subModulePath+".mjs") {
					// follow main module type or it's a `.mjs` file
					npm.Module = pkg.SubModule
				} else {
					npm.Main = pkg.SubModule
				}
				npm.Types = ""
				if existsFile(path.Join(subModulePath, "index.d.ts")) {
					npm.Types = path.Join(pkg.SubModule, "index.d.ts")
				} else if existsFile(path.Join(subModulePath + ".d.ts")) {
					npm.Types = pkg.SubModule + ".d.ts"
				}
				// reslove sub-module using `exports` conditions if exists
				if npm.Exports != nil && !isTsx {
					if om, ok := npm.Exports.(*OrderedMap); ok {
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
								task.resolveConditions(&npm, exports, npm.Type)
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
									task.resolveConditions(&npm, exports, npm.Type)
									break
								}
							}
						}
					}
				}
			}
		}
	}

	if task.target == "types" || isTypesOnlyPackage(npm) {
		return
	}

	nodeEnv := "production"
	if task.dev {
		nodeEnv = "development"
	}

	if npm.Module != "" && !forceCjsOnly {
		modulePath, namedExports, erro := esmLexer(wd, npm.Name, npm.Module)
		if erro == nil {
			npm.Module = modulePath
			ret.NamedExports = namedExports
			ret.HasDefaultExport = includes(namedExports, "default")
			return
		}
		if erro.Error() != "not a module" {
			err = fmt.Errorf("esmLexer: %s", erro)
			return
		}

		npm.Main = npm.Module
		npm.Module = ""

		var cjs cjsLexerResult
		cjs, err = cjsLexer(wd, path.Join(wd, "node_modules", pkg.Name, modulePath), nodeEnv)
		if err == nil && cjs.Error != "" {
			err = fmt.Errorf("cjsLexer: %s", cjs.Error)
		}
		if err != nil {
			return
		}
		reexport = cjs.Reexport
		ret.HasDefaultExport = cjs.HasDefaultExport
		ret.NamedExports = cjs.NamedExports
		log.Warnf("fake ES module '%s' of '%s'", npm.Main, npm.Name)
		return
	}

	if npm.Main != "" {
		// install peer dependencies when using `requireMode`
		if includes(requireModeAllowList, pkg.Name) && len(npm.PeerDependencies) > 0 {
			pkgs := make([]string, len(npm.PeerDependencies))
			i := 0
			for n, v := range npm.PeerDependencies {
				pkgs[i] = n + "@" + v
				i++
			}
			err = pnpmInstall(wd, pkgs...)
			if err != nil {
				return
			}
		}

		var cjs cjsLexerResult
		moduleName := npm.Name
		if pkg.SubModule != "" {
			moduleName += "/" + pkg.SubModule
		}
		cjs, err = cjsLexer(wd, moduleName, nodeEnv)
		if err == nil && cjs.Error != "" {
			err = fmt.Errorf("cjsLexer: %s", cjs.Error)
		}
		if err != nil {
			return
		}
		reexport = cjs.Reexport
		ret.HasDefaultExport = cjs.HasDefaultExport
		ret.NamedExports = cjs.NamedExports
	}
	return
}

func (task *BuildTask) normalizeNpmPackage(p NpmPackageInfo) NpmPackageInfo {
	if task.pkg.FromGithub {
		if p.Name != task.pkg.Name {
			p.PkgName = p.Name
			p.Name = task.pkg.Name
		}
		p.Version = task.pkg.Version
	} else {
		p.Version = strings.TrimPrefix(p.Version, "v")
	}

	if p.Types == "" && p.Typings != "" {
		p.Types = p.Typings
	}

	if len(p.TypesVersions) > 0 {
		var usedCondition string
		for c, e := range p.TypesVersions {
			if c == "*" && strings.HasPrefix(c, ">") || strings.HasPrefix(c, ">=") {
				if usedCondition == "" || c == "*" || c > usedCondition {
					if om, ok := e.(*OrderedMap); ok {
						d, ok := om.m["*"]
						if !ok {
							d, ok = om.m["."]
						}
						if ok {
							if a, ok := d.([]interface{}); ok && len(a) > 0 {
								if t, ok := a[0].(string); ok {
									usedCondition = c
									if strings.HasSuffix(t, "*") {
										f := p.Types
										if f == "" {
											f = "index.d.ts"
										}
										t = path.Join(t[:len(t)-1], f)
									}
									p.Types = t
								}
							}
						}
					}
				}
			}
		}
	}

	if exports := p.Exports; exports != nil {
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
				task.resolveConditions(&p, v, p.Type)
			} else {
				/*
					exports: {
						"require": "./cjs/index.js",
						"import": "./esm/index.js"
					}
				*/
				task.resolveConditions(&p, om, p.Type)
			}
		} else if s, ok := exports.(string); ok {
			/*
			  exports: "./esm/index.js"
			*/
			task.resolveConditions(&p, s, p.Type)
		}
	}

	nmDir := path.Join(task.wd, "node_modules")
	if p.Module == "" {
		if p.JsNextMain != "" && existsFile(path.Join(nmDir, p.Name, p.JsNextMain)) {
			p.Module = p.JsNextMain
		} else if p.ES2015 != "" && existsFile(path.Join(nmDir, p.Name, p.ES2015)) {
			p.Module = p.ES2015
		} else if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
		}
	}

	if p.Main == "" && p.Module == "" {
		if existsFile(path.Join(nmDir, p.Name, "index.mjs")) {
			p.Module = "./index.mjs"
		} else if existsFile(path.Join(nmDir, p.Name, "index.js")) {
			p.Main = "./index.js"
		} else if existsFile(path.Join(nmDir, p.Name, "index.cjs")) {
			p.Main = "./index.cjs"
		}
	}

	if p.Module != "" && !strings.HasPrefix(p.Module, "./") && !strings.HasPrefix(p.Module, "../") {
		p.Module = "." + utils.CleanPath(p.Module)
	}
	if p.Main != "" && !strings.HasPrefix(p.Main, "./") && !strings.HasPrefix(p.Module, "../") {
		p.Main = "." + utils.CleanPath(p.Main)
	}

	if !task.isServerTarget() {
		var browserModule string
		var browserMain string
		if p.Module != "" {
			m, ok := p.Browser[p.Module]
			if ok {
				browserModule = m
			}
		} else if p.Main != "" {
			m, ok := p.Browser[p.Main]
			if ok {
				browserMain = m
			}
		}
		if browserModule == "" && browserMain == "" {
			if m := p.Browser["."]; m != "" && existsFile(path.Join(nmDir, p.Name, m)) {
				isEsm, _, _ := validateJS(path.Join(nmDir, p.Name, m))
				if isEsm {
					browserModule = m
				} else {
					browserMain = m
				}
			}
		}
		if browserModule != "" {
			p.Module = browserModule
		} else if browserMain != "" {
			p.Main = browserMain
		}
	}

	if p.Types == "" && p.Main != "" {
		if strings.HasSuffix(p.Main, ".d.ts") {
			p.Types = p.Main
			p.Main = ""
		} else {
			name, _ := utils.SplitByLastByte(p.Main, '.')
			maybeTypesPath := name + ".d.ts"
			if existsFile(path.Join(nmDir, p.Name, maybeTypesPath)) {
				p.Types = maybeTypesPath
			} else {
				dir, _ := utils.SplitByLastByte(p.Main, '/')
				maybeTypesPath := dir + "/index.d.ts"
				if existsFile(path.Join(nmDir, p.Name, maybeTypesPath)) {
					p.Types = maybeTypesPath
				}
			}
		}
	}

	if p.Types == "" && p.Module != "" {
		if strings.HasSuffix(p.Module, ".d.ts") {
			p.Types = p.Module
			p.Module = ""
		} else {
			name, _ := utils.SplitByLastByte(p.Module, '.')
			maybeTypesPath := name + ".d.ts"
			if existsFile(path.Join(nmDir, p.Name, maybeTypesPath)) {
				p.Types = maybeTypesPath
			} else {
				dir, _ := utils.SplitByLastByte(p.Module, '/')
				maybeTypesPath := dir + "/index.d.ts"
				if existsFile(path.Join(nmDir, p.Name, maybeTypesPath)) {
					p.Types = maybeTypesPath
				}
			}
		}
	}

	return p
}

// see https://nodejs.org/api/packages.html
func (task *BuildTask) resolveConditions(p *NpmPackageInfo, exports interface{}, pType string) {
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
		s, ok := value.(string)
		if ok && s != "" {
			switch key {
			case "types":
				p.Types = s
			case "typings":
				p.Typings = s
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
	switch task.target {
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
	if task.dev {
		targetConditions = append(targetConditions, "development")
	}
	if task.args.conditions.Len() > 0 {
		targetConditions = append(task.args.conditions.Values(), targetConditions...)
	}
	for _, condition := range append(targetConditions, conditions...) {
		v, ok := om.m[condition]
		if ok {
			task.resolveConditions(p, v, "module")
			return
		}
	}
	for _, condition := range append(targetConditions, "require", "node", "default") {
		v, ok := om.m[condition]
		if ok {
			task.resolveConditions(p, v, "commonjs")
			break
		}
	}
}

func esmLexer(wd string, packageName string, moduleSpecifier string) (resolvedName string, namedExports []string, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	resolvedName = moduleSpecifier
	if !existsFile(path.Join(pkgDir, resolvedName)) {
		for _, ext := range esExts {
			name := moduleSpecifier + ext
			if existsFile(path.Join(pkgDir, name)) {
				resolvedName = name
				break
			}
		}
	}
	if !existsFile(path.Join(pkgDir, resolvedName)) {
		if endsWith(resolvedName, esExts...) {
			name, ext := utils.SplitByLastByte(resolvedName, '.')
			fixedName := name + "/index." + ext
			if existsFile(path.Join(pkgDir, fixedName)) {
				resolvedName = fixedName
			}
		} else if existsDir(path.Join(pkgDir, moduleSpecifier)) {
			for _, ext := range esExts {
				name := path.Join(moduleSpecifier, "index"+ext)
				if existsFile(path.Join(pkgDir, name)) {
					resolvedName = name
					break
				}
			}
		}
	}
	if !existsFile(path.Join(pkgDir, resolvedName)) {
		for _, ext := range esExts {
			if strings.HasSuffix(resolvedName, "index/index"+ext) {
				resolvedName = strings.TrimSuffix(resolvedName, "/index"+ext) + ext
				break
			}
		}
	}

	isESM, _namedExports, err := validateJS(path.Join(pkgDir, resolvedName))
	if err != nil {
		return
	}

	if !isESM {
		err = errors.New("not a module")
		return
	}

	namedExports = _namedExports
	return
}

func validateJS(filename string) (isESM bool, namedExports []string, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	parserOpts := js_parser.OptionsFromConfig(&config.Options{
		JSX: config.JSXOptions{
			Parse: endsWith(filename, ".jsx", ".tsx"),
		},
		TS: config.TSOptions{
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
