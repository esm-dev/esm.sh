package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
)

var (
	regexpImportExportExpr  = regexp.MustCompile(`^(import|export)(\s+type)?\s*('|"|[a-zA-Z0-9_\$]+(\s+from|,\s*\{)|\*|\{)`)
	regexpFromExpr          = regexp.MustCompile(`(\}|\*|\s)from\s*('|")`)
	regexpImportPathExpr    = regexp.MustCompile(`^import\s*('|")`)
	regexpImportCallExpr    = regexp.MustCompile(`(import|require)\(('|").+?('|")\)`)
	regexpDeclareModuleExpr = regexp.MustCompile(`declare\s+module\s*('|").+?('|")`)
	regexpReferenceTag      = regexp.MustCompile(`<reference\s+(path|types)\s*=\s*('|")(.+?)('|")\s*/?>`)
)

var (
	bytesSigleQoute   = []byte{'\''}
	bytesDoubleQoute  = []byte{'"'}
	bytesCommentStart = []byte{'/', '*'}
	bytesCommentEnd   = []byte{'*', '/'}
	bytesDoubleSlash  = []byte{'/', '/'}
	bytesStripleSlash = []byte{'/', '/', '/'}
)

func walkDts(r io.Reader, buf *bytes.Buffer, resolve func(specifier string, kind string, position int) string) (err error) {
	var commentScope bool
	var importExportScope bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		token, trimedSpaces := trimSpace(scanner.Bytes())
		buf.Write(trimedSpaces)
	CheckCommentScope:
		if !commentScope && bytes.HasPrefix(token, bytesCommentStart) {
			commentScope = true
		}
		if commentScope {
			endIndex := bytes.Index(token, bytesCommentEnd)
			if endIndex > -1 {
				commentScope = false
				buf.Write(token[:endIndex+2])
				if rest := token[endIndex+2:]; len(rest) > 0 {
					token, trimedSpaces = trimSpace(rest)
					buf.Write(trimedSpaces)
					goto CheckCommentScope
				}
			} else {
				buf.Write(token)
			}
		} else if bytes.HasPrefix(token, bytesStripleSlash) {
			rest := bytes.TrimPrefix(token, bytesStripleSlash)
			if regexpReferenceTag.Match(rest) {
				a := regexpReferenceTag.FindAllSubmatch(rest, 1)
				format := string(a[0][1])
				path := string(a[0][3])
				if format == "path" || format == "types" {
					if format == "path" && !isRelativeSpecifier(path) {
						path = "./" + path
					}
					kind := "referenceTypes"
					if format == "path" {
						kind = "referencePath"
					}
					res := resolve(path, kind, buf.Len())
					if format == "types" && isHttpSepcifier(res) {
						format = "path"
					}
					fmt.Fprintf(buf, `/// <reference %s="%s" />`, format, res)
				} else {
					buf.Write(token)
				}
			} else {
				buf.Write(token)
			}
		} else if bytes.HasPrefix(token, bytesDoubleSlash) {
			buf.Write(token)
		} else {
			var i int
			inlineScanner := bufio.NewScanner(bytes.NewReader(token))
			inlineScanner.Split(splitInlineToken)
			for inlineScanner.Scan() {
				if i > 0 {
					buf.WriteByte(';')
				}
				inlineToken, trimedSpaces := trimSpace(inlineScanner.Bytes())
				buf.Write(trimedSpaces)
				if len(inlineToken) > 0 {

					// TypeScript may start raising a diagnostic when ESM declaration files use `export =`
					// see https://github.com/microsoft/TypeScript/issues/51321
					// if bytes.HasPrefix(inlineToken, []byte("export=")) || bytes.HasPrefix(inlineToken, []byte("export =")) {
					// 	buf.WriteString("// ")
					// 	buf.Write(inlineToken)
					// 	i++
					// 	continue
					// }

					if !importExportScope && regexpImportExportExpr.Match(inlineToken) {
						importExportScope = true
					}
					if importExportScope {
						if regexpFromExpr.Match(inlineToken) || regexpImportPathExpr.Match(inlineToken) {
							q := bytesSigleQoute
							a := bytes.Split(inlineToken, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(inlineToken, q)
							}
							if len(a) == 3 {
								buf.Write(a[0])
								buf.Write(q)
								buf.WriteString(resolve(string(a[1]), "importExpr", buf.Len()))
								buf.Write(q)
								buf.Write(a[2])
							} else {
								buf.Write(inlineToken)
							}
							importExportScope = false
						} else if regexpImportCallExpr.Match(inlineToken) {
							buf.Write(regexpImportCallExpr.ReplaceAllFunc(inlineToken, func(importCallExpr []byte) []byte {
								q := bytesSigleQoute
								a := bytes.Split(importCallExpr, q)
								if len(a) != 3 {
									q = bytesDoubleQoute
									a = bytes.Split(importCallExpr, q)
								}
								if len(a) == 3 {
									buf := bytes.NewBuffer(nil)
									buf.Write(a[0])
									buf.Write(q)
									buf.WriteString(resolve(string(a[1]), "importCall", buf.Len()))
									buf.Write(q)
									buf.Write(a[2])
									return buf.Bytes()
								}
								return importCallExpr
							}))
						} else {
							buf.Write(inlineToken)
						}
					} else if bytes.HasPrefix(inlineToken, []byte("declare")) && regexpDeclareModuleExpr.Match(token) {
						q := bytesSigleQoute
						a := bytes.Split(inlineToken, q)
						if len(a) != 3 {
							q = bytesDoubleQoute
							a = bytes.Split(inlineToken, q)
						}
						if len(a) == 3 {
							buf.Write(a[0])
							buf.Write(q)
							buf.WriteString(resolve(string(a[1]), "declareModule", buf.Len()))
							buf.Write(q)
							buf.Write(a[2])
						} else {
							buf.Write(inlineToken)
						}
					} else if regexpImportCallExpr.Match(inlineToken) {
						buf.Write(regexpImportCallExpr.ReplaceAllFunc(inlineToken, func(importCallExpr []byte) []byte {
							q := bytesSigleQoute
							a := bytes.Split(importCallExpr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(importCallExpr, q)
							}
							if len(a) == 3 {
								buf := bytes.NewBuffer(nil)
								buf.Write(a[0])
								buf.Write(q)
								buf.WriteString(resolve(string(a[1]), "importCall", buf.Len()))
								buf.Write(q)
								buf.Write(a[2])
								return buf.Bytes()
							}
							return importCallExpr
						}))
					} else {
						buf.Write(inlineToken)
					}
				}
				if i > 0 && importExportScope {
					importExportScope = false
				}
				i++
			}
			err = inlineScanner.Err()
			if err != nil {
				return
			}
		}
		buf.WriteByte('\n')
	}
	err = scanner.Err()
	return
}

func splitInlineToken(data []byte, atEOF bool) (advance int, token []byte, err error) {
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
