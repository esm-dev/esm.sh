package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/esbuild-internal/test"
	"github.com/ije/gox/utils"
)

var (
	regVersionPath    = regexp.MustCompile(`([^/])@v?[\d\.]+/`)
	regFromExpression = regexp.MustCompile(`(\s|})from\s*("|')`)
	regReferenceTag   = regexp.MustCompile(`^<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/>$`)
	regDeclareModule  = regexp.MustCompile(`^declare\s+module\s*('|")([^'"]+)("|')`)
)

func parseModuleExports(filepath string) (exports []string, ok bool, err error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return
	}
	log := logger.NewDeferLog()
	ast, pass := js_parser.Parse(log, test.SourceForTest(string(data)), config.Options{})
	if pass {
		ok = ast.HasES6Exports
		if ok {
			for name := range ast.NamedExports {
				exports = append(exports, name)
			}
		}
	}
	return
}

func copyDTS(hostname string, nodeModulesDir string, saveDir string, dts string) (err error) {
	saveFilePath := path.Join(saveDir, dts)
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

	fi, err := os.Lstat(saveFilePath)
	if err == nil {
		if fi.IsDir() {
			os.Remove(saveFilePath)
		} else {
			// do not repeat
			return
		}
	}

	deps := map[string]struct{}{}
	dmodules := map[string]struct{}{}
	rewriteFn := func(importPath string) string {
		if isValidatedESImportPath(importPath) {
			if !strings.HasSuffix(importPath, ".d.ts") {
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
				importPath = ensureExt(strings.TrimSuffix(importPath, ".js"), ".d.ts")
			}
		} else {
			// ignore builtin node modules
			if _, ok := builtInNodeModules[importPath]; ok {
				polyfill, ok := polyfilledBuiltInNodeModules[importPath]
				if ok {
					p, err := nodeEnv.getPackageInfo(polyfill, "latest")
					if err == nil {
						return getTypesPath(p)
					}
					return polyfill
				}
				return importPath
			}
			pkgName, subpath := utils.SplitByFirstByte(importPath, '/')
			if strings.HasPrefix(pkgName, "@") {
				n, s := utils.SplitByFirstByte(subpath, '/')
				pkgName = fmt.Sprintf("%s/%s", pkgName, n)
				subpath = s
			}
			// self
			if strings.HasPrefix(dts, pkgName) {
				return importPath
			}
			packageJSONFile := path.Join(nodeModulesDir, "@types", pkgName, "package.json")
			if !fileExists(packageJSONFile) {
				packageJSONFile = path.Join(nodeModulesDir, pkgName, "package.json")
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
				p, err := nodeEnv.getPackageInfo(importPath, "latest")
				if err != nil && err.Error() == fmt.Sprintf("npm: package '%s' not found", importPath) {
					p, err = nodeEnv.getPackageInfo("@types/"+importPath, "latest")
				}
				if err == nil {
					if subpath != "" {
						importPath = fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(utils.CleanPath(subpath), ".d.ts"))
					} else {
						importPath = getTypesPath(p)
					}
				} else {
					if !isValidatedESImportPath(importPath) {
						importPath = "./" + importPath
					}
					importPath = ensureExt(importPath, ".d.ts")
				}
			}
		}
		deps[importPath] = struct{}{}
		if !isValidatedESImportPath(importPath) {
			if hostname == "localhost" {
				return fmt.Sprintf("http://localhost/%s", importPath)
			}
			return fmt.Sprintf("https://%s/%s", hostname, importPath)
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
			if startsWith(pure, "import ", "export ", "import{", "export{") {
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
					if !isValidatedESImportPath(path) {
						path = "./" + path
					}
				}
				if format == "types" && path == "node" {
					buf.WriteString(nodeTypes)
				} else {
					fmt.Fprintf(buf, `/// <reference %s="%s" />`, format, rewriteFn(path))
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
				if hostname == "localhost" {
					buf.WriteString("http://localhost/")
					dmodules[fmt.Sprintf("http://localhost/%s", a[1])] = struct{}{}
				} else {
					buf.WriteString("https://")
					buf.WriteString(hostname)
					buf.WriteString("/")
					dmodules[fmt.Sprintf("https://%s/%s", hostname, a[1])] = struct{}{}
				}
				buf.WriteString(a[1])
				buf.WriteString(q)
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
				exp := strings.TrimSpace(text)
				buf.WriteString(text[:strings.Index(text, exp)])
				if exp != "" {
					if importExportScope || startsWith(exp, "import ", "export ", "import{", "export{") {
						importExportScope = true
						end := regFromExpression.MatchString(exp)
						if end {
							importExportScope = false
							q := "'"
							a := strings.Split(exp, q)
							if len(a) != 3 {
								q = `"`
								a = strings.Split(exp, q)
							}
							if len(a) == 3 {
								buf.WriteString(a[0])
								buf.WriteString(q)
								buf.WriteString(rewriteFn(a[1]))
								buf.WriteString(q)
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
		buf.WriteString(spacesOnRight)
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	if err != nil {
		return
	}

	if len(dmodules) > 0 {
		buf.WriteByte('\n')
		for dm := range dmodules {
			fmt.Fprintf(buf, `declare module "%s@*" {%s`, dm, EOL)
			fmt.Fprintf(buf, `    export * from "%s";%s`, dm, EOL)
			fmt.Fprintf(buf, `    export { default } from "%s";%s`, dm, EOL)
			fmt.Fprintf(buf, `}%s`, EOL)
		}
		buf.WriteByte('\n')
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
				pkg, subpath := utils.SplitByFirstByte(dep, '/')
				if strings.HasPrefix(pkg, "@") {
					n, _ := utils.SplitByFirstByte(subpath, '/')
					pkg = fmt.Sprintf("%s/%s", pkg, n)
				}
				err = copyDTS(hostname, nodeModulesDir, saveDir, path.Join(pkg, dep))
			} else {
				err = copyDTS(hostname, nodeModulesDir, saveDir, path.Join(path.Dir(dts), dep))
			}
		} else {
			err = copyDTS(hostname, nodeModulesDir, saveDir, dep)
		}
		if err != nil {
			os.Remove(saveFilePath)
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
