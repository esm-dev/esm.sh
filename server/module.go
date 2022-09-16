package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"esm.sh/server/storage"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/gox/utils"
)

// ESM defines the ES Module meta
type ModuleMeta struct {
	Exports       []string `json:"-"`
	ExportDefault bool     `json:"d"`
	CJS           bool     `json:"c"`
	TypesOnly     bool     `json:"o"`
	Dts           string   `json:"t"`
	PackageCSS    bool     `json:"s"`
}

func initModule(wd string, pkg Pkg, target string, isDev bool) (esm *ModuleMeta, npm *NpmPackage, err error) {
	packageDir := path.Join(wd, "node_modules", pkg.Name)
	packageFile := path.Join(packageDir, "package.json")

	var p NpmPackage
	err = utils.ParseJSONFile(packageFile, &p)
	if err != nil {
		return
	}

	npm = fixNpmPackage(wd, p, target, isDev)
	esm = &ModuleMeta{}

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

	if npm.Module == "" && npm.Main != "" && (strings.Contains(npm.Main, "/npm/") || strings.Contains(npm.Main, "/es/") || strings.HasSuffix(npm.Main, ".mjs")) {
		npm.Module = npm.Main
	}

	if npm.Module != "" {
		modulePath, exportDefault, reason := checkESM(wd, npm.Name, npm.Module)
		if reason == nil {
			npm.Module = modulePath
			esm.ExportDefault = exportDefault
		} else if reason.Error() == "not a module" {
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
			err = fmt.Errorf("checkESM: %s", reason)
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

func findModule(id string) (esm *ModuleMeta, err error) {
	store, _, err := db.Get(id)
	if err == nil {
		if v, ok := store["meta"]; ok {
			err = json.Unmarshal([]byte(v), &esm)
		} else if v, ok := store["esm"]; ok {
			var old OldMeta
			err = json.Unmarshal([]byte(v), &old)
			if err == nil {
				esm = &ModuleMeta{
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

func checkESM(wd string, packageName string, moduleSpecifier string) (resolveName string, exportDefault bool, err error) {
	pkgDir := path.Join(wd, "node_modules", packageName)
	resolveName = moduleSpecifier
	switch path.Ext(moduleSpecifier) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
	default:
		resolveName = moduleSpecifier + ".js"
		if !fileExists(path.Join(pkgDir, resolveName)) {
			resolveName = moduleSpecifier + ".mjs"
		}
	}
	if !fileExists(path.Join(pkgDir, resolveName)) && dirExists(path.Join(pkgDir, moduleSpecifier)) {
		resolveName = path.Join(moduleSpecifier, "index.js")
		if !fileExists(path.Join(pkgDir, resolveName)) {
			resolveName = path.Join(moduleSpecifier, "index.mjs")
		}
	}
	filename := path.Join(pkgDir, resolveName)
	switch path.Ext(filename) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
	default:
		filename += ".js"
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		Contents:       string(data),
		IdentifierName: "stdin",
	}, js_parser.Options{})
	if pass {
		esm := ast.ExportsKind == js_ast.ExportsESM
		if !esm {
			err = errors.New("not a module")
			return
		}
		for name := range ast.NamedExports {
			if name == "default" {
				exportDefault = true
				break
			}
		}
	}
	return
}
