package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
)

func newTestBuildMetaDB(t *testing.T, backStorage storage.Storage) *BuildMetaDB {
	t.Helper()
	db, err := NewBuildMetaDB(filepath.Join(t.TempDir(), "esm.db"), backStorage)
	if err != nil {
		t.Fatal(err)
	}
	// [warning] queue tests use synctest.Wait, which does not advance the batch timer
	db.MaxBatchSize = 1
	t.Cleanup(func() { db.Close() })
	return db
}

func reopenTestBuildMetaDB(t *testing.T, db *BuildMetaDB) *BuildMetaDB {
	t.Helper()
	filename := db.Path()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewBuildMetaDB(filename, db.storage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	return reopened
}

func TestBuildMetaPersistence(t *testing.T) {
	for _, source := range []string{"current", "legacy", "missing", "purged"} {
		t.Run(source, func(t *testing.T) {
			key := "/persist-meta@1.0.0/es2022/pkg.mjs"
			storeKey := normalizeMetaStoreKey(key)
			data := encodeBuildMeta(&BuildMeta{ExportDefault: true})
			var requests []string
			client := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = client })
			http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				if r.Method != "GET" && r.Method != "HEAD" {
					t.Errorf("metadata changed old storage: %s %s", r.Method, r.URL.Path)
				}
				hit := source == "current" && r.URL.Path == "/"+storeKey || (source == "legacy" || source == "purged") && r.URL.Path == "/meta/"+path.Base(storeKey) || source == "purged" && r.Method == "HEAD"
				if !hit {
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{
					"Content-Length": {fmt.Sprint(len(data))},
					"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
				}}, nil
			})}
			s3, err := storage.NewS3Storage(&storage.StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			db := newTestBuildMetaDB(t, s3)
			value, err := db.Get(key)
			if source == "missing" || source == "purged" {
				if err != storage.ErrNotFound {
					t.Fatalf("get: %v", err)
				}
			} else if err != nil || !bytes.Equal(value, data) {
				t.Fatalf("import: %q, %v", value, err)
			}
			want := []string{"GET /" + storeKey}
			if source != "current" {
				want = append(want, "GET /meta/"+path.Base(storeKey))
			}
			if source == "legacy" || source == "purged" {
				want = append(want, "HEAD /meta-purged/persist-meta@1.0.0")
			}
			if !reflect.DeepEqual(requests, want) {
				t.Fatalf("import requests: %v, want %v", requests, want)
			}
			db = reopenTestBuildMetaDB(t, db)
			db.storage = buildQueueTestStorage{get: func() (*BuildMeta, error) {
				t.Error("restart repeated the import")
				return nil, errors.New("unexpected import")
			}}
			got, gotErr := db.Get(key)
			if !bytes.Equal(got, value) || gotErr != err {
				t.Fatalf("restart: %q, %v; want %q, %v", got, gotErr, value, err)
			}
			if err := db.Put(key, data); err != nil {
				t.Fatal(err)
			}
			db = reopenTestBuildMetaDB(t, db)
			if got, err := db.Get(key); err != nil || !bytes.Equal(got, data) {
				t.Fatalf("persisted write: %q, %v", got, err)
			}
			if err := db.Delete(key); err != nil {
				t.Fatal(err)
			}
			db = reopenTestBuildMetaDB(t, db)
			if _, err := db.Get(key); err != storage.ErrNotFound {
				t.Fatalf("deleted metadata was restored: %v", err)
			}
			if !reflect.DeepEqual(requests, want) {
				t.Fatalf("local changes reached S3: %v", requests)
			}
		})
	}
}

func TestBuildMetaImportRetry(t *testing.T) {
	for _, failure := range []string{"get", "body", "marker"} {
		t.Run(failure, func(t *testing.T) {
			key := "/retry-meta@1.0.0/es2022/pkg.mjs"
			data := encodeBuildMeta(&BuildMeta{})
			failed := true
			client := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = client })
			http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
				if failure == "marker" && r.URL.Path == "/"+normalizeMetaStoreKey(key) {
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				}
				if failed && (failure == "get" || failure == "marker" && r.Method == "HEAD") {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("<Error><Code>Unavailable</Code></Error>"))}, nil
				}
				if r.Method == "HEAD" {
					return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
				}
				var body io.Reader = bytes.NewReader(data)
				if failed && failure == "body" {
					body = iotest.ErrReader(io.ErrUnexpectedEOF)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(body), Header: http.Header{
					"Content-Length": {fmt.Sprint(len(data))},
					"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
				}}, nil
			})}
			s3, err := storage.NewS3Storage(&storage.StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			db := newTestBuildMetaDB(t, s3)
			if _, err := db.Get(key); err == nil || err == storage.ErrNotFound {
				t.Fatalf("import failure was hidden: %v", err)
			}
			db = reopenTestBuildMetaDB(t, db)
			failed = false
			if got, err := db.Get(key); err != nil || !bytes.Equal(got, data) {
				t.Fatalf("failed import was not retried: %q, %v", got, err)
			}
		})
	}
}

