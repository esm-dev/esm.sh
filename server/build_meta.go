package server

import (
	"bytes"
	"errors"
)

type BuildMeta struct {
	CJS           bool
	HasCSS        bool
	TypesOnly     bool
	ExportDefault bool
	Dts           string
	Imports       []string
}

func encodeBuildMeta(meta *BuildMeta) []byte {
	buf, recycle := NewBuffer()
	defer recycle()
	if meta.CJS {
		buf.Write([]byte{'j', '\n'})
	}
	if meta.HasCSS {
		buf.Write([]byte{'c', '\n'})
	}
	if meta.TypesOnly {
		buf.Write([]byte{'t', '\n'})
	}
	if meta.ExportDefault {
		buf.Write([]byte{'e', '\n'})
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
	lines := bytes.Split(data, []byte{'\n'})
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
			meta.HasCSS = true
		case ll == 1 && line[0] == 't':
			meta.TypesOnly = true
		case ll == 1 && line[0] == 'e':
			meta.ExportDefault = true
		case ll > 2 && line[0] == 'd' && line[1] == ':':
			meta.Dts = string(line[2:])
		case ll > 2 && line[0] == 'i' && line[1] == ':':
			meta.Imports = append(meta.Imports, string(line[2:]))
		default:
			return nil, errors.New("invalid build meta")
		}
	}
	return meta, nil
}
