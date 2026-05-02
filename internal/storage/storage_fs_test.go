package storage

import (
	"bytes"
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
