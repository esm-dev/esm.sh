package deno

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateDenoVersionRequiresExactVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a shell script")
	}
	denoPath := filepath.Join(t.TempDir(), "deno")
	if err := os.WriteFile(denoPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$FAKE_DENO_VERSION\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DENO_VERSION", version)
	if err := validateDenoVersion(denoPath, version); err != nil {
		t.Fatal(err)
	}
	if err := validateDenoVersion(denoPath, "2.6.9"); err == nil {
		t.Fatal("accepted a Deno version other than the requested pin")
	}
}
