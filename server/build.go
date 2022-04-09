package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"esm.sh/server/storage"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/crypto/rs"
	"github.com/ije/gox/utils"
)

type BuildTask struct {
	BuildVersion int               `json:"buildVersion"`
	Pkg          Pkg               `json:"pkg"`
	Alias        map[string]string `json:"alias"`
	Deps         PkgSlice          `json:"deps"`
	Target       string            `json:"target"`
	BundleMode   bool              `json:"bundle"`
	NoRequire    bool              `json:"noRequire"`
	DevMode      bool              `json:"dev"`

	// state
	id    string
	wd    string
	stage string
}

func (task *BuildTask) resolvePrefix() string {
	alias := []string{}
	if len(task.Alias) > 0 {
		var ss sort.StringSlice
		for name, to := range task.Alias {
			ss = append(ss, fmt.Sprintf("%s:%s", name, to))
		}
		ss.Sort()
		alias = append(alias, fmt.Sprintf("alias:%s", strings.Join(ss, ",")))
	}
	if len(task.Deps) > 0 {
		var ss sort.StringSlice
		for _, pkg := range task.Deps {
			ss = append(ss, fmt.Sprintf("%s@%s", pkg.Name, pkg.Version))
		}
		ss.Sort()
		alias = append(alias, fmt.Sprintf("deps:%s", strings.Join(ss, ",")))
	}
	if len(alias) > 0 {
		return fmt.Sprintf("X-%s/", btoaUrl(strings.Join(alias, ",")))
	}
	return ""
}

func (task *BuildTask) ID() string {
	if task.id != "" {
		return task.id
	}

	pkg := task.Pkg
	name := path.Base(pkg.Name)

	if pkg.Submodule != "" {
		name = pkg.Submodule
	}
	name = strings.TrimSuffix(name, ".js")
	if task.NoRequire {
		name += ".nr"
	}
	if task.DevMode {
		name += ".development"
	}
	if task.BundleMode {
		name += ".bundle"
	}

	task.id = fmt.Sprintf(
		"v%d/%s@%s/%s%s/%s.js",
		task.BuildVersion,
		pkg.Name,
		pkg.Version,
		task.resolvePrefix(),
		task.Target,
		name,
	)
	if task.Target == "types" {
		task.id = strings.TrimSuffix(task.id, ".js")
	}
	return task.id
}

func (task *BuildTask) getImportPath(pkg Pkg, prefix string, infectDeps bool) string {
	if infectDeps && task.Deps.Len() > 0 {
		path := fmt.Sprintf(
			"/%s?target=%s&pin=v%d&deps=%s",
			pkg.String(),
			task.Target,
			task.BuildVersion,
			task.Deps.String(),
		)
		if task.DevMode {
			path += "&dev"
		}
		return path
	}

	name := path.Base(pkg.Name)
	if pkg.Submodule != "" {
		name = pkg.Submodule
	}
	name = strings.TrimSuffix(name, ".js")
	if task.DevMode {
		name += ".development"
	}

	return fmt.Sprintf(
		"/v%d/%s@%s/%s%s/%s.js",
		task.BuildVersion,
		pkg.Name,
		pkg.Version,
		prefix,
		task.Target,
		name,
	)
}

