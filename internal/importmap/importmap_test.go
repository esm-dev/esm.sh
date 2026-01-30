package importmap

import (
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestAddPackages(t *testing.T) {
	// 1. add packages
	{
		im := ImportMap{}
		warnings, errors := im.AddImportFromSpecifier("react@19")
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		warnings, errors = im.AddImportFromSpecifier("react-dom@19")
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 4 {
			t.Fatalf("Expected 4 imports, got %d", im.Imports.Len())
		}
		keys := im.Imports.Keys()
		if keys[0] != "react" || keys[1] != "react-dom" || keys[2] != "react-dom/" || keys[3] != "react/" {
			t.Fatalf("Expected [react react-dom react-dom/ react/], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "scheduler" || keys[1] != "scheduler/" {
			t.Fatalf("Expected [scheduler scheduler/], got %v", keys)
		}
	}

	// 2. add peer dependencies to `imports`
	{
		im := ImportMap{}
		warnings, errors := im.AddImportFromSpecifier("react-dom@19")
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 4 {
			t.Fatalf("Expected 4 imports, got %d", im.Imports.Len())
		}
		keys := im.Imports.Keys()
		if keys[0] != "react" || keys[1] != "react-dom" || keys[2] != "react-dom/" || keys[3] != "react/" {
			t.Fatalf("Expected [react react-dom react-dom/ react/], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "scheduler" || keys[1] != "scheduler/" {
			t.Fatalf("Expected [scheduler scheduler/], got %v", keys)
		}
	}

	// 3. resolve dependencies without conflicts
	{
		im := ImportMap{}
		warnings, errors := im.AddImportFromSpecifier("loose-envify@1.1.0")
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
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}

		warnings, errors = im.AddImportFromSpecifier("react@18")
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 4 {
			t.Fatalf("Expected 4 imports, got %d", im.Imports.Len())
		}
		keys = im.Imports.Keys()
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" || keys[2] != "react" || keys[3] != "react/" {
			t.Fatalf("Expected [loose-envify loose-envify/ react react/], got %v", keys)
		}
		if v, ok := im.Imports.Get("loose-envify"); !ok || v != "https://esm.sh/*loose-envify@1.1.0/es2022/loose-envify.mjs" {
			t.Fatalf("Expected loose-envify to be resolved to loose-envify@1.1.0, got %s", v)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope = im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
	}

	// 4. resolve dependencies with conflicts
	{
		im := ImportMap{}
		warnings, errors := im.AddImportFromSpecifier("loose-envify@1.0.0")
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
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		if len(im.scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.scopes))
		}
		scope := im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}

		warnings, errors = im.AddImportFromSpecifier("react@18")
		if len(errors) > 0 {
			t.Fatalf("Expected no errors, got %d", len(errors))
		}
		if len(warnings) > 0 {
			t.Fatalf("Expected no warnings, got %d", len(warnings))
		}
		if im.Imports.Len() != 4 {
			t.Fatalf("Expected 4 imports, got %d", im.Imports.Len())
		}
		keys = im.Imports.Keys()
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" || keys[2] != "react" || keys[3] != "react/" {
			t.Fatalf("Expected [loose-envify loose-envify/ react react/], got %v", keys)
		}
		if v, ok := im.Imports.Get("loose-envify"); !ok || v != "https://esm.sh/*loose-envify@1.0.0/es2022/loose-envify.mjs" {
			t.Fatalf("Expected loose-envify to be resolved to loose-envify@1.0.0, got %s", v)
		}
		if len(im.scopes) != 3 {
			t.Fatalf("Expected 3 scopes, got %d", len(im.scopes))
		}
		scope = im.scopes["https://esm.sh/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
		if v, ok := scope.Get("js-tokens"); !ok || v != "https://esm.sh/js-tokens@1.0.3/es2022/js-tokens.mjs" {
			t.Fatalf("Expected js-tokens to be resolved to js-tokens@1.0.3, got %s", v)
		}
		scope = im.scopes["https://esm.sh/*react@18.3.1/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		scope = im.scopes["https://esm.sh/*loose-envify@1.4.0/"]
		if scope.Len() != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", scope.Len())
		}
		keys = scope.Keys()
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
		if v, ok := scope.Get("js-tokens"); !ok || v != "https://esm.sh/js-tokens@4.0.0/es2022/js-tokens.mjs" {
			t.Fatalf("Expected js-tokens to be resolved to js-tokens@4.0.0, got %s", v)
		}
	}

	// 4. with config
	{
		im := ImportMap{
			config: Config{
				CDN:    "https://next.esm.sh",
				Target: "esnext",
				SRI: SRIConfig{
					Algorithm: "sha512",
				},
			},
		}
		warnings, errors := im.AddImportFromSpecifier("react@19")
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
		if keys[0] != "react" || keys[1] != "react/" {
			t.Fatalf("Expected [react react/], got %v", keys)
		}
		if url, ok := im.Imports.Get("react"); !ok || !strings.HasPrefix(url, "https://next.esm.sh/react@19.") || !strings.HasSuffix(url, "/esnext/react.mjs") {
			t.Fatalf("Expected react to be resolved to https://next.esm.sh/react@19.x.x/esnext/react.mjs, got %s", url)
		}
	}
}

