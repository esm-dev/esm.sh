package server

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
)

func TestParseBuildArgs(t *testing.T) {
	aliases := make([]string, maxBuildArgItems)
	deps := make([]string, maxBuildArgItems)
	for i := range maxBuildArgItems {
		aliases[i] = fmt.Sprintf("package%d:target%d/subpath", i, i)
		deps[i] = fmt.Sprintf("package%d@1.0.%d", i, i)
	}
	if _, err := parseAliasArg(strings.Join(aliases, ","), maxBuildArgBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := parseDepsArg(strings.Join(deps, ","), maxBuildArgBytes); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseExternalArg("react,@radix-ui,node:fs"); err != nil {
		t.Fatal(err)
	}
	if conditions, err := parseConditionsArg("browser,react-server,browser"); err != nil || len(conditions) != 2 {
		t.Fatalf("unexpected conditions: %v, %v", conditions, err)
	}
	if _, err := parseAliasArg("react/jsx-runtime:preact/jsx-runtime,react:preact@>=10", maxBuildArgBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := parseDepsArg("react@>=18", maxBuildArgBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := parseAliasArg(strings.Join(append(aliases, "extra:target"), ","), maxBuildArgBytes); err == nil {
		t.Fatal("expected too many aliases to be rejected")
	}
	if _, err := parseDepsArg(strings.Join(append(deps, "extra@1.0.0"), ","), maxBuildArgBytes); err == nil {
		t.Fatal("expected too many dependencies to be rejected")
	}
	if _, err := parseAliasArg(strings.Repeat("a", maxBuildArgBytes+1), maxBuildArgBytes); err == nil {
		t.Fatal("expected oversized aliases to be rejected")
	}
	if _, err := parseDepsArg(strings.Repeat("a", maxBuildArgBytes+1), maxBuildArgBytes); err == nil {
		t.Fatal("expected oversized dependencies to be rejected")
	}
	if _, _, err := parseExternalArg(strings.Repeat("a", maxBuildArgBytes+1)); err == nil {
		t.Fatal("expected oversized externals to be rejected")
	}
	if _, err := parseConditionsArg(strings.Repeat("a", maxBuildArgBytes+1)); err == nil {
		t.Fatal("expected oversized conditions to be rejected")
	}

	externals := make([]string, maxBuildArgItems+1)
	conditions := make([]string, maxBuildArgItems+1)
	for i := range externals {
		externals[i] = fmt.Sprintf("external%d", i)
		conditions[i] = fmt.Sprintf("condition%d", i)
	}
	if _, _, err := parseExternalArg(strings.Join(externals, ",")); err == nil {
		t.Fatal("expected too many externals to be rejected")
	}
	if _, err := parseConditionsArg(strings.Join(conditions, ",")); err == nil {
		t.Fatal("expected too many conditions to be rejected")
	}

	for _, value := range []string{
		"../react:preact",
		"react:../../preact",
		"react:https://example.com/preact",
		"react:preact?dev",
		"react:preact%2fcompat",
		"react:preact/compat&external=*",
		"react:preact/compat=other",
	} {
		if _, err := parseAliasArg(value, maxBuildArgBytes); err == nil {
			t.Errorf("expected alias %q to be rejected", value)
		}
	}
	for _, value := range []string{"../react@1.0.0", "react@1.0.0/subpath", "react@1.0.0?dev", "react@1.0.0&external=*"} {
		if _, err := parseDepsArg(value, maxBuildArgBytes); err == nil {
			t.Errorf("expected dependency %q to be rejected", value)
		}
	}
	for _, value := range []string{"../react", "@bad/scope/extra", "node:not-a-builtin"} {
		if _, _, err := parseExternalArg(value); err == nil {
			t.Errorf("expected external %q to be rejected", value)
		}
	}
	for _, value := range []string{"bad condition", "bad\ncondition", "bad/condition", "browser&external=*", "quoted\"condition", "`template`"} {
		if _, err := parseConditionsArg(value); err == nil {
			t.Errorf("expected condition %q to be rejected", value)
		}
	}

	parsed, err := parseDepsArg("react@18.2.0,react@19.0.0", maxBuildArgBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed["react"] != "19.0.0" {
		t.Fatalf("unexpected duplicate dependency result: %#v", parsed)
	}

	if _, err := decodeBuildArgs("X-" + btoaUrl("a"+strings.Repeat("a", maxBuildArgBytes+1))); err == nil {
		t.Fatal("expected an oversized encoded alias to be rejected")
	}
	if _, err := decodeBuildArgs("X-" + btoaUrl(strings.Repeat("r", maxEncodedBuildArgBytes+1))); err == nil {
		t.Fatal("expected oversized encoded build args to be rejected")
	}
}

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

	reversedConditions := encodeBuildArgs(BuildArgs{Conditions: []string{"worker", "react-server"}}, false)
	if reversedConditions == encodeBuildArgs(BuildArgs{Conditions: []string{"react-server", "worker"}}, false) {
		t.Fatal("condition order must be part of the build key")
	}
	if reversedConditions == btoaUrl("cworker,react-server") {
		t.Fatal("condition builds must not reuse legacy cache keys")
	}
	legacy, err := decodeBuildArgs("X-" + btoaUrl("cworker,react-server"))
	if err != nil || !slices.Equal(legacy.Conditions, []string{"worker", "react-server"}) {
		t.Fatalf("failed to decode legacy conditions: %#v, %v", legacy.Conditions, err)
	}
	if _, err := decodeBuildArgs("X-" + btoaUrl("cworker\nCreact-server")); err == nil {
		t.Fatal("mixed legacy and current conditions must be rejected")
	}

	longAliases := make(map[string]string, maxBuildArgItems)
	for i := range maxBuildArgItems {
		longAliases[fmt.Sprintf("package%d", i)] = fmt.Sprintf("%s%d", strings.Repeat("a", 140), i)
	}
	encoded := encodeBuildArgs(BuildArgs{Alias: longAliases}, false)
	if len(encoded) <= base64.RawURLEncoding.EncodedLen(maxBuildArgBytes) {
		t.Fatal("round-trip fixture does not exceed the direct query limit")
	}
	decoded, err := decodeBuildArgs("X-" + encoded)
	if err != nil || !maps.Equal(decoded.Alias, longAliases) {
		t.Fatalf("failed to round-trip canonical build args: %v", err)
	}
}

func TestResolveConditionOrder(t *testing.T) {
	conditions := npm.NewJSONObject(
		[]string{"alpha", "beta"},
		map[string]any{"alpha": "./alpha.js", "beta": "./beta.js"},
	)
	for _, test := range []struct {
		order []string
		want  string
	}{
		{[]string{"alpha", "beta"}, "./alpha.js"},
		{[]string{"beta", "alpha"}, "./beta.js"},
	} {
		ctx := BuildContext{args: BuildArgs{Conditions: test.order}}
		if entry := ctx.resolveConditionExportEntry(conditions, "module"); entry.main != test.want {
			t.Fatalf("conditions %v resolved %q, want %q", test.order, entry.main, test.want)
		}
	}
}

func TestNormalizeBuildArgsCanonicalizesUnusedVariants(t *testing.T) {
	const (
		usedName            = "normalize-used-fixture"
		replacementName     = "normalize-replacement-fixture"
		implicitName        = "normalize-implicit-fixture"
		dependencyAliasName = "normalize-dependency-alias-fixture"
		aliasedName         = "normalize-aliased-fixture"
	)
	t.Cleanup(func() {
		cacheStore.Range(func(key, _ any) bool {
			s := key.(string)
			for _, name := range []string{usedName, replacementName, implicitName, dependencyAliasName, aliasedName} {
				if strings.HasPrefix(s, "npm:"+name+"@") || strings.HasPrefix(s, "404:"+name+"@") {
					cacheStore.Delete(key)
				}
			}
			return true
		})
	})
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, version := path.Split(strings.TrimPrefix(r.URL.Path, "/"))
		name = strings.TrimSuffix(name, "/")
		if version == "0.0.0-missing" {
			http.NotFound(w, r)
			return
		}
		resolved := map[string]string{
			usedName:        "2.0.0",
			replacementName: "1.2.3",
			implicitName:    "2.0.0",
			aliasedName:     "3.1.0",
		}[name]
		if resolved == "" || (version != "latest" && version != resolved) {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"name":%q,"version":%q}`, name, resolved)
	}))
	defer registry.Close()
	reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: registry.URL + "/"}}
	reg.versionRouteSupported.Store(1)
	npmrc := &NpmRC{globalRegistry: reg, scopedRegistries: map[string]*NpmRegistry{}}

	wd := t.TempDir()
	pkgDir := path.Join(wd, "node_modules", "fixture")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(pkgDir, "package.json"), []byte(`{"name":"fixture","version":"1.0.0"}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := BuildContext{
		npmrc:   npmrc,
		wd:      wd,
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0"},
		args: BuildArgs{
			Alias: map[string]string{"unused": "preact"},
			Deps:  map[string]string{"unused": "1.0.0"},
		},
		target: "es2022",
	}
	rawPath := ctx.Path()
	canonicalPath := (&BuildContext{
		esmPath: ctx.esmPath,
		target:  ctx.target,
	}).Path()
	if err := ctx.normalizeBuildArgs(); err != nil {
		t.Fatal(err)
	}
	if len(ctx.args.Alias) != 0 || len(ctx.args.Deps) != 0 {
		t.Fatalf("unused build arguments were not removed: %#v", ctx.args)
	}
	if ctx.Path() != canonicalPath || ctx.rawPath != rawPath {
		t.Fatalf("build path was not canonicalized: raw=%q got=%q want=%q", rawPath, ctx.Path(), canonicalPath)
	}

	if err := os.WriteFile(path.Join(pkgDir, "package.json"), []byte(fmt.Sprintf(`{"name":"fixture","version":"1.0.0","dependencies":{%q:"^1.0.0"}}`, usedName)), 0644); err != nil {
		t.Fatal(err)
	}
	ctx = BuildContext{
		npmrc:   npmrc,
		wd:      wd,
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0"},
		args: BuildArgs{
			Alias: map[string]string{usedName: replacementName},
			Deps:  map[string]string{usedName: "2.0.0"},
		},
		target: "es2022",
	}
	rawPath = ctx.Path()
	if err := ctx.normalizeBuildArgs(); err != nil {
		t.Fatal(err)
	}
	if ctx.args.Alias[usedName] != replacementName+"@1.2.3" || ctx.args.Deps[usedName] != "2.0.0" {
		t.Fatalf("used build arguments were removed: %#v", ctx.args)
	}
	if ctx.Path() == rawPath || ctx.rawPath != rawPath {
		t.Fatalf("meaningful build path was not canonicalized: raw=%q normalized=%q", rawPath, ctx.Path())
	}

	ctx = BuildContext{
		npmrc:   npmrc,
		wd:      wd,
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0", SubPath: implicitName},
		args: BuildArgs{
			Alias:    map[string]string{implicitName: replacementName},
			Deps:     map[string]string{implicitName: "2.0.0"},
			External: *set.NewReadOnly(implicitName),
		},
		target: "es2022",
	}
	rawPath = ctx.Path()
	if err := ctx.normalizeBuildArgs(); err != nil {
		t.Fatal(err)
	}
	if ctx.args.Alias[implicitName] != replacementName+"@1.2.3" || ctx.args.Deps[implicitName] != "2.0.0" || !ctx.args.External.Has(implicitName) {
		t.Fatalf("subpath dependency arguments were removed: %#v", ctx.args)
	}
	if ctx.Path() == rawPath || ctx.rawPath != rawPath {
		t.Fatalf("subpath dependency path was not canonicalized: raw=%q normalized=%q", rawPath, ctx.Path())
	}

	if err := os.WriteFile(path.Join(pkgDir, "package.json"), []byte(fmt.Sprintf(
		`{"name":"fixture","version":"1.0.0","dependencies":{%q:"^1.0.0",%q:"npm:%s@3.1.0"}}`,
		usedName,
		dependencyAliasName,
		aliasedName,
	)), 0644); err != nil {
		t.Fatal(err)
	}
	ctx = BuildContext{
		npmrc: npmrc,
		wd:    wd,
		pkgJson: &npm.PackageJSON{Dependencies: map[string]string{
			usedName:            "^1.0.0",
			dependencyAliasName: "npm:" + aliasedName + "@3.1.0",
		}},
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0"},
		args: BuildArgs{
			Alias: map[string]string{usedName: dependencyAliasName + "/compat"},
		},
		target: "es2022",
	}
	if err := ctx.normalizeBuildArgs(); err != nil {
		t.Fatal(err)
	}
	if ctx.args.Alias[usedName] != aliasedName+"@3.1.0/compat" {
		t.Fatalf("npm alias selector was not canonicalized: %#v", ctx.args.Alias)
	}

	ctx = BuildContext{
		npmrc:   npmrc,
		wd:      wd,
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0"},
		args:    BuildArgs{Deps: map[string]string{usedName: "0.0.0-missing"}},
		target:  "es2022",
	}
	if err := ctx.normalizeBuildArgs(); err == nil {
		t.Fatal("nonexistent dependency version was accepted")
	}

	ctx = BuildContext{
		npmrc:   npmrc,
		wd:      wd,
		esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0"},
		args:    BuildArgs{Alias: map[string]string{usedName: replacementName + "@0.0.0-missing"}},
		target:  "es2022",
	}
	if err := ctx.normalizeBuildArgs(); err == nil {
		t.Fatal("nonexistent alias target version was accepted")
	}
}

func TestTypeBuildPathIncludesArgs(t *testing.T) {
	for _, subPath := range []string{"index.d.ts", "index.d.mts", "index.d.cts", "source.ts", "source.tsx"} {
		plain := (&BuildContext{
			esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0", SubPath: subPath},
			target:  "types",
		}).Path()
		withArgs := (&BuildContext{
			esmPath: EsmPath{PkgName: "fixture", PkgVersion: "1.0.0", SubPath: subPath},
			args:    BuildArgs{Conditions: []string{"browser"}},
			target:  "types",
		}).Path()
		if plain == withArgs {
			t.Errorf("%s build path does not include args", subPath)
		}
	}
}
