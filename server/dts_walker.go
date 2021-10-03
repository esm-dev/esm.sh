package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	regFromExpr          = regexp.MustCompile(`(}|\s)from\s*("|')`)
	regImportPlainExpr   = regexp.MustCompile(`import\s*("|')`)
	regImportCallExpr    = regexp.MustCompile(`import\((('[^']+')|("[^"]+"))\)`)
	regDeclareModuleExpr = regexp.MustCompile(`declare\s+module\s*('|")([^'"]+)("|')`)
	regReferenceTag      = regexp.MustCompile(`<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/?>`)
)

func walkDts(r io.Reader, buf *bytes.Buffer, resolve func(path string, kind string, position int) string) (err error) {
	var commentScope bool
	var importExportScope bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		token := strings.TrimSpace(line)
		spacesOnLeftSize := strings.Index(line, token)
		buf.WriteString(line[:spacesOnLeftSize])
	Re:
		if commentScope || strings.HasPrefix(token, "/*") {
			commentScope = true
			endIndex := strings.Index(token, "*/")
			if endIndex > -1 {
				commentScope = false
				buf.WriteString(token[:endIndex+2])
				if rest := token[endIndex+2:]; rest != "" {
					token = strings.TrimSpace(rest)
					buf.WriteString(rest[:strings.Index(rest, token)])
					goto Re
				}
			} else {
				buf.WriteString(token)
			}
		} else if strings.HasPrefix(token, "///") {
			rest := strings.TrimPrefix(token, "///")
			if regReferenceTag.MatchString(rest) {
				a := regReferenceTag.FindAllStringSubmatch(rest, 1)
				format := a[0][1]
				path := a[0][3]
				if format == "path" || format == "types" {
					if format == "path" && !isLocalImport(path) {
						path = "./" + path
					}
					res := resolve(path, "reference "+format, buf.Len())
					if format == "types" && res != path {
						format = "path"
					}
					fmt.Fprintf(buf, `/// <reference %s="%s" />`, format, res)
				} else {
					buf.WriteString(token)
				}
			} else {
				buf.WriteString(token)
			}
		} else if strings.HasPrefix(token, "//") {
			buf.WriteString(token)
		} else if strings.HasPrefix(token, "declare") && regDeclareModuleExpr.MatchString(token) {
			q := "'"
			a := strings.Split(token, q)
			if len(a) != 3 {
				q = `"`
				a = strings.Split(token, q)
			}
			if len(a) == 3 {
				buf.WriteString(a[0])
				buf.WriteString(q)
				buf.WriteString(resolve(a[1], "declare module", buf.Len()))
				buf.WriteString(q)
				buf.WriteString(a[2])
			} else {
				buf.WriteString(token)
			}
		} else if i := strings.Index(token, "/*"); i > 0 {
			if startsWith(token, "import ", "import\"", "import'", "import{", "export ", "export{") {
				importExportScope = true
			}
			buf.WriteString(token[:i])
			commentScope = true
			token = token[i:]
			goto Re
		} else {
			tokens := strings.Split(token, ";")
			for i, text := range tokens {
				if i > 0 {
					buf.WriteByte(';')
				}
				inlineToken := strings.TrimSpace(text)
				buf.WriteString(text[:strings.Index(text, inlineToken)])
				if inlineToken != "" {
					if importExportScope || startsWith(inlineToken, "import ", "import\"", "import'", "import{", "export ", "export{") {
						importExportScope = true
						if regFromExpr.MatchString(inlineToken) || regImportPlainExpr.MatchString(inlineToken) {
							importExportScope = false
							q := "'"
							a := strings.Split(inlineToken, q)
							if len(a) != 3 {
								q = `"`
								a = strings.Split(inlineToken, q)
							}
							if len(a) == 3 {
								buf.WriteString(a[0])
								buf.WriteString(q)
								buf.WriteString(resolve(a[1], "import", buf.Len()))
								buf.WriteString(q)
								buf.WriteString(a[2])
							} else {
								buf.WriteString(inlineToken)
							}
						} else if regImportCallExpr.MatchString(inlineToken) {
							buf.WriteString(regImportCallExpr.ReplaceAllStringFunc(inlineToken, func(importCallExpr string) string {
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
									buf.WriteString(resolve(a[1], "import", buf.Len()))
									buf.WriteString(q)
									buf.WriteString(a[2])
									return buf.String()
								}
								return importCallExpr
							}))
						} else {
							buf.WriteString(inlineToken)
						}
					} else {
						if regImportCallExpr.MatchString(inlineToken) {
							buf.WriteString(regImportCallExpr.ReplaceAllStringFunc(inlineToken, func(importCallExpr string) string {
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
									buf.WriteString(resolve(a[1], "import", buf.Len()))
									buf.WriteString(q)
									buf.WriteString(a[2])
									return buf.String()
								}
								return importCallExpr
							}))
						} else {
							buf.WriteString(inlineToken)
						}
					}
				}
				if i > 0 && importExportScope {
					importExportScope = false
				}
			}
		}
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	return
}
