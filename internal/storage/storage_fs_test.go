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
	fs, err := NewFSStorage(&StorageOptions{Type: "fs", Endpoint: root})
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
