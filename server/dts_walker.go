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

func walkDts(r io.Reader, buf *bytes.Buffer, resolve func(path string, declaredModule bool, position int) string) (err error) {
	var commentScope bool
	var importExportScope bool
	scanner := bufio.NewScanner(r)
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
			rest := strings.TrimPrefix(pure, "///")
			if regReferenceTag.MatchString(rest) {
				a := regReferenceTag.FindAllStringSubmatch(rest, 1)
				format := a[0][1]
				path := a[0][3]
				if format == "path" || format == "types" {
					if format == "path" && !isLocalImport(path) {
						path = "./" + path
					}
					res := resolve(path, false, buf.Len())
					if format == "types" && res != path {
						format = "path"
					}
					fmt.Fprintf(buf, `/// <reference %s="%s" />`, format, res)
				} else {
					buf.WriteString(pure)
				}
			} else {
				buf.WriteString(pure)
			}
		} else if strings.HasPrefix(pure, "//") {
			buf.WriteString(pure)
		} else if strings.HasPrefix(pure, "declare") && regDeclareModuleExpr.MatchString(pure) {
			q := "'"
			a := strings.Split(pure, q)
			if len(a) != 3 {
				q = `"`
				a = strings.Split(pure, q)
			}
			if len(a) == 3 {
				buf.WriteString(a[0])
				buf.WriteString(q)
				buf.WriteString(resolve(a[1], true, buf.Len()))
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
								buf.WriteString(resolve(a[1], false, buf.Len()))
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
									buf.WriteString(resolve(a[1], false, buf.Len()))
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
									buf.WriteString(resolve(a[1], false, buf.Len()))
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
