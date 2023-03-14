package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/ije/gox/utils"
)

// ESM defines the ES Module meta
type ESM struct {
	Exports       []string `json:"-"`
	ExportDefault bool     `json:"d"`
	CJS           bool     `json:"c"`
	TypesOnly     bool     `json:"o"`
	Dts           string   `json:"t"`
	PackageCSS    bool     `json:"s"`
}

func initModule(wd string, pkg Pkg, target string, isDev bool) (esm *ESM, npm NpmPackage, err error) {
	var p NpmPackage
	err = utils.ParseJSONFile(path.Join(wd, "node_modules", pkg.Name, "package.json"), &p)
	if err != nil {
		return
	}

	npm = fixNpmPackage(wd, p, target, isDev)
	esm = &ESM{}

	defer func() {
		esm.CJS = npm.Main != "" && npm.Module == ""
		esm.TypesOnly = isTypesOnlyPackage(npm)
	}()

	nodeEnv := "production"
	if isDev {
		nodeEnv = "development"
	}

	if pkg.Submodule != "" {
		if strings.HasSuffix(pkg.Submodule, ".d.ts") {
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
				np := fixNpmPackage(wd, p, target, isDev)
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
				if p.DefinedExports != nil {
					if m, ok := p.DefinedExports.(map[string]interface{}); ok {
						for name, defines := range m {
							if name == "./"+pkg.Submodule || name == "./"+pkg.Submodule+".js" {
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
								resolvePackageExports(&npm, defines, target, isDev, npm.Type)
								resolved = true
								break
							} else if strings.HasSuffix(name, "/*") && strings.HasPrefix("./"+pkg.Submodule, strings.TrimSuffix(name, "*")) {
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
									resolvePackageExports(&npm, defines, target, isDev, npm.Type)
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
		modulePath, exportDefault, erro := resovleESModule(wd, npm.Name, npm.Module)
		if erro == nil {
			npm.Module = modulePath
			esm.ExportDefault = exportDefault
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
		npm.Main = npm.Module
		npm.Module = ""
		esm.ExportDefault = ret.ExportDefault
		esm.Exports = ret.Exports
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
			err = runYarnAdd(wd, false, pkgs...)
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
		esm.ExportDefault = ret.ExportDefault
		esm.Exports = ret.Exports
	}
	return
}

func queryESMBuild(id string) (*ESM, bool) {
	value, err := db.Get(id)
	if err == nil && value != nil {
		var esm ESM
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

func resovleESModule(wd string, packageName string, moduleSpecifier string) (resolveName string, hasDefaultExport bool, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	switch path.Ext(moduleSpecifier) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		resolveName = moduleSpecifier
	default:
		resolveName = moduleSpecifier + ".mjs"
		if !fileExists(path.Join(pkgDir, resolveName)) {
			resolveName = moduleSpecifier + ".js"
		}
		if !fileExists(path.Join(pkgDir, resolveName)) && dirExists(path.Join(pkgDir, moduleSpecifier)) {
			resolveName = path.Join(moduleSpecifier, "index.mjs")
			if !fileExists(path.Join(pkgDir, resolveName)) {
				resolveName = path.Join(moduleSpecifier, "index.js")
			}
		}
	}

	isESM, _hasDefaultExport, err := validateJS(path.Join(pkgDir, resolveName))
	if err != nil {
		return
	}

	if !isESM {
		err = errors.New("not a module")
		return
	}

	hasDefaultExport = _hasDefaultExport
	return
}

func fixNpmPackage(wd string, p NpmPackage, target string, isDev bool) NpmPackage {
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
				resolvePackageExports(&p, v, target, isDev, p.Type)
			} else {
				/*
					exports: {
						"require": "./cjs/index.js",
						"import": "./esm/index.js"
					}
				*/
				resolvePackageExports(&p, m, target, isDev, p.Type)
			}
		} else if s, ok := exports.(string); ok {
			/*
			  exports: "./esm/index.js"
			*/
			resolvePackageExports(&p, s, target, isDev, p.Type)
		}
	}

	nmDir := path.Join(wd, "node_modules")
	if p.Module == "" {
		if p.JsNextMain != "" && fileExists(path.Join(nmDir, p.Name, p.JsNextMain)) {
			p.Module = p.JsNextMain
		} else if p.ES2015 != "" && fileExists(path.Join(nmDir, p.Name, p.ES2015)) {
			p.Module = p.ES2015
		} else if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs") || strings.HasSuffix(p.Main, ".esm.js") || strings.Contains(p.Main, "/esm/") || strings.Contains(p.Main, "/es/")) {
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
