package server

import (
	"testing"
)

func TestEncodeBuildArgs(t *testing.T) {
	external := NewSet()
	conditions := []string{"react-server"}
	external.Add("baz")
	external.Add("bar")
	buildArgsString := encodeBuildArgs(
		BuildArgs{
			alias: map[string]string{"a": "b"},
			deps: map[string]string{
				"c": "1.0.0",
				"d": "1.0.0",
				"e": "1.0.0",
			},
			external:          external,
			conditions:        conditions,
			externalRequire:   true,
			keepNames:         true,
			ignoreAnnotations: true,
		},
		false,
	)
	args, err := decodeBuildArgs(buildArgsString)
	if err != nil {
		t.Fatal(err)
	}
	if len(args.alias) != 1 || args.alias["a"] != "b" {
		t.Fatal("invalid alias")
	}
	if len(args.deps) != 3 {
		t.Fatal("invalid deps")
	}
	if args.external.Len() != 2 {
		t.Fatal("invalid external")
	}
	if len(args.conditions) != 1 || args.conditions[0] != "react-server" {
		t.Fatal("invalid conditions")
	}
	if !args.externalRequire {
		t.Fatal("ignoreRequire should be true")
	}
	if !args.keepNames {
		t.Fatal("keepNames should be true")
	}
	if !args.ignoreAnnotations {
		t.Fatal("ignoreAnnotations should be true")
	}
}
