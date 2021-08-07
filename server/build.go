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
	alias  map[string]string
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
	name := path.Base(pkg.name)
	query := ""
	qs := []string{}
	if pkg.submodule != "" {
		name = pkg.submodule
	}
	if strings.HasSuffix(name, ".js") {
		name = strings.TrimSuffix(name, ".js")
	}
	if task.isDev {
		name += ".development"
	}
	if task.bundle {
		name += ".bundle"
	}
	if len(task.alias) > 0 {
		var ss sort.StringSlice
		for name, to := range task.alias {
			ss = append(ss, fmt.Sprintf("%s:%s", name, to))
		}
		ss.Sort()
		qs = append(qs, fmt.Sprintf("alias:%s", strings.Join(ss, ",")))

	}
	if len(task.deps) > 0 {
		var ss sort.StringSlice
		for _, pkg := range task.deps {
			ss = append(ss, fmt.Sprintf("%s@%s", pkg.name, pkg.version))
		}
		ss.Sort()
		qs = append(qs, fmt.Sprintf("deps:%s", strings.Join(ss, ",")))
	}
	if len(qs) > 0 {
		query = fmt.Sprintf("X-%s/", btoaUrl(strings.Join(qs, ",")))
	}

	task.id = fmt.Sprintf(
		"v%d/%s@%s/%s%s/%s.js",
		VERSION,
		pkg.name,
		pkg.version,
		query,
		task.target,
		name,
	)
	return task.id
}

func (task *buildTask) getImportPath(pkg pkg) string {
	name := path.Base(pkg.name)
	if pkg.submodule != "" {
		name = pkg.submodule
	}
	if strings.HasSuffix(name, ".js") {
		name = strings.TrimSuffix(name, ".js")
	}
	if task.isDev {
		name += ".development"
	}
	if task.bundle {
		name += ".bundle"
	}

	return fmt.Sprintf(
		"/v%d/%s@%s/%s/%s.js",
		VERSION,
		pkg.name,
		pkg.version,
		task.target,
		name,
	)
}

func (task *buildTask) build() (esm *ESM, pkgCSS bool, err error) {
	hasher := sha1.New()
	hasher.Write([]byte(task.ID()))
	task.wd = path.Join(os.TempDir(), "esm-build-"+hex.EncodeToString(hasher.Sum(nil)))
	ensureDir(task.wd)
	defer os.RemoveAll(task.wd)

	env := "production"
	if task.isDev {
		env = "development"
	}

	esm, err = initESM(task.wd, task.pkg, true, task.alias)
	if err != nil {
		log.Warn("init ESM:", err)
		return
	}
	defer func() {
		if err != nil {
			esm = nil
		}
	}()

	cjs := esm.Module == ""

	var entryPoint string
	var input *api.StdinOptions
	if cjs {
		buf := bytes.NewBuffer(nil)
		importPath := task.pkg.ImportPath()
		if len(esm.Exports) > 0 {
			fmt.Fprintf(buf, `import * as __star from "%s";%s`, importPath, "\n")
			fmt.Fprintf(buf, `export const { %s } = __star;%s`, strings.Join(esm.Exports, ","), "\n")
		}
		fmt.Fprintf(buf, `export { default } from "%s";`, importPath)
		input = &api.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: task.wd,
			Sourcefile: "export.js",
		}
	} else {
		entryPoint = path.Join(task.wd, "node_modules", task.pkg.name, esm.Module)
	}
	minify := !task.isDev
	define := map[string]string{
		"__filename":                  fmt.Sprintf(`"https://%s/%s"`, config.cdnDomain, task.ID()),
		"__dirname":                   fmt.Sprintf(`"https://%s/%s"`, config.cdnDomain, path.Dir(task.ID())),
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
					specifier := strings.TrimSuffix(args.Path, "/")
					if alias, ok := task.alias[specifier]; ok {
						specifier = alias
					}

					// should resolve entryPoint
					if strings.HasPrefix(specifier, task.wd) {
						return api.OnResolveResult{}, nil
					}

					// should resolve:
					// 1. current package/module itself
					// 2. local imports
					// 3. sub-modules of deps (cjs)
					if specifier == task.pkg.ImportPath() ||
						isLocalImport(specifier) ||
						(cjs &&
							((!strings.HasPrefix(specifier, "@") && len(strings.Split(specifier, "/")) > 1) ||
								(strings.HasPrefix(specifier, "@") && len(strings.Split(specifier, "/")) > 2))) {
						return api.OnResolveResult{}, nil
					}

					// bundle all deps except peer deps in bundle mode
					if task.bundle && !builtInNodeModules[specifier] {
						_, ok := esm.PeerDependencies[specifier]
						if !ok {
							return api.OnResolveResult{}, nil
						}
					}

					// external
					external.Add(specifier)
					return api.OnResolveResult{Path: "__ESM_SH_EXTERNAL__:" + specifier, External: true}, nil
				},
			)
		},
	}

