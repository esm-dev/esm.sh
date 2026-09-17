package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
)

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
	db := NewBuildMetaDB(s3)
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
