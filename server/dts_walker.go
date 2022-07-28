package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
)

var (
	regFromExpr          = regexp.MustCompile(`(}|\s)from\s*("|')`)
	regImportPlainExpr   = regexp.MustCompile(`import\s*("|')`)
	regImportCallExpr    = regexp.MustCompile(`import\((('[^']+')|("[^"]+"))\)`)
	regDeclareModuleExpr = regexp.MustCompile(`declare\s+module\s*('|")([^'"]+)("|')`)
	regReferenceTag      = regexp.MustCompile(`<reference\s+(path|types)\s*=\s*('|")([^'"]+)("|')\s*/?>`)
)

var (
	bytesSigleQoute   = []byte{'\''}
	bytesDoubleQoute  = []byte{'"'}
	bytesCommentStart = []byte{'/', '*'}
	bytesCommentEnd   = []byte{'*', '/'}
	bytesDoubleSlash  = []byte{'/', '/'}
	bytesStripleSlash = []byte{'/', '/', '/'}
)

func walkDts(r io.Reader, buf *bytes.Buffer, resolve func(path string, kind string, position int) string) (err error) {
	var commentScope bool
	var importExportScope bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		token, leftSpaces := trimSpace(scanner.Bytes())
		buf.Write(leftSpaces)
	Re:
		if !commentScope && bytes.HasPrefix(token, bytesCommentStart) {
			commentScope = true
		}
		if commentScope {
			endIndex := bytes.Index(token, bytesCommentEnd)
			if endIndex > -1 {
				commentScope = false
				buf.Write(token[:endIndex+2])
				if rest := token[endIndex+2:]; len(rest) > 0 {
					token, leftSpaces = trimSpace(rest)
					buf.Write(leftSpaces)
					goto Re
				}
			} else {
				buf.Write(token)
			}
		} else if bytes.HasPrefix(token, bytesStripleSlash) {
			rest := bytes.TrimPrefix(token, bytesStripleSlash)
			if regReferenceTag.Match(rest) {
				a := regReferenceTag.FindAllSubmatch(rest, 1)
				format := string(a[0][1])
				path := string(a[0][3])
				if format == "path" || format == "types" {
					if format == "path" && !isLocalImport(path) {
						path = "./" + path
					}
					res := resolve(path, "reference "+format, buf.Len())
					if format == "types" && isRemoteImport(res) {
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
				inlineToken, leftSpaces := trimSpace(inlineScanner.Bytes())
				buf.Write(leftSpaces)
				if len(inlineToken) > 0 {
					if !importExportScope && startsWith(string(inlineToken), "import ", "import\"", "import'", "import{", "export ", "export{") {
						importExportScope = true
					}
					if importExportScope {
						if regFromExpr.Match(inlineToken) || regImportPlainExpr.Match(inlineToken) {
							importExportScope = false
							q := bytesSigleQoute
							a := bytes.Split(inlineToken, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(inlineToken, q)
							}
							if len(a) == 3 {
								buf.Write(a[0])
								buf.Write(q)
								buf.WriteString(resolve(string(a[1]), "import", buf.Len()))
								buf.Write(q)
								buf.Write(a[2])
							} else {
								buf.Write(inlineToken)
							}
						} else if regImportCallExpr.Match(inlineToken) {
							buf.Write(regImportCallExpr.ReplaceAllFunc(inlineToken, func(importCallExpr []byte) []byte {
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
									buf.WriteString(resolve(string(a[1]), "import", buf.Len()))
									buf.Write(q)
									buf.Write(a[2])
									return buf.Bytes()
								}
								return importCallExpr
							}))
						} else {
							buf.Write(inlineToken)
						}
					} else if bytes.HasPrefix(inlineToken, []byte("declare")) && regDeclareModuleExpr.Match(token) {
						q := bytesSigleQoute
						a := bytes.Split(inlineToken, q)
						if len(a) != 3 {
							q = bytesDoubleQoute
							a = bytes.Split(inlineToken, q)
						}
						if len(a) == 3 {
							buf.Write(a[0])
							buf.Write(q)
							buf.WriteString(resolve(string(a[1]), "declare module", buf.Len()))
							buf.Write(q)
							buf.Write(a[2])
						} else {
							buf.Write(inlineToken)
						}
					} else if regImportCallExpr.Match(inlineToken) {
						buf.Write(regImportCallExpr.ReplaceAllFunc(inlineToken, func(importCallExpr []byte) []byte {
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
								buf.WriteString(resolve(string(a[1]), "import", buf.Len()))
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
