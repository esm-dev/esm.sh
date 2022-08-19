package server

import (
	"testing"
)

func TestAliasDepsPrefix(t *testing.T) {
	external := newStringSet()
	external.Add("foo")
	prefix := encodeBuildArgsPrefix(map[string]string{"a": "b"}, PkgSlice{
		Pkg{Name: "b", Version: "1.0.0"},
		Pkg{Name: "d", Version: "1.0.0"},
		Pkg{Name: "c", Version: "1.0.0"},
	}, external, "0.128.0")
	a, d, e, dsv, err := decodeBuildArgsPrefix(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a["a"] != "b" {
		t.Fatal("invalid alias")
	}
	if len(d) != 3 {
		t.Fatal("invalid deps")
	}
	if len(e) != 1 {
		t.Fatal("invalid external")
	}
	if dsv != "0.128.0" {
		t.Fatal("invalid denoStdVersion")
	}
	t.Log(a, d, e)
}
