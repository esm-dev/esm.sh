package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"esm.sh/server/storage"

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

func initModule(wd string, pkg Pkg, target string, isDev bool) (esm *ESM, npm *NpmPackage, err error) {
	packageDir := path.Join(wd, "node_modules", pkg.Name)
	packageFile := path.Join(packageDir, "package.json")

	var p NpmPackage
	err = utils.ParseJSONFile(packageFile, &p)
	if err != nil {
		return
	}

	npm = fixNpmPackage(wd, &p, target, isDev)
	esm = &ESM{}

	defer func() {
		esm.CJS = npm.Module == ""
		esm.TypesOnly = npm.Module == "" && npm.Main == "" && npm.Types != ""
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
				np := fixNpmPackage(wd, &p, target, isDev)
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
							if name == "./"+pkg.Submodule {
								/**
								  exports: {
								    "./lib/core": {
								      "require": "./lib/core.js",
								      "import": "./es/core.js"
								    }
								  }
								*/
								resolvePackageExports(npm, defines, target, isDev, npm.Type)
								resolved = true
								break
							} else if strings.HasSuffix(name, "/*") && strings.HasPrefix("./"+pkg.Submodule, strings.TrimSuffix(name, "*")) {
								/**
								  exports: {
								    "./lib/languages/*": {
								      "require": "./lib/languages/*.js",
								      "import": "./es/languages/*.js"
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
									}
									defines = newDefines
								} else if s, ok := defines.(string); ok && name != s {
									defines = strings.Replace(s, "*", suffix, -1)
									hasDefines = true
								}
								if hasDefines {
									resolvePackageExports(npm, defines, target, isDev, npm.Type)
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

	if target == "types" {
		return
	}

	if npm.Main == "" && npm.Module == "" {
		if fileExists(path.Join(packageDir, "index.mjs")) {
			npm.Module = "./index.mjs"
		} else if fileExists(path.Join(packageDir, "index.js")) {
			npm.Main = "./index.js"
		} else if fileExists(path.Join(packageDir, "index.cjs")) {
			npm.Main = "./index.cjs"
		}
	}

	// for pure types packages
	if npm.Main == "" && npm.Module == "" && npm.Types != "" {
		return
	}

	if npm.Module != "" {
		modulePath, exportDefault, erro := parseESModule(wd, npm.Name, npm.Module)
		if erro == nil {
			npm.Module = modulePath
			esm.ExportDefault = exportDefault
		} else if erro.Error() == "not a module" {
			var ret cjsExportsResult
			ret, err = parseCJSModuleExports(wd, path.Join(pkg.Name, strings.TrimSuffix(npm.Module, ".js")), nodeEnv)
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
			log.Warnf("fake module from '%s' of '%s'", npm.Main, npm.Name)
		} else {
			err = fmt.Errorf("parseESModule: %s", erro)
			return
		}
	} else if npm.Main != "" {
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
		// if ret.Error != "" && strings.Contains(ret.Error, "Unexpected export statement in CJS module") {
		//   if pkg.Submodule != "" {
		//     esm.Module = pkg.Submodule
		//   } else {
		//     esm.Module = esm.Main
		//   }
		//   resolved, exportDefault, err := checkESM(wd, esm.Name, esm.Module)
		//   if err != nil {
		//     return nil, err
		//   }
		//   esm.Module = resolved
		//   esm.ExportDefault = exportDefault
		// }
	}

	if path.Dir(npm.Main) != "" && npm.Types == "" {
		typesPath := path.Join(path.Dir(npm.Main), "index.d.ts")
		if fileExists(path.Join(packageDir, typesPath)) {
			npm.Types = typesPath
		}
	}

	return
}

type OldMeta struct {
	*NpmPackage
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Dts           string   `json:"dts"`
	PackageCSS    bool     `json:"packageCSS"`
}

func findModule(id string) (esm *ESM, err error) {
	store, _, err := db.Get(id)
	if err == nil {
		if v, ok := store["meta"]; ok {
			err = json.Unmarshal([]byte(v), &esm)
		} else if v, ok := store["esm"]; ok {
			var old OldMeta
			err = json.Unmarshal([]byte(v), &old)
			if err == nil {
				esm = &ESM{
					CJS:           old.Module == "" && old.Main != "",
					ExportDefault: old.ExportDefault,
					TypesOnly:     old.Module == "" && old.Main == "" && old.Types != "",
					Dts:           old.Dts,
					PackageCSS:    old.PackageCSS,
				}
			}
		} else {
			err = fmt.Errorf("bad data")
		}
		if err != nil {
			db.Delete(id)
			err = storage.ErrNotFound
			return
		}

		var exists bool
		exists, _, _, err = fs.Exists(path.Join("builds", id))
		if err == nil && !exists {
			db.Delete(id)
			esm = nil
			err = storage.ErrNotFound
			return
		}
	}
	return
}

func parseESModule(wd string, packageName string, moduleSpecifier string) (resolveName string, hasDefaultExport bool, err error) {
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

	isESM, _hasDefaultExport, err := parseJS(path.Join(pkgDir, resolveName))
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

func fixNpmPackage(wd string, np *NpmPackage, target string, isDev bool) *NpmPackage {
	exports := np.DefinedExports

	if exports != nil {
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
				resolvePackageExports(np, v, target, isDev, np.Type)
			} else {
				/*
					exports: {
						"require": "./cjs/index.js",
						"import": "./esm/index.js"
					}
				*/
				resolvePackageExports(np, m, target, isDev, np.Type)
			}
		} else if s, ok := exports.(string); ok {
			/*
			  exports: "./esm/index.js"
			*/
			resolvePackageExports(np, s, target, isDev, np.Type)
		}
	}

	nmDir := path.Join(wd, "node_modules")
	if np.Module == "" {
		if np.JsNextMain != "" && fileExists(path.Join(nmDir, np.Name, np.JsNextMain)) {
			np.Module = np.JsNextMain
		} else if np.ES2015 != "" && fileExists(path.Join(nmDir, np.Name, np.ES2015)) {
			np.Module = np.ES2015
		} else if np.Main != "" && (np.Type == "module" || strings.HasSuffix(np.Main, ".mjs") || strings.HasSuffix(np.Main, ".esm.js") || strings.Contains(np.Main, "/esm/") || strings.Contains(np.Main, "/es/")) {
			np.Module = np.Main
		}
	}

	if np.Browser != "" && fileExists(path.Join(nmDir, np.Name, np.Browser)) {
		isEsm, _, _ := parseJS(path.Join(nmDir, np.Name, np.Browser))
		if isEsm {
			log.Infof("%s@%s: use `browser` field as module: %s", np.Name, np.Version, np.Browser)
			np.Module = np.Browser
		}
	}

	if np.Types == "" && np.Typings != "" {
		np.Types = np.Typings
	}

	return np
}
