package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
)

var (
	regexpImportDecl        = regexp.MustCompile(`^import(\s+type)?\s*('|"|[\w\$]+|\*|\{)`)
	regexpExportDecl        = regexp.MustCompile(`^export(\s+type)?\s*(\*|\{)`)
	regexpFromExpr          = regexp.MustCompile(`(\}|\*|\s)from\s*['"]`)
	regexpImportPathDecl    = regexp.MustCompile(`^import\s*['"]`)
	regexpImportCallExpr    = regexp.MustCompile(`(import|require)\(['"][^'"]+['"]\)`)
	regexpDeclareModuleStmt = regexp.MustCompile(`^declare\s+module\s*['"].+?['"]`)
	regexpTSReferenceTag    = regexp.MustCompile(`^\s*<reference\s+(path|types)\s*=\s*['"](.+?)['"].+>`)
)

var (
	bytesSingleQoute  = []byte{'\''}
	bytesDoubleQoute  = []byte{'"'}
	bytesCommentStart = []byte{'/', '*'}
	bytesCommentEnd   = []byte{'*', '/'}
	bytesDoubleSlash  = []byte{'/', '/'}
	bytesStripleSlash = []byte{'/', '/', '/'}
)

type TsImportKind uint8

const (
	TsReferenceTypes TsImportKind = iota
	TsReferencePath
	TsImportDecl
	TsImportCall
	TsDeclareModule
)

// a simple dts lexer for resolving import path
func parseDts(r io.Reader, w *bytes.Buffer, resolve func(specifier string, kind TsImportKind, position int) (resovledPath string, err error)) (err error) {
	var multiLineComment bool
	var importOrExportDeclFound bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line, trimedSpaces := trimSpace(scanner.Bytes())
		w.Write(trimedSpaces)
	CheckCommentScope:
		if !multiLineComment && bytes.HasPrefix(line, bytesCommentStart) {
			multiLineComment = true
		}
		if multiLineComment {
			endIndex := bytes.Index(line, bytesCommentEnd)
			if endIndex > -1 {
				multiLineComment = false
				w.Write(line[:endIndex+2])
				if rest := line[endIndex+2:]; len(rest) > 0 {
					line, trimedSpaces = trimSpace(rest)
					w.Write(trimedSpaces)
					goto CheckCommentScope
				}
			} else {
				w.Write(line)
			}
		} else if bytes.HasPrefix(line, bytesStripleSlash) {
			rest := bytes.TrimPrefix(line, bytesStripleSlash)
			if regexpTSReferenceTag.Match(rest) {
				a := regexpTSReferenceTag.FindAllSubmatch(rest, 1)
				format := string(a[0][1])
				path := string(a[0][2])
				if format == "path" || format == "types" {
					if format == "path" {
						if !isRelPathSpecifier(path) {
							path = "./" + path
						}
					}
					kind := TsReferenceTypes
					if format == "path" {
						kind = TsReferencePath
					}
					var res string
					res, err = resolve(path, kind, w.Len())
					if err != nil {
						return
					}
					fmt.Fprintf(w, `/// <reference %s="%s" />`, format, res)
				} else {
					w.Write(line)
				}
			} else {
				w.Write(line)
			}
		} else if bytes.HasPrefix(line, bytesDoubleSlash) {
			w.Write(line)
		} else {
			var i int
			exprScanner := bufio.NewScanner(bytes.NewReader(line))
			exprScanner.Split(splitExpr)
			for exprScanner.Scan() {
				if i > 0 {
					w.WriteByte(';')
				}
				expr, trimedLeftSpaces := trimSpace(exprScanner.Bytes())
				w.Write(trimedLeftSpaces)
				if len(expr) > 0 {
					if regexpImportCallExpr.Match(expr) {
						expr = regexpImportCallExpr.ReplaceAllFunc(expr, func(importCallExpr []byte) []byte {
							q := bytesSingleQoute
							a := bytes.Split(importCallExpr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(importCallExpr, q)
							}
							if len(a) == 3 {
								tmp, recycle := NewBuffer()
								defer recycle()
								tmp.Write(a[0])
								tmp.Write(q)
								var res string
								res, err = resolve(string(a[1]), TsImportCall, w.Len())
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

					if !importOrExportDeclFound && (bytes.HasPrefix(expr, []byte("import")) && regexpImportDecl.Match(expr)) || (bytes.HasPrefix(expr, []byte("export")) && regexpExportDecl.Match(expr)) {
						importOrExportDeclFound = true
					}
					if bytes.HasPrefix(expr, []byte("declare")) && regexpDeclareModuleStmt.Match(expr) {
						q := bytesSingleQoute
						a := bytes.Split(expr, q)
						if len(a) != 3 {
							q = bytesDoubleQoute
							a = bytes.Split(expr, q)
						}
						if len(a) == 3 {
							w.Write(a[0])
							w.Write(q)
							var res string
							res, err = resolve(string(a[1]), TsDeclareModule, w.Len())
							if err != nil {
								return
							}
							w.WriteString(res)
							w.Write(q)
							w.Write(a[2])
						} else {
							w.Write(expr)
						}
					} else if importOrExportDeclFound {
						if regexpFromExpr.Match(expr) || (bytes.HasPrefix(expr, []byte("import")) && regexpImportPathDecl.Match(expr)) {
							q := bytesSingleQoute
							a := bytes.Split(expr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(expr, q)
							}
							if len(a) == 3 {
								w.Write(a[0])
								w.Write(q)
								var res string
								res, err = resolve(string(a[1]), TsImportDecl, w.Len())
								if err != nil {
									return
								}
								w.WriteString(res)
								w.Write(q)
								w.Write(a[2])
							} else {
								w.Write(expr)
							}
							importOrExportDeclFound = false
						} else {
							w.Write(expr)
						}
					} else {
						w.Write(expr)
					}
				}
				i++
			}
			err = exprScanner.Err()
			if err != nil {
				return
			}
		}
		w.WriteByte('\n')
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
