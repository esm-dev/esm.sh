package server

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

func CopyDTS(wd string, resolvePrefix string, dts string) (dtsPath string, err error) {
	return copyDTS(wd, resolvePrefix, dts, newStringSet())
}

func copyDTS(wd string, resolvePrefix string, dts string, tracing *stringSet) (dtsPath string, err error) {
	// don't copy repeatly
	if tracing.Has(resolvePrefix + dts) {
		return
	}
	tracing.Add(resolvePrefix + dts)

	a := strings.Split(utils.CleanPath(dts)[1:], "/")
	versionedName := a[0]
	subPath := a[1:]
	if strings.HasPrefix(versionedName, "@") {
		versionedName = strings.Join(a[0:2], "/")
		subPath = a[2:]
	}
	pkgName, _ := utils.SplitByLastByte(versionedName, '@')
	if pkgName == "" {
		pkgName = versionedName
	}

	dtsPath = path.Join(append([]string{
		fmt.Sprintf("/v%d", VERSION), versionedName, resolvePrefix,
	}, subPath...)...)
	savePath := path.Join("types", dtsPath)
	exists, err := fs.Exists(savePath)
	if err != nil || exists {
		return
	}

	imports := newStringSet()
	allDeclareModules := newStringSet()
	entryDeclareModules := []string{}

	dtsFilePath := path.Join(wd, "node_modules", regVersionPath.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		return
	}

	dtsBuffer := bytes.NewBuffer(nil)
	err = walkDts(dtsFile, dtsBuffer, func(importPath string, kind string, position int) string {
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
	err = walkDts(dtsBuffer, buf, func(importPath string, kind string, position int) string {
		if kind == "declare module" {
			// resove `declare module "xxx" {}`, and the "xxx" must equal to the `pkgName`
			if importPath == pkgName {
				origin := ""
				if config.cdnDomain == "localhost" || strings.HasPrefix(config.cdnDomain, "localhost:") {
					origin = fmt.Sprintf("http://%s", config.cdnDomain)
				} else if config.cdnDomain != "" {
					origin = fmt.Sprintf("https://%s", config.cdnDomain)
				}
				res := fmt.Sprintf("%s/%s", origin, pkgName)
				entryDeclareModules = append(entryDeclareModules, fmt.Sprintf("%s:%d", res, position+len(res)+1))
				return res
			}
			return importPath
		}

		if allDeclareModules.Has(importPath) {
			return importPath
		}

		if isLocalImport(importPath) {
			if importPath == "." {
				importPath = "./index.d.ts"
			}
			if importPath == ".." {
				importPath = "../index.d.ts"
			}
			if !strings.HasSuffix(importPath, ".d.ts") {
				if fileExists(path.Join(dtsDir, importPath, "index.d.ts")) {
					importPath = strings.TrimSuffix(importPath, "/") + "/index.d.ts"
				} else {
					var p NpmPackage
					packageJSONFile := path.Join(dtsDir, importPath, "package.json")
					if fileExists(packageJSONFile) && utils.ParseJSONFile(packageJSONFile, &p) == nil {
						types := getTypesPath(wd, p, "")
						if types != "" {
							_, typespath := utils.SplitByFirstByte(types, '/')
							importPath = strings.TrimSuffix(importPath, "/") + "/" + typespath
						} else {
							importPath = ensureSuffix(strings.TrimSuffix(importPath, ".js"), ".d.ts")
						}
					} else {
						importPath = ensureSuffix(strings.TrimSuffix(importPath, ".js"), ".d.ts")
					}
				}
			}
			imports.Add(importPath)
		} else {
			if importPath == "node" {
				importPath = fmt.Sprintf("/v%d/node.ns.d.ts", VERSION)
			} else {
				if _, ok := builtInNodeModules[importPath]; ok {
					importPath = "@types/node/" + importPath
				}
				if _, ok := builtInNodeModules["node:"+importPath]; ok {
					importPath = "@types/node/" + strings.TrimPrefix(importPath, "node:")
				}
				info, subpath, formPackageJSON, err := node.getPackageInfo(wd, importPath, "latest")
				if err == nil {
					if !strings.HasPrefix(info.Name, "@types/") && info.Types == "" && info.Typings == "" {
						p, _, b, e := node.getPackageInfo(wd, path.Join("@types", info.Name), "latest")
						if e == nil {
							info = p
							formPackageJSON = b
						}
					}
					if info.Types != "" || info.Typings != "" {
						versioned := info.Name + "@" + info.Version
						// copy dependent dts files in the node_modules directory in current build context
						if formPackageJSON {
							dts := subpath
							if dts == "" {
								if info.Types != "" {
									dts = info.Types
								} else if info.Typings != "" {
									dts = info.Typings
								}
							}
							if dts != "" {
								if !strings.HasSuffix(dts, ".d.ts") {
									if fileExists(path.Join(wd, "node_modules", info.Name, dts, "index.d.ts")) {
										dts = path.Join(dts, "index.d.ts")
									} else if fileExists(path.Join(wd, "node_modules", info.Name, dts+".d.ts")) {
										dts = dts + ".d.ts"
									}
								}
								imports.Add(path.Join(versioned, dts))
							}
						}
						if subpath == "" {
							if info.Types != "" {
								importPath = path.Join(versioned, resolvePrefix, utils.CleanPath(info.Types))
							} else if info.Typings != "" {
								importPath = path.Join(versioned, resolvePrefix, utils.CleanPath(info.Typings))
							}
						} else {
							importPath = path.Join(versioned, resolvePrefix, utils.CleanPath(subpath))
						}
						importPath = fmt.Sprintf("/v%d/%s", VERSION, importPath)
						if !strings.HasSuffix(importPath, ".d.ts") {
							importPath += "...d.ts"
						}
					}
				}
			}

			// `<reference types="..." />` should be a full URL in deno.
			if kind == "reference types" && (!strings.HasPrefix(importPath, "https://") || !strings.HasPrefix(importPath, "http://")) {
				origin := ""
				if config.cdnDomain == "localhost" || strings.HasPrefix(config.cdnDomain, "localhost:") {
					origin = fmt.Sprintf("http://%s", config.cdnDomain)
				} else if config.cdnDomain != "" {
					origin = fmt.Sprintf("https://%s", config.cdnDomain)
				}
				importPath = origin + importPath
			}
		}

		return importPath
	})
	if err != nil {
		return
	}

	dtsData := buf.Bytes()
	dataLen := buf.Len()
	if len(entryDeclareModules) > 0 {
		for _, record := range entryDeclareModules {
			name, istr := utils.SplitByLastByte(record, ':')
			i, _ := strconv.Atoi(istr)
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
				fmt.Fprintf(buf, `%sdeclare module "%s@*" `, "\n", name)
				fmt.Fprintf(buf, strings.TrimSpace(b.String()))
			}
		}
	}

	err = fs.WriteFile(savePath, buf)
	if err != nil {
		return
	}

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
		_, err = copyDTS(wd, resolvePrefix, importDts, tracing)
		if err != nil {
			break
		}
	}

	return
}

