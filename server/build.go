package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/postui/postdb"
	"github.com/postui/postdb/q"
)

type buildTask struct {
	id     string
	wd     string
	pkg    pkg
	deps   pkgSlice
	target string
	isDev  bool
}

func (task *buildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.pkg
	target := task.target
	filename := path.Base(pkg.name)
	if pkg.submodule != "" {
		filename = pkg.submodule
	}
	if task.isDev {
		filename += ".development"
	}
	if len(task.deps) > 0 {
		sort.Sort(task.deps)
		target = fmt.Sprintf("deps=%s/%s", strings.ReplaceAll(task.deps.String(), "/", "_"), target)
	}
	task.id = fmt.Sprintf(
		"v%d/%s@%s/%s/%s",
		VERSION,
		pkg.name,
		pkg.version,
		target,
		filename,
	)
	return task.id
}

func (task *buildTask) buildESM() (esm *ESMeta, packageCSS bool, err error) {
	hasher := sha1.New()
	hasher.Write([]byte(task.ID()))
	task.wd = path.Join(os.TempDir(), "esm-build-"+hex.EncodeToString(hasher.Sum(nil)))
	ensureDir(task.wd)
	defer os.RemoveAll(task.wd)

	esmeta, err := initBuild(task.wd, task.pkg, true)
	if err != nil {
		return
	}

	start := time.Now()
	buf := bytes.NewBuffer(nil)
	exports := newStringSet()
	hasDefaultExport := false
	importPath := task.pkg.ImportPath()
	env := "production"
	if task.isDev {
		env = "development"
	}
	for _, name := range esmeta.Exports {
		if name == "default" {
			hasDefaultExport = true
		} else if name != "import" {
			exports.Add(name)
		}
	}
	if esmeta.Module != "" {
		if exports.Size() > 0 {
			fmt.Fprintf(buf, `export {%s} from "%s";%s`, strings.Join(exports.Values(), ","), importPath, "\n")
		}
		if hasDefaultExport {
			fmt.Fprintf(buf, `export {default} from "%s";`, importPath)
		}
	} else {
		if exports.Size() > 0 {
			fmt.Fprintf(buf, `export {%s,default} from "%s";%s`, strings.Join(exports.Values(), ","), importPath, "\n")
		} else {
			fmt.Fprintf(buf, `export {default} from "%s";`, importPath)
		}
	}
	input := &api.StdinOptions{
		Contents:   buf.String(),
		ResolveDir: task.wd,
		Sourcefile: "export.js",
	}
	minify := !task.isDev
	define := map[string]string{
		"__filename":                  fmt.Sprintf(`"https://%s/%s.js"`, config.domain, task.ID()),
		"__dirname":                   fmt.Sprintf(`"https://%s/%s"`, config.domain, path.Dir(task.ID())),
		"process":                     "__process$",
		"Buffer":                      "__Buffer$",
		"setImmediate":                "__setImmediate$",
		"clearImmediate":              "clearTimeout",
		"require.resolve":             "__rResolve$",
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, env),
		"global":                      "__global$",
		"global.process":              "__process$",
		"global.Buffer":               "__Buffer$",
		"global.setImmediate":         "__setImmediate$",
		"global.clearImmediate":       "clearTimeout",
		"global.require.resolve":      "__rResolve$",
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	external := newStringSet()
	esmResolverPlugin := api.Plugin{
		Name: "esm-resolver",
		Setup: func(plugin api.PluginBuild) {
			plugin.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					p := args.Path
					importName := task.pkg.name
					if smod := task.pkg.submodule; smod != "" {
						importName += "/" + smod
					}
					// bundle modules:
					// 1. the package self
					// 2. submodules of the package
					// 3. submodules of other packages
					if p == importName ||
						isFileImportPath(p) ||
						(!strings.HasPrefix(p, "@") && len(strings.Split(p, "/")) > 1) ||
						(strings.HasPrefix(p, "@") && len(strings.Split(p, "/")) > 2) {
						return api.OnResolveResult{}, nil
					}
					external.Add(p)
					return api.OnResolveResult{Path: "esm_sh_external://" + p, External: true}, nil
				},
			)
		},
	}
	result := api.Build(api.BuildOptions{
		Stdin:             input,
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Target:            targets[task.target],
		Format:            api.FormatESModule,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Define:            define,
		Plugins:           []api.Plugin{esmResolverPlugin},
	})
	if len(result.Errors) > 0 {
		err = errors.New("esbuild: " + result.Errors[0].Text)
		return
	}
	for _, w := range result.Warnings {
		log.Warn(w.Text)
	}

	cssMark := []byte{0}
	for _, file := range result.OutputFiles {
		outputContent := file.Contents
		if strings.HasSuffix(file.Path, ".js") {
			jsHeader := bytes.NewBufferString(fmt.Sprintf(
				"/* esm.sh - esbuild bundle(%s) %s %s */\n",
				task.pkg.String(),
				strings.ToLower(task.target),
				env,
			))
			eol := "\n"
			if !task.isDev {
				eol = ""
			}

			// replace external imports/requires
			for _, name := range external.Values() {
				var importPath string
				if task.target == "deno" {
					_, yes := denoStdNodeModules[name]
					if yes {
						importPath = fmt.Sprintf("/v%d/_deno_std_node_%s.js", VERSION, name)
					}
				}
				if name == "buffer" {
					importPath = fmt.Sprintf("/v%d/_node_buffer.js", VERSION)
				}
				if importPath == "" {
					polyfill, ok := polyfilledBuiltInNodeModules[name]
					if ok {
						p, submodule, e := node.getPackageInfo(polyfill, "latest")
						if e == nil {
							filename := path.Base(p.Name)
							if submodule != "" {
								filename = submodule
							}
							if task.isDev {
								filename += ".development"
							}
							importPath = fmt.Sprintf(
								"/v%d/%s@%s/%s/%s.js",
								VERSION,
								p.Name,
								p.Version,
								task.target,
								filename,
							)
						} else {
							err = e
							return
						}
					} else {
						_, err := embedFS.Open(fmt.Sprintf("polyfills/node_%s.js", name))
						if err == nil {
							importPath = fmt.Sprintf("/v%d/_node_%s.js", VERSION, name)
						}
					}
				}
				if importPath == "" {
					packageFile := path.Join(task.wd, "node_modules", name, "package.json")
					if fileExists(packageFile) {
						var p NpmPackage
						if utils.ParseJSONFile(packageFile, &p) == nil {
							suffix := ".js"
							if task.isDev {
								suffix = ".development.js"
							}
							importPath = fmt.Sprintf(
								"/v%d/%s@%s/%s/%s%s",
								VERSION,
								p.Name,
								p.Version,
								task.target,
								path.Base(p.Name),
								suffix,
							)
						}
					}
				}
				if importPath == "" {
					version := "latest"
					for _, dep := range task.deps {
						if name == dep.name {
							version = dep.version
							break
						}
					}
					if version == "latest" {
						for n, v := range esmeta.Dependencies {
							if name == n {
								version = v
								break
							}
						}
					}
					if version == "latest" {
						for n, v := range esmeta.PeerDependencies {
							if name == n {
								version = v
								break
							}
						}
					}
					p, submodule, e := node.getPackageInfo(name, version)
					if e == nil {
						filename := path.Base(p.Name)
						if submodule != "" {
							filename = submodule
						}
						if task.isDev {
							filename += ".development"
						}
						importPath = fmt.Sprintf(
							"/v%d/%s@%s/%s/%s.js",
							VERSION,
							p.Name,
							p.Version,
							task.target,
							filename,
						)
					}
				}
				if importPath == "" {
					importPath = fmt.Sprintf("/_error.js?type=resolve&name=%s", name)
				}
				buf := bytes.NewBuffer(nil)
				identifier := identify(name)
				slice := bytes.Split(outputContent, []byte(fmt.Sprintf("\"esm_sh_external://%s\"", name)))
				commonjs := false
				commonjsImported := false
				for i, p := range slice {
					if commonjs {
						p = bytes.TrimPrefix(p, []byte{')'})
					}
					commonjs = bytes.HasSuffix(p, []byte("require("))
					if commonjs {
						p = bytes.TrimSuffix(p, []byte("require("))
						if !commonjsImported {
							wrote := false
							versionPrefx := fmt.Sprintf("/v%d/", VERSION)
							if strings.HasPrefix(importPath, versionPrefx) {
								pkg, err := parsePkg(strings.TrimPrefix(importPath, versionPrefx))
								if err == nil {
									// here the submodule should be always empty
									pkg.submodule = ""
									esmeta, err := initBuild(task.wd, *pkg, false)
									if err == nil {
										hasDefaultExport := false
										if len(esmeta.Exports) > 0 {
											for _, name := range esmeta.Exports {
												if name == "default" || name == "__esModule" {
													hasDefaultExport = true
													break
												}
											}
										} else {
											hasDefaultExport = true
										}
										if hasDefaultExport {
											fmt.Fprintf(jsHeader, `import __%s$ from "%s";%s`, identifier, importPath, eol)
										} else {
											fmt.Fprintf(jsHeader, `import * as __%s$ from "%s";%s`, identifier, importPath, eol)
										}
										wrote = true
									}
								}
							}
							if !wrote {
								fmt.Fprintf(jsHeader, `import __%s$ from "%s";%s`, identifier, importPath, eol)
							}
							commonjsImported = true
						}
					}
					buf.Write(p)
					if i < len(slice)-1 {
						if commonjs {
							buf.WriteString(fmt.Sprintf("__%s$", identifier))
						} else {
							buf.WriteString(fmt.Sprintf("\"%s\"", importPath))
						}
					}
				}
				outputContent = buf.Bytes()
			}

			// add nodejs/deno compatibility
			if bytes.Contains(outputContent, []byte("__process$")) {
				fmt.Fprintf(jsHeader, `import __process$ from "/v%d/_node_process.js";%s__process$.env.NODE_ENV="%s";%s`, VERSION, eol, env, eol)
			}
			if bytes.Contains(outputContent, []byte("__Buffer$")) {
				fmt.Fprintf(jsHeader, `import { Buffer as __Buffer$ } from "/v%d/_node_buffer.js";%s`, VERSION, eol)
			}
			if bytes.Contains(outputContent, []byte("__global$")) {
				fmt.Fprintf(jsHeader, `var __global$ = window;%s`, eol)
			}
			if bytes.Contains(outputContent, []byte("__setImmediate$")) {
				fmt.Fprintf(jsHeader, `var __setImmediate$ = (cb, args) => setTimeout(cb, 0, ...args);%s`, eol)
			}
			if bytes.Contains(outputContent, []byte("__rResolve$")) {
				fmt.Fprintf(jsHeader, `var __rResolve$ = p => p;%s`, eol)
			}

			saveFilePath := path.Join(config.storageDir, "builds", task.ID()+".js")
			ensureDir(path.Dir(saveFilePath))

			var file *os.File
			file, err = os.Create(saveFilePath)
			if err != nil {
				return
			}
			defer file.Close()

			_, err = io.Copy(file, jsHeader)
			if err != nil {
				return
			}

			_, err = io.Copy(file, bytes.NewReader(outputContent))
			if err != nil {
				return
			}
		} else if strings.HasSuffix(file.Path, ".css") {
			saveFilePath := path.Join(config.storageDir, "builds", task.ID()+".css")
			ensureDir(path.Dir(saveFilePath))
			file, e := os.Create(saveFilePath)
			if e != nil {
				err = e
				return
			}
			defer file.Close()

			_, err = io.Copy(file, bytes.NewReader(outputContent))
			if err != nil {
				return
			}
			cssMark = []byte{1}
		}
	}

	log.Debugf("esbuild %s %s %s in %v", task.pkg.String(), task.target, env, time.Now().Sub(start))

	err = task.handleDTS(esmeta)
	if err != nil {
		return
	}

	_, err = db.Put(
		q.Alias(task.ID()),
		q.KV{
			"esmeta": utils.MustEncodeJSON(esmeta),
			"css":    cssMark,
		},
	)
	if err != nil && err == postdb.ErrDuplicateAlias {
		err = nil
	}
	if err != nil {
		return
	}

	esm = esmeta
	packageCSS = cssMark[0] == 1
	return
}

