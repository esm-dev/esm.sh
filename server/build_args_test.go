package server

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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
	for _, external := range []string{"node:fs", "example", "example/*", "broken/*", "node:sqlite", "node:test/reporters"} {
		t.Run(external, func(t *testing.T) {
			args := BuildArgs{External: *set.NewReadOnly(external)}
			err := resolveBuildArgs(context.Background(), nil, wd, &args, EsmPath{PkgName: "example", PkgVersion: "1.0.0", SubPath: "sub"})
			if err != nil {
				t.Fatal(err)
			}
			if !args.External.Has(external) {
				t.Fatalf("lost external %q", external)
			}
		})
	}
}

func TestResolveBuildArgsExternalPatterns(t *testing.T) {
	wd := t.TempDir()
	for name, contents := range map[string]string{
		"example":         `{"name":"example","dependencies":{"dep":"1.0.0"}}`,
		"dep":             `{"name":"dep","dependencies":{"css-in-js-utils":"2.0.1","@scope/pkg":"1.0.0"}}`,
		"css-in-js-utils": `{"name":"css-in-js-utils"}`,
		"@scope/pkg":      `{"name":"@scope/pkg"}`,
	} {
		pkgDir := filepath.Join(wd, "node_modules", name)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, external := range []string{"css-in-js-utils/*", "css-in-js-utils/lib/*", "@scope/pkg/*", "@scope/*", "@scope", "css-in-js-utils", "@scope/pkg"} {
		t.Run(external, func(t *testing.T) {
			args := BuildArgs{External: *set.NewReadOnly(external, "unrelated/*")}
			if err := resolveBuildArgs(context.Background(), nil, wd, &args, EsmPath{PkgName: "example", PkgVersion: "1.0.0"}); err != nil {
				t.Fatal(err)
			}
			if !args.External.Has(external) || args.External.Has("unrelated/*") {
				t.Fatalf("unexpected external patterns: %v", args.External.Values())
			}
		})
	}
}

func TestResolveBuildArgsDependencyOverrides(t *testing.T) {
	oldConfig, transport := config, http.DefaultTransport
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, http.DefaultTransport = oldConfig, transport })
	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	for name, contents := range map[string]string{
		"override-root":   `{"name":"override-root","version":"2.0.0","dependencies":{"override-direct":"0.0.0"}}`,
		"override-direct": `{"name":"override-direct","version":"2.0.0","dependencies":{"override-leaf":"0.0.0"}}`,
		"override-leaf":   `{"name":"override-leaf","version":"2.0.0"}`,
	} {
		pkgDir := filepath.Join(npmrc.StoreDir(), name+"@2.0.0", "node_modules", name)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { deleteCacheItem("npm:" + name + "@2.0.0") })
	}
	for _, installed := range []bool{false, true} {
		wd := t.TempDir()
		if installed {
			pkgDir := filepath.Join(wd, "node_modules", "override-direct")
			if err := os.MkdirAll(pkgDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"override-direct","version":"1.0.0"}`), 0644); err != nil {
				t.Fatal(err)
			}
		}
		want := map[string]string{"override-direct": "2.0.0", "override-leaf": "2.0.0"}
		args := BuildArgs{Deps: maps.Clone(want)}
		args.Deps["unrelated"] = "1.0.0"
		if err := resolveBuildArgs(context.Background(), npmrc, wd, &args, EsmPath{PkgName: "override-root", PkgVersion: "2.0.0"}); err != nil {
			t.Errorf("installed=%v: %v", installed, err)
			continue
		}
		if !maps.Equal(args.Deps, want) {
			t.Errorf("installed=%v: deps = %v, want %v", installed, args.Deps, want)
		}
	}
}

func TestResolveBuildArgsCanceled(t *testing.T) {
	oldConfig, transport := config, http.DefaultTransport
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, http.DefaultTransport = oldConfig, transport })
	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	// a stalled registry
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		args := BuildArgs{Deps: map[string]string{"react": "18.0.0"}}
		err := resolveBuildArgs(ctx, npmrc, t.TempDir(), &args, EsmPath{PkgName: "canceled-root", PkgVersion: "1.0.0"})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
			t.Fatalf("err = %v after %v, want %v after 1s", err, time.Since(start), context.DeadlineExceeded)
		}
	})
}
