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
	regexpImportPathDecl    = regexp.MustCompile(`^import\s*['"]`)
	regexpDeclareModuleStmt = regexp.MustCompile(`^declare\s+module\s*['"]`)
	regexpTSReferenceTag    = regexp.MustCompile(`^\s*<reference\s+(path|types)\s*=\s*['"](.+?)['"].+>`)
)

var (
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
	var importOrExportDeclDepth int
	var importOrExportDeclMayEnd bool
	writeSpecifier := func(stmt []byte, quoteStart int, kind TsImportKind) (bool, error) {
		quoteEnd := findDtsStringEnd(stmt, quoteStart)
		if quoteEnd < 0 {
			return false, nil
		}
		res, err := resolve(string(stmt[quoteStart+1:quoteEnd]), kind, w.Len()+quoteStart+1)
		if err != nil {
			return true, err
		}
		w.Write(stmt[:quoteStart+1])
		w.WriteString(res)
		w.Write(stmt[quoteEnd:])
		return true, nil
	}

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
		} else if after, ok := bytes.CutPrefix(line, bytesStripleSlash); ok {
			if a := regexpTSReferenceTag.FindSubmatch(after); a != nil {
				format := string(a[1])
				specifier := string(a[2])
				if format == "path" && !isRelPathSpecifier(specifier) {
					specifier = "./" + specifier
				}
				kind := TsReferenceTypes
				if format == "path" {
					kind = TsReferencePath
				}
				res, err := resolve(specifier, kind, w.Len())
				if err != nil {
					return err
				}
				if res != "" {
					fmt.Fprintf(w, `/// <reference %s="%s" />`, format, res)
				} else {
					fmt.Fprintf(w, `// ignored <reference %s="%s" />`, format, specifier)
				}
			} else {
				w.Write(line)
			}
		} else if bytes.HasPrefix(line, bytesDoubleSlash) {
			w.Write(line)
		} else {
			stmtIndex := 0
			for {
				advance, rawStmt, _ := splitJSStmt(line, true)
				if stmtIndex > 0 {
					w.WriteByte(';')
				}
				stmt, trimedLeftSpaces := trimSpace(rawStmt)
				w.Write(trimedLeftSpaces)
				if len(stmt) > 0 {
					if importOrExportDeclFound && importOrExportDeclMayEnd {
						if findDtsFromSpecifier(stmt) < 0 {
							importOrExportDeclFound = false
							importOrExportDeclDepth = 0
						}
						importOrExportDeclMayEnd = false
					}

					if !importOrExportDeclFound && ((bytes.HasPrefix(stmt, []byte("import")) && regexpImportDecl.Match(stmt)) || (bytes.HasPrefix(stmt, []byte("export")) && regexpExportDecl.Match(stmt))) {
						importOrExportDeclFound = true
						importOrExportDeclDepth = 0
					}

					var rewritten bytes.Buffer
					last := 0
					for i := 0; i < len(stmt); {
						c := stmt[i]
						if c == '\'' || c == '"' || c == '`' {
							end := findDtsStringEnd(stmt, i)
							if end < 0 {
								break
							}
							i = end + 1
							continue
						}
						if c == '/' && i+1 < len(stmt) {
							if stmt[i+1] == '/' {
								break
							}
							if stmt[i+1] == '*' {
								end := bytes.Index(stmt[i+2:], bytesCommentEnd)
								if end < 0 {
									multiLineComment = true
									break
								}
								i += end + 4
								continue
							}
						}

						wordLen := 0
						if bytes.HasPrefix(stmt[i:], []byte("import")) {
							wordLen = 6
						} else if bytes.HasPrefix(stmt[i:], []byte("require")) {
							wordLen = 7
						}
						if wordLen > 0 {
							beforeOK := i == 0 || (stmt[i-1] != '.' && !isDtsIdentifierByte(stmt[i-1]))
							after := i + wordLen
							afterOK := after == len(stmt) || !isDtsIdentifierByte(stmt[after])
							if beforeOK && afterOK {
								for after < len(stmt) && (stmt[after] == ' ' || stmt[after] == '\t') {
									after++
								}
								if after < len(stmt) && stmt[after] == '(' {
									after++
									for after < len(stmt) && (stmt[after] == ' ' || stmt[after] == '\t') {
										after++
									}
									if after < len(stmt) && (stmt[after] == '\'' || stmt[after] == '"') {
										end := findDtsStringEnd(stmt, after)
										if end > after+1 {
											position := w.Len() + rewritten.Len() + after + 1 - last
											res, err := resolve(string(stmt[after+1:end]), TsImportCall, position)
											if err != nil {
												return err
											}
											rewritten.Write(stmt[last : after+1])
											rewritten.WriteString(res)
											last = end
											i = end + 1
											continue
										}
									}
								}
							}
						}
						i++
					}
					if last > 0 {
						rewritten.Write(stmt[last:])
						stmt = rewritten.Bytes()
					}

					if importOrExportDeclFound {
						importOrExportDeclDepth += bytes.Count(stmt, []byte{'{'}) - bytes.Count(stmt, []byte{'}'})
					}

					wrote := false
					if match := regexpDeclareModuleStmt.FindIndex(stmt); match != nil {
						wrote, err = writeSpecifier(stmt, match[1]-1, TsDeclareModule)
						if err != nil {
							return
						}
					} else if importOrExportDeclFound {
						quoteStart := findDtsFromSpecifier(stmt)
						if quoteStart < 0 && bytes.HasPrefix(stmt, []byte("import")) {
							if match := regexpImportPathDecl.FindIndex(stmt); match != nil {
								quoteStart = match[1] - 1
							}
						}
						if quoteStart >= 0 {
							wrote, err = writeSpecifier(stmt, quoteStart, TsImportDecl)
							if err != nil {
								return
							}
							if wrote {
								importOrExportDeclFound = false
								importOrExportDeclDepth = 0
								importOrExportDeclMayEnd = false
							}
						}
					}
					if !wrote {
						w.Write(stmt)
					}
					if advance > 0 && importOrExportDeclFound {
						importOrExportDeclFound = false
						importOrExportDeclDepth = 0
						importOrExportDeclMayEnd = false
					}
				}
				stmtIndex++
				if advance == 0 {
					break
				}
				line = line[advance:]
			}
			if importOrExportDeclFound && importOrExportDeclDepth <= 0 {
				importOrExportDeclMayEnd = true
			}
		}
		w.WriteByte('\n')
	}
	return scanner.Err()
}

