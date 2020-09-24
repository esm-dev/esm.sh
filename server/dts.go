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
var reImortExportFrom = regexp.MustCompile(`(\s|})from\s*("|')`)
var reReferenceTag = regexp.MustCompile(`^<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/>$`)

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
			fi, err := os.Lstat(path.Join(dtsDir, importPath, "index.d.ts"))
			if err == nil && !fi.IsDir() {
				importPath = importPath + "/index.d.ts"
			} else {
				importPath += ".d.ts"
			}
		} else {
			maybePackage, subpath := utils.SplitByFirstByte(importPath, '/')
			if strings.HasPrefix(maybePackage, "@") {
				n, s := utils.SplitByFirstByte(subpath, '/')
				maybePackage = fmt.Sprintf("%s/%s", maybePackage, n)
				subpath = s
			}
			packageJSONFile := path.Join(nodeModulesDir, "@types", maybePackage, "package.json")
			fi, err := os.Lstat(packageJSONFile)
			if err != nil && os.IsNotExist(err) {
				packageJSONFile = path.Join(nodeModulesDir, maybePackage, "package.json")
				fi, err = os.Lstat(packageJSONFile)
			}
			if err == nil && !fi.IsDir() {
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
	commentContext := false
	importExportContext := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
	Re:
		if commentContext || strings.HasPrefix(line, "/*") {
			commentContext = true
			endIndex := strings.Index(line, "*/")
			if endIndex > -1 {
				commentContext = false
				buf.WriteString(line[:endIndex])
				buf.WriteString("*/")
				if rest := line[endIndex+2:]; rest != "" {
					line = strings.TrimSpace(rest)
					buf.WriteString(rest[:strings.Index(rest, line)])
					goto Re
				}
			} else {
				buf.WriteString(line)
			}
		} else if strings.HasPrefix(line, "///") {
			s := strings.TrimSpace(strings.TrimPrefix(line, "///"))
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
				buf.WriteString(line)
			}
		} else {
			exps := strings.Split(line, ";")
			for i, exp := range exps {
				if exp != "" {
					if importExportContext || strings.HasPrefix(exp, "import") || strings.HasPrefix(exp, "export") {
						importExportContext = true
						end := reImortExportFrom.MatchString(exp)
						if end {
							importExportContext = false
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
				if i < len(exps)-1 {
					buf.WriteByte(';')
				}
				if i > 0 && importExportContext {
					importExportContext = false
				}
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

func isValidatedESImportPath(importPath string) bool {
	return strings.HasPrefix(importPath, "/") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../")
}
