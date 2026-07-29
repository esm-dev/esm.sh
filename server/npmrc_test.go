package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestSameURLOrigin(t *testing.T) {
	registryUrl, _ := url.Parse("https://registry.example/package")
	for _, test := range []struct {
		url  string
		want bool
	}{
		{"https://registry.example/tarball.tgz", true},
		{"https://REGISTRY.EXAMPLE:443/tarball.tgz", true},
		{"http://registry.example/tarball.tgz", false},
		{"https://registry.example:444/tarball.tgz", false},
		{"https://tarballs.example/tarball.tgz", false},
	} {
		tarballUrl, _ := url.Parse(test.url)
		if got := sameURLOrigin(registryUrl, tarballUrl); got != test.want {
			t.Errorf("sameURLOrigin(%q, %q) = %v, want %v", registryUrl, tarballUrl, got, test.want)
		}
	}
}

func TestFetchPackageTarballAuthorization(t *testing.T) {
	var tarball bytes.Buffer
	gw := gzip.NewWriter(&tarball)
	tw := tar.NewWriter(gw)
	content := []byte(`{"name":"test-package","version":"1.0.0"}`)
	if err := tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	authorization := make(chan string, 1)
	tarballServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write(tarball.Bytes())
	}))
	defer tarballServer.Close()

	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))
	for _, test := range []struct {
		name     string
		registry NpmRegistryConfig
		want     string
	}{
		{"same-origin bearer token", NpmRegistryConfig{Registry: tarballServer.URL + "/", Token: "secret"}, "Bearer secret"},
		{"cross-origin bearer token", NpmRegistryConfig{Registry: "https://registry.example/", Token: "secret"}, ""},
		{"same-origin basic auth", NpmRegistryConfig{Registry: tarballServer.URL + "/", User: "user", Password: "password"}, basicAuth},
		{"cross-origin basic auth", NpmRegistryConfig{Registry: "https://registry.example/", User: "user", Password: "password"}, ""},
		{"backup registry", NpmRegistryConfig{Registry: "https://registry.example/", BackupRegistry: tarballServer.URL + "/", Token: "secret"}, "Bearer secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := &NpmRegistry{NpmRegistryConfig: test.registry}
			if err := fetchPackageTarballContext(context.Background(), reg, t.TempDir(), "test-package", tarballServer.URL+"/test-package.tgz"); err != nil {
				t.Fatal(err)
			}
			if got := <-authorization; got != test.want {
				t.Fatalf("expected Authorization header %q, got %q", test.want, got)
			}
		})
	}

	redirectAuthorization := make(chan string, 1)
	crossOriginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectAuthorization <- r.Header.Get("Authorization")
		_, _ = w.Write(tarball.Bytes())
	}))
	defer crossOriginServer.Close()
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, crossOriginServer.URL+"/test-package.tgz", http.StatusFound)
	}))
	defer registryServer.Close()

	reg := &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: registryServer.URL + "/", Token: "secret"}}
	if err := fetchPackageTarballContext(context.Background(), reg, t.TempDir(), "test-package", registryServer.URL+"/test-package.tgz"); err != nil {
		t.Fatal(err)
	}
	if got := <-redirectAuthorization; got != "" {
		t.Fatalf("expected redirect to strip Authorization header, got %q", got)
	}
}

func TestExtractPackageTarball(t *testing.T) {
	b := make([]byte, 16)
	rand.Read(b)
	installDir := filepath.Join(os.TempDir(), hex.EncodeToString(b))
	defer os.RemoveAll(installDir)

	// Create a malicious tarball with path traversal
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a normal file
	content := []byte("export const foo = 'bar';")
	header := &tar.Header{
		Name:     "package/index.js",
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	// Add a large file
	largeContent := make([]byte, 1024*1024*51)
	rand.Read(largeContent)
	header = &tar.Header{
		Name:     "package/large.txt",
		Mode:     0644,
		Size:     int64(len(largeContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(largeContent); err != nil {
		t.Fatal(err)
	}

	// add a link
	header = &tar.Header{
		Name:     "package/passwd.txt",
		Mode:     0644,
		Typeflag: tar.TypeLink,
		Linkname: "/etc/passwd",
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}

	// Add a malicious file with path traversal
	bad := []byte("bad")
	header = &tar.Header{
		Name:     "/../../../bad/bad.txt",
		Mode:     0644,
		Size:     int64(len(bad)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(bad); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()

	// Call extractPackageTarball with the malicious tarball
	if err := extractPackageTarball(installDir, "test-package", bytes.NewReader(buf.Bytes())); err != nil {
		t.Errorf("extractPackageTarball returned error: %v", err)
	}
	if !existsFile(filepath.Join(installDir, "node_modules", "test-package", "index.js")) {
		t.Fatal("index.js should be extracted")
	}
	if existsFile(filepath.Join(installDir, "node_modules", "test-package", "large.txt")) {
		t.Fatal("large.txt should not be extracted")
	}
	if existsFile(filepath.Join(installDir, "node_modules", "test-package", "passwd.txt")) {
		t.Fatal("passwd.txt should not be extracted")
	}
	if !existsFile(filepath.Join(installDir, "node_modules", "test-package", "bad.txt")) {
		t.Fatal("bad.txt should be extracted in the root directory")
	}
}
