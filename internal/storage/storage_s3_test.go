package storage

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestS3StorageFSCacheKey(t *testing.T) {
	s3 := &s3Storage{}
	hash := "3fdadb34f247fde94adfb18268ac0caeed539ae7ad1380035d1366943b85ca7a"
	got := s3.fsCacheKey("meta/" + hash)
	want := "meta/3f/dadb34f247fde94adfb18268ac0caeed539ae7ad1380035d1366943b85ca7a"
	if got != want {
		t.Fatalf("invalid cache key %q, want %q", got, want)
	}
	if got = s3.fsCacheKey("v135/react@19.2.0/esnext/react.mjs"); got != "v135/react@19.2.0/esnext/react.mjs" {
		t.Fatalf("invalid cache key %q", got)
	}
}

func TestS3StoragePutIfAbsent(t *testing.T) {
	stored := map[string][]byte{}
	conflict := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "*" {
			t.Errorf("missing If-None-Match header")
		}
		if r.URL.Path == "/retry.mjs" && conflict {
			conflict = false
			w.WriteHeader(http.StatusConflict)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, ok := stored[r.URL.Path]; ok {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		stored[r.URL.Path] = data
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s3, err := NewS3Storage(&StorageOptions{
		Type:            "s3",
		Endpoint:        server.URL,
		Region:          "auto",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		CacheDir:        t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s3.PutIfAbsent("module.mjs", bytes.NewBufferString("first"))
	if err != nil || !created {
		t.Fatalf("first PutIfAbsent: created %v, err %v", created, err)
	}
	backend := s3.(*s3Storage)
	if err = backend.fsCache.Put("module.mjs", bytes.NewBufferString("stale")); err != nil {
		t.Fatal(err)
	}
	created, err = s3.PutIfAbsent("module.mjs", bytes.NewBufferString("second"))
	if err != nil || created {
		t.Fatalf("second PutIfAbsent: created %v, err %v", created, err)
	}
	if string(stored["/module.mjs"]) != "first" {
		t.Fatalf("stored content was overwritten: %q", stored["/module.mjs"])
	}
	if _, err = backend.fsCache.Stat("module.mjs"); err != ErrNotFound {
		t.Fatalf("stale filesystem cache was not invalidated: %v", err)
	}

	created, err = s3.PutIfAbsent("retry.mjs", bytes.NewBufferString("retried"))
	if err != nil || !created {
		t.Fatalf("retried PutIfAbsent: created %v, err %v", created, err)
	}
	if string(stored["/retry.mjs"]) != "retried" {
		t.Fatalf("unexpected retry content: %q", stored["/retry.mjs"])
	}
}

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
	created, err := s3.PutIfAbsent(dirname+"/locked.txt", bytes.NewBufferString("first"))
	if err != nil || !created {
		t.Fatalf("first PutIfAbsent: created %v, err %v", created, err)
	}
	created, err = s3.PutIfAbsent(dirname+"/locked.txt", bytes.NewBufferString("second"))
	if err != nil || created {
		t.Fatalf("second PutIfAbsent: created %v, err %v", created, err)
	}

	keys, err := s3.List(dirname + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 4 {
		t.Fatalf("invalid keys length(%d), expected 4", len(keys))
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

	r, _, err = s3.Get(dirname + "/locked.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(r)
	r.Close()
	if err != nil || string(data) != "first" {
		t.Fatalf("invalid locked content(%s), expected 'first'", string(data))
	}

	err = s3.Delete(dirname + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Delete(dirname + "/%23/hello+world!")
	if err != nil {
		t.Fatal(err)
	}
	err = s3.Delete(dirname + "/locked.txt")
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
