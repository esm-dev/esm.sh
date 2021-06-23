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
	bundle bool
}

func (task *buildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.pkg
	deps := ""
	target := task.target
	name := path.Base(pkg.name)
	if pkg.submodule != "" {
		name = pkg.submodule
	}
	if task.isDev {
		name += ".development"
	}
	if task.bundle {
		name += ".bundle"
	}
	if len(task.deps) > 0 {
		sort.Sort(task.deps)
		deps = fmt.Sprintf("deps=%s/", strings.ReplaceAll(task.deps.String(), "/", "_"))
	}
	task.id = fmt.Sprintf(
		"v%d/%s@%s/%s%s/%s",
		VERSION,
		pkg.name,
		pkg.version,
		deps,
		target,
		name,
	)
	return task.id
}

func (task *buildTask) buildESM() (esm *ESMeta, pkgCSS bool, err error) {
	hasher := sha1.New()
	hasher.Write([]byte(task.ID()))
	task.wd = path.Join(os.TempDir(), "esm-build-"+hex.EncodeToString(hasher.Sum(nil)))
	ensureDir(task.wd)
	defer os.RemoveAll(task.wd)

	env := "production"
	if task.isDev {
		env = "development"
	}
	esmeta, err := initBuild(task.wd, task.pkg, true, env)
	if err != nil {
		return
	}

	start := time.Now()
	buf := bytes.NewBuffer(nil)
	importPath := task.pkg.ImportPath()
	exports := newStringSet()
	hasDefaultExport := false
	for _, name := range esmeta.Exports {
		if name == "default" {
			hasDefaultExport = true
		} else if name != "import" {
			exports.Add(name)
		}
	}
	if exports.Size() > 0 {
		fmt.Fprintf(buf, `import * as __star from "%s";%s`, importPath, "\n")
		fmt.Fprintf(buf, `export const { %s } = __star;%s`, strings.Join(exports.Values(), ","), "\n")
	}
	if esmeta.Module == "" || hasDefaultExport {
		fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
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
	extraExternal := newStringSet()
	esmResolverPlugin := api.Plugin{
		Name: "esm-resolver",
		Setup: func(plugin api.PluginBuild) {
			plugin.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					p := strings.TrimSuffix(args.Path, "/")
					importName := task.pkg.name
					if s := task.pkg.submodule; s != "" {
						importName += "/" + s
					}

					// should resolve:
					// 1. current package itself
					// 2. sub-modules of current package
					// 3. sub-modules of other packages
					if p == importName ||
						isFileImportPath(p) ||
						(!strings.HasPrefix(p, "@") && len(strings.Split(p, "/")) > 1) ||
						(strings.HasPrefix(p, "@") && len(strings.Split(p, "/")) > 2) {
						return api.OnResolveResult{}, nil
					}

					// bundle all deps except peer deps in bundle mode
					if task.bundle && !builtInNodeModules[p] {
						_, ok := esmeta.PeerDependencies[p]
						if !ok {
							return api.OnResolveResult{}, nil
						}
					}

					external.Add(p)
					return api.OnResolveResult{Path: "__ESM_SH_EXTERNAL__:" + p, External: true}, nil
				},
			)
		},
	}
	for name := range builtInNodeModules {
		if name != task.pkg.name {
			external.Add(name)
		}
	}

