package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"time"

	"esm.sh/server/storage"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/esbuild-internal/test"
	"github.com/ije/gox/utils"
)

// ESM defines the ES Module meta
type Module struct {
	*NpmPackage
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Dts           string   `json:"dts"`
	PackageCSS    bool     `json:"packageCSS"`
}

func initModule(wd string, pkg Pkg, target string, isDev bool) (esm *Module, err error) {
	packageDir := path.Join(wd, "node_modules", pkg.Name)
	packageFile := path.Join(packageDir, "package.json")

	var p NpmPackage
	err = utils.ParseJSONFile(packageFile, &p)
	if err != nil {
		return
	}

	esm = &Module{
		NpmPackage: fixNpmPackage(p, target, isDev),
	}

	if pkg.Submodule != "" {
		if strings.HasSuffix(pkg.Submodule, ".d.ts") {
			if strings.HasSuffix(pkg.Submodule, "~.d.ts") {
				submodule := strings.TrimSuffix(pkg.Submodule, "~.d.ts")
				subDir := path.Join(wd, "node_modules", esm.Name, submodule)
				if fileExists(path.Join(subDir, "index.d.ts")) {
					esm.Types = path.Join(submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					esm.Types = submodule + ".d.ts"
				}
			} else {
				esm.Types = pkg.Submodule
			}
		} else {
			subDir := path.Join(wd, "node_modules", esm.Name, pkg.Submodule)
			packageFile := path.Join(subDir, "package.json")
			if fileExists(packageFile) {
				var p NpmPackage
				err = utils.ParseJSONFile(packageFile, &p)
				if err != nil {
					return
				}
				np := fixNpmPackage(p, target, isDev)
				if np.Module != "" {
					esm.Module = path.Join(pkg.Submodule, np.Module)
				} else {
					esm.Module = ""
				}
				if p.Main != "" {
					esm.Main = path.Join(pkg.Submodule, p.Main)
				} else {
					esm.Main = path.Join(pkg.Submodule, "index.js")
				}
				esm.Types = ""
				if p.Types != "" {
					esm.Types = path.Join(pkg.Submodule, p.Types)
				} else if p.Typings != "" {
					esm.Types = path.Join(pkg.Submodule, p.Typings)
				} else if fileExists(path.Join(subDir, "index.d.ts")) {
					esm.Types = path.Join(pkg.Submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					esm.Types = pkg.Submodule + ".d.ts"
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
								resolvePackageExports(esm.NpmPackage, defines, target, isDev)
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
									resolvePackageExports(esm.NpmPackage, defines, target, isDev)
									resolved = true
								}
							}
						}
					}
				}
				if !resolved {
					if esm.Type == "module" || esm.Module != "" {
						// follow main module type
						esm.Module = pkg.Submodule
					} else {
						esm.Main = pkg.Submodule
					}
					esm.Types = ""
					if fileExists(path.Join(subDir, "index.d.ts")) {
						esm.Types = path.Join(pkg.Submodule, "index.d.ts")
					} else if fileExists(path.Join(subDir + ".d.ts")) {
						esm.Types = pkg.Submodule + ".d.ts"
					}
				}
			}
		}
	}

	if target == "types" {
		return
	}

	if esm.Main == "" && esm.Module == "" {
		if fileExists(path.Join(packageDir, "index.mjs")) {
			esm.Module = "./index.mjs"
		} else if fileExists(path.Join(packageDir, "index.js")) {
			esm.Main = "./index.js"
		}
	}

	// for pure types packages
	if esm.Main == "" && esm.Module == "" && esm.Types != "" {
		return
	}

	if esm.Module == "" && esm.Main != "" && (strings.Contains(esm.Main, "/esm/") || strings.Contains(esm.Main, "/es/") || strings.HasSuffix(esm.Main, ".mjs")) {
		esm.Module = esm.Main
	}

	if esm.Module != "" {
		modulePath, exportDefault, err := checkESM(wd, esm.Name, esm.Module)
		if err == nil {
			esm.Module = modulePath
			esm.ExportDefault = exportDefault
		} else {
			log.Warnf("fake module from '%s' of '%s': %v", esm.Module, esm.Name, err)
			esm.Main = esm.Module
			esm.Module = ""
		}
	}

	if esm.Module == "" && esm.Main != "" {
		nodeEnv := "production"
		if isDev {
			nodeEnv = "development"
		}
		for i := 0; i < 3; i++ {
			var ret cjsExportsResult
			ret, err = parseCJSModuleExports(wd, pkg.ImportPath(), nodeEnv)
			if err != nil {
				return
			}
			if ret.Error == "" {
				esm.ExportDefault = ret.ExportDefault
				esm.Exports = ret.Exports
				break
			}
			err = fmt.Errorf("parseCJSModuleExports: %s", ret.Error)
			if i == 2 || !strings.Contains(ret.Error, "Can't resolve") {
				return
			}
			// retry after 50ms
			time.Sleep(50 * time.Millisecond)
		}
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

	if path.Dir(esm.Main) != "" && esm.Types == "" {
		typesPath := path.Join(path.Dir(esm.Main), "index.d.ts")
		if fileExists(path.Join(packageDir, typesPath)) {
			esm.Types = typesPath
		}
	}

	return
}

func findModule(id string) (esm *Module, err error) {
	store, _, err := db.Get(id)
	if err == nil {
		err = json.Unmarshal([]byte(store["esm"]), &esm)
		if err != nil {
			db.Delete(id)
			err = storage.ErrNotFound
			return
		}

		var exists bool
		exists, _, err = fs.Exists(path.Join("builds", id))
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
	if dirExists(path.Join(pkgDir, moduleSpecifier)) {
		f := path.Join(moduleSpecifier, "index.mjs")
		if !fileExists(path.Join(pkgDir, f)) {
			f = path.Join(moduleSpecifier, "index.js")
		}
		moduleSpecifier = f
	}
	filename := path.Join(pkgDir, moduleSpecifier)
	switch path.Ext(filename) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
	default:
		filename += ".js"
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug)
	ast, pass := js_parser.Parse(log, test.SourceForTest(string(data)), js_parser.Options{})
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
	resolveName = moduleSpecifier
	return
}
