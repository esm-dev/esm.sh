package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ije/gox/set"
)

func TestEncodeBuildArgs(t *testing.T) {
	conditions := []string{"react-server"}
	buildArgsString := encodeBuildArgs(
		BuildArgs{
			Alias: map[string]string{"a": "b"},
			Deps: map[string]string{
				"c": "1.0.0",
				"d": "1.0.0",
				"e": "1.0.0",
			},
			External:          *set.NewReadOnly("baz", "bar"),
			Conditions:        conditions,
			ExternalRequire:   true,
			KeepNames:         true,
			IgnoreAnnotations: true,
		},
		false,
	)
	args, err := decodeBuildArgs(buildArgsString)
	if err != nil {
		t.Fatal(err)
	}
	if len(args.Alias) != 1 || args.Alias["a"] != "b" {
		t.Fatal("invalid alias")
	}
	if len(args.Deps) != 3 {
		t.Fatal("invalid deps")
	}
	if args.External.Len() != 2 {
		t.Fatal("invalid external")
	}
	if len(args.Conditions) != 1 || args.Conditions[0] != "react-server" {
		t.Fatal("invalid conditions")
	}
	if !args.ExternalRequire {
		t.Fatal("ignoreRequire should be true")
	}
	if !args.KeepNames {
		t.Fatal("keepNames should be true")
	}
	if !args.IgnoreAnnotations {
		t.Fatal("ignoreAnnotations should be true")
	}
}

func TestResolveBuildArgsExternalWithoutWalkingDeps(t *testing.T) {
	wd := t.TempDir()
	for name, contents := range map[string]string{
		"example": `{"name":"example","dependencies":{"broken":"1.0.0"}}`,
		"broken":  `invalid package metadata`,
	} {
		pkgDir := filepath.Join(wd, "node_modules", name)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, external := range []string{"node:fs", "example"} {
		t.Run(external, func(t *testing.T) {
			args := BuildArgs{External: *set.NewReadOnly(external)}
			err := resolveBuildArgs(nil, wd, &args, EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: "sub"})
			if err != nil {
				t.Fatal(err)
			}
			if !args.External.Has(external) {
				t.Fatalf("lost external %q", external)
			}
		})
	}
}
