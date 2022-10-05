package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/ije/gox/utils"
)

func (task *BuildTask) CopyDTS(dts string, buildVersion int) (n int, err error) {
	buildArgsPrefix := encodeBuildArgsPrefix(task.BuildArgs, task.Pkg, true)
	tracing := newStringSet()
	err = task.copyDTS(dts, buildVersion, buildArgsPrefix, tracing)
	if err == nil {
		n = tracing.Size()
	}
	return
}

func (task *BuildTask) copyDTS(dts string, buildVersion int, aliasDepsPrefix string, tracing *stringSet) (err error) {
	// don't copy repeatly
	if tracing.Has(aliasDepsPrefix + dts) {
		return
	}
	tracing.Add(aliasDepsPrefix + dts)

	var taskPkgInfo NpmPackage
	taskPkgJsonPath := path.Join(task.wd, "node_modules", task.Pkg.Name, "package.json")
	err = utils.ParseJSONFile(taskPkgJsonPath, &taskPkgInfo)
	if err != nil {
		return
	}

	pkgNameInfo := parsePkgNameInfo(utils.CleanPath(dts)[1:])
	versionedName := pkgNameInfo.Fullname
	subPath := strings.Split(pkgNameInfo.Submodule, "/")
	pkgName, _ := utils.SplitByLastByte(versionedName, '@')
	if pkgName == "" {
		pkgName = versionedName
	}

	buildBasePath := fmt.Sprintf("/v%d", buildVersion)
	cdnOriginAndBasePath := task.CdnOrigin + task.BasePath
	cdnOriginAndBuildBasePath := task.CdnOrigin + task.BasePath + buildBasePath

	dtsPath := utils.CleanPath(strings.Join(append([]string{
		buildBasePath,
		versionedName,
		aliasDepsPrefix,
	}, subPath...), "/"))
	savePath := "types" + dtsPath
	exists, _, _, err := fs.Exists(savePath)
	if err != nil || exists {
		return
	}

	imports := newStringSet()
	internalDeclareModules := newStringSet()
	entryDeclareModules := []string{}

	dtsFilePath := path.Join(task.wd, "node_modules", regFullVersionPath.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		return
	}

	pass1Buf := bytes.NewBuffer(nil)
	err = walkDts(dtsFile, pass1Buf, func(importPath string, kind string, position int) string {
		if kind == "declare module" {
			internalDeclareModules.Add(importPath)
		}
		if kind == "import expr" || kind == "import call" {
			imports.Add(importPath)
		}
		return importPath
	})
	// close the opened dts file
	dtsFile.Close()
	if err != nil {
		return
	}

	for _, importPath := range imports.Values() {
		if !internalDeclareModules.Has(importPath) {
			internalDeclareModules.Remove(importPath)
		}
	}
	imports.Reset()

	buf := bytes.NewBuffer(nil)
	if pkgName == "@types/node" {
		fmt.Fprintf(buf, "/// <reference path=\"%s/node.ns.d.ts\" />\n", cdnOriginAndBuildBasePath)
	}
	err = walkDts(pass1Buf, buf, func(importPath string, kind string, position int) string {
		// resove `declare module "xxx" {}`
		if kind == "declare module" {
			moduleName := pkgName
			if len(subPath) > 0 {
				moduleName += "/" + strings.Join(subPath, "/")
				if strings.HasSuffix(moduleName, "/index.d.ts") {
					moduleName = strings.TrimSuffix(moduleName, "/index.d.ts")
				} else {
					moduleName = strings.TrimSuffix(moduleName, ".d.ts")
				}
			}
			if strings.HasPrefix(importPath, "node:") {
				importPath = "@types/node/" + strings.TrimPrefix(importPath, "node:")
			}
			// current module
			if importPath == moduleName {
				if strings.HasPrefix(moduleName, "@types/node/") {
					return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", cdnOriginAndBuildBasePath, nodeTypesVersion, strings.TrimPrefix(moduleName, "@types/node/"))
				}
				url := fmt.Sprintf("%s/%s", cdnOriginAndBasePath, moduleName)
				entryDeclareModules = append(entryDeclareModules, fmt.Sprintf("%s:%d", moduleName, position+len(url)+1))
				return url
			}
			if internalDeclareModules.Has(importPath) {
				return importPath
			}
		}

		if task.external.Has("*") && !isLocalImport(importPath) {
			return importPath
		}

		// fix import path
		switch importPath {
		case "node-fetch":
			importPath = "node-fetch-native"
		case "estree", "estree-jsx", "unist", "react", "react-dom":
			importPath = fmt.Sprintf("@types/%s", importPath)
		}

		// fix some weird import paths
		if kind == "import call" {
			if task.Pkg.Name == "@mdx-js/mdx" {
				if (strings.Contains(dts, "plugin/recma-document") || strings.Contains(dts, "plugin/recma-jsx-rewrite")) && importPath == "@types/estree" {
					importPath = "@types/estree-jsx"
				}
			}
			if strings.HasPrefix(dts, "remark-rehype") && importPath == "mdast-util-to-hast/lib" {
				importPath = "mdast-util-to-hast"
			}
		}

		// use `?alias`
		to, ok := task.alias[importPath]
		if ok {
			importPath = to
		}

		if internalDeclareModules.Has(importPath) || task.external.Has(importPath) {
			return importPath
		}

		if isLocalImport(importPath) {
			if importPath == "." {
				importPath = "./index.d.ts"
			}
			if importPath == ".." {
				importPath = "../index.d.ts"
			}
			// some types is using `.js` extname
			importPath = strings.TrimSuffix(importPath, ".js")
			if !strings.HasSuffix(importPath, ".d.ts") {
				if fileExists(path.Join(dtsDir, importPath, "index.d.ts")) {
					importPath = strings.TrimSuffix(importPath, "/") + "/index.d.ts"
				} else if fileExists(path.Join(dtsDir, importPath+".d.ts")) {
					importPath = importPath + ".d.ts"
				} else {
					var p NpmPackage
					packageJSONFile := path.Join(dtsDir, importPath, "package.json")
					if fileExists(packageJSONFile) && utils.ParseJSONFile(packageJSONFile, &p) == nil {
						if p.Types != "" {
							importPath = strings.TrimSuffix(importPath, "/") + utils.CleanPath(p.Types)
						} else if p.Typings != "" {
							importPath = strings.TrimSuffix(importPath, "/") + utils.CleanPath(p.Typings)
						}
					}
				}
			}
			if strings.HasSuffix(dts, ".d.ts") && !strings.HasSuffix(dts, "~.d.ts") {
				imports.Add(importPath)
			}
		} else {
			if importPath == "node" {
				importPath = fmt.Sprintf("%s/node.ns.d.ts", cdnOriginAndBuildBasePath)
				return importPath
			}
			if strings.HasPrefix(importPath, "node:") {
				importPath = fmt.Sprintf("%s/@types/node@%s/%s.d.ts", cdnOriginAndBuildBasePath, nodeTypesVersion, strings.TrimPrefix(importPath, "node:"))
				return importPath
			}
			if _, ok := builtInNodeModules[importPath]; ok {
				importPath = fmt.Sprintf("%s/@types/node@%s/%s.d.ts", cdnOriginAndBuildBasePath, nodeTypesVersion, importPath)
				return importPath
			}

			pkgNameInfo := parsePkgNameInfo(importPath)
			depTypePkgName := pkgNameInfo.Fullname
			maybeVersion := []string{"latest"}
			if v, ok := taskPkgInfo.Dependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			} else if v, ok := taskPkgInfo.PeerDependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			}

			// use version defined in `?deps`
			if pkg, ok := task.deps.Get(depTypePkgName); ok {
				maybeVersion = []string{
					pkg.Version,
					"latest",
				}
			}

			var (
				info            NpmPackage
				subpath         string
				fromPackageJSON bool
			)
			for _, version := range maybeVersion {
				var pkg *Pkg
				pkg, _, err = parsePkg(importPath)
				if err != nil {
					break
				}
				info, fromPackageJSON, err = getPackageInfo(task.wd, pkg.Name, version)
				if err != nil || ((info.Types == "" && info.Typings == "") && !strings.HasPrefix(info.Name, "@types/")) {
					info, fromPackageJSON, err = getPackageInfo(task.wd, toTypesPackageName(pkg.Name), version)
				}
				if err == nil {
					subpath = pkg.Submodule
					break
				}
			}
			if err != nil {
				return importPath
			}

			pkgBase := info.Name + "@" + info.Version + "/"

			if info.Types != "" || info.Typings != "" {
				// copy dependent dts files in the node_modules directory in current build context
				if fromPackageJSON {
					typesPath := toTypesPath(task.wd, &info, "", "", subpath)
					if strings.HasSuffix(typesPath, ".d.ts") && !strings.HasSuffix(typesPath, "~.d.ts") {
						imports.Add(typesPath)
					}
					importPath = strings.TrimPrefix(typesPath, pkgBase)
				} else {
					if info.Types != "" {
						if subpath != "" && strings.HasSuffix(info.Types, ".d.ts") {
							info.Types = path.Join(subpath, info.Types)
						}
						importPath = utils.CleanPath(info.Types)[1:]
					} else if info.Typings != "" {
						if subpath != "" && strings.HasSuffix(info.Typings, ".d.ts") {
							info.Typings = path.Join(subpath, info.Typings)
						}
						importPath = utils.CleanPath(info.Typings)[1:]
					}
					if !strings.HasSuffix(importPath, ".d.ts") {
						importPath += "~.d.ts"
					}
				}
			}

			pkgBasePath := pkgBase + encodeBuildArgsPrefix(task.BuildArgs, Pkg{Name: info.Name, Version: info.Version}, true)
			importPath = fmt.Sprintf("%s/%s", cdnOriginAndBuildBasePath, pkgBasePath+importPath)
		}

		return importPath
	})
	if err != nil {
		return
	}

	if len(entryDeclareModules) > 0 {
		dtsData := buf.Bytes()
		dataLen := buf.Len()
		for _, record := range entryDeclareModules {
			name, pos := utils.SplitByLastByte(record, ':')
			i, _ := strconv.Atoi(pos)
			b := bytes.NewBuffer(nil)
			open := false
			internal := 0
			for ; i < dataLen; i++ {
				c := dtsData[i]
				b.WriteByte(c)
				if c == '{' {
					if !open {
						open = true
					} else {
						internal++
					}
				} else if c == '}' && open {
					if internal > 0 {
						internal--
					} else {
						open = false
						break
					}
				}
			}
			if b.Len() > 0 {
				pkgNameInfo := parsePkgNameInfo(name)
				name = pkgNameInfo.Fullname
				subpath := ""
				if pkgNameInfo.Submodule != "" {
					subpath = "/" + pkgNameInfo.Submodule
				}

				fmt.Fprintf(buf, `%sdeclare module "%s/%s@*%s" `, "\n", cdnOriginAndBasePath, name, subpath)
				buf.WriteString(strings.TrimSpace(b.String()))
			}
		}
	}

	// workaroud for `@types/node`
	if pkgName == "@types/node" {
		dtsData := buf.Bytes()
		dtsData = bytes.ReplaceAll(dtsData, []byte(" implements NodeJS.ReadableStream"), []byte{})
		dtsData = bytes.ReplaceAll(dtsData, []byte(" implements NodeJS.WritableStream"), []byte{})
		if strings.HasSuffix(savePath, "/buffer.d.ts") {
			dtsData = bytes.ReplaceAll(dtsData, []byte(" export { Buffer };"), []byte(" export const Buffer: Buffer;"))
		}
		if strings.HasSuffix(savePath, "/url.d.ts") || strings.HasSuffix(savePath, "/buffer.d.ts") {
			dtsData, err = removeGlobalBlob(dtsData)
			if err != nil {
				return
			}
		}
		buf = bytes.NewBuffer(dtsData)
	}

	// fix preact/compat types
	if pkgName == "preact" && strings.HasSuffix(savePath, "/compat/src/index.d.ts") {
		dtsData := buf.Bytes()
		dtsData = bytes.ReplaceAll(
			dtsData,
			[]byte("export import ComponentProps = preact.ComponentProps;"),
			[]byte("export import ComponentProps = preact.ComponentProps;\n\n// added by esm.sh\nexport type PropsWithChildren<P = unknown> = P & { children?: preact.ComponentChildren };"),
		)
		buf = bytes.NewBuffer(dtsData)
	}

	err = fs.WriteData(savePath, buf.Bytes())
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, importDts := range imports.Values() {
		if isLocalImport(importDts) {
			if strings.HasPrefix(importDts, "/") {
				pkg, subpath := utils.SplitByFirstByte(importDts, '/')
				if strings.HasPrefix(pkg, "@") {
					n, _ := utils.SplitByFirstByte(subpath, '/')
					pkg = fmt.Sprintf("%s/%s", pkg, n)
				}
				importDts = path.Join(pkg, importDts)
			} else {
				importDts = path.Join(path.Dir(dts), importDts)
			}
		}
		wg.Add(1)
		go func(importDts string) {
			err := task.copyDTS(importDts, buildVersion, aliasDepsPrefix, tracing)
			if err != nil {
				errors = append(errors, err)
			}
			wg.Done()
		}(importDts)
	}
	wg.Wait()

	if len(errors) > 0 {
		err = errors[0]
	}

	return
}

