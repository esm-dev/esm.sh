package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

var (
	regVersionPath    = regexp.MustCompile(`([^/])@\d+\.\d+\.\d+([a-z0-9\.-]+)?/`)
	regFromExpr       = regexp.MustCompile(`(}|\s)from\s*("|')`)
	regImportCallExpr = regexp.MustCompile(`import\((('[^']+')|("[^"]+"))\)`)
	regReferenceTag   = regexp.MustCompile(`^<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/>$`)
	regDeclareModule  = regexp.MustCompile(`^declare\s+module\s*('|")([^'"]+)("|')`)
)

func copyDTS(nodeModulesDir string, dts string) (err error) {
	dtsFilePath := path.Join(nodeModulesDir, regVersionPath.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warnf("copyDTS(%s): %v", dts, err)
			err = nil
		} else if strings.HasSuffix(err.Error(), "is a directory") {
			log.Warnf("copyDTS(%s): %v", dts, err)
			err = nil
		}
		return
	}
	defer dtsFile.Close()

	saveFilePath := path.Join(config.storageDir, fmt.Sprintf("types/v%d", VERSION), dts)
	fi, err := os.Lstat(saveFilePath)
	if err == nil {
		if fi.IsDir() {
			os.Remove(saveFilePath)
		} else {
			// do not repeat
			return
		}
	}

	deps := newStringSet()
	dmodules := []string{}
	rewriteFn := func(importPath string) string {
		if isFileImportPath(importPath) {
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
						types := getTypesPath(nodeModulesDir, p, "")
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
		} else {
			// nodejs builtin modules
			if _, ok := builtInNodeModules[importPath]; ok {
				importPath = "@types/node/" + importPath
			}
			if _, ok := builtInNodeModules["node:"+importPath]; ok {
				importPath = "@types/node/" + strings.TrimPrefix(importPath, "node:")
			}
			pkgName, subpath := utils.SplitByFirstByte(importPath, '/')
			if strings.HasPrefix(pkgName, "@") {
				n, s := utils.SplitByFirstByte(subpath, '/')
				pkgName = fmt.Sprintf("%s/%s", pkgName, n)
				subpath = s
			}
			var p NpmPackage
			packageJSONFile := path.Join(nodeModulesDir, pkgName, "package.json")
			if fileExists(packageJSONFile) {
				utils.ParseJSONFile(packageJSONFile, &p)
			}
			if p.Name == "" || (p.Types == "" && p.Typings == "") {
				packageJSONFile = path.Join(nodeModulesDir, "@types", pkgName, "package.json")
				if fileExists(packageJSONFile) {
					utils.ParseJSONFile(packageJSONFile, &p)
				}
			}
			if p.Name != "" {
				importPath = getTypesPath(nodeModulesDir, p, subpath)
			} else {
				p, _, err := node.getPackageInfo("@types/"+pkgName, "latest")
				if err != nil && err.Error() == fmt.Sprintf("npm: package '%s' not found", pkgName) {
					p, _, err = node.getPackageInfo(pkgName, "latest")
				}
				if err == nil {
					err = yarnAdd(fmt.Sprintf("%s@%s", p.Name, p.Version))
					if err == nil {
						importPath = getTypesPath(nodeModulesDir, p, subpath)
					}
				}
			}
		}
		deps.Add(importPath)
		if !isFileImportPath(importPath) {
			importPath = "/" + importPath
		}
		if strings.HasPrefix(importPath, "/") {
			importPath = fmt.Sprintf("/v%d%s", VERSION, importPath)
		}
		return importPath
	}

	buf := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(dtsFile)
	commentScope := false
	importExportScope := false
	for scanner.Scan() {
		text := scanner.Text()
		pure := strings.TrimSpace(text)
		spaceLeftWidth := strings.Index(text, pure)
		spacesOnRight := text[spaceLeftWidth+len(pure):]
		buf.WriteString(text[:spaceLeftWidth])
	Re:
		if commentScope || strings.HasPrefix(pure, "/*") {
			commentScope = true
			endIndex := strings.Index(pure, "*/")
			if endIndex > -1 {
				commentScope = false
				buf.WriteString(pure[:endIndex])
				buf.WriteString("*/")
				if rest := pure[endIndex+2:]; rest != "" {
					pure = strings.TrimSpace(rest)
					buf.WriteString(rest[:strings.Index(rest, pure)])
					goto Re
				}
			} else {
				buf.WriteString(pure)
			}
		} else if i := strings.Index(pure, "/*"); i > 0 {
			if startsWith(pure, "import ", "export ", "import{", "export{", "import {", "export {") {
				importExportScope = true
			}
			buf.WriteString(pure[:i])
			pure = pure[i:]
			goto Re
		} else if strings.HasPrefix(pure, "///") {
			s := strings.TrimSpace(strings.TrimPrefix(pure, "///"))
			if regReferenceTag.MatchString(s) {
				a := regReferenceTag.FindAllStringSubmatch(s, 1)
				format := a[0][1]
				path := a[0][3]
				if format == "path" {
					if !isFileImportPath(path) {
						path = "./" + path
					}
				}
				if format == "types" {
					if path == "node" {
						path = fmt.Sprintf("/v%d/node.ns.d.ts", VERSION)
					} else {
						path = rewriteFn(path)
					}
					protocol := "https:"
					if config.domain == "localhost" {
						protocol = "http:"
					}
					fmt.Fprintf(buf, `/// <reference path="%s//%s%s" />`, protocol, config.domain, path)
				} else {
					fmt.Fprintf(buf, `/// <reference path="%s" />`, rewriteFn(path))
				}
			} else {
				buf.WriteString(pure)
			}
		} else if strings.HasPrefix(pure, "//") {
			buf.WriteString(pure)
		} else if strings.HasPrefix(pure, "declare") && regDeclareModule.MatchString(pure) {
			q := "'"
			a := strings.Split(pure, q)
			if len(a) != 3 {
				q = `"`
				a = strings.Split(pure, q)
			}
			if len(a) == 3 && strings.HasPrefix(dts, a[1]) {
				buf.WriteString(a[0])
				buf.WriteString(q)
				newname := fmt.Sprintf("https://%s/%s", config.domain, a[1])
				if config.domain == "localhost" {
					newname = fmt.Sprintf("http://localhost/%s", a[1])
				}
				buf.WriteString(newname)
				buf.WriteString(q)
				dmodules = append(dmodules, fmt.Sprintf("%s:%d", newname, buf.Len()))
				buf.WriteString(a[2])
			} else {
				buf.WriteString(pure)
			}
		} else {
			scanner := bufio.NewScanner(strings.NewReader(pure))
			scanner.Split(onSemicolon)
			var i int
			for scanner.Scan() {
				if i > 0 {
					buf.WriteByte(';')
				}
				text := scanner.Text()
				expr := strings.TrimSpace(text)
				buf.WriteString(text[:strings.Index(text, expr)])
				if expr != "" {
					if importExportScope || startsWith(expr, "import ", "export ", "import{", "export{", "import {", "export {") {
						importExportScope = true
						if regFromExpr.MatchString(expr) {
							importExportScope = false
							q := "'"
							a := strings.Split(expr, q)
							if len(a) != 3 {
								q = `"`
								a = strings.Split(expr, q)
							}
							if len(a) == 3 {
								buf.WriteString(a[0])
								buf.WriteString(q)
								buf.WriteString(rewriteFn(a[1]))
								buf.WriteString(q)
								buf.WriteString(a[2])
							} else {
								buf.WriteString(expr)
							}
						} else {
							if regImportCallExpr.MatchString(expr) {
								buf.WriteString(regImportCallExpr.ReplaceAllStringFunc(expr, func(importCallExpr string) string {
									q := "'"
									a := strings.Split(importCallExpr, q)
									if len(a) != 3 {
										q = `"`
										a = strings.Split(importCallExpr, q)
									}
									if len(a) == 3 {
										buf := bytes.NewBuffer(nil)
										buf.WriteString(a[0])
										buf.WriteString(q)
										buf.WriteString(rewriteFn(a[1]))
										buf.WriteString(q)
										buf.WriteString(a[2])
										return buf.String()
									}
									return importCallExpr
								}))
							} else {
								buf.WriteString(expr)
							}
						}
					} else {
						if regImportCallExpr.MatchString(expr) {
							buf.WriteString(regImportCallExpr.ReplaceAllStringFunc(expr, func(importCallExpr string) string {
								q := "'"
								a := strings.Split(importCallExpr, q)
								if len(a) != 3 {
									q = `"`
									a = strings.Split(importCallExpr, q)
								}
								if len(a) == 3 {
									buf := bytes.NewBuffer(nil)
									buf.WriteString(a[0])
									buf.WriteString(q)
									buf.WriteString(rewriteFn(a[1]))
									buf.WriteString(q)
									buf.WriteString(a[2])
									return buf.String()
								}
								return importCallExpr
							}))
						} else {
							buf.WriteString(expr)
						}
					}
				}
				if i > 0 && importExportScope {
					importExportScope = false
				}
				i++
			}
		}
		buf.WriteString(spacesOnRight)
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	if err != nil {
		return
	}

	dtsData := buf.Bytes()
	dataLen := buf.Len()
	if len(dmodules) > 0 {
		for _, record := range dmodules {
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

	ensureDir(path.Dir(saveFilePath))
	saveFile, err := os.Create(saveFilePath)
	if err != nil {
		return
	}

	_, err = io.Copy(saveFile, buf)
	saveFile.Close()
	if err != nil {
		return
	}

	for _, dep := range deps.Values() {
		if isFileImportPath(dep) {
			if strings.HasPrefix(dep, "/") {
				pkg, subpath := utils.SplitByFirstByte(dep, '/')
				if strings.HasPrefix(pkg, "@") {
					n, _ := utils.SplitByFirstByte(subpath, '/')
					pkg = fmt.Sprintf("%s/%s", pkg, n)
				}
				err = copyDTS(nodeModulesDir, path.Join(pkg, dep))
			} else {
				err = copyDTS(nodeModulesDir, path.Join(path.Dir(dts), dep))
			}
		} else {
			err = copyDTS(nodeModulesDir, dep)
		}
		if err != nil {
			os.Remove(saveFilePath)
			return
		}
	}

	return
}

func getTypesPath(nodeModulesDir string, p NpmPackage, subpath string) string {
	var types string
	if subpath != "" {
		var subpkg NpmPackage
		var subtypes string
		subpkgJSONFile := path.Join(nodeModulesDir, p.Name, subpath, "package.json")
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
	return fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureSuffix(path.Join("/", types), ".d.ts"))
}

func onSemicolon(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == ';' {
			return i + 1, data[:i], nil
		}
	}
	if !atEOF {
		return 0, nil, nil
	}
	// There is one final token to be delivered, which may be the empty string.
	// Returning bufio.ErrFinalToken here tells Scan there are no more tokens after this
	// but does not trigger an error to be returned from Scan itself.
	return 0, data, bufio.ErrFinalToken
}