func (task *buildTask) handleDTS(esmeta *ESMeta) (err error) {
	start := time.Now()
	pkg := task.pkg
	nodeModulesDir := path.Join(task.wd, "node_modules")
	versionedName := fmt.Sprintf("%s@%s", esmeta.Name, esmeta.Version)

	var types string
	if esmeta.Types != "" || esmeta.Typings != "" {
		types = getTypesPath(nodeModulesDir, *esmeta.NpmPackage, "")
	} else if pkg.submodule == "" {
		if fileExists(path.Join(nodeModulesDir, pkg.name, "index.d.ts")) {
			types = fmt.Sprintf("%s/%s", versionedName, "index.d.ts")
		} else if !strings.HasPrefix(pkg.name, "@") {
			packageFile := path.Join(nodeModulesDir, "@types", pkg.name, "package.json")
			if fileExists(packageFile) {
				var p NpmPackage
				err := utils.ParseJSONFile(path.Join(nodeModulesDir, "@types", pkg.name, "package.json"), &p)
				if err == nil {
					types = getTypesPath(nodeModulesDir, p, "")
				}
			}
		}
	} else {
		if fileExists(path.Join(nodeModulesDir, pkg.name, pkg.submodule, "index.d.ts")) {
			types = fmt.Sprintf("%s/%s", versionedName, path.Join(pkg.submodule, "index.d.ts"))
		} else if fileExists(path.Join(nodeModulesDir, pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
			types = fmt.Sprintf("%s/%s", versionedName, ensureExt(pkg.submodule, ".d.ts"))
		} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, pkg.submodule, "index.d.ts")) {
			types = fmt.Sprintf("@types/%s/%s", versionedName, path.Join(pkg.submodule, "index.d.ts"))
		} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
			types = fmt.Sprintf("@types/%s/%s", versionedName, ensureExt(pkg.submodule, ".d.ts"))
		}
	}
	if types != "" {
		err = copyDTS(
			nodeModulesDir,
			types,
		)
		if err != nil {
			err = fmt.Errorf("copyDTS(%s): %v", types, err)
			return
		}
		esmeta.Dts = "/" + types
		log.Debug("copy dts in", time.Now().Sub(start))
	}

	return
}