func (task *BuildTask) Build() (esm *Module, err error) {
	prev, err := findModule(task.ID())
	if err == nil {
		return prev, nil
	}

	if task.wd == "" {
		hasher := sha1.New()
		hasher.Write([]byte(task.ID()))
		task.wd = path.Join(os.TempDir(), fmt.Sprintf("esm-build-%s-%s", hex.EncodeToString(hasher.Sum(nil)), rs.Hex.String(8)))
		ensureDir(task.wd)
	}
	defer func() {
		err := os.RemoveAll(task.wd)
		if err != nil {
			log.Warnf("clean build(%s) dir: %v", task.ID(), err)
		}
	}()

	task.stage = "install"
	for i := 0; i < 3; i++ {
		err = yarnAdd(task.wd, fmt.Sprintf("%s@%s", task.Pkg.Name, task.Pkg.Version))
		if err == nil && !fileExists(path.Join(task.wd, "node_modules", task.Pkg.Name, "package.json")) {
			err = fmt.Errorf("yarnAdd(%s): package.json not found", task.Pkg)
		}
		if err == nil {
			break
		}
		if i < 2 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if err != nil {
		return
	}

	return task.build(newStringSet())
}

func (task *BuildTask) build(tracing *stringSet) (esm *Module, err error) {
	if tracing.Has(task.ID()) {
		return
	}
	tracing.Add(task.ID())

	task.stage = "init"
	esm, err = initModule(task.wd, task.Pkg, task.Target, task.DevMode)
	if err != nil {
		return
	}

	if task.Target == "types" {
		if esm.Types != "" {
			dts := esm.Name + "@" + esm.Version + "/" + esm.Types
			task.stage = "transform-dts"
			task.transformDTS(dts)
		}
		return
	}

	if esm.Main == "" && esm.Module == "" && esm.Types != "" {
		dts := esm.Name + "@" + esm.Version + "/" + esm.Types
		task.stage = "transform-dts"
		task.transformDTS(dts)
		task.findDTS(esm)
		task.storeToDB(esm)
		return
	}

	task.stage = "build"
	defer func() {
		if err != nil {
			esm = nil
		}
	}()

	var entryPoint string
	var input *api.StdinOptions

	if esm.Module == "" {
		buf := bytes.NewBuffer(nil)
		importPath := task.Pkg.ImportPath()
		fmt.Fprintf(buf, `import $default from "%s";`, importPath)
		fmt.Fprintf(buf, `import * as $module from "%s";`, importPath)
		if len(esm.Exports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = $module;`, strings.Join(esm.Exports, ","))
		}
		fmt.Fprintf(buf, "const { default: $def, ...$rest } = $module;")
		fmt.Fprintf(buf, "export default $default ?? $def ?? $rest;")
		input = &api.StdinOptions{
			Contents:   buf.String(),
			ResolveDir: task.wd,
			Sourcefile: "mod.js",
		}
	} else {
		entryPoint = path.Join(task.wd, "node_modules", esm.Name, esm.Module)
	}

	nodeEnv := "production"
	if task.DevMode {
		nodeEnv = "development"
	}
	define := map[string]string{
		"__filename":                  fmt.Sprintf(`"https://%s/%s"`, cdnDomain, task.ID()),
		"__dirname":                   fmt.Sprintf(`"https://%s/%s"`, cdnDomain, path.Dir(task.ID())),
		"Buffer":                      "__Buffer$",
		"process":                     "__Process$",
		"setImmediate":                "__setImmediate$",
		"clearImmediate":              "clearTimeout",
		"require.resolve":             "__rResolve$",
		"process.env.NODE_ENV":        fmt.Sprintf(`"%s"`, nodeEnv),
		"global":                      "__global$",
		"global.Buffer":               "__Buffer$",
		"global.process":              "__Process$",
		"global.setImmediate":         "__setImmediate$",
		"global.clearImmediate":       "clearTimeout",
		"global.require.resolve":      "__rResolve$",
		"global.process.env.NODE_ENV": fmt.Sprintf(`"%s"`, nodeEnv),
	}
	external := newStringSet()
	extraExternal := newStringSet()
	esmResolverPlugin := api.Plugin{
		Name: "esm.sh-resolver",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					if strings.HasPrefix(args.Path, "data:") {
						return api.OnResolveResult{External: true}, nil
					}

					specifier := strings.TrimSuffix(args.Path, "/")

					// resolve nodejs builtin modules like `node:path`
					specifier = strings.TrimPrefix(specifier, "node:")

					// resolve `?alias` query
					if len(task.Alias) > 0 {
						if name, ok := task.Alias[specifier]; ok {
							specifier = name
						}
					}

					// bundles all dependencies in `bundle` mode, apart from peer dependencies
					if task.BundleMode && !extraExternal.Has(specifier) {
						a := strings.Split(specifier, "/")
						pkgName := a[0]
						if len(a) > 1 && specifier[0] == '@' {
							pkgName = a[1]
						}
						if !builtInNodeModules[pkgName] {
							_, ok := esm.PeerDependencies[pkgName]
							if !ok {
								return api.OnResolveResult{}, nil
							}
						}
					}

					// splits modules based on the `exports` defines in package.json,
					// see https://nodejs.org/api/packages.html
					if strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || specifier == ".." {
						resolvedPath := path.Join(path.Dir(args.Importer), specifier)
						// in macOS, the dir `/private/var/` is equal to `/var/`
						if strings.HasPrefix(resolvedPath, "/private/var/") {
							resolvedPath = strings.TrimPrefix(resolvedPath, "/private")
						}
						modulePath := "." + strings.TrimPrefix(resolvedPath, path.Join(task.wd, "node_modules", esm.Name))
						v, ok := esm.DefinedExports.(map[string]interface{})
						if ok {
							for export, paths := range v {
								m, ok := paths.(map[string]interface{})
								if ok && export != "." {
									for _, value := range m {
										s, ok := value.(string)
										if ok && s != "" {
											match := modulePath == s || modulePath+".js" == s || modulePath+".mjs" == s
											if !match {
												if a := strings.Split(s, "*"); len(a) == 2 {
													prefix := a[0]
													suffix := a[1]
													if (strings.HasPrefix(modulePath, prefix)) &&
														(strings.HasSuffix(modulePath, suffix) ||
															strings.HasSuffix(modulePath+".js", suffix) ||
															strings.HasSuffix(modulePath+".mjs", suffix)) {
														matchName := strings.TrimPrefix(strings.TrimSuffix(modulePath, suffix), prefix)
														export = strings.Replace(export, "*", matchName, -1)
														match = true
													}
												}
											}
											if match {
												url := path.Join(esm.Name, export)
												if url == task.Pkg.ImportPath() {
													return api.OnResolveResult{}, nil
												}
												external.Add(url)
												return api.OnResolveResult{Path: "__ESM_SH_EXTERNAL:" + url, External: true}, nil
											}
										}
									}
								}
							}
						}
					}

					// bundles undefined relative imports or the package/module it self
					if isLocalImport(specifier) || specifier == task.Pkg.ImportPath() {
						return api.OnResolveResult{}, nil
					}

					// ignore `require()` of esm package
					if task.NoRequire && (args.Kind == api.ResolveJSRequireCall || args.Kind == api.ResolveJSRequireResolve) {
						return api.OnResolveResult{Path: specifier, External: true}, nil
					}

					// dynamic external
					external.Add(specifier)
					return api.OnResolveResult{Path: "__ESM_SH_EXTERNAL:" + specifier, External: true}, nil
				},
			)
		},
	}

esbuild:
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Target:            targets[task.Target],
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  !task.DevMode,
		MinifyIdentifiers: !task.DevMode,
		MinifySyntax:      !task.DevMode,
		Plugins:           []api.Plugin{esmResolverPlugin},
		Loader: map[string]api.Loader{
			".wasm":  api.LoaderDataURL,
			".svg":   api.LoaderDataURL,
			".png":   api.LoaderDataURL,
			".webp":  api.LoaderDataURL,
			".ttf":   api.LoaderDataURL,
			".eot":   api.LoaderDataURL,
			".woff":  api.LoaderDataURL,
			".woff2": api.LoaderDataURL,
		},
	}
	if task.Target == "node" {
		options.Platform = api.PlatformNode
	} else {
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
		if strings.HasPrefix(msg, "Could not resolve \"") {
			// but current package/module can not mark as external
			if strings.Contains(msg, fmt.Sprintf("Could not resolve \"%s\"", task.Pkg.ImportPath())) {
				err = fmt.Errorf("Could not resolve \"%s\"", task.Pkg.ImportPath())
				return
			}
			log.Warnf("esbuild(%s): %s", task.ID(), msg)
			name := strings.Split(msg, "\"")[1]
			if !extraExternal.Has(name) {
				extraExternal.Add(name)
				external.Add(name)
				goto esbuild
			}
		} else if strings.HasPrefix(msg, "No matching export in \"") && strings.Contains(msg, "for import \"default\"") {
			input = &api.StdinOptions{
				Contents:   fmt.Sprintf(`import "%s";export default null;`, task.Pkg.ImportPath()),
				ResolveDir: task.wd,
				Sourcefile: "mod.js",
			}
			goto esbuild
		}
		err = errors.New("esbuild: " + msg)
		return
	}

	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Text, "Could not resolve \"") {
			log.Warnf("esbuild(%s): %s", task.ID(), w.Text)
		}
	}

	for _, file := range result.OutputFiles {
		outputContent := file.Contents
		if strings.HasSuffix(file.Path, ".js") {
			buf := bytes.NewBufferString(fmt.Sprintf(
				"/* esm.sh - esbuild bundle(%s) %s %s */\n",
				task.Pkg.String(),
				strings.ToLower(task.Target),
				nodeEnv,
			))
			eol := "\n"
			if !task.DevMode {
				eol = ""
			}

			// replace external imports/requires
			for _, name := range external.Values() {
				var importPath string
				// remote imports
				if isRemoteImport(name) {
					importPath = name
				}
				// is sub-module
				if importPath == "" && strings.HasPrefix(name, task.Pkg.Name+"/") {
					submodule := strings.TrimPrefix(name, task.Pkg.Name+"/")
					subPkg := Pkg{
						Name:      task.Pkg.Name,
						Version:   task.Pkg.Version,
						Submodule: submodule,
					}
					subTask := &BuildTask{
						wd:           task.wd, // use current wd to avoid reinstall
						BuildVersion: task.BuildVersion,
						Pkg:          subPkg,
						Alias:        task.Alias,
						Deps:         task.Deps,
						Target:       task.Target,
						DevMode:      task.DevMode,
					}
					subTask.build(tracing)
					if err != nil {
						return
					}
					importPath = task.getImportPath(subPkg, task.resolvePrefix(), false)
				}
				// is builtin `buffer` module
				if importPath == "" && name == "buffer" {
					if task.Target == "node" {
						importPath = "buffer"
					} else {
						importPath = fmt.Sprintf("/v%d/node_buffer.js", task.BuildVersion)
					}
				}
				// is builtin node module
				if importPath == "" && builtInNodeModules[name] {
					if task.Target == "node" {
						importPath = name
					} else if task.Target == "deno" && denoStdNodeModules[name] {
						importPath = fmt.Sprintf("https://deno.land/std@%s/node/%s.ts", denoStdVersion, name)
					} else {
						polyfill, ok := polyfilledBuiltInNodeModules[name]
						if ok {
							p, submodule, _, e := getPackageInfo(task.wd, polyfill, "latest")
							if e != nil {
								err = e
								return
							}
							importPath = task.getImportPath(Pkg{
								Name:      p.Name,
								Version:   p.Version,
								Submodule: submodule,
							}, "", false)
							importPath = strings.TrimSuffix(importPath, ".js") + ".bundle.js"
						} else {
							_, err := embedFS.ReadFile(fmt.Sprintf("server/embed/polyfills/node_%s.js", name))
							if err == nil {
								importPath = fmt.Sprintf("/v%d/node_%s.js", task.BuildVersion, name)
							} else {
								importPath = fmt.Sprintf(
									"/error.js?type=unsupported-nodejs-builtin-module&name=%s&importer=%s",
									name,
									task.Pkg.Name,
								)
							}
						}
					}
				}
				// use version defined by `?deps` query
				if importPath == "" {
					for _, dep := range task.Deps {
						if name == dep.Name || strings.HasPrefix(name, dep.Name+"/") {
							var submodule string
							if name != dep.Name {
								submodule = strings.TrimPrefix(name, dep.Name+"/")
							}
							importPath = task.getImportPath(Pkg{
								Name:      dep.Name,
								Version:   dep.Version,
								Submodule: submodule,
							}, "", false)
							break
						}
					}
				}
				// force the dependency version of `react` equals to react-dom's version
				if importPath == "" && task.Pkg.Name == "react-dom" && name == "react" {
					importPath = task.getImportPath(Pkg{
						Name:    name,
						Version: task.Pkg.Version,
					}, "", false)
				}
				// common npm dependency
				if importPath == "" {
					version := "latest"
					if v, ok := esm.Dependencies[name]; ok {
						version = v
					} else if v, ok := esm.PeerDependencies[name]; ok {
						version = v
					}
					p, submodule, _, e := getPackageInfo(task.wd, name, version)
					if e != nil {
						err = e
						return
					}

					pkg := Pkg{
						Name:      p.Name,
						Version:   p.Version,
						Submodule: submodule,
					}
					t := &BuildTask{
						BuildVersion: task.BuildVersion,
						Pkg:          pkg,
						Alias:        task.Alias,
						Deps:         task.Deps,
						Target:       task.Target,
						DevMode:      task.DevMode,
					}

					_, _err := findModule(t.ID())
					if _err == storage.ErrNotFound {
						buildQueue.Add(t)
					}

					importPath = task.getImportPath(pkg, "", true)
				}
				if importPath == "" {
					err = fmt.Errorf("Could not resolve \"%s\" (Imported by \"%s\")", name, task.Pkg.Name)
					return
				}
				buffer := bytes.NewBuffer(nil)
				identifier := identify(name)
				slice := bytes.Split(outputContent, []byte(fmt.Sprintf("\"__ESM_SH_EXTERNAL:%s\"", name)))
				cjsContext := false
				cjsImports := newStringSet()
				for i, p := range slice {
					if cjsContext {
						p = bytes.TrimPrefix(p, []byte{')'})
						var marked bool
						if _, ok := builtInNodeModules[name]; !ok {
							pkg, _, err := parsePkg(name)
							if err == nil && !fileExists(path.Join(task.wd, "node_modules", pkg.Name, "package.json")) {
								for i := 0; i < 3; i++ {
									err = yarnAdd(task.wd, fmt.Sprintf("%s@%s", pkg.Name, pkg.Version))
									if err == nil && !fileExists(path.Join(task.wd, "node_modules", pkg.Name, "package.json")) {
										err = fmt.Errorf("yarnAdd(%s): package.json not found", pkg)
									}
									if err == nil {
										break
									}
									if i < 2 {
										time.Sleep(100 * time.Millisecond)
									}
								}
							}
							if err == nil {
								dep, err := initModule(task.wd, *pkg, task.Target, task.DevMode)
								if err == nil {
									if bytes.HasPrefix(p, []byte{'.'}) {
										// right shift to strip the object `key`
										shift := 0
										for i, l := 1, len(p); i < l; i++ {
											c := p[i]
											if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$' {
												shift++
											} else {
												break
											}
										}
										// support edge case like `require('htmlparser').Parser`
										importName := string(p[1 : shift+1])
										for _, v := range dep.Exports {
											if v == importName {
												cjsImports.Add(importName)
												marked = true
												p = p[1:]
												break
											}
										}
									}
									// if the dependency is an es module without `default` export, then use star import
									if !marked && dep.Module != "" && !dep.ExportDefault {
										cjsImports.Add("*")
										marked = true
									}
									// if the dependency is an cjs module with `default` export, then use star import
									if !marked && dep.Module == "" && dep.ExportDefault {
										cjsImports.Add("__esModule")
										marked = true
									}
								}
							}
						}
						if !marked {
							cjsImports.Add("default")
						}
					}
					cjsContext = bytes.HasSuffix(p, []byte{'('}) && !bytes.HasSuffix(p, []byte("import("))
					if cjsContext {
						// left shift to strip the `require` ident generated by esbuild
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
					}
					buffer.Write(p)
					if i < len(slice)-1 {
						if cjsContext {
							buffer.WriteString(fmt.Sprintf("__%s$", identifier))
						} else {
							buffer.WriteString(fmt.Sprintf("\"%s\"", importPath))
						}
					}
				}

				if cjsImports.Size() > 0 {
					buf := bytes.NewBuffer(nil)
					// todo: spread `?alias` and `?deps`
					for _, importName := range cjsImports.Values() {
						if name == "object-assign" {
							fmt.Fprintf(buf, `const __%s$ = Object.assign;%s`, identifier, eol)
						} else {
							switch importName {
							case "default":
								fmt.Fprintf(buf, `import __%s$ from "%s";%s`, identifier, importPath, eol)
							case "*":
								fmt.Fprintf(buf, `import * as __%s$ from "%s";%s`, identifier, importPath, eol)
							case "__esModule":
								fmt.Fprintf(buf, `import * as __%s$$ from "%s";const __%s$=Object.assign({__esModule:true},__%s$$);%s`, identifier, importPath, identifier, identifier, eol)
							default:
								fmt.Fprintf(buf, `import { %s as __%s$%s } from "%s";%s`, importName, identifier, importName, importPath, eol)
							}
						}
					}
					outputContent = make([]byte, buf.Len()+buffer.Len())
					copy(outputContent, buf.Bytes())
					copy(outputContent[buf.Len():], buffer.Bytes())
				} else {
					outputContent = buffer.Bytes()
				}
			}

			// add nodejs/deno compatibility
			if task.Target != "node" {
				if bytes.Contains(outputContent, []byte("__Process$")) {
					if task.Target == "deno" {
						fmt.Fprintf(buf, `import __Process$ from "https://deno.land/std@%s/node/process.ts";%s`, denoStdVersion, eol)
					} else {
						fmt.Fprintf(buf, `import __Process$ from "/v%d/node_process.js";%s`, task.BuildVersion, eol)
					}
				}
				if bytes.Contains(outputContent, []byte("__Buffer$")) {
					if task.Target == "deno" {
						fmt.Fprintf(buf, `import  { Buffer as __Buffer$ } from "https://deno.land/std@%s/node/buffer.ts";%s`, denoStdVersion, eol)
					} else {
						fmt.Fprintf(buf, `import { Buffer as __Buffer$ } from "/v%d/node_buffer.js";%s`, task.BuildVersion, eol)
					}
				}
				if bytes.Contains(outputContent, []byte("__global$")) {
					fmt.Fprintf(buf, `var __global$ = globalThis || (typeof window !== "undefined" ? window : self);%s`, eol)
				}
				if bytes.Contains(outputContent, []byte("__setImmediate$")) {
					fmt.Fprintf(buf, `var __setImmediate$ = (cb, ...args) => setTimeout(cb, 0, ...args);%s`, eol)
				}
				if bytes.Contains(outputContent, []byte("__rResolve$")) {
					fmt.Fprintf(buf, `var __rResolve$ = p => p;%s`, eol)
				}
			}

			if task.Target == "deno" {
				if task.DevMode {
					outputContent = bytes.Replace(outputContent, []byte("typeof window !== \"undefined\""), []byte("typeof document !== \"undefined\""), -1)
				} else {
					outputContent = bytes.Replace(outputContent, []byte("typeof window<\"u\""), []byte("typeof document<\"u\""), -1)
				}
			}

			_, err = buf.Write(outputContent)
			if err != nil {
				return
			}

			err = fs.WriteData(path.Join("builds", task.ID()), buf.Bytes())
			if err != nil {
				return
			}
		} else if strings.HasSuffix(file.Path, ".css") {
			err = fs.WriteData(path.Join("builds", strings.TrimSuffix(task.ID(), ".js")+".css"), outputContent)
			if err != nil {
				return
			}
			esm.PackageCSS = true
		}
	}

	task.findDTS(esm)
	task.storeToDB(esm)
	return
}

func (task *BuildTask) storeToDB(esm *Module) {
	dbErr := db.Put(
		task.ID(),
		"build",
		storage.Store{
			"esm": string(utils.MustEncodeJSON(esm)),
		},
	)
	if dbErr != nil {
		log.Errorf("db: %v", dbErr)
	}
}

func (task *BuildTask) findDTS(esm *Module) {
	name := task.Pkg.Name
	submodule := task.Pkg.Submodule

	var dts string
	if esm.Types != "" {
		dts = toTypesPath(task.wd, *esm.NpmPackage, submodule)
	} else if !strings.HasPrefix(name, "@types/") {
		versions := []string{"latest"}
		versionParts := strings.Split(task.Pkg.Version, ".")
		if len(versionParts) > 2 {
			versions = append([]string{
				"~" + strings.Join(versionParts[:2], "."), // minor
				"^" + versionParts[0],                     // major
			}, versions...)
		}
		typesPkgName := toTypesPackageName(name)
		pkg, ok := task.Deps.Get(typesPkgName)
		if ok {
			versions = append([]string{pkg.Version}, versions...)
		}
		for _, version := range versions {
			p, _, _, err := getPackageInfo(task.wd, typesPkgName, version)
			if err == nil {
				dts = toTypesPath(task.wd, p, submodule)
				break
			}
		}
	}
	if dts != "" {
		esm.Dts = fmt.Sprintf("/v%d/%s", task.BuildVersion, dts)
	}
}

func (task *BuildTask) transformDTS(dts string) {
	start := time.Now()
	n, err := task.CopyDTS(dts, task.BuildVersion)
	if err != nil && os.IsExist(err) {
		log.Errorf("copyDTS(%s): %v", dts, err)
		return
	}
	log.Debugf("copy dts '%s' (%d files copied) in %v", dts, n, time.Since(start))
}