esbuild:
	start := time.Now()
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Target:            targets[task.target],
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Plugins:           []api.Plugin{esmResolverPlugin},
	}
	if task.target != "node" {
		options.Define = define
	}
	if entryPoint != "" {
		options.EntryPoints = []string{entryPoint}
	} else {
		options.Stdin = input
	}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		// mark the missing module as external to exclude it from the bundle
		msg := result.Errors[0].Text
		if strings.HasPrefix(msg, "Could not resolve \"") && strings.Contains(msg, "mark it as external to exclude it from the bundle") {
			// but current package/module can not mark as external
			if strings.Contains(msg, fmt.Sprintf("Could not resolve \"%s\"", task.pkg.ImportPath())) {
				err = fmt.Errorf("Could not resolve \"%s\"", task.pkg.ImportPath())
				return
			}
			log.Warnf("esbuild(%s): %s", task.ID(), msg)
			name := strings.Split(msg, "\"")[1]
			if !extraExternal.Has(name) {
				external.Add(name)
				extraExternal.Add(name)
				goto esbuild
			}
		} else if strings.HasPrefix(msg, "No matching export in \"") && strings.Contains(msg, "for import \"default\"") {
			if cjs {
				input = &api.StdinOptions{
					Contents:   fmt.Sprintf(`import "%s";`, task.pkg.ImportPath()),
					ResolveDir: task.wd,
					Sourcefile: "export.js",
				}
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
				// is local imports
				if isLocalImport(name) {
					filename := name
					if path.Ext(filename) == "" {
						filename += ".js"
					}
					importPath = "/" + path.Join(path.Dir(task.ID()), filename)
				}
				// is builtin `buffer` module
				if importPath == "" && name == "buffer" {
					if task.target == "node" {
						importPath = "buffer"
					} else {
						importPath = fmt.Sprintf("/v%d/node_buffer.js", VERSION)
					}
				}
				// is builtin node module
				if importPath == "" && builtInNodeModules[name] {
					if task.target == "node" {
						importPath = name
					} else if task.target == "deno" && denoStdNodeModules[name] {
						importPath = fmt.Sprintf("/v%d/deno_std_node_%s.js", VERSION, name)
					} else {
						polyfill, ok := polyfilledBuiltInNodeModules[name]
						if ok {
							p, submodule, e := node.getPackageInfo(polyfill, "latest")
							if e != nil {
								err = e
								return
							}
							importPath = task.getImportPath(pkg{
								name:      p.Name,
								version:   p.Version,
								submodule: submodule,
							})
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
				// get package info via `deps` query
				if importPath == "" {
					for _, dep := range task.deps {
						if name == dep.ImportPath() {
							importPath = task.getImportPath(dep)
							break
						}
					}
				}
				// is sub-module
				if importPath == "" && strings.HasPrefix(name, task.pkg.name+"/") {
					submodule := strings.TrimPrefix(name, task.pkg.name+"/")
					importPath = task.getImportPath(pkg{
						name:      task.pkg.name,
						version:   task.pkg.version,
						submodule: submodule,
					})
				}
				// get package info from NPM
				if importPath == "" {
					version := "latest"
					if v, ok := esm.Dependencies[name]; ok {
						version = v
					} else if v, ok := esm.PeerDependencies[name]; ok {
						version = v
					}
					p, submodule, e := node.getPackageInfo(name, version)
					if e == nil {
						importPath = task.getImportPath(pkg{
							name:      p.Name,
							version:   p.Version,
							submodule: submodule,
						})
					}
				}
				if importPath == "" {
					err = fmt.Errorf("Could not resolve \"%s\"  (Imported by \"%s\")", name, task.pkg.name)
					return
				}
				buf := bytes.NewBuffer(nil)
				identifier := identify(name)
				slice := bytes.Split(outputContent, []byte(fmt.Sprintf("\"__ESM_SH_EXTERNAL__:%s\"", name)))
				cjsContext := false
				cjsImported := false
				for i, p := range slice {
					if cjsContext {
						p = bytes.TrimPrefix(p, []byte{')'})
					}
					cjsContext = bytes.HasSuffix(p, []byte{'('})
					if cjsContext {
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
						if !cjsImported {
							wrote := false
							versionPrefx := fmt.Sprintf("/v%d/", VERSION)
							if strings.HasPrefix(importPath, versionPrefx) {
								pkg, err := parsePkg(strings.TrimPrefix(importPath, versionPrefx))
								if err == nil {
									// here the submodule should be always empty
									pkg.submodule = ""
									_, installed := esm.Dependencies[name]
									if !installed {
										_, installed = esm.PeerDependencies[name]
									}
									meta, err := initESM(task.wd, *pkg, !installed, nil)
									if err == nil && meta.Module != "" {
										if !meta.ExportDefault {
											fmt.Fprintf(jsHeader, `import * as __%s$ from "%s";%s`, identifier, importPath, eol)
											wrote = true
										}
									}
								}
							}
							if !wrote {
								fmt.Fprintf(jsHeader, `import __%s$ from "%s";%s`, identifier, importPath, eol)
							}
							cjsImported = true
						}
					}
					buf.Write(p)
					if i < len(slice)-1 {
						if cjsContext {
							buf.WriteString(fmt.Sprintf("__%s$", identifier))
						} else {
							buf.WriteString(fmt.Sprintf("\"%s\"", importPath))
						}
					}
				}
				outputContent = buf.Bytes()
			}

			// add nodejs/deno compatibility
			if task.target != "node" {
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
			}

			saveFilePath := path.Join(config.storageDir, "builds", task.ID())
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
			saveFilePath := path.Join(config.storageDir, "builds", strings.TrimSuffix(task.ID(), ".js")+".css")
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

	err = task.handleDTS(esm)
	if err != nil {
		return
	}

	_, err = db.Put(
		q.Alias(task.ID()),
		q.KV{
			"esm": utils.MustEncodeJSON(esm),
			"css": cssMark,
		},
	)
	if err != nil && err == postdb.ErrDuplicateAlias {
		err = nil
	}
	if err != nil {
		return
	}

	pkgCSS = cssMark[0] == 1
	return
}

func (task *buildTask) handleDTS(esm *ESM) (err error) {
	start := time.Now()
	pkg := task.pkg
	nodeModulesDir := path.Join(task.wd, "node_modules")
	versionedName := fmt.Sprintf("%s@%s", esm.Name, esm.Version)

	var types string
	if esm.Types != "" || esm.Typings != "" {
		types = getTypesPath(nodeModulesDir, *esm.NpmPackage, "")
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
		esm.Dts = "/" + types
		log.Debug("copy dts in", time.Now().Sub(start))
	}

	return
}
