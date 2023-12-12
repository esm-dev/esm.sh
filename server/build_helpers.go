package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

func (task *BuildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.Pkg
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"

	if pkg.FromEsmsh {
		name = "mod"
	}
	if pkg.SubModule != "" {
		name = pkg.SubModule
		extname = ".js"
	}
	if task.Target == "raw" {
		extname = ""
	}
	if task.Dev {
		name += ".development"
	}
	if task.BundleDeps {
		name += ".bundle"
	} else if task.NoBundle {
		name += ".bundless"
	}

	task.id = fmt.Sprintf(
		"%s%s/%s@%s/%s%s/%s%s",
		task.getBuildVersion(task.Pkg),
		task.ghPrefix(),
		pkg.Name,
		pkg.Version,
		encodeBuildArgsPrefix(task.Args, task.Pkg, task.Target == "types"),
		task.Target,
		name,
		extname,
	)
	if task.Target == "types" {
		task.id = strings.TrimSuffix(task.id, extname)
	}
	return task.id
}

func (task *BuildTask) ghPrefix() string {
	if task.Pkg.FromGithub {
		return "/gh"
	}
	return ""
}

func (task *BuildTask) getImportPath(pkg Pkg, buildArgsPrefix string) string {
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"
	if pkg.SubModule != "" {
		name = pkg.SubModule
		extname = ".js"
	}
	if pkg.FromEsmsh {
		name = "mod"
	}
	// workaround for es5-ext weird "/#/" path
	if pkg.Name == "es5-ext" {
		name = strings.ReplaceAll(name, "/#/", "/$$/")
	}
	if task.Dev {
		name += ".development"
	}
	return fmt.Sprintf(
		"%s/%s/%s@%s/%s%s/%s%s",
		cfg.CdnBasePath,
		task.getBuildVersion(pkg),
		pkg.Name,
		pkg.Version,
		buildArgsPrefix,
		task.Target,
		name,
		extname,
	)
}

func (task *BuildTask) getBuildVersion(pkg Pkg) string {
	if stableBuild[pkg.Name] {
		return "stable"
	}
	return fmt.Sprintf("v%d", task.BuildVersion)
}

func (task *BuildTask) getSavepath() string {
	if stableBuild[task.Pkg.Name] {
		return path.Join(fmt.Sprintf("builds/v%d", STABLE_VERSION), strings.TrimPrefix(task.ID(), "stable/"))
	}
	return path.Join("builds", task.ID())
}

func (task *BuildTask) getPackageInfo(name string) (pkg Pkg, p NpmPackageInfo, fromPackageJSON bool, err error) {
	pkgName, _, subpath := splitPkgPath(name)
	var version string
	if pkg, ok := task.Args.deps.Get(pkgName); ok {
		version = pkg.Version
	} else if v, ok := task.npm.Dependencies[pkgName]; ok {
		version = v
	} else if v, ok = task.npm.PeerDependencies[pkgName]; ok {
		version = v
	} else {
		version = "latest"
	}
	p, fromPackageJSON, err = getPackageInfo(task.installDir, pkgName, version)
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
	return task.Target == "deno" || task.Target == "denonext" || task.Target == "node"
}

func (task *BuildTask) isDenoTarget() bool {
	return task.Target == "deno" || task.Target == "denonext"
}

