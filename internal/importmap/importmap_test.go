package importmap

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
)

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
