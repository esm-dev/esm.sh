package storage

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestS3Storage(t *testing.T) {
	endpint := os.Getenv("GO_TEST_S3_ENDPOINT")
	if endpint == "" {
		t.Skip("env GO_TEST_S3_ENDPOINT not set")
	}
	s3, err := NewS3Storage(&StorageOptions{
		Type:            "s3",
		Endpoint:        endpint,
		Region:          os.Getenv("GO_TEST_S3_REGION"),
		AccessKeyID:     os.Getenv("GO_TEST_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("GO_TEST_S3_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		t.Fatal(err)
	}

	dirname := os.Getenv("GO_TEST_S3_ROOTDIR")
	if dirname == "" {
		dirname = "test"
	}

	// clean up
	_, err = s3.DeleteAll(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}

	err = s3.Put(dirname+"/hello.txt", bytes.NewReader([]byte("Hello, world!")))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put(dirname+"/foo/bar.txt", bytes.NewBufferString("foobar~"))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put(dirname+"/%23/hello+world!", TeeReader(bytes.NewReader([]byte("Hello, world!")), io.Discard))
	if err != nil {
		t.Fatal(err)
	}

	keys, err := s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("invalid keys length(%d), expected 3", len(keys))
	}

	stat, err := s3.Stat(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 13 {
		t.Fatalf("invalid size(%d), expected 13", stat.Size())
	}

	r, stat, err := s3.Get(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if stat.Size() != 13 {
		t.Fatalf("invalid size(%d), expected 13", stat.Size())
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Hello, world!" {
		t.Fatalf("invalid content(%s), expected 'Hello, world!'", string(data))
	}

	stat, err = s3.Stat(dirname + "/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 7 {
		t.Fatalf("invalid size(%d), expected 7", stat.Size())
	}

	r, stat, err = s3.Get(dirname + "/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if stat.Size() != 7 {
		t.Fatalf("invalid size(%d), expected 7", stat.Size())
	}

	data, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foobar~" {
		t.Fatalf("invalid content(%s), expected 'foobar~'", string(data))
	}

	err = s3.Delete(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Delete(dirname + "/%23/hello+world!")
	if err != nil {
		t.Fatal(err)
	}

	keys, err = s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("invalid keys length(%d), expected 1", len(keys))
	}

	deleted, err := s3.DeleteAll(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Fatalf("invalid deleted keys length(%d), expected 1", len(deleted))
	}

	keys, err = s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("invalid keys length(%d), expected 0", len(keys))
	}
}
