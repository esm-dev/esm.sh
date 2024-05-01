package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFS(t *testing.T) {
	root := filepath.Clean(os.TempDir())
	fs, err := OpenFS("local:" + root)
	if err != nil {
		t.Error(err)
		return
	}

	lfs, ok := fs.(*localFSLayer)
	if !ok {
		t.Fatal("not a local FS")
	}
	if lfs.root != root {
		t.Fatalf("invalid root '%s', should be '%s'", lfs.root, root)
	}

	n, err := fs.WriteFile("foo.txt", bytes.NewBufferString("bar"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("invalid written(%d), shoud be 3", n)
	}

	fi, err := fs.Stat("foo.txt")
	if err != nil {
		t.Fatal(err)
	}

	if fi.Size() != 3 {
		t.Fatalf("invalid file size(%d), shoud be 3", fi.Size())
	}

	f, err := fs.Open("foo.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "bar" {
		t.Fatalf("invalid file content('%s'), shoud be 'bar'", string(data))
	}

	err = fs.Remove("foo.txt")
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs.Stat("foo.txt")
	if err != ErrNotFound {
		t.Fatalf("File should be not existent")
	}

	_, err = fs.Open("foo.txt")
	if err != ErrNotFound {
		t.Fatalf("File should be not existent")
	}
}
