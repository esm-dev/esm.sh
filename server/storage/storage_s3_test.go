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
		Endpint:         endpint,
		Region:          os.Getenv("GO_TEST_S3_REGION"),
		AccessKeyID:     os.Getenv("GO_TEST_S3_ACCESS_KEYI_ID"),
		SecretAccessKey: os.Getenv("GO_TEST_S3_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// clear all files in 'test/' directory
	_, err = s3.DeleteAll("test/")
	if err != nil {
		t.Fatal(err)
	}

	err = s3.Put("test/hello.txt", bytes.NewBufferString("Hello, world!"))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put("test/foo/bar.txt", bytes.NewBufferString("abcdefghijklmnopqrstuvwxyz!"))
	if err != nil {
		t.Fatal(err)
	}

	keys, err := s3.List("test/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("invalid keys length(%d), expected 2", len(keys))
	}

	stat, err := s3.Stat("test/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 13 {
		t.Fatalf("invalid size(%d), expected 13", stat.Size())
	}

	r, err := s3.Get("test/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Hello, world!" {
		t.Fatalf("invalid content(%s), expected 'Hello, world!'", string(data))
	}

	stat, err = s3.Stat("test/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 27 {
		t.Fatalf("invalid size(%d), expected 27", stat.Size())
	}

	r, err = s3.Get("test/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdefghijklmnopqrstuvwxyz!" {
		t.Fatalf("invalid content(%s), expected 'abcdefghijklmnopqrstuvwxyz!'", string(data))
	}

	err = s3.Delete("test/hello.txt")
	if err != nil {
		t.Fatal(err)
	}

	keys, err = s3.List("test/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("invalid keys length(%d), expected 1", len(keys))
	}

	deleted, err := s3.DeleteAll("test/")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Fatalf("invalid deleted keys length(%d), expected 1", len(deleted))
	}

	keys, err = s3.List("test/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("invalid keys length(%d), expected 0", len(keys))
	}
}