// to remove `global { ... }`
func removeGlobalBlob(input []byte) (output []byte, err error) {
	start := bytes.Index(input, []byte("global {"))
	if start == -1 {
		return input, nil
	}
	dep := 1
	for i := start + 8; i < len(input); i++ {
		c := input[i]
		if c == '{' {
			dep++
		} else if c == '}' {
			dep--
		}
		if dep == 0 {
			return bytes.Join([][]byte{input[:start], input[i+1:]}, nil), nil
		}
	}
	return nil, errors.New("removeGlobalBlob: global block not end")
}

func toTypesPath(wd string, p *NpmPackage, version string, buildArgsPrefix string, subpath string) string {
	var types string
	if subpath != "" {
		types = subpath
		packageJSONFile := path.Join(wd, "node_modules", p.Name, subpath, "package.json")
		if fileExists(packageJSONFile) {
			var sp NpmPackage
			if utils.ParseJSONFile(packageJSONFile, &sp) == nil {
				if sp.Types != "" {
					types = path.Join(subpath, sp.Types)
				} else if sp.Typings != "" {
					types = path.Join(subpath, sp.Typings)
				}
			}
		}
	} else if p.Types != "" {
		types = p.Types
	} else if p.Typings != "" {
		types = p.Typings
	} else {
		return ""
	}

	if !strings.HasSuffix(types, ".d.ts") {
		pkgDir := path.Join(wd, "node_modules", p.Name)
		if fileExists(path.Join(pkgDir, types, "index.d.ts")) {
			types = types + "/index.d.ts"
		} else if fileExists(path.Join(pkgDir, types+".d.ts")) {
			types = types + ".d.ts"
		} else {
			types = types + "~.d.ts" // dynamic
		}
	}

	if version == "" {
		version = p.Version
	}
	return fmt.Sprintf("%s@%s/%s%s", p.Name, version, buildArgsPrefix, utils.CleanPath(types)[1:])
}