esbuild:
	result := api.Build(api.BuildOptions{
		Stdin:             input,
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Target:            targets[task.target],
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		External:          external.Values(),
		Define:            define,
		Plugins:           []api.Plugin{esmResolverPlugin},
	})

	if len(result.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := result.Errors[0].Text
		if strings.HasPrefix(msg, "Could not resolve \"") && strings.Contains(msg, "mark it as external to exclude it from the bundle") {
			log.Warnf("esbuild(%s): %s", task.ID(), msg)
			name := strings.Split(msg, "\"")[1]
			if !extraExternal.Has(name) {
				external.Add(name)
				extraExternal.Add(name)
				goto esbuild
			}
		}
		err = errors.New("esbuild: " + msg)
		return
	}

	for _, w := range result.Warnings {
		log.Warnf("esbuild(%s): %s", task.ID(), w.Text)
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
				if name == "buffer" {
					importPath = fmt.Sprintf("/v%d/node_buffer.js", VERSION)
				}
				if importPath == "" && builtInNodeModules[name] {
					if task.target == "deno" && denoStdNodeModules[name] {
						importPath = fmt.Sprintf("/v%d/deno_std_node_%s.js", VERSION, name)
					} else {
						polyfill, ok := polyfilledBuiltInNodeModules[name]
						if ok {
							p, submodule, e := node.getPackageInfo(polyfill, "latest")
							if e != nil {
								err = e
								return
							}
							filename := path.Base(p.Name)
							if submodule != "" {
								filename = submodule
							}
							if task.isDev {
								filename += ".development"
							}
							if task.bundle {
								filename += ".bundle"
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
							_, err := embedFS.Open(fmt.Sprintf("embed/polyfills/node_%s.js", name))
							if err == nil {
								importPath = fmt.Sprintf("/v%d/node_%s.js", VERSION, name)
							} else {
								importPath = fmt.Sprintf(
									"/error.js?type=unsupported-nodejs-builtin-module&name=%s&importer=%s",
									name,
									task.pkg.name,
								)
							}
						}
					}
				}
				if importPath == "" {
					packageFile := path.Join(task.wd, "node_modules", name, "package.json")
					if fileExists(packageFile) {
						var p NpmPackage
						if utils.ParseJSONFile(packageFile, &p) == nil {
							suffix := ""
							if task.isDev {
								suffix = ".development"
							}
							if task.bundle {
								suffix = ".bundle"
							}
							suffix += ".js"
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
						if task.bundle {
							filename += ".bundle"
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
					importPath = fmt.Sprintf(
						"/error.js?type=resolve&name=%s&importer=%s",
						name,
						task.pkg.name,
					)
				}
				buf := bytes.NewBuffer(nil)
				identifier := identify(name)
				slice := bytes.Split(outputContent, []byte(fmt.Sprintf("\"__ESM_SH_EXTERNAL__:%s\"", name)))
				commonjsContext := false
				commonjsImported := false
				for i, p := range slice {
					if commonjsContext {
						p = bytes.TrimPrefix(p, []byte{')'})
					}
					commonjsContext = bytes.HasSuffix(p, []byte{'('})
					if commonjsContext {
						shift := 0
						for i := len(p) - 2; i >= 0; i-- {
							c := p[i]
							if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$' {
								shift++
							} else {
								break
							}
						}
						if shift > 0 {
							p = p[0 : len(p)-(shift+1)]
						}
						if !commonjsImported {
							wrote := false
							versionPrefx := fmt.Sprintf("/v%d/", VERSION)
							if strings.HasPrefix(importPath, versionPrefx) {
								pkg, err := parsePkg(strings.TrimPrefix(importPath, versionPrefx))
								if err == nil {
									// here the submodule should be always empty
									pkg.submodule = ""
									_, installed := esmeta.Dependencies[name]
									if !installed {
										_, installed = esmeta.PeerDependencies[name]
									}
									meta, err := initBuild(task.wd, *pkg, !installed, env)
									if err == nil && meta.Module != "" {
										hasDefaultExport := false
										if len(meta.Exports) > 0 {
											for _, name := range meta.Exports {
												if name == "default" {
													hasDefaultExport = true
													break
												}
											}
										}
										if !hasDefaultExport {
											fmt.Fprintf(jsHeader, `import * as __%s$ from "%s";%s`, identifier, importPath, eol)
											wrote = true
										}
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
						if commonjsContext {
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
				fmt.Fprintf(jsHeader, `import __process$ from "/v%d/node_process.js";%s__process$.env.NODE_ENV="%s";%s`, VERSION, eol, env, eol)
			}
			if bytes.Contains(outputContent, []byte("__Buffer$")) {
				fmt.Fprintf(jsHeader, `import { Buffer as __Buffer$ } from "/v%d/node_buffer.js";%s`, VERSION, eol)
			}
			if bytes.Contains(outputContent, []byte("__global$")) {
				fmt.Fprintf(jsHeader, `var __global$ = window;%s`, eol)
			}
			if bytes.Contains(outputContent, []byte("__setImmediate$")) {
				fmt.Fprintf(jsHeader, `var __setImmediate$ = (cb, ...args) => setTimeout(cb, 0, ...args);%s`, eol)
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
	pkgCSS = cssMark[0] == 1
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
		} else if fileExists(path.Join(nodeModulesDir, pkg.name, ensureSuffix(pkg.submodule, ".d.ts"))) {
			types = fmt.Sprintf("%s/%s", versionedName, ensureSuffix(pkg.submodule, ".d.ts"))
		} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, pkg.submodule, "index.d.ts")) {
			types = fmt.Sprintf("@types/%s/%s", versionedName, path.Join(pkg.submodule, "index.d.ts"))
		} else if fileExists(path.Join(nodeModulesDir, "@types", pkg.name, ensureSuffix(pkg.submodule, ".d.ts"))) {
			types = fmt.Sprintf("@types/%s/%s", versionedName, ensureSuffix(pkg.submodule, ".d.ts"))
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

func initBuild(buildDir string, pkg pkg, install bool, env string) (esmeta *ESMeta, err error) {
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
			if e != nil && os.IsExist(e) {
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
		if e != nil && os.IsExist(e) {
			err = e
			return
		}
		if esm {
			esmeta.Exports = exports
			log.Debug(p.Name, len(esmeta.Exports), "exports as es moudle")
		} else {
			// fake module
			esmeta.Module = ""
		}
	}

	if esmeta.Module == "" {
		ret, err := parseCJSModuleExports(buildDir, pkg.ImportPath(), env)
		if err != nil {
			log.Warn(err)
		}
		esmeta.Exports = ret.Exports
		log.Debug(p.Name, len(esmeta.Exports), "exports as cjs")
	}
	return
}
