package importmap

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const indexHtml = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Hello, world!</title>
  <script type="importmap">
    {
	    "config": {
		    "cdn": "https://esm.sh"
	    },
      "imports": {
        "react": "https://esm.sh/react@19.1.0",
        "react/": "https://esm.sh/react@19.1.0/",
        "react-dom": "https://esm.sh/*react-dom@19.1.0",
        "react-dom/": "https://esm.sh/*react-dom@19.1.0/"
      },
      "scopes": {
        "https://esm.sh/": {
          "scheduler": "https://esm.sh/scheduler@0.26.0",
          "scheduler/": "https://esm.sh/scheduler@0.26.0/"
        }
      }
    }
  </script>
</head>
<body>
  <h1>Hello, world!</h1>
</body>
</html>
`

func TestParseFromHtmlFile(t *testing.T) {
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "index.html")
	err := os.WriteFile(htmlFile, []byte(indexHtml), 0644)
	if err != nil {
		t.Fatalf("Failed to write HTML file: %v", err)
	}
	importMap, err := ParseFromHtmlFile(htmlFile)
	if err != nil {
		t.Fatalf("Failed to parse import map: %v", err)
	}
	if importMap.Config.Cdn != "https://esm.sh" {
		t.Fatalf("Expected CDN 'https://esm.sh', got '%s'", importMap.Config.Cdn)
	}
	if len(importMap.Imports) != 4 {
		t.Fatalf("Expected 4 imports, got %d", len(importMap.Imports))
	}
	if len(importMap.Scopes) != 1 {
		t.Fatalf("Expected 1 scope, got %d", len(importMap.Scopes))
	}
	if len(importMap.Scopes["https://esm.sh/"]) != 2 {
		t.Fatalf("Expected 2 imports in scope, got %d", len(importMap.Scopes["https://esm.sh/"]))
	}
}

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
}

func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