func TestScopeKeys(t *testing.T) {
	scopeKeys := ScopeKeys{
		"https://esm.sh/",
		"https://esm.sh/*react@18.3.1/",
		"https://esm.sh/*loose-envify@1.4.0/",
	}
	sort.Sort(scopeKeys)
	if scopeKeys[0] != "https://esm.sh/*react@18.3.1/" || scopeKeys[1] != "https://esm.sh/*loose-envify@1.4.0/" || scopeKeys[2] != "https://esm.sh/" {
		t.Fatalf("Expected [https://esm.sh/*react@18.3.1/ https://esm.sh/*loose-envify@1.4.0/ https://esm.sh/], got %v", scopeKeys)
	}
}

func TestResolve(t *testing.T) {
	im := ImportMap{}
	im.AddImportFromSpecifier("loose-envify@1.0.0")
	im.AddImportFromSpecifier("react@18")
	referrer, _ := url.Parse("file:///main.js")
	path, ok := im.Resolve("react", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/*react@18.3.1/es2022/react.mjs" {
		t.Fatalf("Expected path to be https://esm.sh/*react@18.3.1/es2022/react.mjs, got %s", path)
	}
	path, ok = im.Resolve("react/jsx-runtime", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/*react@18.3.1&target=es2022/jsx-runtime" {
		t.Fatalf("Expected path to be https://esm.sh/*react@18.3.1&target=es2022/jsx-runtime, got %s", path)
	}
	path, ok = im.Resolve("loose-envify", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/*loose-envify@1.0.0/es2022/loose-envify.mjs" {
		t.Fatalf("Expected path to be https://esm.sh/*loose-envify@1.0.0/es2022/loose-envify.mjs, got %s", path)
	}
	referrer, _ = url.Parse("https://esm.sh/*react@18.3.1/es2022/react.mjs")
	path, ok = im.Resolve("loose-envify", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/*loose-envify@1.4.0/es2022/loose-envify.mjs" {
		t.Fatalf("Expected path to be https://esm.sh/*loose-envify@1.4.0/es2022/loose-envify.mjs, got %s", path)
	}
	_, ok = im.Resolve("js-tokens", referrer)
	if ok {
		t.Fatalf("Expected ok to be false, got true")
	}
	referrer, _ = url.Parse("https://esm.sh/*loose-envify@1.0.0/es2022/loose-envify.mjs")
	path, ok = im.Resolve("js-tokens", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/js-tokens@1.0.3/es2022/js-tokens.mjs" {
		t.Fatalf("Expected path to be https://esm.sh/js-tokens@1.0.3/es2022/js-tokens.mjs, got %s", path)
	}
	referrer, _ = url.Parse("https://esm.sh/*loose-envify@1.4.0/es2022/loose-envify.mjs")
	path, ok = im.Resolve("js-tokens", referrer)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if path != "https://esm.sh/js-tokens@4.0.0/es2022/js-tokens.mjs" {
		t.Fatalf("Expected path to be https://esm.sh/js-tokens@4.0.0/es2022/js-tokens.mjs, got %s", path)
	}
}
