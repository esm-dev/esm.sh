package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/esbuild-internal/test"
	"github.com/ije/gox/utils"
	"github.com/postui/postdb/q"
)

// ESM defines the ES Module meta
type ESM struct {
	*NpmPackage
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Dts           string   `json:"dts"`
}

func initESM(wd string, pkg pkg, deps pkgSlice) (esm *ESM, err error) {
	versions := map[string]string{}
	for _, dep := range deps {
		versions[dep.name] = dep.version
	}

	packageFile := path.Join(wd, "node_modules", pkg.name, "package.json")
	install := !fileExists(packageFile)
	if install {
		installList := []string{
			fmt.Sprintf("%s@%s", pkg.name, pkg.version),
		}
		if !strings.HasPrefix(pkg.name, "@") {
			var info NpmPackage
			info, _, err = node.getPackageInfo("@types/"+pkg.name, "latest")
			if err != nil && err.Error() != fmt.Sprintf("npm: package '@types/%s' not found", pkg.name) {
				return
			}
			if info.Types != "" || info.Typings != "" || info.Main != "" {
				if version, ok := versions[info.Name]; ok {
					installList = append(installList, fmt.Sprintf("%s@%s", info.Name, version))
				} else {
					installList = append(installList, fmt.Sprintf("%s@%s", info.Name, info.Version))
				}
			}
		}
		err = yarnAdd(wd, installList...)
		if err != nil {
			return
		}
	}

	var p NpmPackage
	err = utils.ParseJSONFile(packageFile, &p)
	if err != nil {
		return
	}

	esm = &ESM{
		NpmPackage: fixNpmPackage(p),
	}

	if pkg.submodule != "" {
		packageFile := path.Join(wd, "node_modules", esm.Name, pkg.submodule, "package.json")
		if fileExists(packageFile) {
			var p NpmPackage
			err = utils.ParseJSONFile(packageFile, &p)
			if err != nil {
				return
			}
			if p.Main != "" {
				esm.Main = path.Join(pkg.submodule, p.Main)
			} else {
				esm.Main = ""
			}
			np := fixNpmPackage(p)
			if np.Module != "" {
				esm.Module = path.Join(pkg.submodule, np.Module)
			} else {
				esm.Module = ""
			}
			if p.Types != "" {
				esm.Types = path.Join(pkg.submodule, p.Types)
			} else {
				esm.Types = ""
			}
			if p.Typings != "" {
				esm.Typings = path.Join(pkg.submodule, p.Typings)
			} else {
				esm.Typings = ""
			}
		} else {
			var defined bool
			if p.DefinedExports != nil {
				if m, ok := p.DefinedExports.(map[string]interface{}); ok {
					for name, v := range m {
						/**
						exports: {
							"./lib/core": {
								"require": "./lib/core.js",
								"import": "./es/core.js"
							}
						}
						*/
						if name == "./"+pkg.submodule {
							useDefinedExports(esm.NpmPackage, v)
							defined = true
							break
							/**
							exports: {
								"./lib/languages/*": {
									"require": "./lib/languages/*.js",
									"import": "./es/languages/*.js"
								},
							}
							*/
						} else if strings.HasSuffix(name, "/*") && strings.HasPrefix("./"+pkg.submodule, strings.TrimSuffix(name, "*")) {
							suffix := strings.TrimPrefix("./"+pkg.submodule, strings.TrimSuffix(name, "*"))
							if m, ok := v.(map[string]interface{}); ok {
								for key, value := range m {
									s, ok := value.(string)
									if ok {
										m[key] = strings.Replace(s, "*", suffix, -1)
									}
								}
							}
							useDefinedExports(esm.NpmPackage, v)
							defined = true
						}
					}
				}
			}
			if !defined {
				if esm.Main != "" {
					esm.Main = path.Join(path.Dir(esm.Main), pkg.submodule)
				} else {
					esm.Main = "./" + pkg.submodule
				}
				if esm.Module != "" {
					esm.Module = path.Join(path.Dir(esm.Module), pkg.submodule)
				}
				esm.Types = ""
				esm.Typings = ""
			}
		}
	}

	if esm.Module == "" && strings.HasSuffix(esm.Main, ".mjs") {
		esm.Module = esm.Main
	}

	if esm.Module != "" {
		resolved, exportDefault, err := checkESM(wd, esm.Name, esm.Module)
		if err != nil {
			log.Warnf("fake module from '%s' of %s: %v", esm.Module, esm.Name, err)
			esm.Module = ""
		} else {
			esm.Module = resolved
			esm.ExportDefault = exportDefault
		}
	}

	if esm.Module == "" {
		ret, err := parseCJSModuleExports(wd, pkg.ImportPath())
		if err != nil {
			return nil, fmt.Errorf("parseCJSModuleExports: %v", err)
		}
		if strings.Contains(ret.Error, "Unexpected export statement in CJS module") {
			if pkg.submodule != "" {
				esm.Module = pkg.submodule
			} else {
				esm.Module = esm.Main
			}
			resolved, exportDefault, err := checkESM(wd, esm.Name, esm.Module)
			if err != nil {
				return nil, err
			}
			esm.Module = resolved
			esm.ExportDefault = exportDefault
		} else {
			esm.Exports = ret.Exports
			esm.ExportDefault = true
		}
	}
	return
}

func findESM(id string) (esm *ESM, pkgCSS bool, ok bool) {
	post, err := db.Get(q.Alias(id), q.Select("esm", "css"))
	if err == nil {
		err = json.Unmarshal(post.KV["esm"], &esm)
		if err != nil {
			db.Delete(q.Alias(id))
			return
		}

		if !fileExists(path.Join(config.storageDir, "builds", id)) {
			db.Delete(q.Alias(id))
			return
		}

		if val := post.KV["css"]; len(val) == 1 && val[0] == 1 {
			pkgCSS = fileExists(path.Join(config.storageDir, "builds", strings.TrimSuffix(id, ".js")+".css"))
		}
		ok = true
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
	log := logger.NewDeferLog()
	ast, pass := js_parser.Parse(log, test.SourceForTest(string(data)), js_parser.Options{})
	if pass {
		esm := ast.ExportsKind == js_ast.ExportsESM
		if !esm {
			err = errors.New("not module")
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
