package server

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/ije/gox/utils"
)

func (task *BuildTask) CopyDTS(dts string, buildVersion int) (n int, err error) {
	resolveArgsPrefix := encodeResolveArgsPrefix(task.Alias, task.Deps, task.External)
	tracing := newStringSet()
	err = task.copyDTS(dts, buildVersion, resolveArgsPrefix, tracing)
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
	cdnOriginAndBasePath := task.CdnOrigin + basePath
	cdnOriginAndBuildBasePath := task.CdnOrigin + basePath + buildBasePath

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
	allDeclareModules := newStringSet()
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
			allDeclareModules.Add(importPath)
		}
		return importPath
	})
	// close the opened dts file
	dtsFile.Close()
	if err != nil {
		return
	}

	buf := bytes.NewBuffer(nil)
	if pkgName == "@types/node" {
		fmt.Fprintf(buf, "/// <reference path=\"%s/node.ns.d.ts\" />\n", cdnOriginAndBuildBasePath)
	}
	err = walkDts(pass1Buf, buf, func(importPath string, kind string, position int) string {
		// resove `declare module "xxx" {}`, and the "xxx" must equal to the `moduleName`
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
			if importPath == moduleName {
				if strings.HasPrefix(moduleName, "@types/node/") {
					return fmt.Sprintf("%s/%s.d.ts", cdnOriginAndBuildBasePath, moduleName)
				}
				res := fmt.Sprintf("%s/%s", cdnOriginAndBasePath, moduleName)
				entryDeclareModules = append(entryDeclareModules, fmt.Sprintf("%s:%d", moduleName, position+len(res)+1))
				return res
			}
			return importPath
		}

		to, ok := task.Alias[importPath]
		if ok {
			importPath = to
		}
		if importPath == "node-fetch" {
			importPath = "node-fetch-native"
		}

		if allDeclareModules.Has(importPath) || task.External.Has(importPath) {
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
				importPath = fmt.Sprintf("%s/@types/node/%s.d.ts", cdnOriginAndBuildBasePath, strings.TrimPrefix(importPath, "node:"))
				return importPath
			}
			if _, ok := builtInNodeModules[importPath]; ok {
				importPath = fmt.Sprintf("%s/@types/node/%s.d.ts", cdnOriginAndBuildBasePath, importPath)
				return importPath
			}

			pkgNameInfo := parsePkgNameInfo(importPath)
			depTypePkgName := pkgNameInfo.Fullname
			versions := []string{"latest"}
			if v, ok := taskPkgInfo.Dependencies[depTypePkgName]; ok {
				versions = []string{v, "latest"}
			} else if v, ok := taskPkgInfo.PeerDependencies[depTypePkgName]; ok {
				versions = []string{v, "latest"}
			}

			// use version defined in `?deps`
			if pkg, ok := task.Deps.Get(depTypePkgName); ok {
				versionParts := strings.Split(pkg.Version, ".")
				if len(versionParts) > 2 {
					versions = []string{
						"~" + strings.Join(versionParts[:2], "."), // minor
						"^" + versionParts[0],                     // major
						"latest",
					}
				}
			}

			var (
				info            NpmPackage
				subpath         string
				fromPackageJSON bool
			)
			for _, version := range versions {
				info, subpath, fromPackageJSON, err = getPackageInfo(task.wd, importPath, version)
				if err != nil || ((info.Types == "" && info.Typings == "") && !strings.HasPrefix(info.Name, "@types/")) {
					info, _, fromPackageJSON, err = getPackageInfo(task.wd, toTypesPackageName(importPath), version)
				}
				if err == nil {
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

			alias, deps := fixResolveArgs(task.Alias, task.Deps, info.Name)
			pkgBasePath := pkgBase + encodeResolveArgsPrefix(alias, deps, task.External)

			// CDN URL
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

func toTypesPath(wd string, p *NpmPackage, version string, prefix string, subpath string) string {
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
	return fmt.Sprintf("%s@%s/%s%s", p.Name, version, prefix, utils.CleanPath(types)[1:])
}
