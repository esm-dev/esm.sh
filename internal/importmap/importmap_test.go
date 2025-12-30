package importmap

import (
	"net/url"
	"sort"
	"testing"
)

func TestAddPackages(t *testing.T) {
	// 1. add packages
	{
		im := ImportMap{}
		updated := im.AddPackages([]string{"react@19", "react-dom@19"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 4 {
			t.Fatalf("Expected 4 imports, got %d", len(im.Imports))
		}
		keys := getKeys(im.Imports)
		if keys[0] != "react" || keys[1] != "react-dom" || keys[2] != "react-dom/" || keys[3] != "react/" {
			t.Fatalf("Expected [react react-dom react-dom/ react/], got %v", keys)
		}
		if len(im.Scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.Scopes))
		}
		scope := im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "scheduler" || keys[1] != "scheduler/" {
			t.Fatalf("Expected [scheduler scheduler/], got %v", keys)
		}
	}

	// 2. add peer dependencies to `imports`
	{
		im := ImportMap{}
		updated := im.AddPackages([]string{"react-dom@19"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 4 {
			t.Fatalf("Expected 4 imports, got %d", len(im.Imports))
		}
		keys := getKeys(im.Imports)
		if keys[0] != "react" || keys[1] != "react-dom" || keys[2] != "react-dom/" || keys[3] != "react/" {
			t.Fatalf("Expected [react react-dom react-dom/ react/], got %v", keys)
		}
		if len(im.Scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.Scopes))
		}
		scope := im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "scheduler" || keys[1] != "scheduler/" {
			t.Fatalf("Expected [scheduler scheduler/], got %v", keys)
		}
	}

	// 3. resolve dependencies without conflicts
	{
		im := ImportMap{}
		updated := im.AddPackages([]string{"loose-envify@1.1.0"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 2 {
			t.Fatalf("Expected 2 imports, got %d", len(im.Imports))
		}
		keys := make([]string, 0, len(im.Imports))
		for k := range im.Imports {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		if len(im.Scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.Scopes))
		}
		scope := im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}

		updated = im.AddPackages([]string{"react@18"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 4 {
			t.Fatalf("Expected 4 imports, got %d", len(im.Imports))
		}
		keys = getKeys(im.Imports)
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" || keys[2] != "react" || keys[3] != "react/" {
			t.Fatalf("Expected [loose-envify loose-envify/ react react/], got %v", keys)
		}
		if im.Imports["loose-envify"] != "https://esm.sh/*loose-envify@1.1.0/es2022/loose-envify.mjs" {
			t.Fatalf("Expected loose-envify to be resolved to loose-envify@1.1.0, got %s", im.Imports["loose-envify"])
		}
		if len(im.Scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.Scopes))
		}
		scope = im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
	}

	// 4. resolve dependencies with conflicts
	{
		im := ImportMap{}
		updated := im.AddPackages([]string{"loose-envify@1.0.0"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 2 {
			t.Fatalf("Expected 2 imports, got %d", len(im.Imports))
		}
		keys := make([]string, 0, len(im.Imports))
		for k := range im.Imports {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		if len(im.Scopes) != 1 {
			t.Fatalf("Expected 1 scope, got %d", len(im.Scopes))
		}
		scope := im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}

		updated = im.AddPackages([]string{"react@18"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 4 {
			t.Fatalf("Expected 4 imports, got %d", len(im.Imports))
		}
		keys = getKeys(im.Imports)
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" || keys[2] != "react" || keys[3] != "react/" {
			t.Fatalf("Expected [loose-envify loose-envify/ react react/], got %v", keys)
		}
		if im.Imports["loose-envify"] != "https://esm.sh/*loose-envify@1.0.0/es2022/loose-envify.mjs" {
			t.Fatalf("Expected loose-envify to be resolved to loose-envify@1.0.0, got %s", im.Imports["loose-envify"])
		}
		if len(im.Scopes) != 3 {
			t.Fatalf("Expected 3 scopes, got %d", len(im.Scopes))
		}
		scope = im.Scopes["https://esm.sh/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
		if scope["js-tokens"] != "https://esm.sh/js-tokens@1.0.3/es2022/js-tokens.mjs" {
			t.Fatalf("Expected js-tokens to be resolved to js-tokens@1.0.3, got %s", scope["js-tokens"])
		}
		scope = im.Scopes["https://esm.sh/*react@18.3.1/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "loose-envify" || keys[1] != "loose-envify/" {
			t.Fatalf("Expected [loose-envify loose-envify/], got %v", keys)
		}
		scope = im.Scopes["https://esm.sh/*loose-envify@1.4.0/"]
		if len(scope) != 2 {
			t.Fatalf("Expected 2 imports in scope, got %d", len(scope))
		}
		keys = getKeys(scope)
		if keys[0] != "js-tokens" || keys[1] != "js-tokens/" {
			t.Fatalf("Expected [js-tokens js-tokens/], got %v", keys)
		}
		if scope["js-tokens"] != "https://esm.sh/js-tokens@4.0.0/es2022/js-tokens.mjs" {
			t.Fatalf("Expected js-tokens to be resolved to js-tokens@4.0.0, got %s", scope["js-tokens"])
		}
	}

	// 4. with config
	{
		im := ImportMap{
			Config: Config{
				Cdn:       "https://next.esm.sh",
				Target:    "esnext",
				Integrity: true,
			},
		}
		updated := im.AddPackages([]string{"react@19"})
		if !updated {
			t.Fatalf("Expected updated to be true, got false")
		}
		if len(im.Imports) != 2 {
			t.Fatalf("Expected 2 imports, got %d", len(im.Imports))
		}
		keys := getKeys(im.Imports)
		if keys[0] != "react" || keys[1] != "react/" {
			t.Fatalf("Expected [react react/], got %v", keys)
		}
		if im.Imports["react"] != "https://next.esm.sh/react@19.2.3/esnext/react.mjs" {
			t.Fatalf("Expected react to be resolved to https://next.esm.sh/react@19.2.3/esnext/react.mjs, got %s", im.Imports["react"])
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
	im.AddPackages([]string{"loose-envify@1.0.0"})
	im.AddPackages([]string{"react@18"})
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

func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
