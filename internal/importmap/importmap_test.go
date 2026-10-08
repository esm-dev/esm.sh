package importmap

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestAddImportDependencyVersions(t *testing.T) {
	const origin = "https://importmap-versions.test"
	for _, tc := range []struct {
		version string
		want    string
	}{
		{"1.0.0", "1.0.0"},
		{"2.0.0", "2.0.0"},
		{"^1.0.0", "1.0.0"},
		{"^2.0.0", "2.0.0"},
	} {
		for _, mode := range []struct{ peer, direct bool }{{false, false}, {false, true}, {true, false}} {
			t.Run(fmt.Sprintf("%s/peer=%t/direct=%t", tc.version, mode.peer, mode.direct), func(t *testing.T) {
				im := Blank()
				im.SetConfig(Config{CDN: origin})
				if mode.direct {
					warnings, errors := im.AddImport(ImportMeta{Import: Import{Name: "dep", Version: "2.0.0"}}, true)
					if len(warnings) != 0 || len(errors) != 0 {
						t.Fatalf("warnings %v, errors %v", warnings, errors)
					}
				}
				for version, resolved := range map[string]string{"2.0.0": "2.0.0", tc.version: tc.want} {
					cacheKey := origin + "/dep@" + version + "?meta"
					fetchCache.Store(cacheKey, ImportMeta{Import: Import{Name: "dep", Version: resolved}})
					t.Cleanup(func() { fetchCache.Delete(cacheKey) })
				}
				for i, version := range []string{"2.0.0", tc.version} {
					imp := ImportMeta{Import: Import{Name: fmt.Sprintf("pkg-%d", i), Version: "1.0.0"}}
					if mode.peer {
						imp.PeerImports = []string{"/dep@" + version}
					} else {
						imp.Imports = []string{"/dep@" + version}
					}
					warnings, errors := im.AddImport(imp, true)
					if len(errors) != 0 {
						t.Fatal(errors)
					}
					if mode.peer && i == 1 && tc.want != "2.0.0" {
						if len(warnings) != 1 || !strings.Contains(warnings[0], "unmet "+tc.version) {
							t.Fatalf("expected peer warning, got %v", warnings)
						}
					} else if len(warnings) != 0 {
						t.Fatal(warnings)
					}
				}
				for i, version := range []string{"2.0.0", tc.want} {
					if mode.peer {
						version = "2.0.0"
					}
					referrer, _ := url.Parse(fmt.Sprintf("%s/*pkg-%d@1.0.0/es2022/pkg-%d.mjs", origin, i, i))
					want := origin + "/dep@" + version + "/es2022/dep.mjs"
					if got, ok := im.Resolve("dep", referrer); !ok || got != want {
						t.Errorf("Resolve(dep, %s) = %q, %v; want %q", referrer, got, ok, want)
					}
				}
			})
		}
	}
}

