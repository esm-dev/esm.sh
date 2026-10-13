package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/log"
)

func TestInstallCjsModuleLexerRetry(t *testing.T) {
	oldClient, oldConfig := http.DefaultClient, config
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { http.DefaultClient, config = oldClient, oldConfig })
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	const binary = "lexer executable"
	if _, err := io.WriteString(w, binary); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	requests := 0
	http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		data := compressed.Bytes()
		if requests == 1 {
			data = data[:len(data)-3]
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	filename := filepath.Join(config.WorkDir, "bin", "cjs-module-lexer-"+cjsModuleLexerVersion)
	if err := installCjsModuleLexerContext(context.Background()); err == nil {
		t.Fatal("expected truncated download to fail")
	}
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatalf("failed download left an executable: %v", err)
	}
	if err := installCjsModuleLexerContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || string(data) != binary {
		t.Fatalf("retry did not install the binary: requests=%d, contents=%q", requests, data)
	}
	info, err := os.Stat(filename)
	if err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("installed binary is not executable: %v", err)
	}
}

func TestInstallCjsModuleLexerDev(t *testing.T) {
	if !DEBUG {
		t.Skip("requires debug build")
	}
	root := t.TempDir()
	projectDir := filepath.Join(root, "esm.sh")
	nativeDir := filepath.Join(root, "cjs-module-lexer", "target", "release")
	for _, dir := range []string{projectDir, nativeDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(projectDir)
	if err := os.WriteFile(filepath.Join(nativeDir, "native"), []byte("dev lexer"), 0755); err != nil {
		t.Fatal(err)
	}
	oldConfig, oldVersion := config, cjsModuleLexerVersion
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { config, cjsModuleLexerVersion = oldConfig, oldVersion })
	if err := installCjsModuleLexerContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(config.WorkDir, "bin", "cjs-module-lexer-"+cjsModuleLexerVersion))
	if err != nil || string(data) != "dev lexer" {
		t.Fatalf("dev lexer is not at the execution path: %q, %v", data, err)
	}
}

func TestCjsModuleLexerReexportRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a shell script")
	}
	oldConfig := config
	once, hadOnce := onceMap.Load("install-cjs-module-lexer")
	t.Cleanup(func() {
		config = oldConfig
		if hadOnce {
			onceMap.Store("install-cjs-module-lexer", once)
		} else {
			onceMap.Delete("install-cjs-module-lexer")
		}
	})
	t.Chdir(t.TempDir())
	const missing = `failed to resolve reexport: NotFound("@slack/types")`
	const header = "thread 'main' (1234) panicked at native/src/main.rs:90:65:"
	for _, tt := range []struct {
		name       string
		stderr     string
		alwaysFail bool
		wantRuns   int
		wantError  string
	}{
		{name: "legacy", stderr: "thread 'main' panicked at native/src/main.rs:71:62:\n" + missing, wantRuns: 2},
		{name: "leading newline", stderr: "\nthread 'main' panicked at native/src/main.rs:71:62:\n" + missing, wantRuns: 2},
		{name: "process ID", stderr: "\n" + header + "\n" + missing + "\nnote: run with RUST_BACKTRACE=1\n", wantRuns: 2},
		{name: "retry fails", stderr: "\n" + header + "\n" + missing, alwaysFail: true, wantRuns: 2, wantError: missing},
		{name: "unrelated panic", stderr: "\n" + header + "\nunexpected token\n" + missing, wantRuns: 1, wantError: "unexpected token"},
		{name: "incomplete panic", stderr: "thread 'main' panicked at native/src/main.rs:71:62:", wantRuns: 1, wantError: "thread 'main' panicked at native/src/main.rs:71:62:"},
		{name: "plain error", stderr: "cannot read source", wantRuns: 1, wantError: "cannot read source"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config = &Config{WorkDir: t.TempDir()}
			wd := t.TempDir()
			npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
			depDir := filepath.Join(npmrc.StoreDir(), "@slack/types@1.0.0", "node_modules", "@slack/types")
			for _, dir := range []string{filepath.Join(config.WorkDir, "bin"), filepath.Join(wd, "node_modules"), depDir} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			for filename, data := range map[string]string{
				filepath.Join(depDir, "package.json"): `{"name":"@slack/types","version":"1.0.0"}`,
				filepath.Join(wd, "stderr"):           tt.stderr,
			} {
				if err := os.WriteFile(filename, []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.alwaysFail {
				if err := os.WriteFile(filepath.Join(wd, "fail"), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			const script = `#!/bin/sh
printf 'run\n' >> runs
if [ -f node_modules/@slack/types/package.json ] && [ ! -f fail ]; then
  printf 'WebClient\n'
  exit 0
fi
cat stderr >&2
exit 1
`
			if err := os.WriteFile(filepath.Join(config.WorkDir, "bin", "cjs-module-lexer-"+cjsModuleLexerVersion), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			logger, err := log.New("")
			if err != nil {
				t.Fatal(err)
			}
			logger.SetOutput(io.Discard)
			ctx := &BuildContext{
				wd: wd, npmrc: npmrc, logger: logger,
				esmPath: EsmPath{PkgName: "example", PkgVersion: "1.0.0"},
				pkgJson: &npm.PackageJSON{Name: "example", Version: "1.0.0", Dependencies: map[string]string{"@slack/types": "1.0.0"}},
			}
			result, err := cjsModuleLexer(ctx, "./index.js")
			if tt.wantError != "" {
				if err == nil || err.Error() != "cjsModuleLexer: "+tt.wantError {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
			} else if err != nil || !slices.Equal(result.Exports, []string{"WebClient"}) {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			runs, err := os.ReadFile(filepath.Join(wd, "runs"))
			if err != nil || strings.Count(string(runs), "run\n") != tt.wantRuns {
				t.Fatalf("runs = %q, error = %v, want %d runs", runs, err, tt.wantRuns)
			}
			if installed := existsFile(filepath.Join(wd, "node_modules", "@slack/types", "package.json")); installed != (tt.wantRuns == 2) {
				t.Fatalf("dependency installed = %v, want %v", installed, tt.wantRuns == 2)
			}
		})
	}
}

func TestInstallCjsModuleLexerOffline(t *testing.T) {
	oldClient, oldConfig := http.DefaultClient, config
	config = &Config{WorkDir: t.TempDir()}
	t.Cleanup(func() { http.DefaultClient, config = oldClient, oldConfig })
	blocked := errors.New("GitHub is blocked")
	requests := 0
	http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, blocked
	})}
	if err := installCjsModuleLexerContext(context.Background()); !errors.Is(err, blocked) || requests != 1 {
		t.Fatalf("missing lexer did not require GitHub: requests=%d, error=%v", requests, err)
	}
	binDir := filepath.Join(config.WorkDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(binDir, "cjs-module-lexer-"+cjsModuleLexerVersion)
	if err := os.WriteFile(filename, []byte("preinstalled lexer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installCjsModuleLexerContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("preinstalled lexer required GitHub: requests=%d", requests)
	}
}
