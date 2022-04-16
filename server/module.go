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

type OldModule struct {
	*NpmPackage
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Dts           string   `json:"dts"`
	PackageCSS    bool     `json:"packageCSS"`
}

// ESM defines the ES Module meta
type Module struct {
	Name          string   `json:"n"`
	Version       string   `json:"v"`
	CJS           bool     `json:"c"`
	ExportDefault bool     `json:"d"`
	Exports       []string `json:"-"`
	TypesOnly     bool     `json:"o"`
	Dts           string   `json:"t"`
	PackageCSS    bool     `json:"s"`
}

func initModule(wd string, pkg Pkg, target string, isDev bool) (esm *Module, npm *NpmPackage, err error) {
	packageDir := path.Join(wd, "node_modules", pkg.Name)
	packageFile := path.Join(packageDir, "package.json")

	var p NpmPackage
	err = utils.ParseJSONFile(packageFile, &p)
	if err != nil {
		return
	}

	npm = fixNpmPackage(p, target, isDev)
	esm = &Module{
		Name:    npm.Name,
		Version: npm.Version,
	}

	defer func() {
		esm.CJS = npm.Module == ""
		esm.TypesOnly = npm.Module == "" && npm.Main == "" && npm.Types != ""
	}()

	if pkg.Submodule != "" {
		if strings.HasSuffix(pkg.Submodule, ".d.ts") {
			if strings.HasSuffix(pkg.Submodule, "~.d.ts") {
				submodule := strings.TrimSuffix(pkg.Submodule, "~.d.ts")
				subDir := path.Join(wd, "node_modules", esm.Name, submodule)
				if fileExists(path.Join(subDir, "index.d.ts")) {
					npm.Types = path.Join(submodule, "index.d.ts")
				} else if fileExists(path.Join(subDir + ".d.ts")) {
					npm.Types = submodule + ".d.ts"
				}
			} else {
				npm.Types = pkg.Submodule
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
								resolvePackageExports(npm, defines, target, isDev)
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
									resolvePackageExports(npm, defines, target, isDev)
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
		modulePath, exportDefault, err := checkESM(wd, npm.Name, npm.Module)
		if err == nil {
			npm.Module = modulePath
			esm.ExportDefault = exportDefault
		} else {
			log.Warnf("fake module from '%s' of '%s': %v", npm.Module, npm.Name, err)
			npm.Main = npm.Module
			npm.Module = ""
		}
	}

	if npm.Module == "" && npm.Main != "" {
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

	if path.Dir(npm.Main) != "" && npm.Types == "" {
		typesPath := path.Join(path.Dir(npm.Main), "index.d.ts")
		if fileExists(path.Join(packageDir, typesPath)) {
			npm.Types = typesPath
		}
	}

	return
}

func findModule(id string) (esm *Module, err error) {
	store, _, err := db.Get(id)
	if err == nil {
		err = json.Unmarshal([]byte(store["esm"]), &esm)
		if err == nil && esm.Name == "" {
			var old OldModule
			err = json.Unmarshal([]byte(store["esm"]), &old)
			if err == nil && old.Name != "" {
				esm = &Module{
					Name:          old.Name,
					Version:       old.Version,
					CJS:           old.Module == "" && old.Main != "",
					ExportDefault: old.ExportDefault,
					TypesOnly:     old.Module == "" && old.Main == "" && old.Types != "",
					Dts:           old.Dts,
					PackageCSS:    old.PackageCSS,
				}
				// update db
				db.Put(id, "build", storage.Store{
					"esm": string(utils.MustEncodeJSON(esm)),
				})
			}
		}
		if err != nil || esm.Name == "" {
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
