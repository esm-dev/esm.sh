package server

import (
	"fmt"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

func getCacheSize() int {
	size := 0
	cacheStore.Range(func(key, value any) bool {
		if item, ok := value.(*cacheItem); ok && item.exp >= 0 {
			size++
		}
		return true
	})
	return size
}

func TestCache(t *testing.T) {
	for i := range 100 {
		withCache(fmt.Sprintf("^0.0.%d", i), 10*time.Millisecond, func() ([]byte, string, error) {
			return []byte{byte(i)}, fmt.Sprintf("0.0.%d", i), nil
		})
	}

	size := getCacheSize()
	if size != 200 {
		t.Fatalf("expected 200 items in cache, got %d", size)
	}

	gc(time.Now())
	size = getCacheSize()
	if size != 200 {
		t.Fatalf("expected 200 items in cache, got %d", size)
	}

	time.Sleep(100 * time.Millisecond)
	gc(time.Now())
	size = getCacheSize()
	if size != 0 {
		t.Fatalf("expected 0 items in cache, got %d", size)
	}
}

func TestLRUCache(t *testing.T) {
	cacheLRU, _ = lru.New[string, any](1000)

	for i := range 2000 {
		withLRUCache(fmt.Sprintf("item-%d", i), func() ([]byte, error) {
			return []byte{byte(i % 256)}, nil
		})
	}
	if l := cacheLRU.Len(); l != 1000 {
		t.Fatalf("expected 1000 items in cache, got %d", l)
	}

	// the `gc` function does not remove items from the LRU cache
	gc(time.Now())
	if l := cacheLRU.Len(); l != 1000 {
		t.Fatalf("expected 1000 items in cache, got %d", l)
	}
}