func (task *BuildTask) analyze(forceCjsOnly bool) (esm *ESMBuild, npm NpmPackageInfo, reexport string, err error) {
	wd := task.wd
	pkg := task.Pkg

	var p NpmPackageInfo
	err = utils.ParseJSONFile(path.Join(wd, "node_modules", pkg.Name, "package.json"), &p)
	if err != nil {
		return
	}
	npm = task.fixNpmPackage(p)

	// Check if the supplied path name is actually a main export.
	// See: https://github.com/esm-dev/esm.sh/issues/578
	if pkg.SubPath == path.Clean(npm.Main) || pkg.SubPath == path.Clean(npm.Module) {
		task.Pkg.SubModule = ""
		npm = task.fixNpmPackage(p)
	}

	esm = &ESMBuild{}

	defer func() {
		esm.FromCJS = npm.Module == "" && npm.Main != ""
		esm.TypesOnly = isTypesOnlyPackage(npm)
	}()

	if pkg.SubModule != "" {
		if endsWith(pkg.SubModule, ".d.ts", ".d.mts") {
			if strings.HasSuffix(pkg.SubModule, "~.d.ts") {
				submodule := strings.TrimSuffix(pkg.SubModule, "~.d.ts")
				subDir := path.Join(wd, "node_modules", npm.Name, submodule)
				if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = submodule + ".d.ts"
				}
			} else {
				npm.Types = pkg.SubModule
			}
		} else {
			subDir := path.Join(wd, "node_modules", npm.Name, pkg.SubModule)
			packageFile := path.Join(subDir, "package.json")
			if fileExists(packageFile) {
				var p NpmPackageInfo
				err = utils.ParseJSONFile(packageFile, &p)
				if err != nil {
					return
				}
				if p.Version == "" {
					// use parent package version if submodule package.json doesn't have version
					p.Version = npm.Version
				}
				np := task.fixNpmPackage(p)
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
				} else if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(pkg.SubModule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = pkg.SubModule + ".d.ts"
				}
			} else {
				fp := path.Join(wd, "node_modules", npm.Name, pkg.SubModule+".mjs")
				if npm.Type == "module" || npm.Module != "" || fileExists(fp) {
					// follow main module type
					npm.Module = pkg.SubModule
				} else {
					npm.Main = pkg.SubModule
				}
				npm.Types = ""
				if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(pkg.SubModule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = pkg.SubModule + ".d.ts"
				}
				// reslove sub-module using `exports` conditions if exists
				if npm.PkgExports != nil {
					if om, ok := npm.PkgExports.(*orderedMap); ok {
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
								task.applyConditions(&npm, exports, npm.Type)
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
								if om, ok := exports.(*orderedMap); ok {
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
									task.applyConditions(&npm, exports, npm.Type)
									break
								}
							}
						}
					}
				}
			}
		}
	}

	if task.Target == "types" || isTypesOnlyPackage(npm) {
		return
	}

	nodeEnv := "production"
	if task.Dev {
		nodeEnv = "development"
	}

	if npm.Module != "" && !forceCjsOnly {
		modulePath, namedExports, erro := esmLexer(wd, npm.Name, npm.Module)
		if erro == nil {
			npm.Module = modulePath
			esm.NamedExports = namedExports
			esm.HasExportDefault = includes(namedExports, "default")
			return
		}
		if erro != nil && erro.Error() != "not a module" {
			err = fmt.Errorf("esmLexer: %s", erro)
			return
		}

		npm.Main = npm.Module
		npm.Module = ""

		var ret cjsExportsResult
		ret, err = cjsLexer(wd, path.Join(wd, "node_modules", pkg.Name, modulePath), nodeEnv)
		if err == nil && ret.Error != "" {
			err = fmt.Errorf("cjsLexer: %s", ret.Error)
		}
		if err != nil {
			return
		}
		reexport = ret.Reexport
		esm.HasExportDefault = ret.ExportDefault
		esm.NamedExports = ret.Exports
		log.Warnf("fake ES module '%s' of '%s'", npm.Main, npm.Name)
		return
	}

	if npm.Main != "" {
		// install peer dependencies when using `invokeMode`
		if includes(invokeModeAllowList, pkg.Name) && len(npm.PeerDependencies) > 0 {
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
		var ret cjsExportsResult
		ret, err = cjsLexer(wd, pkg.ImportPath(), nodeEnv)
		if err == nil && ret.Error != "" {
			err = fmt.Errorf("cjsLexer: %s", ret.Error)
		}
		if err != nil {
			return
		}
		reexport = ret.Reexport
		esm.HasExportDefault = ret.ExportDefault
		esm.NamedExports = ret.Exports
	}
	return
}

