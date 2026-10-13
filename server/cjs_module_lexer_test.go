package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallCjsModuleLexerRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cjs-module-lexer ships prebuilt binaries only for darwin/linux")
	}
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