func TestBuildMetaLegacyMarkerPersists(t *testing.T) {
	for _, purged := range []bool{false, true} {
		t.Run(fmt.Sprint(purged), func(t *testing.T) {
			fs, err := storage.NewFSStorage(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			marker := "meta-purged/legacy-meta@1.0.0"
			if purged {
				if err := fs.Put(marker, strings.NewReader("")); err != nil {
					t.Fatal(err)
				}
			}
			db := newTestBuildMetaDB(t, fs)
			for _, file := range []string{"a", "b"} {
				key := "/legacy-meta@1.0.0/es2022/" + file + ".mjs"
				if err := fs.Put("meta/"+path.Base(normalizeMetaStoreKey(key)), bytes.NewReader(encodeBuildMeta(&BuildMeta{}))); err != nil {
					t.Fatal(err)
				}
				_, err := db.Get(key)
				if purged && err != storage.ErrNotFound || !purged && err != nil {
					t.Fatalf("legacy metadata: %v", err)
				}
				if file == "a" {
					if purged {
						if err := fs.Delete(marker); err != nil {
							t.Fatal(err)
						}
					} else if err := fs.Put(marker, strings.NewReader("")); err != nil {
						t.Fatal(err)
					}
					db = reopenTestBuildMetaDB(t, db)
				}
			}
		})
	}
}

func TestBuildMetaS3ConcurrentSubmodules(t *testing.T) {
	client := http.DefaultClient
	defer func() { http.DefaultClient = client }()
	started, release := make(chan struct{}), make(chan struct{})
	firstPath := "/viem@2.56.6/es2022/slow.mjs"
	data := string(encodeBuildMeta(&BuildMeta{}))
	http.DefaultClient = &http.Client{Transport: ghTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/"+normalizeMetaStoreKey(firstPath) {
			close(started)
			<-release
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: http.Header{
			"Content-Length": {fmt.Sprint(len(data))},
			"Last-Modified":  {time.Now().UTC().Format(http.TimeFormat)},
		}}, nil
	})}
	s3, err := storage.NewS3Storage(&storage.StorageOptions{Endpoint: "https://storage.test", AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	db := newTestBuildMetaDB(t, s3)
	db.MaxBatchSize = 1000
	q := NewBuildQueue(16, time.Minute)
	finished := make(chan error, 16)
	for i := range 16 {
		buildPath := fmt.Sprintf("/viem@2.56.6/es2022/submodule-%d.mjs", i)
		if i == 0 {
			buildPath = firstPath
		}
		cacheLRU.Remove(buildPath)
		go func() {
			_, err := q.Build(context.Background(), &BuildContext{path: buildPath, metaDB: db})
			finished <- err
		}()
		if i == 0 {
			<-started
		}
	}
	completed := 0
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
WAIT:
	for completed < 15 {
		select {
		case err := <-finished:
			completed++
			if err != nil {
				t.Error(err)
			}
		case <-timer.C:
			t.Errorf("one S3 lookup blocked unrelated submodules: completed %d/15; queue size %d", completed, len(q.Snapshot()))
			break WAIT
		}
	}
	close(release)
	for completed < 16 {
		if err := <-finished; err != nil {
			t.Error(err)
		}
		completed++
	}
}

func TestEncodeBuildMeta(t *testing.T) {
	meta1 := &BuildMeta{
		CJS:           true,
		CSSInJS:       true,
		TypesOnly:     true,
		ExportDefault: true,
		CSSEntry:      "./index.css",
		Dts:           "./types/index.d.ts",
		Imports:       []string{"/react@19.2.4?target=es2022", "/react-dom@19.2.4?target=es2022"},
		Integrity:     "sha384-...",
	}
	data := encodeBuildMeta(meta1)
	meta2, err := decodeBuildMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta1, meta2) {
		t.Fatalf("meta mismatch: %+v != %+v", meta1, meta2)
	}
	meta3, err := decodeBuildMeta([]byte("ESM\r\nwhatever"))
	if err != nil {
		t.Fatal(err)
	}
	metaEmpty := &BuildMeta{}
	if !reflect.DeepEqual(meta3, metaEmpty) {
		t.Fatalf("meta mismatch: %+v != %+v", meta3, metaEmpty)
	}
}

func TestEncodeBuildMetaSkipsMultilineValues(t *testing.T) {
	meta, err := decodeBuildMeta(encodeBuildMeta(&BuildMeta{
		CSSEntry: "./a.css\nt",
		Dts:      "/x.d.ts\nt",
		Imports:  []string{"/a.mjs", "/b.mjs\nt\nd:.evil.com/x.d.ts"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if meta.TypesOnly || meta.CSSEntry != "" || meta.Dts != "" || !reflect.DeepEqual(meta.Imports, []string{"/a.mjs"}) {
		t.Fatalf("a multiline value injected fields: %+v", meta)
	}
}
