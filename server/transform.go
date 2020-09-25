package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/ije/gox/utils"
)

var reVersion = regexp.MustCompile(`([^/])@[\d\.]+/`)
var reFromExpression = regexp.MustCompile(`(\s|})from\s*("|')`)
var reAsExpression = regexp.MustCompile(`([0-9a-zA-Z\_\$])\s+as\s+([a-zA-Z\_\$])`)
var reReferenceTag = regexp.MustCompile(`^<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/>$`)

func toRequire(code []byte) (output []byte) {
	buf := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(bytes.NewReader(code))
	commentScope := false
	importScope := false
	importDestructionScope := false
	for scanner.Scan() {
		text := scanner.Text()
		pure := strings.TrimSpace(text)
		buf.WriteString(text[:strings.Index(text, pure)])
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
		} else if strings.HasPrefix(pure, "//") {
			buf.WriteString(pure)
		} else {
			var i int
			scanner := bufio.NewScanner(strings.NewReader(pure))
			scanner.Split(onSemicolon)
			for scanner.Scan() {
				if i > 0 {
					buf.WriteByte(';')
				}
				text := scanner.Text()
				exp := strings.TrimSpace(text)
				buf.WriteString(text[:strings.Index(text, exp)])
				if exp != "" {
					if importScope || startsWith(exp, "import ", "import{") {
						importScope = true
						if importDestructionScope || strings.ContainsRune(exp, '{') {
							if !importDestructionScope {
								a, b := utils.SplitByFirstByte(exp, '{')
								exp = a + "{" + reAsExpression.ReplaceAllString(b, "$1: $2")
							} else {
								exp = reAsExpression.ReplaceAllString(exp, "$1: $2")
							}
							importDestructionScope = true
							end := strings.ContainsRune(exp, '}')
							if end {
								a, b := utils.SplitByFirstByte(exp, '}')
								exp = reAsExpression.ReplaceAllString(a, "$1: $2") + "}" + b
								importDestructionScope = false
							}
						}
						if strings.HasPrefix(exp, "import") {
							exp = "const" + strings.TrimPrefix(exp, "import")
						}
						end := reFromExpression.MatchString(exp)
						if end {
							importScope = false
							sp := "'"
							a := strings.Split(exp, sp)
							if len(a) != 3 {
								sp = `"`
								a = strings.Split(exp, sp)
							}
							if len(a) == 3 {
								buf.WriteString(strings.TrimSuffix(strings.TrimSpace(a[0]), "from"))
								buf.WriteString("= require(")
								buf.WriteString(sp)
								buf.WriteString(a[1])
								buf.WriteString(sp)
								buf.WriteByte(')')
								buf.WriteString(a[2])
							} else {
								buf.WriteString(exp)
							}
						} else {
							buf.WriteString(exp)
						}
					} else {
						buf.WriteString(exp)
					}
				}
				if i > 0 && importScope {
					importScope = false
				}
				i++
			}
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func copyDTS(nodeModulesDir string, saveDir string, dts string) (err error) {
	saveFilePath := path.Join(saveDir, dts)
	dtsFilePath := path.Join(nodeModulesDir, reVersion.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("copyDTS", dts, err)
			err = nil
		} else if strings.HasSuffix(err.Error(), "is a directory") {
			log.Warn("copyDTS", dts, err)
			err = nil
		}
		return
	}
	defer dtsFile.Close()

	fi, err := os.Lstat(saveFilePath)
	if err == nil {
		if fi.IsDir() {
			os.RemoveAll(saveFilePath)
		} else {
			// do not repeat
			return
		}
	}

	deps := map[string]struct{}{}
	rewritePath := func(importPath string) string {
		if isValidatedESImportPath(importPath) && !strings.HasSuffix(importPath, ".d.ts") {
			if fileExists(path.Join(dtsDir, importPath, "index.d.ts")) {
				importPath = strings.TrimSuffix(importPath, "/") + "/index.d.ts"
			} else {
				packageJSONFile := path.Join(dtsDir, importPath, "package.json")
				if fileExists(packageJSONFile) {
					var p NpmPackage
					if utils.ParseJSONFile(packageJSONFile, &p) == nil {
						types := getTypesPath(p)
						if types != "" {
							_, typespath := utils.SplitByFirstByte(types, '/')
							importPath = strings.TrimSuffix(importPath, "/") + "/" + typespath
						}
					}
				}
			}
			importPath = ensureExt(importPath, ".d.ts")
		} else {
			maybePackage, subpath := utils.SplitByFirstByte(importPath, '/')
			if strings.HasPrefix(maybePackage, "@") {
				n, s := utils.SplitByFirstByte(subpath, '/')
				maybePackage = fmt.Sprintf("%s/%s", maybePackage, n)
				subpath = s
			}
			packageJSONFile := path.Join(nodeModulesDir, "@types", maybePackage, "package.json")
			if !fileExists(packageJSONFile) {
				packageJSONFile = path.Join(nodeModulesDir, maybePackage, "package.json")
			}
			if fileExists(packageJSONFile) {
				var p NpmPackage
				if utils.ParseJSONFile(packageJSONFile, &p) == nil {
					if subpath != "" {
						importPath = fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(utils.CleanPath(subpath), ".d.ts"))
					} else {
						importPath = getTypesPath(p)
					}
				}
			} else {
				if !isValidatedESImportPath(importPath) {
					importPath = "./" + importPath
				}
				importPath = ensureExt(importPath, ".d.ts")
			}
		}
		deps[importPath] = struct{}{}
		if !isValidatedESImportPath(importPath) {
			return "/" + importPath
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
		buf.WriteString(text[:strings.Index(text, pure)])
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
		} else if strings.HasPrefix(pure, "///") {
			s := strings.TrimSpace(strings.TrimPrefix(pure, "///"))
			if reReferenceTag.MatchString(s) {
				a := reReferenceTag.FindAllStringSubmatch(s, 1)
				format := a[0][1]
				path := a[0][3]
				if format == "path" {
					if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "/") {
						path = "./" + path
					}
				}
				buf.WriteString(fmt.Sprintf(`/// <reference %s="%s" />`, format, rewritePath(path)))
			} else {
				buf.WriteString(pure)
			}
		} else if strings.HasPrefix(pure, "//") {
			buf.WriteString(pure)
		} else {
			var i int
			scanner := bufio.NewScanner(strings.NewReader(pure))
			scanner.Split(onSemicolon)
			for scanner.Scan() {
				if i > 0 {
					buf.WriteByte(';')
				}
				text := scanner.Text()
				exp := strings.TrimSpace(text)
				buf.WriteString(text[:strings.Index(text, exp)])
				if exp != "" {
					if importExportScope || startsWith(exp, "import ", "export ", "import{", "export{") {
						importExportScope = true
						end := reFromExpression.MatchString(exp)
						if end {
							importExportScope = false
							sp := "'"
							a := strings.Split(exp, sp)
							if len(a) != 3 {
								sp = `"`
								a = strings.Split(exp, sp)
							}
							if len(a) == 3 {
								buf.WriteString(a[0])
								buf.WriteString(sp)
								buf.WriteString(rewritePath(a[1]))
								buf.WriteString(sp)
								buf.WriteString(a[2])
							} else {
								buf.WriteString(exp)
							}
						} else {
							buf.WriteString(exp)
						}
					} else {
						buf.WriteString(exp)
					}
				}
				if i > 0 && importExportScope {
					importExportScope = false
				}
				i++
			}
		}
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	if err != nil {
		return
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

	for dep := range deps {
		if isValidatedESImportPath(dep) {
			if strings.HasPrefix(dep, "/") {
				maybePackage, subpath := utils.SplitByFirstByte(dep, '/')
				if strings.HasPrefix(maybePackage, "@") {
					n, _ := utils.SplitByFirstByte(subpath, '/')
					maybePackage = fmt.Sprintf("%s/%s", maybePackage, n)
				}
				err = copyDTS(nodeModulesDir, saveDir, path.Join(maybePackage, dep))
			} else {
				err = copyDTS(nodeModulesDir, saveDir, path.Join(path.Dir(dts), dep))
			}
		} else {
			err = copyDTS(nodeModulesDir, saveDir, dep)
		}
		if err != nil {
			os.RemoveAll(saveFilePath)
			return
		}
	}

	return
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