func (task *BuildTask) fixNpmPackage(p NpmPackageInfo) NpmPackageInfo {
	if task.Pkg.FromGithub {
		p.Name = task.Pkg.Name
		p.Version = task.Pkg.Version
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
					if om, ok := e.(*orderedMap); ok {
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

	if exports := p.PkgExports; exports != nil {
		if om, ok := exports.(*orderedMap); ok {
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
				task.applyConditions(&p, v, p.Type)
			} else {
				/*
					exports: {
						"require": "./cjs/index.js",
						"import": "./esm/index.js"
					}
				*/
				task.applyConditions(&p, om, p.Type)
			}
		} else if s, ok := exports.(string); ok {
			/*
			  exports: "./esm/index.js"
			*/
			task.applyConditions(&p, s, p.Type)
		}
	}

	nmDir := path.Join(task.wd, "node_modules")
	if p.Module == "" {
		if p.JsNextMain != "" && fileExists(path.Join(nmDir, p.Name, p.JsNextMain)) {
			p.Module = p.JsNextMain
		} else if p.ES2015 != "" && fileExists(path.Join(nmDir, p.Name, p.ES2015)) {
			p.Module = p.ES2015
		} else if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
		}
	}

	if p.Main == "" && p.Module == "" {
		if fileExists(path.Join(nmDir, p.Name, "index.mjs")) {
			p.Module = "./index.mjs"
		} else if fileExists(path.Join(nmDir, p.Name, "index.js")) {
			p.Main = "./index.js"
		} else if fileExists(path.Join(nmDir, p.Name, "index.cjs")) {
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
			if m := p.Browser["."]; m != "" && fileExists(path.Join(nmDir, p.Name, m)) {
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
			if fileExists(path.Join(nmDir, p.Name, maybeTypesPath)) {
				p.Types = maybeTypesPath
			} else {
				dir, _ := utils.SplitByLastByte(p.Main, '/')
				maybeTypesPath := dir + "/index.d.ts"
				if fileExists(path.Join(nmDir, p.Name, maybeTypesPath)) {
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
			if fileExists(path.Join(nmDir, p.Name, maybeTypesPath)) {
				p.Types = maybeTypesPath
			} else {
				dir, _ := utils.SplitByLastByte(p.Module, '/')
				maybeTypesPath := dir + "/index.d.ts"
				if fileExists(path.Join(nmDir, p.Name, maybeTypesPath)) {
					p.Types = maybeTypesPath
				}
			}
		}
	}

	return p
}

// see https://nodejs.org/api/packages.html
func (task *BuildTask) applyConditions(p *NpmPackageInfo, exports interface{}, pType string) {
	s, ok := exports.(string)
	if ok {
		if pType == "module" {
			p.Module = s
		} else {
			p.Main = s
		}
		return
	}

	om, ok := exports.(*orderedMap)
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
	switch task.Target {
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
	if task.Dev {
		targetConditions = append(targetConditions, "development")
	}
	if task.Args.conditions.Len() > 0 {
		targetConditions = append(task.Args.conditions.Values(), targetConditions...)
	}
	for _, condition := range append(targetConditions, conditions...) {
		v, ok := om.m[condition]
		if ok {
			task.applyConditions(p, v, "module")
			return
		}
	}
	for _, condition := range append(targetConditions, "require", "node", "default") {
		v, ok := om.m[condition]
		if ok {
			task.applyConditions(p, v, "commonjs")
			break
		}
	}
}

func queryESMBuild(id string) (*ESMBuild, bool) {
	value, err := db.Get(id)
	if err == nil && value != nil {
		var esm ESMBuild
		err = json.Unmarshal(value, &esm)
		if err == nil {
			if strings.HasPrefix(id, "stable/") {
				id = fmt.Sprintf("v%d/", STABLE_VERSION) + strings.TrimPrefix(id, "stable/")
			}
			if !esm.TypesOnly {
				_, err = fs.Stat(path.Join("builds", id))
			}
			if err == nil || os.IsExist(err) {
				return &esm, true
			}
		}
		// delete the invalid db entry
		db.Delete(id)
	}
	return nil, false
}

var jsExts = []string{".mjs", ".js", ".jsx", ".mts", ".ts", ".tsx"}

func esmLexer(wd string, packageName string, moduleSpecifier string) (resolvedName string, namedExports []string, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	resolvedName = moduleSpecifier
	if !fileExists(path.Join(pkgDir, resolvedName)) {
		for _, ext := range jsExts {
			name := moduleSpecifier + ext
			if fileExists(path.Join(pkgDir, name)) {
				resolvedName = name
				break
			}
		}
	}
	if !fileExists(path.Join(pkgDir, resolvedName)) {
		if endsWith(resolvedName, jsExts...) {
			name, ext := utils.SplitByLastByte(resolvedName, '.')
			fixedName := name + "/index." + ext
			if fileExists(path.Join(pkgDir, fixedName)) {
				resolvedName = fixedName
			}
		} else if dirExists(path.Join(pkgDir, moduleSpecifier)) {
			for _, ext := range jsExts {
				name := path.Join(moduleSpecifier, "index"+ext)
				if fileExists(path.Join(pkgDir, name)) {
					resolvedName = name
					break
				}
			}
		}
	}
	if !fileExists(path.Join(pkgDir, resolvedName)) {
		for _, ext := range jsExts {
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

func copyRawBuildFile(id string, name string, dir string) (err error) {
	var r io.ReadCloser
	var f *os.File
	r, err = fs.OpenFile(path.Join("publish", strings.TrimPrefix(id, "~"), name))
	if err != nil {
		if err == storage.ErrNotFound {
			return nil
		}
		return fmt.Errorf("open file failed: %s", name)
	}
	defer r.Close()
	ensureDir(dir)
	f, err = os.OpenFile(path.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return
}

func bundleNodePolyfill(name string, globalName string, namedExport string, target api.Target) ([]byte, error) {
	ret := api.Build(api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents: fmt.Sprintf(`import * as e from "node_%s.js";globalThis.%s=e.%s`, name, globalName, namedExport),
			Loader:   api.LoaderJS,
		},
		Write:             false,
		Bundle:            true,
		Target:            target,
		Format:            api.FormatIIFE,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Plugins: []api.Plugin{{
			Name: "esm",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(
					api.OnResolveOptions{Filter: ".*"},
					func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						return api.OnResolveResult{Path: path.Join("server/embed/polyfills/", args.Path), Namespace: "embed"}, nil
					},
				)
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "embed"},
					func(args api.OnLoadArgs) (api.OnLoadResult, error) {
						data, err := embedFS.ReadFile(args.Path)
						if err != nil {
							return api.OnLoadResult{}, err
						}
						contents := string(data)
						return api.OnLoadResult{
							Contents: &contents,
							Loader:   api.LoaderJS,
						}, nil
					},
				)
			}}},
	})
	if ret.Errors != nil && len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.OutputFiles[0].Contents, nil
}

func bundleHotScript(code string, target api.Target) ([]byte, error) {
	ret := api.Build(api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents: code,
			Loader:   api.LoaderTS,
		},
		Write:             false,
		Bundle:            true,
		Target:            target,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsInline,
		Plugins: []api.Plugin{{
			Name: "esm",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(
					api.OnResolveOptions{Filter: ".*"},
					func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						if args.Kind == api.ResolveJSDynamicImport || isHttpSepcifier(args.Path) {
							return api.OnResolveResult{Path: args.Path, External: true}, nil
						}
						return api.OnResolveResult{Path: path.Join("server/embed", args.Path), Namespace: "embed"}, nil
					},
				)
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "embed"},
					func(args api.OnLoadArgs) (api.OnLoadResult, error) {
						data, err := embedFS.ReadFile(args.Path + ".ts")
						if err != nil {
							return api.OnLoadResult{}, err
						}
						contents := string(data)
						return api.OnLoadResult{
							Contents: &contents,
							Loader:   api.LoaderTS,
						}, nil
					},
				)
			}}},
	})
	if ret.Errors != nil && len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.OutputFiles[0].Contents, nil
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
