package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

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
