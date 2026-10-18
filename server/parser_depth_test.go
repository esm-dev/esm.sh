package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/gox/log"
)

func TestParserDepthLimit(t *testing.T) {
	if os.Getenv("ESM_TEST_PARSER_DEPTH") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestParserDepthLimit$", "-test.v")
		cmd.Env = append(os.Environ(), "ESM_TEST_PARSER_DEPTH=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("parser subprocess failed: %v\n%s", err, output)
		}
		return
	}

	// A regression must kill only this child, without allocating a 1 GB stack.
	debug.SetMaxStack(32 * MB)
	code := "export default " + strings.Repeat("(", 300000) + "0" + strings.Repeat(")", 300000)
	if len(code) > MB {
		t.Fatal("payload exceeds the transform size limit")
	}
	previousConfig, previousNpmRC, transport := config, defaultNpmRC, http.DefaultTransport
	testConfig := *config
	testConfig.WorkDir = t.TempDir()
	config, defaultNpmRC = &testConfig, nil
	t.Cleanup(func() {
		config, defaultNpmRC, http.DefaultTransport = previousConfig, previousNpmRC, transport
	})
	http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected request: %s", r.URL)
		return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
	})
	fs, err := storage.NewFSStorage(filepath.Join(config.WorkDir, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	handler := esmRouter(fs, logger)

	for _, lang := range []string{"js", "jsx", "ts", "tsx"} {
		t.Run("transform/"+lang, func(t *testing.T) {
			for _, source := range []string{code, "export default 42"} {
				body, err := json.Marshal(TransformOptions{Lang: lang, Code: source})
				if err != nil {
					t.Fatal(err)
				}
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, httptest.NewRequest("POST", "/transform", bytes.NewReader(body)))
				if source == code {
					if res.Code != 400 || !strings.Contains(res.Body.String(), "Source nesting is too deep") {
						t.Fatalf("expected nesting error, got HTTP %d: %s", res.Code, res.Body.String())
					}
				} else if res.Code != 200 {
					t.Fatalf("normal request failed after rejection: HTTP %d: %s", res.Code, res.Body.String())
				}
			}
		})
	}

	t.Run("minify", func(t *testing.T) {
		if _, err := minify(code, esbuild.LoaderJS, esbuild.ESNext); err == nil || !strings.Contains(err.Error(), "Source nesting is too deep") {
			t.Fatalf("expected nesting error, got %v", err)
		}
	})

	t.Run("export analysis", func(t *testing.T) {
		if _, err := parseJS("index.mjs", []byte(code)); err == nil {
			t.Fatal("expected export analysis to reject deep nesting")
		}
	})

	for _, entry := range []bool{true, false} {
		name := "parser-depth-entry"
		files := map[string]string{"index.mjs": code}
		if !entry {
			name = "parser-depth-import"
			files = map[string]string{
				"index.mjs":  `import value from "./nested.mjs"; export default value;`,
				"nested.mjs": code,
			}
		}
		t.Run(name, func(t *testing.T) {
			files["package.json"] = `{"name":"` + name + `","version":"1.0.0","type":"module","main":"index.mjs"}`
			dir := filepath.Join(config.WorkDir, "npm", name+"@1.0.0", "node_modules", name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			for filename, content := range files {
				if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/"+name+"@1.0.0?target=esnext&bundle&no-dts", nil))
			if res.Code != 500 || (!strings.Contains(res.Body.String(), "Source nesting is too deep") && !strings.Contains(res.Body.String(), "invalid syntax")) {
				t.Fatalf("expected build error, got HTTP %d: %s", res.Code, res.Body.String())
			}
		})
	}
}