func getTypesPath(wd string, p NpmPackage, subpath string) string {
	var types string
	if subpath != "" {
		var subpkg NpmPackage
		var subtypes string
		subpkgJSONFile := path.Join(wd, "node_modules", p.Name, subpath, "package.json")
		if fileExists(subpkgJSONFile) && utils.ParseJSONFile(subpkgJSONFile, &subpkg) == nil {
			if subpkg.Types != "" {
				subtypes = subpkg.Types
			} else if subpkg.Typings != "" {
				subtypes = subpkg.Typings
			}
		}
		if subtypes != "" {
			types = path.Join("/", subpath, subtypes)
		} else {
			types = subpath
		}
	} else {
		if p.Types != "" {
			types = p.Types
		} else if p.Typings != "" {
			types = p.Typings
		} else if p.Main != "" {
			types = strings.TrimSuffix(p.Main, ".js")
		} else {
			types = "index.d.ts"
		}
	}

	if !strings.HasSuffix(types, ".d.ts") {
		if fileExists(path.Join(wd, "node_modules", p.Name, types, "index.d.ts")) {
			types = types + "/index.d.ts"
		} else if fileExists(path.Join(wd, "node_modules", p.Name, types+".d.ts")) {
			types = types + ".d.ts"
		}
	}

	return fmt.Sprintf("%s@%s/%s", p.Name, p.Version, strings.TrimPrefix(ensureSuffix(types, ".d.ts"), "/"))
}
