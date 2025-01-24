package server

import (
	"fmt"
	"testing"
)

func TestLRUCache(t *testing.T) {
	for i := 0; i < 2000; i++ {
		withLRUCache(fmt.Sprintf("item-%d", i), func() ([]byte, error) {
			return []byte{byte(i % 256)}, nil
		})
	}
	if l := cacheLRU.Len(); l != 1000 {
		t.Fatalf("expected 1000 items in cache, got %d", l)
	}
}