func TestFormatJSON(t *testing.T) {
	for _, source := range []string{
		`{"imports":{"quote\"\\\n\t":"data:text/javascript,export default \"hello\\world\""}}`,
		`{"config":{"cdn":"https://cdn.test/\"\\\n","target":"esnext\"\\\t"},"imports":{}}`,
		`{"imports":{},"scopes":{"https://example.com/\"\\\n/":{"quote\"\\\t":"data:text/javascript,export default \"hello\""}}}`,
		`{"imports":{},"integrity":{"url\"\\\n":"sha384-\"\\\t"}}`,
	} {
		t.Run(source, func(t *testing.T) {
			im, err := Parse(nil, []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			var want ImportMapJson
			if err := json.Unmarshal([]byte(source), &want); err != nil {
				t.Fatal(err)
			}
			for _, indent := range []int{0, 2} {
				data := im.FormatJSON(indent)
				var got ImportMapJson
				if err := json.Unmarshal([]byte(data), &got); err != nil {
					t.Fatalf("invalid JSON: %v\n%s", err, data)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got %#v, want %#v", got, want)
				}
			}
			if _, err := json.Marshal(im); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFormatJSONEmptyValues(t *testing.T) {
	for _, entries := range []string{
		`"a":"./a.js","z":null`,
		`"a":"./a.js","z":""`,
		`"a":null,"b":"./b.js","c":null,"d":"./d.js","z":null`,
		`"z":null`,
	} {
		t.Run(entries, func(t *testing.T) {
			source := fmt.Sprintf(`{"imports":{%s},"scopes":{"/app/":{%s}},"integrity":{%s}}`, entries, entries, entries)
			im, err := Parse(nil, []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			data := im.FormatJSON(0)
			var got ImportMapJson
			if err := json.Unmarshal([]byte(data), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, data)
			}
			im.Imports.Range(func(key, value string) bool {
				if value != "" && (got.Imports[key] != value || got.Scopes["/app/"][key] != value || got.Integrity[key] != value) {
					t.Errorf("lost mapping %q: %q", key, value)
				}
				return true
			})
		})
	}
}

func TestAddImportConcurrentResults(t *testing.T) {
	const count = 64
	im := Blank()
	imp := ImportMeta{Import: Import{Name: "root", Version: "1.0.0"}}
	for i := range count {
		name := fmt.Sprintf("peer-%d", i)
		im.Imports.Set(name, "https://esm.sh/"+name+"@1.0.0")
		imp.PeerImports = append(imp.PeerImports, "/"+name+"@^2.0.0")
		imp.Imports = append(imp.Imports, fmt.Sprintf("invalid-%d", i))
	}
	warnings, errors := im.AddImport(imp, true)
	if len(warnings) != count || len(errors) != count {
		t.Fatalf("got %d warnings and %d errors, want %d each", len(warnings), len(errors), count)
	}
	for i := range count {
		if !strings.HasPrefix(warnings[i], fmt.Sprintf("incorrect peer dependency peer-%d@1.0.0", i)) {
			t.Errorf("unexpected warning: %q", warnings[i])
		}
		if errors[i].Error() != fmt.Sprintf("invalid pathname or url: invalid-%d", i) {
			t.Errorf("unexpected error: %v", errors[i])
		}
	}
}

func TestAddImportConcurrentScopes(t *testing.T) {
	const count = 64
	const origin = "https://importmap-concurrent.test"
	im := Blank()
	im.SetConfig(Config{CDN: origin})
	imp := ImportMeta{Import: Import{Name: "root", Version: "1.0.0"}}
	for i := range count {
		name := fmt.Sprintf("dependency-%d", i)
		im.Imports.Set(name, origin+"/"+name+"@1.0.0")
		imp.Imports = append(imp.Imports, "/"+name+"@^2.0.0")
		cacheKey := origin + "/" + name + "@^2.0.0?meta"
		fetchCache.Store(cacheKey, ImportMeta{Import: Import{Name: name, Version: "2.0.0"}})
		t.Cleanup(func() { fetchCache.Delete(cacheKey) })
	}
	warnings, errors := im.AddImport(imp, true)
	if len(warnings) != 0 || len(errors) != 0 {
		t.Fatalf("warnings %v, errors %v", warnings, errors)
	}
	imports, ok := im.GetScopeImports(origin + "/" + imp.EsmSpecifier() + "/")
	if !ok || imports.Len() != count {
		t.Fatalf("missing dependency scope entries: %v", imports)
	}
	for i := range count {
		name := fmt.Sprintf("dependency-%d", i)
		if got, ok := imports.Get(name); !ok || got != origin+"/"+name+"@2.0.0/es2022/"+name+".mjs" {
			t.Errorf("incorrect scoped dependency %s: %q", name, got)
		}
	}
}

func TestResolveConcurrentScopes(t *testing.T) {
	im := Blank()
	im.Imports.Set("pkg", "./default.js")
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			<-start
			for j := range 8 {
				if got, ok := im.Resolve("pkg", nil); !ok || got != "file:///default.js" {
					t.Errorf("unexpected default import: %q", got)
				}
				scope := fmt.Sprintf("https://example.com/%d/%d/", i, j)
				im.SetScopeImports(scope, newImports(map[string]string{"pkg": "./scoped.js"}))
				referrer, _ := url.Parse(scope + "main.js")
				if got, ok := im.Resolve("pkg", referrer); !ok || got != "file:///scoped.js" {
					t.Errorf("unexpected scoped import: %q", got)
				}
			}
		})
	}
	close(start)
	wg.Wait()
}

func TestResolveLongestPrefix(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/app/import-map.json")
	im, err := Parse(baseURL, []byte(`{
		"imports": {
			"pkg/": "./default/",
			"pkg/nested/": "./nested/",
			"pkg/nested/exact": "./exact.js"
		},
		"scopes": {
			"https://example.com/scoped/": {
				"pkg/": "./scoped/",
				"pkg/nested/": "./scoped-nested/"
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		specifier string
		referrer  string
		want      string
	}{
		{"pkg/nested/file.js?raw#section", "https://example.com/main.js", "https://example.com/app/nested/file.js?raw#section"},
		{"pkg/nested/exact", "https://example.com/main.js", "https://example.com/app/exact.js"},
		{"pkg/nested/file.js", "https://example.com/scoped/main.js", "https://example.com/app/scoped-nested/file.js"},
	} {
		referrer, _ := url.Parse(tc.referrer)
		for range 100 {
			if got, ok := im.Resolve(tc.specifier, referrer); !ok || got != tc.want {
				t.Fatalf("Resolve(%q, %q) = %q, %v; want %q", tc.specifier, tc.referrer, got, ok, tc.want)
			}
		}
	}
}

func TestAddPackages(t *testing.T) {
	// 1. add imports
	{
		im := Blank()
		warnings, errors := im.AddImportFromSpecifier("react@19", false)
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		warnings, errors = im.AddImportFromSpecifier("react-dom@19/client", false)
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 2 {
			t.Fatalf("Expected 2 imports, got %d", im.Imports.Len())
		}
		keys := im.Imports.Keys()
		sort.Strings(keys)
		if keys[0] != "react" || keys[1] != "react-dom/client" {
			t.Fatalf("Expected [react react-dom/client], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		sort.Strings(keys)
		if keys[0] != "react-dom" || keys[1] != "scheduler" {
			t.Fatalf("Expected [react-dom scheduler], got %v", keys)
		}
	}

	// 2. add peer imports to `imports`
	{
		im := Blank()
		warnings, errors := im.AddImportFromSpecifier("react-dom@19", false)
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 2 {
			t.Fatalf("Expected 2 imports, got %d", im.Imports.Len())
		}
		keys := im.Imports.Keys()
		sort.Strings(keys)
		if keys[0] != "react" || keys[1] != "react-dom" {
			t.Fatalf("Expected [react react-dom], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 0 {
			t.Fatalf("Expected 0 imports in scope, got %d", scope.Len())
		}
	}

	// 3. with config
	{
		im := &ImportMap{
			config: Config{
				CDN:    "https://cdn.esm.sh",
				Target: "esnext",
			},
			Imports:   newImports(nil),
			scopes:    make(map[string]*Imports),
			integrity: newImports(nil),
		}
		warnings, errors := im.AddImportFromSpecifier("react@19", false)
		if len(errors) > 0 {
			t.Fatalf("Errors: %v", errors)
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 1 {
			t.Fatalf("Expected 1 imports, got %d", im.Imports.Len())
		}
		keys := im.Imports.Keys()
		if keys[0] != "react" {
			t.Fatalf("Expected [react], got %v", keys)
		}
		if url, ok := im.Imports.Get("react"); !ok || !strings.HasPrefix(url, "https://cdn.esm.sh/react@19.") || !strings.HasSuffix(url, "/esnext/react.mjs") {
			t.Fatalf("Expected react to be resolved to https://cdn.esm.sh/react@19.x.x/esnext/react.mjs, got %s", url)
		}
	}
}

func TestResolve(t *testing.T) {
	im := Blank()
	_, errors := im.AddImportFromSpecifier("react-dom@19.2.4/client", false)
	if len(errors) > 0 {
		t.Fatalf("Failed to add react-dom/client: %v", errors)
	}
	referrer, _ := url.Parse("file:///main.js")
	modUrl, ok := im.Resolve("react", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if !strings.HasPrefix(modUrl, "https://esm.sh/react@19.") || !strings.HasSuffix(modUrl, "/es2022/react.mjs") {
		t.Fatalf("Expected react to be resolved to https://esm.sh/react@19.x.x/es2022/react.mjs, got %s", modUrl)
	}
	modUrl, ok = im.Resolve("react-dom/client", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if !strings.HasPrefix(modUrl, "https://esm.sh/*react-dom@19.") || !strings.HasSuffix(modUrl, "/es2022/client.mjs") {
		t.Fatalf("Expected react-dom/client to be resolved to https://esm.sh/*react-dom@19.x.x/es2022/client.mjs, got %s", modUrl)
	}
	_, ok = im.Resolve("react-dom", referrer)
	if ok {
		t.Fatalf("Expected ok to be false, got true")
	}
	_, ok = im.Resolve("scheduler", referrer)
	if ok {
		t.Fatalf("Expected ok to be false, got true")
	}
	referrer, _ = url.Parse("https://esm.sh/*react-dom@19.2.4/es2022/client.mjs")
	modUrl, ok = im.Resolve("react-dom", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if !strings.HasPrefix(modUrl, "https://esm.sh/*react-dom@19.") || !strings.HasSuffix(modUrl, "/es2022/react-dom.mjs") {
		t.Fatalf("Expected react-dom/client to be resolved to https://esm.sh/*react-dom@19.x.x/es2022/react-dom.mjs, got %s", modUrl)
	}
	modUrl, ok = im.Resolve("scheduler", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if !strings.HasPrefix(modUrl, "https://esm.sh/scheduler@0.27.") || !strings.HasSuffix(modUrl, "/es2022/scheduler.mjs") {
		t.Fatalf("Expected scheduler to be resolved to https://esm.sh/scheduler@0.27.x/es2022/scheduler.mjs, got %s", modUrl)
	}
}
