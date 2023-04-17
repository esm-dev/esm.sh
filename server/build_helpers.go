package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/ije/gox/utils"
)

func (task *BuildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.Pkg
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"

	if pkg.Submodule != "" {
		name = pkg.Submodule
		extname = ".js"
	}
	if task.Target == "raw" {
		extname = ""
	}
	if task.Dev {
		name += ".development"
	}
	if task.Bundle {
		name += ".bundle"
	}

	task.id = fmt.Sprintf(
		"%s%s/%s@%s/%s%s/%s%s",
		task.getBuildVersion(task.Pkg),
		task.ghPrefix(),
		pkg.Name,
		pkg.Version,
		encodeBuildArgsPrefix(task.BuildArgs, task.Pkg.Name, task.Target == "types"),
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

func (task *BuildTask) getImportPath(pkg Pkg, prefix string) string {
	name := strings.TrimSuffix(path.Base(pkg.Name), ".js")
	extname := ".mjs"
	if pkg.Submodule != "" {
		name = strings.TrimSuffix(strings.TrimSuffix(pkg.Submodule, ".js"), ".mjs")
		extname = ".js"
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
		cfg.BasePath,
		task.getBuildVersion(pkg),
		pkg.Name,
		pkg.Version,
		prefix,
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

func (task *BuildTask) getRealWD() string {
	if task.realWd == "" {
		if l, e := filepath.EvalSymlinks(path.Join(task.wd, "node_modules", task.Pkg.Name)); e == nil {
			if strings.HasPrefix(task.Pkg.Name, "@") {
				task.realWd = path.Join(l, "../../..")
			} else {
				task.realWd = path.Join(l, "../..")
			}
		} else {
			task.realWd = task.wd
		}
	}
	return task.realWd
}

func (task *BuildTask) getPackageInfo(name string, version string) (info NpmPackage, fromPackageJSON bool, err error) {
	return getPackageInfo(task.getRealWD(), name, version)
}

func (task *BuildTask) analyze() (esm *ESMBuild, npm NpmPackage, reexport string, err error) {
	pkg := task.Pkg
	wd := task.wd
	target := task.Target
	isDev := task.Dev

	err = utils.ParseJSONFile(path.Join(wd, "node_modules", pkg.Name, "package.json"), &npm)
	if err != nil {
		return
	}

	npm = task.fixNpmPackage(npm)

	// Check if the supplied path name is actually a main export.
	// See: https://github.com/esm-dev/esm.sh/issues/578
	if pkg.Subpath == path.Clean(npm.Main) || pkg.Subpath == path.Clean(npm.Module) {
		task.Pkg.Submodule = ""
	}

	esm = &ESMBuild{}

	defer func() {
		esm.CJS = npm.Main != "" && npm.Module == ""
		esm.TypesOnly = isTypesOnlyPackage(npm)
	}()

	nodeEnv := "production"
	if isDev {
		nodeEnv = "development"
	}

	if pkg.Submodule != "" {
		if endsWith(pkg.Submodule, ".d.ts", ".d.mts") {
			if strings.HasSuffix(pkg.Submodule, "~.d.ts") {
				submodule := strings.TrimSuffix(pkg.Submodule, "~.d.ts")
				subDir := path.Join(wd, "node_modules", npm.Name, submodule)
				if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = submodule + ".d.ts"
				}
			} else {
				npm.Types = pkg.Submodule
			}
		} else {
			subDir := path.Join(wd, "node_modules", npm.Name, pkg.Submodule)
			packageFile := path.Join(subDir, "package.json")
			if fileExists(packageFile) {
				var p NpmPackage
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
					npm.Module = path.Join(pkg.Submodule, np.Module)
				} else {
					npm.Module = ""
				}
				if p.Main != "" {
					npm.Main = path.Join(pkg.Submodule, p.Main)
				} else {
					npm.Main = path.Join(pkg.Submodule, "index.js")
				}
				npm.Types = ""
				if p.Types != "" {
					npm.Types = path.Join(pkg.Submodule, p.Types)
				} else if p.Typings != "" {
					npm.Types = path.Join(pkg.Submodule, p.Typings)
				} else if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(pkg.Submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = pkg.Submodule + ".d.ts"
				}
			} else {
				var resolved bool
				if npm.DefinedExports != nil {
					if m, ok := npm.DefinedExports.(map[string]interface{}); ok {
						for name, defines := range m {
							if name == "./"+pkg.Submodule || name == "./"+pkg.Submodule+".js" || name == "./"+pkg.Submodule+".mjs" {
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
								task.applyConditions(&npm, defines, npm.Type)
								resolved = true
								break
							} else if strings.HasSuffix(name, "*") && strings.HasPrefix("./"+pkg.Submodule, strings.TrimSuffix(name, "*")) {
								/**
								  exports: {
								    "./lib/languages/*": {
								      "require": "./lib/languages/*.js",
								      "import": "./esm/languages/*.js"
								    },
								  }
								*/
								suffix := strings.TrimPrefix("./"+pkg.Submodule, strings.TrimSuffix(name, "*"))
								hasDefines := false
								if m, ok := defines.(map[string]interface{}); ok {
									newDefines := map[string]interface{}{}
									for key, value := range m {
										if s, ok := value.(string); ok && s != name {
											newDefines[key] = strings.Replace(s, "*", suffix, -1)
											hasDefines = true
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
											subNewDefinies := map[string]interface{}{}
											for subKey, subValue := range s {
												if s1, ok := subValue.(string); ok && s1 != name {
													subNewDefinies[subKey] = strings.Replace(s1, "*", suffix, -1)
													hasDefines = true
												}
											}
											newDefines[key] = subNewDefinies
										}
									}
									defines = newDefines
								} else if s, ok := defines.(string); ok && name != s {
									defines = strings.Replace(s, "*", suffix, -1)
									hasDefines = true
								}
								if hasDefines {
									task.applyConditions(&npm, defines, npm.Type)
									resolved = true
								}
							}
						}
					}
				}

				if !resolved {
					if npm.Type == "module" || npm.Module != "" {
						// follow main module type
						npm.Module = pkg.Submodule
					} else {
						npm.Main = pkg.Submodule
					}
					npm.Types = ""
					if fileExists(path.Join(subDir, "index.d.ts")) {
						npm.Types = path.Join(pkg.Submodule, "index.d.ts")
					} else if fileExists(path.Join(subDir + ".d.ts")) {
						npm.Types = pkg.Submodule + ".d.ts"
					}
				}
			}
		}
	}

	if target == "types" || isTypesOnlyPackage(npm) {
		return
	}

	if npm.Module != "" {
		modulePath, namedExports, erro := resovleESModule(wd, npm.Name, npm.Module)
		if erro == nil {
			npm.Module = modulePath
			esm.NamedExports = namedExports
			esm.HasExportDefault = includes(namedExports, "default")
			return
		}
		if erro != nil && erro.Error() != "not a module" {
			err = fmt.Errorf("resovleESModule: %s", erro)
			return
		}

		var ret cjsExportsResult
		ret, err = parseCJSModuleExports(wd, path.Join(wd, "node_modules", pkg.Name, modulePath), nodeEnv)
		if err == nil && ret.Error != "" {
			err = fmt.Errorf("parseCJSModuleExports: %s", ret.Error)
		}
		if err != nil {
			return
		}
		reexport = ret.Reexport
		npm.Main = npm.Module
		npm.Module = ""
		esm.HasExportDefault = ret.ExportDefault
		esm.NamedExports = ret.Exports
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
		var ret cjsExportsResult
		ret, err = parseCJSModuleExports(wd, pkg.ImportPath(), nodeEnv)
		if err == nil && ret.Error != "" {
			err = fmt.Errorf("parseCJSModuleExports: %s", ret.Error)
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

func (task *BuildTask) fixNpmPackage(p NpmPackage) NpmPackage {
	if task.Pkg.FromGithub {
		p.Name = task.Pkg.Name
		p.Version = task.Pkg.Version
	}
	if exports := p.DefinedExports; exports != nil {
		if m, ok := exports.(map[string]interface{}); ok {
			v, ok := m["."]
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
				task.applyConditions(&p, m, p.Type)
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

	browserMain := p.Browser["."]
	if browserMain != "" && fileExists(path.Join(nmDir, p.Name, browserMain)) {
		isEsm, _, _ := validateJS(path.Join(nmDir, p.Name, browserMain))
		if isEsm {
			log.Infof("%s@%s: use `browser` field as module: %s", p.Name, p.Version, browserMain)
			p.Module = browserMain
		}
	}

	if p.Types == "" && p.Typings != "" {
		p.Types = p.Typings
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
func (task *BuildTask) applyConditions(p *NpmPackage, exports interface{}, pType string) {
	s, ok := exports.(string)
	if ok {
		if pType == "module" {
			p.Module = s
		} else {
			p.Main = s
		}
		return
	}

	m, ok := exports.(map[string]interface{})
	if ok {
		targetConditions := []string{"browser"}
		conditions := []string{"import", "module", "es2015"}
		switch task.Target {
		case "deno", "denonext":
			targetConditions = []string{"deno", "worker", "browser"}
			// priority use `node` condition for solid.js (< 1.5.6) in deno
			if (p.Name == "solid-js" || strings.HasPrefix(p.Name, "solid-js/")) && semverLessThan(p.Version, "1.5.6") {
				targetConditions = []string{"node"}
			}
		case "node":
			targetConditions = []string{"node"}
		}
		_, hasRequireCondition := m["require"]
		_, hasNodeCondition := m["node"]
		if pType == "module" || hasRequireCondition || hasNodeCondition {
			conditions = append(conditions, "default")
		}
		if task.Dev {
			targetConditions = append(targetConditions, "development")
		}
		if task.conditions.Size() > 0 {
			targetConditions = append(task.conditions.Values(), targetConditions...)
		}
		for _, condition := range append(targetConditions, conditions...) {
			v, ok := m[condition]
			if ok {
				task.applyConditions(p, v, "module")
				break
			}
		}
		if p.Module == "" {
			conditions := []string{"require", "node", "default"}
			for _, condition := range append(targetConditions, conditions...) {
				v, ok := m[condition]
				if ok {
					task.applyConditions(p, v, "")
					break
				}
			}
		}
		for key, value := range m {
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
			_, err = fs.Stat(path.Join("builds", id))
			if err == nil {
				return &esm, true
			}
		}

		// delete the invalid db entry
		db.Delete(id)
	}
	return nil, false
}

func resovleESModule(wd string, packageName string, moduleSpecifier string) (resolvedName string, namedExports []string, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	switch path.Ext(moduleSpecifier) {
	case ".mjs", ".js", ".jsx", ".mts", ".ts", ".tsx":
		resolvedName = moduleSpecifier
	default:
		resolvedName = moduleSpecifier + ".mjs"
		if !fileExists(path.Join(pkgDir, resolvedName)) {
			resolvedName = moduleSpecifier + ".js"
		}
		if !fileExists(path.Join(pkgDir, resolvedName)) && dirExists(path.Join(pkgDir, moduleSpecifier)) {
			resolvedName = path.Join(moduleSpecifier, "index.mjs")
			if !fileExists(path.Join(pkgDir, resolvedName)) {
				resolvedName = path.Join(moduleSpecifier, "index.js")
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