func initBuild(buildDir string, pkg pkg, install bool) (esmeta *ESMeta, err error) {
	var p NpmPackage
	p, _, err = node.getPackageInfo(pkg.name, pkg.version)
	if err != nil {
		return
	}

	esmeta = &ESMeta{
		NpmPackage: &p,
	}
	installList := []string{
		fmt.Sprintf("%s@%s", pkg.name, pkg.version),
	}
	pkgDir := path.Join(buildDir, "node_modules", esmeta.Name)
	if esmeta.Types == "" && esmeta.Typings == "" && !strings.HasPrefix(pkg.name, "@") {
		var info NpmPackage
		info, _, err = node.getPackageInfo("@types/"+pkg.name, "latest")
		if err == nil {
			if info.Types != "" || info.Typings != "" || info.Main != "" {
				installList = append(installList, fmt.Sprintf("%s@%s", info.Name, info.Version))
			}
		} else if err.Error() != fmt.Sprintf("npm: package '@types/%s' not found", pkg.name) {
			return
		}
	}
	if esmeta.Module == "" && esmeta.Type == "module" {
		esmeta.Module = esmeta.Main
	}
	if esmeta.Module == "" && esmeta.DefinedExports != nil {
		v, ok := esmeta.DefinedExports.(map[string]interface{})
		if ok {
			m, ok := v["import"]
			if ok {
				s, ok := m.(string)
				if ok && s != "" {
					esmeta.Module = s
				}
			}
		}
	}
	if pkg.submodule != "" {
		esmeta.Main = pkg.submodule
		esmeta.Module = ""
		esmeta.Types = ""
		esmeta.Typings = ""
	}

	if install {
		for n, v := range esmeta.PeerDependencies {
			installList = append(installList, fmt.Sprintf("%s@%s", n, v))
		}
		err = yarnAdd(buildDir, installList...)
		if err != nil {
			return
		}
	}

	if pkg.submodule != "" {
		packageFile := path.Join(pkgDir, pkg.submodule, "package.json")
		if fileExists(packageFile) {
			var p NpmPackage
			err = utils.ParseJSONFile(packageFile, &p)
			if err != nil {
				return
			}
			if p.Main != "" {
				esmeta.Main = path.Join(pkg.submodule, p.Main)
			}
			if p.Module != "" {
				esmeta.Module = path.Join(pkg.submodule, p.Module)
			} else if esmeta.Type == "module" && p.Main != "" {
				esmeta.Module = path.Join(pkg.submodule, p.Main)
			}
			if p.Types != "" {
				esmeta.Types = path.Join(pkg.submodule, p.Types)
			}
			if p.Typings != "" {
				esmeta.Typings = path.Join(pkg.submodule, p.Typings)
			}
		} else {
			exports, esm, e := parseESModuleExports(buildDir, path.Join(esmeta.Name, pkg.submodule))
			if e != nil {
				err = e
				return
			}
			if esm {
				esmeta.Module = pkg.submodule
				esmeta.Exports = exports
			}
		}
	}

	if esmeta.Module != "" {
		exports, esm, e := parseESModuleExports(buildDir, path.Join(esmeta.Name, esmeta.Module))
		if e != nil {
			err = e
			return
		}
		if esm {
			esmeta.Exports = exports

		} else {
			// fake module
			esmeta.Module = ""
		}
	}

	if esmeta.Module == "" {
		ret, err := parseCJSModuleExports(buildDir, pkg.ImportPath())
		if err != nil {
			log.Warn(err)
		}
		esmeta.Exports = ret.Exports
	}
	return
}
