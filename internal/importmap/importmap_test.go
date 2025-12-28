package importmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFromHtmlFile(t *testing.T) {
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "index.html")
	err := os.WriteFile(htmlFile, []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Hello, world!</title>
  <script type="importmap">
    {
	    "$cdn": "https://esm.sh",
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
</html>`), 0644)
	if err != nil {
		t.Fatalf("Failed to write HTML file: %v", err)
	}
	importMap, err := ParseFromHtmlFile(htmlFile)
	if err != nil {
		t.Fatalf("Failed to parse import map: %v", err)
	}
	if importMap.Cdn != "https://esm.sh" {
		t.Fatalf("Expected CDN 'https://esm.sh', got '%s'", importMap.Cdn)
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

func TestAddPackage(t *testing.T) {

}
