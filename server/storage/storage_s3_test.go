package storage

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestS3Storage(t *testing.T) {
	os.Setenv("GO_TEST_S3_ENDPOINT", "https://d5197bc43c609ab3101c8fc931edb5e7.r2.cloudflarestorage.com/esm-dev")
	os.Setenv("GO_TEST_S3_ACCESS_KEY_ID", "3216f30c76c54bbf7bbc5ee7c7b1353f")
	os.Setenv("GO_TEST_S3_SECRET_ACCESS_KEY", "d398da3375f93b467adc913ffc8394acc7126e3112434c9a286a208f8d430810")

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

	// clear all files in 'test/' directory
	_, err = s3.DeleteAll("test/")
	if err != nil {
		t.Fatal(err)
	}

	err = s3.Put("test/hello.txt", bytes.NewReader([]byte("Hello, world!")))
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Put("test/foo/bar.txt", bytes.NewReader([]byte("abcdefghijklmnopqrstuvwxyz!")))
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

	r, stat, err := s3.Get("test/hello.txt")
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

	stat, err = s3.Stat("test/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 27 {
		t.Fatalf("invalid size(%d), expected 27", stat.Size())
	}

	r, stat, err = s3.Get("test/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if stat.Size() != 27 {
		t.Fatalf("invalid size(%d), expected 27", stat.Size())
	}

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
