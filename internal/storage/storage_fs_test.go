package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ije/gox/crypto/rand"
)

func TestFSStoragePutKeepsCompleteContent(t *testing.T) {
	fs, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = fs.Put("test.txt", bytes.NewBufferString("original")); err != nil {
		t.Fatal(err)
	}

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- fs.Put("test.txt", reader) }()
	if _, err = writer.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	keys, err := fs.List("")
	if err != nil || len(keys) != 1 || keys[0] != "test.txt" {
		t.Fatalf("List during write: keys %v, error %v", keys, err)
	}
	content, stat, err := fs.Get("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(content)
	content.Close()
	if err != nil || string(data) != "original" || stat.Size() != 8 {
		t.Fatalf("read during write: content %q, size %d, error %v", data, stat.Size(), err)
	}

	writeErr := errors.New("interrupted write")
	writer.CloseWithError(writeErr)
	if err = <-done; !errors.Is(err, writeErr) {
		t.Fatalf("Put returned %v, want %v", err, writeErr)
	}
	content, _, err = fs.Get("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(content)
	content.Close()
	if err != nil || string(data) != "original" {
		t.Fatalf("read after failed write: content %q, error %v", data, err)
	}
	keys, err = fs.List("")
	if err != nil || len(keys) != 1 || keys[0] != "test.txt" {
		t.Fatalf("List after failed write: keys %v, error %v", keys, err)
	}
	if err = fs.Put("test.txt", bytes.NewBufferString("replacement")); err != nil {
		t.Fatal(err)
	}
	content, stat, err = fs.Get("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(content)
	content.Close()
	if err != nil || string(data) != "replacement" || stat.Size() != 11 {
		t.Fatalf("read after replacement: content %q, size %d, error %v", data, stat.Size(), err)
	}
}

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
		if _, err = fs.List(k); err != ErrInvalidStorageKey {
			t.Fatalf("List(%q): want ErrInvalidStorageKey, got %v", k, err)
		}
		if _, err = fs.DeleteAll(k); err != ErrInvalidStorageKey {
			t.Fatalf("DeleteAll(%q): want ErrInvalidStorageKey, got %v", k, err)
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

func TestFSStorageRejectSymlinkEscape(t *testing.T) {
	for _, operation := range []string{"Put", "PutNewDir", "Get", "Stat", "Delete", "List", "DeleteAll"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.Mkdir(filepath.Join(outside, "nested"), 0755); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(outside, "nested", "file.txt")
			if err := os.WriteFile(victim, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
				t.Fatal(err)
			}
			fs, err := NewFSStorage(root)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "Put":
				err = fs.Put("link/nested/file.txt", strings.NewReader("replacement"))
			case "PutNewDir":
				err = fs.Put("link/new/file.txt", strings.NewReader("replacement"))
			case "Get":
				var content io.ReadCloser
				content, _, err = fs.Get("link/nested/file.txt")
				if content != nil {
					content.Close()
				}
			case "Stat":
				_, err = fs.Stat("link/nested/file.txt")
			case "Delete":
				err = fs.Delete("link/nested/file.txt")
			case "List":
				_, err = fs.List("link/nested")
			case "DeleteAll":
				_, err = fs.DeleteAll("link/nested")
			}
			if err == nil {
				t.Errorf("%s followed a symlink outside storage", operation)
			}
			if data, err := os.ReadFile(victim); err != nil || string(data) != "original" {
				t.Errorf("outside file changed: content %q, error %v", data, err)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 1 {
				t.Errorf("outside directory changed: entries %v, error %v", entries, err)
			}
		})
	}
}

func TestFSStorageContainedSymlink(t *testing.T) {
	root := t.TempDir()
	fs, err := NewFSStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("link/nested/file.txt", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	content, _, err := fs.Get("link/nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(content)
	content.Close()
	if err != nil || string(data) != "data" {
		t.Fatalf("Get: content %q, error %v", data, err)
	}
	if _, err := fs.Stat("link/nested/file.txt"); err != nil {
		t.Fatal(err)
	}
	keys, err := fs.DeleteAll("link/nested/")
	if err != nil || len(keys) != 1 || keys[0] != "link/nested/file.txt" {
		t.Fatalf("DeleteAll: keys %v, error %v", keys, err)
	}
	for _, prefix := range []string{"", "/", ".", "./", "real/.."} {
		if _, err := fs.DeleteAll(prefix); err == nil {
			t.Errorf("DeleteAll(%q) accepted the storage root", prefix)
		}
	}
}

func TestFSStoragePutSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	fs, err := NewFSStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- fs.Put("dir/file.txt", reader) }()
	if _, err := writer.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "dir"), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "moved"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file: entries %v, error %v", entries, err)
	}
	tmpfile := filepath.Join(outside, entries[0].Name())
	if err := os.WriteFile(tmpfile, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	if err := <-done; err == nil {
		t.Fatal("Put followed a swapped directory symlink")
	}
	if data, err := os.ReadFile(tmpfile); err != nil || string(data) != "outside" {
		t.Fatalf("outside temporary file changed: content %q, error %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside destination: %v", err)
	}
}

func TestFSStorageDirectoryIsNotFound(t *testing.T) {
	fs, err := NewFSStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("a/b.mjs/c.mjs", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat("a/b.mjs"); err != ErrNotFound {
		t.Fatalf("Stat(dir): %v", err)
	}
	if content, _, err := fs.Get("a/b.mjs"); err != ErrNotFound {
		if content != nil {
			content.Close()
		}
		t.Fatalf("Get(dir): %v", err)
	}
	if _, err := fs.Stat("a/b.mjs/c.mjs"); err != nil {
		t.Fatal(err)
	}
}
