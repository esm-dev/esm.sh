package server

import (
	"reflect"
	"testing"
)

func TestEncodeBuildMeta(t *testing.T) {
	meta1 := &BuildMeta{
		CJS:           true,
		CSSInJS:       true,
		TypesOnly:     true,
		ExportDefault: true,
		CSSEntry:      "index.css",
		Dts:           "index.d.ts",
		Imports:       []string{"/react@19.2.4?target=es2022", "/react-dom@19.2.4?target=es2022"},
		Integrity:     "sha384-...",
	}
	data := encodeBuildMeta(meta1)
	meta2, err := decodeBuildMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta1, meta2) {
		t.Fatalf("meta mismatch: %+v != %+v", meta1, meta2)
	}
}
