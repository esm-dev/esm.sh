package server

import (
	"testing"
)

func TestAliasDepsPrefix(t *testing.T) {
	external := newStringSet()
	external.Add("foo")
	prefix := encodeBuildArgsPrefix(
		BuildArgs{
			alias: map[string]string{"a": "b"},
			deps: PkgSlice{
				Pkg{Name: "c", Version: "1.0.0"},
				Pkg{Name: "d", Version: "1.0.0"},
				Pkg{Name: "e", Version: "1.0.0"},
				Pkg{Name: "foo", Version: "1.0.0"}, // to be avoided
			},
			external:          external,
			denoStdVersion:    "0.128.0",
			ignoreRequire:     true,
			keepNames:         true,
			ignoreAnnotations: true,
			sourcemap:         true,
		},
		Pkg{Name: "foo"},
		false,
	)
	args, err := decodeBuildArgsPrefix(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(args.alias) != 1 || args.alias["a"] != "b" {
		t.Fatal("invalid alias")
	}
	if len(args.deps) != 3 {
		t.Fatal("invalid deps")
	}
	if args.external.Size() != 1 {
		t.Fatal("invalid external")
	}
	if args.denoStdVersion != "0.128.0" {
		t.Fatal("invalid denoStdVersion")
	}
	if !args.ignoreRequire {
		t.Fatal("ignoreRequire should be true")
	}
	if !args.keepNames {
		t.Fatal("keepNames should be true")
	}
	if !args.ignoreAnnotations {
		t.Fatal("ignoreAnnotations should be true")
	}
	if !args.sourcemap {
		t.Fatal("sourcemap should be true")
	}
	t.Log(prefix, args)
}
