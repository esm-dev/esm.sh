package server

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ije/gox/log"
	bolt "go.etcd.io/bbolt"
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
		negativeCache.put("404-path:pkg@1.0.0/raw:missing", "File Not Found", 0)
		negativeCache.put("404:pkg@latest", "package not found", packageNotFoundTTL)
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
		time.Sleep(negativeCacheMaxTTL)
		negativeCache.gc(time.Now())
		if keys, err := negativeCache.delete("", true); err != nil || len(keys) != 0 {
			t.Fatalf("ttl 0 records outlived the max ttl: %v, %v", keys, err)
		}
	})
}

func TestNegativeCacheLongKey(t *testing.T) {
	useNegativeCache(t)
	key := "404-path:react@18.2.0/raw:" + strings.Repeat("a/", 4096) + "x.png?raw"
	negativeCache.put(key, "not found", time.Minute)
	if negativeCache.get(key) != "not found" {
		t.Fatal("long key was not cached")
	}
	if err := negativeCache.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("records")).ForEach(func(k, _ []byte) error {
			if len(k) > 512 {
				t.Fatalf("stored key has %d bytes", len(k))
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if keys, err := negativeCache.delete("404-path:react@18.2.0/", true); err != nil || len(keys) != 1 {
		t.Fatalf("purge by prefix deleted %v: %v", keys, err)
	}
}
