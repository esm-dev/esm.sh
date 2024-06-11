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
	regexpImportFromExpr    = regexp.MustCompile(`^import(\s+type)?\s*('|"|[\w\$]+|\*|\{)`)
	regexpExportFromExpr    = regexp.MustCompile(`^export(\s+type)?\s*(\*|\{)`)
	regexpFromExpr          = regexp.MustCompile(`(\}|\*|\s)from\s*['"]`)
	regexpImportPathExpr    = regexp.MustCompile(`^import\s*['"]`)
	regexpImportCallExpr    = regexp.MustCompile(`(import|require)\(['"][^'"]+['"]\)`)
	regexpDeclareModuleExpr = regexp.MustCompile(`^declare\s+module\s*['"].+?['"]`)
	regexpReferenceTag      = regexp.MustCompile(`^\s*<reference\s+(path|types)\s*=\s*['"](.+?)['"].+>`)
)

var (
	bytesSingleQoute  = []byte{'\''}
	bytesDoubleQoute  = []byte{'"'}
	bytesCommentStart = []byte{'/', '*'}
	bytesCommentEnd   = []byte{'*', '/'}
	bytesDoubleSlash  = []byte{'/', '/'}
	bytesStripleSlash = []byte{'/', '/', '/'}
)

func walkDts(r io.Reader, buf *bytes.Buffer, resolve func(specifier string, kind string, position int) (resovledPath string, err error)) (err error) {
	var multiLineComment bool
	var importExportExpr bool
	// var declareModuleScope bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line, trimedSpaces := trimSpace(scanner.Bytes())
		buf.Write(trimedSpaces)
	CheckCommentScope:
		if !multiLineComment && bytes.HasPrefix(line, bytesCommentStart) {
			multiLineComment = true
		}
		if multiLineComment {
			endIndex := bytes.Index(line, bytesCommentEnd)
			if endIndex > -1 {
				multiLineComment = false
				buf.Write(line[:endIndex+2])
				if rest := line[endIndex+2:]; len(rest) > 0 {
					line, trimedSpaces = trimSpace(rest)
					buf.Write(trimedSpaces)
					goto CheckCommentScope
				}
			} else {
				buf.Write(line)
			}
		} else if bytes.HasPrefix(line, bytesStripleSlash) {
			rest := bytes.TrimPrefix(line, bytesStripleSlash)
			if regexpReferenceTag.Match(rest) {
				a := regexpReferenceTag.FindAllSubmatch(rest, 1)
				format := string(a[0][1])
				path := string(a[0][2])
				if format == "path" || format == "types" {
					if format == "path" {
						if !isRelativeSpecifier(path) {
							path = "./" + path
						}
					}
					kind := "referenceTypes"
					if format == "path" {
						kind = "referencePath"
					}
					var res string
					res, err = resolve(path, kind, buf.Len())
					if err != nil {
						return
					}
					if format == "types" && strings.HasPrefix(res, "{ESM_CDN_ORIGIN}") {
						format = "path"
					}
					fmt.Fprintf(buf, `/// <reference %s="%s" />`, format, res)
				} else {
					buf.Write(line)
				}
			} else {
				buf.Write(line)
			}
		} else if bytes.HasPrefix(line, bytesDoubleSlash) {
			buf.Write(line)
		} else {
			var i int
			exprScanner := bufio.NewScanner(bytes.NewReader(line))
			exprScanner.Split(splitExpr)
			for exprScanner.Scan() {
				if i > 0 {
					buf.WriteByte(';')
				}
				expr, trimedLeftSpaces := trimSpace(exprScanner.Bytes())
				buf.Write(trimedLeftSpaces)
				if len(expr) > 0 {
					// TypeScript may start raising a diagnostic when ESM declaration files use `export =`
					// see https://github.com/microsoft/TypeScript/issues/51321
					// if bytes.HasPrefix(inlineToken, []byte("export=")) || bytes.HasPrefix(inlineToken, []byte("export =")) {
					// 	buf.WriteString("// ")
					// 	buf.Write(inlineToken)
					// 	i++
					// 	continue
					// }

					// resoving `import('lib')`
					if regexpImportCallExpr.Match(expr) {
						expr = regexpImportCallExpr.ReplaceAllFunc(expr, func(importCallExpr []byte) []byte {
							q := bytesSingleQoute
							a := bytes.Split(importCallExpr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(importCallExpr, q)
							}
							if len(a) == 3 {
								tmp := bytes.NewBuffer(nil)
								tmp.Write(a[0])
								tmp.Write(q)
								var res string
								res, err = resolve(string(a[1]), "importCall", buf.Len())
								if err != nil {
									return importCallExpr
								}
								tmp.WriteString(res)
								tmp.Write(q)
								tmp.Write(a[2])
								return tmp.Bytes()
							}
							return importCallExpr
						})
					}

					if !importExportExpr && (bytes.HasPrefix(expr, []byte("import")) && regexpImportFromExpr.Match(expr)) || (bytes.HasPrefix(expr, []byte("export")) && regexpExportFromExpr.Match(expr)) {
						importExportExpr = true
					}
					if bytes.HasPrefix(expr, []byte("declare")) && regexpDeclareModuleExpr.Match(expr) {
						q := bytesSingleQoute
						a := bytes.Split(expr, q)
						if len(a) != 3 {
							q = bytesDoubleQoute
							a = bytes.Split(expr, q)
						}
						if len(a) == 3 {
							buf.Write(a[0])
							buf.Write(q)
							var res string
							res, err = resolve(string(a[1]), "declareModule", buf.Len())
							if err != nil {
								return
							}
							buf.WriteString(res)
							buf.Write(q)
							buf.Write(a[2])
						} else {
							buf.Write(expr)
						}
					} else if importExportExpr {
						if regexpFromExpr.Match(expr) || (bytes.HasPrefix(expr, []byte("import")) && regexpImportPathExpr.Match(expr)) {
							q := bytesSingleQoute
							a := bytes.Split(expr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(expr, q)
							}
							if len(a) == 3 {
								buf.Write(a[0])
								buf.Write(q)
								var res string
								res, err = resolve(string(a[1]), "importExpr", buf.Len())
								if err != nil {
									return
								}
								buf.WriteString(res)
								buf.Write(q)
								buf.Write(a[2])
							} else {
								buf.Write(expr)
							}
							importExportExpr = false
						} else {
							buf.Write(expr)
						}
					} else {
						buf.Write(expr)
					}
				}
				i++
			}
			err = exprScanner.Err()
			if err != nil {
				return
			}
		}
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	return
}

// A split function for a Scanner
func splitExpr(data []byte, atEOF bool) (advance int, token []byte, err error) {
	var commentScope bool
	var stringScope byte
	for i := 0; i < len(data); i++ {
		var prev, next byte
		if i > 0 {
			prev = data[i-1]
		}
		if i+1 < len(data) {
			next = data[i+1]
		}
		c := data[i]
		switch c {
		case '/':
			if stringScope == 0 {
				if commentScope {
					if prev == '*' {
						commentScope = false
					}
				} else if next == '*' {
					commentScope = true
				}
			}
		case '\'', '"', '`':
			if !commentScope {
				if stringScope == 0 {
					stringScope = c
				} else if stringScope == c && prev != '\\' {
					stringScope = 0
				}
			}
		case ';':
			if stringScope == 0 && !commentScope {
				return i + 1, data[:i], nil
			}
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

func trimSpace(line []byte) ([]byte, []byte) {
	s := 0
	l := len(line)
	for i := 0; i < l; i++ {
		c := line[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s++
	}
	e := l
	for i := l - 1; i >= s; i-- {
		c := line[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		e--
	}
	return line[s:e], line[:s]
}
