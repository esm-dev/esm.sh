package server

import (
	"bytes"
	"errors"
	"strings"

	"github.com/ije/gox/utils"
)

type BuildMeta struct {
	CJS           bool
	CSSInJS       bool
	TypesOnly     bool
	ExportDefault bool
	CSSEntry      string
	Dts           string
	Imports       []string
}

func encodeBuildMeta(meta *BuildMeta) []byte {
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{'E', 'S', 'M', '\r', '\n'})
	if meta.CJS {
		buf.Write([]byte{'j', '\n'})
	}
	if meta.CSSInJS {
		buf.Write([]byte{'c', '\n'})
	}
	if meta.TypesOnly {
		buf.Write([]byte{'t', '\n'})
	}
	if meta.ExportDefault {
		buf.Write([]byte{'e', '\n'})
	}
	if meta.CSSEntry != "" {
		buf.Write([]byte{'.', ':'})
		buf.WriteString(meta.CSSEntry)
		buf.WriteByte('\n')
	}
	if meta.Dts != "" {
		buf.Write([]byte{'d', ':'})
		buf.WriteString(meta.Dts)
		buf.WriteByte('\n')
	}
	if len(meta.Imports) > 0 {
		for _, path := range meta.Imports {
			buf.Write([]byte{'i', ':'})
			buf.WriteString(path)
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

func decodeBuildMeta(data []byte) (*BuildMeta, error) {
	meta := &BuildMeta{}
	if len(data) < 5 || !bytes.Equal(data[:5], []byte{'E', 'S', 'M', '\r', '\n'}) {
		return nil, errors.New("invalid build meta")
	}
	lines := bytes.Split(data[5:], []byte{'\n'})
	n := 0
	for _, line := range lines {
		if len(line) > 2 && line[0] == 'i' && line[1] == ':' {
			n++
		}
	}
	meta.Imports = make([]string, 0, n)
	for _, line := range lines {
		ll := len(line)
		if ll == 0 {
			continue
		}
		switch {
		case ll == 1 && line[0] == 'j':
			meta.CJS = true
		case ll == 1 && line[0] == 'c':
			meta.CSSInJS = true
		case ll == 1 && line[0] == 't':
			meta.TypesOnly = true
		case ll == 1 && line[0] == 'e':
			meta.ExportDefault = true
		case ll > 2 && line[0] == '.' && line[1] == ':':
			meta.CSSEntry = string(line[2:])
		case ll > 2 && line[0] == 'd' && line[1] == ':':
			meta.Dts = string(line[2:])
			if !endsWith(meta.Dts, ".ts", ".mts", ".cts") {
				return nil, errors.New("invalid dts path")
			}
		case ll > 2 && line[0] == 'i' && line[1] == ':':
			importSepcifier := string(line[2:])
			if !strings.HasSuffix(importSepcifier, ".mjs") {
				_, q := utils.SplitByLastByte(importSepcifier, '?')
				if q == "" || !strings.Contains(q, "target=") {
					return nil, errors.New("invalid import specifier")
				}
			}
			meta.Imports = append(meta.Imports, importSepcifier)
		default:
			return nil, errors.New("invalid build meta")
		}
	}
	return meta, nil
}
