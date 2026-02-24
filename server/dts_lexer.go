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
	scanner.Buffer(nil, 1024*1024)
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
					if len(res) > 0 {
						fmt.Fprintf(w, `/// <reference %s="%s" />`, format, res)
					} else {
						fmt.Fprintf(w, `// ignored <reference %s="%s" />`, format, path)
					}
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
			stmtScanner := bufio.NewScanner(bytes.NewReader(line))
			stmtScanner.Buffer(nil, 1024*1024)
			stmtScanner.Split(splitJSStmt)
			for stmtScanner.Scan() {
				if i > 0 {
					w.WriteByte(';')
				}
				stmt, trimedLeftSpaces := trimSpace(stmtScanner.Bytes())
				w.Write(trimedLeftSpaces)
				if len(stmt) > 0 {
					if regexpImportCallExpr.Match(stmt) {
						stmt = regexpImportCallExpr.ReplaceAllFunc(stmt, func(importCallExpr []byte) []byte {
							q := bytesSingleQoute
							a := bytes.Split(importCallExpr, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(importCallExpr, q)
							}
							if len(a) == 3 {
								tmp, recycle := newBuffer()
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

					if !importOrExportDeclFound && (bytes.HasPrefix(stmt, []byte("import")) && regexpImportDecl.Match(stmt)) || (bytes.HasPrefix(stmt, []byte("export")) && regexpExportDecl.Match(stmt)) {
						importOrExportDeclFound = true
					}
					if bytes.HasPrefix(stmt, []byte("declare")) && regexpDeclareModuleStmt.Match(stmt) {
						q := bytesSingleQoute
						a := bytes.Split(stmt, q)
						if len(a) != 3 {
							q = bytesDoubleQoute
							a = bytes.Split(stmt, q)
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
							w.Write(stmt)
						}
					} else if importOrExportDeclFound {
						if regexpFromExpr.Match(stmt) || (bytes.HasPrefix(stmt, []byte("import")) && regexpImportPathDecl.Match(stmt)) {
							q := bytesSingleQoute
							a := bytes.Split(stmt, q)
							if len(a) != 3 {
								q = bytesDoubleQoute
								a = bytes.Split(stmt, q)
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
								w.Write(stmt)
							}
							importOrExportDeclFound = false
						} else {
							w.Write(stmt)
						}
					} else {
						w.Write(stmt)
					}
				}
				i++
			}
			err = stmtScanner.Err()
			if err != nil {
				return
			}
		}
		w.WriteByte('\n')
	}
	err = scanner.Err()
	return
}

// A split function for bufio.Scanner to split javascript statement
func splitJSStmt(data []byte, atEOF bool) (advance int, token []byte, err error) {
	var commentScope bool
	var stringScope byte
	for i := range data {
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

// trimSpace trims leading and trailing spaces, tabs, newlines and carriage returns
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
