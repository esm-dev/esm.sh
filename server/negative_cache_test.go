package server

import (
	"io"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ije/gox/log"
)

func useNegativeCache(t *testing.T) {
	t.Helper()
	previous := negativeCache
	logger := new(log.Logger)
	logger.SetOutput(io.Discard)
	db, err := openNegativeCache(filepath.Join(t.TempDir(), "negative-cache.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	negativeCache = db
	t.Cleanup(func() {
		negativeCache.Close()
		negativeCache = previous
	})
}

func TestNegativeCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		useNegativeCache(t)
		negativeCache.put("gh-too-large:owner/repo@main", "repo is too large", 0)
		negativeCache.put("404:pkg@latest", "package not found", packageNotFoundTTL)
		negativeCache.put("404-path:pkg@1.0.0/raw:missing", "File Not Found", 0)
		filename, logger := negativeCache.Path(), negativeCache.logger
		if err := negativeCache.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		negativeCache, err = openNegativeCache(filename, logger)
		if err != nil {
			t.Fatal(err)
		}
		if negativeCache.get("gh-too-large:owner/repo@main") != "repo is too large" || negativeCache.get("404-path:pkg@1.0.0/raw:missing") != "File Not Found" {
			t.Fatal("permanent records did not survive reopening")
		}
		time.Sleep(packageNotFoundTTL - time.Millisecond)
		if negativeCache.get("404:pkg@latest") == "" {
			t.Fatal("package record expired too early")
		}
		time.Sleep(time.Millisecond)
		if negativeCache.get("404:pkg@latest") != "" {
			t.Fatal("package record survived its expiry")
		}
		negativeCache.gc(time.Now())
		keys, err := negativeCache.delete("404:", true)
		if err != nil || len(keys) != 0 {
			t.Fatalf("expired record was not removed: %v, %v", keys, err)
		}
		if negativeCache.get("gh-too-large:owner/repo@main") == "" || negativeCache.get("gh-too-large:owner/repo@other") != "" {
			t.Fatal("permanent ref record changed")
		}
	})
}
