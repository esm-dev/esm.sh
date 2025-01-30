package server

import (
	"fmt"
	"testing"
	"time"
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
	for i := 0; i < 100; i++ {
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
