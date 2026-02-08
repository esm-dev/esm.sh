package importmap

import (
	"net/url"
	"sort"
	"strings"
	"testing"
)

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
	im.AddImportFromSpecifier("react-dom@19/client", false)
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
