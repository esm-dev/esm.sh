package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

var (
	regVersionPath     = regexp.MustCompile(`([^/])@\d+\.\d+\.\d+([a-z0-9\.-]+)?/`)
	regFromExpr        = regexp.MustCompile(`(}|\s)from\s*("|')`)
	regImportPlainExpr = regexp.MustCompile(`import\s*("|')`)
	regImportCallExpr  = regexp.MustCompile(`import\((('[^']+')|("[^"]+"))\)`)
	regReferenceTag    = regexp.MustCompile(`^<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/>$`)
	regDeclareModule   = regexp.MustCompile(`declare\s+module\s*('|")([^'"]+)("|')`)
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

	dtsFilePath := path.Join(wd, "node_modules", regVersionPath.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsContent, err := ioutil.ReadFile(dtsFilePath)
	if err != nil {
		return
	}

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
	mainDeclareModules := []string{}
	rewriteFn := func(importPath string) string {
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
			if _, ok := builtInNodeModules[importPath]; ok {
				importPath = "@types/node/" + importPath
			}
			if _, ok := builtInNodeModules["node:"+importPath]; ok {
				importPath = "@types/node/" + strings.TrimPrefix(importPath, "node:")
			}
			info, subpath, formPackageJSON, err := node.getPackageInfo(wd, importPath, "latest")
			if err != nil {
				return importPath
			}
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
		return importPath
	}

	for _, a := range regDeclareModule.FindAllSubmatch(dtsContent, -1) {
		allDeclareModules.Add(string(a[2]))
	}

	buf := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(bytes.NewReader(dtsContent))
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
			if startsWith(pure, "import ", "import\"", "import'", "import{", "export ", "export{") {
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
					if !isLocalImport(path) {
						path = "./" + path
					}
				}
				if format == "types" {
					if path == "node" {
						path = fmt.Sprintf("/v%d/node.ns.d.ts", VERSION)
					} else {
						path = rewriteFn(path)
					}
					origin := ""
					if config.cdnDomain != "" {
						origin = fmt.Sprintf("https://%s", config.cdnDomain)
					}
					fmt.Fprintf(buf, `/// <reference path="%s%s" />`, origin, path)
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
			// resove `declare module "xxx" {}`, and the "xxx" must equal to the `pkgName`
			if len(a) == 3 && a[1] == pkgName {
				buf.WriteString(a[0])
				buf.WriteString(q)
				newname := fmt.Sprintf("/%s", a[1])
				if config.cdnDomain != "" {
					newname = fmt.Sprintf("https://%s/%s", config.cdnDomain, a[1])
				}
				buf.WriteString(newname)
				buf.WriteString(q)
				mainDeclareModules = append(mainDeclareModules, fmt.Sprintf("%s:%d", newname, buf.Len()))
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
					if importExportScope || startsWith(expr, "import ", "import\"", "import'", "import{", "export ", "export{") {
						importExportScope = true
						if regFromExpr.MatchString(expr) || regImportPlainExpr.MatchString(expr) {
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
						} else if regImportCallExpr.MatchString(expr) {
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
	if len(mainDeclareModules) > 0 {
		for _, record := range mainDeclareModules {
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
