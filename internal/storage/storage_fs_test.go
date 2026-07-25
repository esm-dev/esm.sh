package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"testing"

	"github.com/ije/gox/crypto/rand"
)

func TestFSStorage(t *testing.T) {
	root := path.Join(os.TempDir(), "storage_test_"+rand.Hex.String(8))
	fs, err := NewFSStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	err = fs.Put("test.txt", bytes.NewBufferString("Hello World!"))
	if err != nil {
		t.Fatal(err)
	}

	err = fs.Put("hello/world.txt", bytes.NewBufferString("Hello World!"))
	if err != nil {
		t.Fatal(err)
	}

	fi, err := fs.Stat("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if fi.Size() != 12 {
		t.Fatalf("invalid file size(%d), shoud be 12", fi.Size())
	}

	f, fi, err := fs.Get("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if fi.Size() != 12 {
		t.Fatalf("invalid file size(%d), shoud be 12", fi.Size())
	}

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "Hello World!" {
		t.Fatalf("invalid file content('%s'), shoud be 'Hello World!'", string(data))
	}

	keys, err := fs.List("")
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 2 {
		t.Fatalf("invalid keys count(%d), shoud be 2", len(keys))
	}

	keys, err = fs.List("hello/")
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Fatalf("invalid keys count(%d), shoud be 1", len(keys))
	}

	if keys[0] != "hello/world.txt" {
		t.Fatalf("invalid key('%s'), shoud be 'hello/world.txt'", keys[0])
	}

	err = fs.Delete("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs.Stat("test.txt")
	if err != ErrNotFound {
		t.Fatalf("File should be not existent")
	}

	_, _, err = fs.Get("test.txt")
	if err != ErrNotFound {
		t.Fatalf("File should be not existent")
	}

	deletedKeys, err := fs.DeleteAll("hello/")
	if err != nil {
		t.Fatal(err)
	}

	if len(deletedKeys) != 1 {
		t.Fatalf("invalid deleted keys count(%d), shoud be 1", len(deletedKeys))
	}

	if deletedKeys[0] != "hello/world.txt" {
		t.Fatalf("invalid deleted key('%s'), shoud be 'hello/world.txt'", deletedKeys[0])
	}

	keys, err = fs.List("")
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 0 {
		t.Fatalf("invalid keys count(%d), shoud be 0", len(keys))
	}
}

func TestFSStoragePutIfAbsent(t *testing.T) {
	fs, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	created, err := fs.PutIfAbsent("locked/module.mjs", bytes.NewBufferString("first"))
	if err != nil || !created {
		t.Fatalf("first PutIfAbsent: created %v, err %v", created, err)
	}
	created, err = fs.PutIfAbsent("locked/module.mjs", bytes.NewBufferString("second"))
	if err != nil || created {
		t.Fatalf("second PutIfAbsent: created %v, err %v", created, err)
	}

	r, _, err := fs.Get("locked/module.mjs")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil || closeErr != nil || string(data) != "first" {
		t.Fatalf("stored content %q, read err %v, close err %v", data, readErr, closeErr)
	}

	created, err = fs.PutIfAbsent("locked/empty.mjs", bytes.NewReader(nil))
	if err != nil || !created {
		t.Fatalf("empty PutIfAbsent: created %v, err %v", created, err)
	}
	created, err = fs.PutIfAbsent("locked/empty.mjs", bytes.NewBufferString("replacement"))
	if err != nil || created {
		t.Fatalf("empty replacement: created %v, err %v", created, err)
	}
	fi, err := fs.Stat("locked/empty.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 0 {
		t.Fatalf("empty stored size %v", fi.Size())
	}

	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 32)
	for i := range 32 {
		go func() {
			created, err := fs.PutIfAbsent("locked/concurrent.mjs", bytes.NewReader([]byte{byte(i)}))
			results <- result{created, err}
		}()
	}
	createdCount := 0
	for range 32 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent PutIfAbsent created %d files", createdCount)
	}
	fi, err = fs.Stat("locked/concurrent.mjs")
	if err != nil || fi.Size() != 1 {
		t.Fatalf("concurrent stored size is not one byte: %v", err)
	}

	if err = fs.Put("locked/atomic.meta", bytes.NewBufferString("first")); err != nil {
		t.Fatal(err)
	}
	if err = fs.Put("locked/atomic.meta", failingReader{}); err == nil {
		t.Fatal("expected failed overwrite")
	}
	r, _, err = fs.Get("locked/atomic.meta")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr = io.ReadAll(r)
	r.Close()
	if readErr != nil || string(data) != "first" {
		t.Fatalf("failed Put changed existing content: %q, err %v", data, readErr)
	}

	if err = os.WriteFile(path.Join(fs.(*fsStorage).root, ".tmp-orphan"), []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}
	keys, err := fs.List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if key == ".tmp-orphan" {
			t.Fatal("temporary storage file was listed")
		}
	}
}

func TestFSStorageRejectPathTraversal(t *testing.T) {
	root := path.Join(os.TempDir(), "storage_traversal_"+rand.Hex.String(8))
	fs, err := NewFSStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	attackKeys := []string{
		"../outside.txt",
		"legacy/../../../tmp/pwned",
		`legacy/v111/react@19.2.0/esnext/../../../gh/a/exp@cafe/foo.md#/../../../../../../../../../../tmp/pwned`,
		"safe/../../etc/passwd",
		"bad\x00surprise",
	}
	for _, k := range attackKeys {
		err = fs.Put(k, bytes.NewBufferString("evil"))
		if err != ErrInvalidStorageKey {
			t.Fatalf("Put(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
		if _, err = fs.Stat(k); err != ErrInvalidStorageKey {
			t.Fatalf("Stat(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
		if _, _, err = fs.Get(k); err != ErrInvalidStorageKey {
			t.Fatalf("Get(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
		if err = fs.Delete(k); err != ErrInvalidStorageKey {
			t.Fatalf("Delete(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
		if _, err = fs.PutIfAbsent(k, bytes.NewBufferString("evil")); err != ErrInvalidStorageKey {
			t.Fatalf("PutIfAbsent(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
	}

	err = fs.Put("ok/sub/file.txt", bytes.NewBufferString("hi"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := fs.Get("ok/sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	b, err := io.ReadAll(got)
	if err != nil || string(b) != "hi" {
		t.Fatalf("Get ok path: content %q err %v", string(b), err)
	}
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return copy(p, "partial"), errors.New("read failed")
}