func findDtsStringEnd(data []byte, start int) int {
	quote := data[start]
	for i := start + 1; i < len(data); i++ {
		if data[i] == quote && !isDtsEscaped(data, i) {
			return i
		}
	}
	return -1
}

func findDtsFromSpecifier(data []byte) int {
	for i := 0; i < len(data); {
		if data[i] == '\'' || data[i] == '"' || data[i] == '`' {
			end := findDtsStringEnd(data, i)
			if end < 0 {
				return -1
			}
			i = end + 1
			continue
		}
		if data[i] == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				return -1
			}
			if data[i+1] == '*' {
				end := bytes.Index(data[i+2:], bytesCommentEnd)
				if end < 0 {
					return -1
				}
				i += end + 4
				continue
			}
		}
		if bytes.HasPrefix(data[i:], []byte("from")) && (i == 0 || !isDtsIdentifierByte(data[i-1])) {
			end := i + 4
			if end == len(data) || !isDtsIdentifierByte(data[end]) {
				for end < len(data) && (data[end] == ' ' || data[end] == '\t') {
					end++
				}
				if end < len(data) && (data[end] == '\'' || data[end] == '"') {
					return end
				}
			}
		}
		i++
	}
	return -1
}

func isDtsIdentifierByte(c byte) bool {
	return c == '$' || c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isDtsEscaped(data []byte, index int) bool {
	n := 0
	for i := index - 1; i >= 0 && data[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// A split function for bufio.Scanner to split javascript statement
func splitJSStmt(data []byte, atEOF bool) (advance int, token []byte, err error) {
	var commentScope bool
	var lineCommentScope bool
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
				if lineCommentScope {
					continue
				} else if commentScope {
					if prev == '*' {
						commentScope = false
					}
				} else if next == '*' {
					commentScope = true
				} else if next == '/' {
					lineCommentScope = true
				}
			}
		case '\'', '"', '`':
			if !commentScope && !lineCommentScope {
				if stringScope == 0 {
					stringScope = c
				} else if stringScope == c && !isDtsEscaped(data, i) {
					stringScope = 0
				}
			}
		case ';':
			if stringScope == 0 && !commentScope && !lineCommentScope {
				return i + 1, data[:i], nil
			}
		}
	}
	if !atEOF {
		return 0, nil, nil
	}
	return 0, data, bufio.ErrFinalToken
}

// trimSpace trims leading and trailing spaces, tabs, newlines and carriage returns
func trimSpace(line []byte) ([]byte, []byte) {
	s := 0
	l := len(line)
	for i := range l {
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
