package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/esm-dev/esm.sh/internal/storage"
)

type buildQueueTestStorage struct {
	storage.Storage
	get func() (*BuildMeta, error)
}

func (s buildQueueTestStorage) Get(string) (io.ReadCloser, storage.Stat, error) {
	meta, err := s.get()
	if err != nil {
		return nil, nil, err
	}
	return io.NopCloser(bytes.NewReader(encodeBuildMeta(meta))), nil, nil
}

func newBuildQueueTestContext(path string, get func() (*BuildMeta, error)) *BuildContext {
	cacheLRU.Remove(path)
	return &BuildContext{path: path, metaDB: NewBuildMetaDB(buildQueueTestStorage{get: get})}
}

func TestBuildQueueScheduling(t *testing.T) {
	for _, concurrency := range []int{-1, 0, 1, 3} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				q := NewBuildQueue(concurrency, 0)
				limit := max(1, concurrency)
				const count = 6
				started := make(chan int, count)
				finished := make(chan int, count)
				release := make([]chan struct{}, count)
				for i := range count {
					release[i] = make(chan struct{})
					build := newBuildQueueTestContext(fmt.Sprintf("/%s/%d", t.Name(), i), func() (*BuildMeta, error) {
						started <- i
						<-release[i]
						return &BuildMeta{Dts: fmt.Sprint(i)}, nil
					})
					go func() {
						meta, err := q.Build(context.Background(), build)
						if err != nil || meta == nil || meta.Dts != fmt.Sprint(i) {
							t.Errorf("build %d: meta=%+v, err=%v", i, meta, err)
						}
						finished <- i
					}()
					synctest.Wait()
					time.Sleep(time.Nanosecond)
				}
				for i, item := range q.Snapshot() {
					status := "pending"
					if i < limit {
						status = "build"
					}
					if item["path"] != fmt.Sprintf("/%s/%d", t.Name(), i) || item["status"] != status || item["waitClients"] != 1 {
						t.Fatalf("unexpected task %d: %+v", i, item)
					}
				}
				if len(started) != limit {
					t.Fatalf("started %d builds, want %d", len(started), limit)
				}
				for i := range limit {
					if got := <-started; got != i {
						t.Fatalf("started %d, want %d", got, i)
					}
				}
				for i := range count {
					close(release[i])
					synctest.Wait()
					if got := <-finished; got != i {
						t.Fatalf("finished %d, want %d", got, i)
					}
					if next := i + limit; next < count {
						if len(started) != 1 {
							t.Fatalf("started %d new builds, want 1", len(started))
						}
						if got := <-started; got != next {
							t.Fatalf("started %d, want %d", got, next)
						}
					}
				}
				if len(q.Snapshot()) != 0 {
					t.Fatal("finished tasks remain in the queue")
				}
			})
		})
	}
}

func TestBuildQueueWaitCancellation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%v", pending), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				q := NewBuildQueue(1, time.Minute)
				unblock := make(chan struct{})
				if pending {
					blocker := newBuildQueueTestContext("/"+t.Name()+"/blocker", func() (*BuildMeta, error) {
						<-unblock
						return &BuildMeta{}, nil
					})
					go func() {
						if _, err := q.Build(context.Background(), blocker); err != nil {
							t.Error(err)
						}
					}()
					synctest.Wait()
				}
				release := make(chan struct{})
				path := "/" + t.Name() + "/shared"
				build := newBuildQueueTestContext(path, func() (*BuildMeta, error) {
					<-release
					return &BuildMeta{ExportDefault: true}, nil
				})
				waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				go func() {
					if _, err := q.Build(waitCtx, build); !errors.Is(err, context.DeadlineExceeded) {
						t.Errorf("expected wait timeout, got %v", err)
					}
				}()
				synctest.Wait()
				time.Sleep(time.Second)
				synctest.Wait()
				for _, item := range q.Snapshot() {
					if item["path"] == path && item["waitClients"] != 0 {
						t.Fatalf("timed out waiter retained: %+v", item)
					}
				}
				if !pending && build.Context().Err() != nil {
					t.Fatal("request timeout canceled the shared build")
				}
				const clients = 64
				finished := make(chan struct{}, clients)
				for range clients {
					go func() {
						meta, err := q.Build(context.Background(), &BuildContext{path: path})
						if err != nil || meta == nil || !meta.ExportDefault {
							t.Errorf("shared result: meta=%+v, err=%v", meta, err)
						}
						finished <- struct{}{}
					}()
				}
				synctest.Wait()
				for _, item := range q.Snapshot() {
					if item["path"] == path && item["waitClients"] != clients {
						t.Fatalf("waiters did not join the existing build: %+v", item)
					}
				}
				close(unblock)
				synctest.Wait()
				close(release)
				synctest.Wait()
				if len(finished) != clients || len(q.Snapshot()) != 0 {
					t.Fatalf("finished %d/%d waiters; queue: %+v", len(finished), clients, q.Snapshot())
				}
				canceled, stop := context.WithCancel(context.Background())
				stop()
				if _, err := q.Build(canceled, nil); !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation before enqueue, got %v", err)
				}
			})
		})
	}
}

func TestBuildQueueFailure(t *testing.T) {
	for _, failure := range []string{"error", "panic", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				q := NewBuildQueue(1, time.Second)
				release := make(chan struct{})
				path := "/" + t.Name()
				want := "failed"
				build := newBuildQueueTestContext(path, nil)
				build.metaDB = NewBuildMetaDB(buildQueueTestStorage{get: func() (*BuildMeta, error) {
					if failure == "timeout" {
						<-build.Context().Done()
					}
					<-release
					switch failure {
					case "panic":
						panic("failed")
					case "timeout":
						return nil, build.Context().Err()
					default:
						return nil, errors.New("failed")
					}
				}})
				if failure == "panic" {
					want = "build panic: failed"
				} else if failure == "timeout" {
					want = "build timeout after 1 seconds"
				}
				errorsReceived := make(chan error, 2)
				go func() {
					_, err := q.Build(context.Background(), build)
					errorsReceived <- err
				}()
				synctest.Wait()
				go func() {
					_, err := q.Build(context.Background(), &BuildContext{path: path})
					errorsReceived <- err
				}()
				nextStarted := make(chan struct{}, 1)
				next := newBuildQueueTestContext(path+"/next", func() (*BuildMeta, error) {
					nextStarted <- struct{}{}
					return &BuildMeta{}, nil
				})
				go func() {
					if _, err := q.Build(context.Background(), next); err != nil {
						t.Error(err)
					}
				}()
				synctest.Wait()
				if failure == "timeout" {
					time.Sleep(time.Second)
					synctest.Wait()
					if len(nextStarted) != 0 {
						t.Fatal("released capacity before the timed out build exited")
					}
				}
				close(release)
				synctest.Wait()
				if len(errorsReceived) != 2 || len(nextStarted) != 1 || len(q.Snapshot()) != 0 {
					t.Fatalf("failure did not release waiters and capacity: errors=%d, next=%d, queue=%+v", len(errorsReceived), len(nextStarted), q.Snapshot())
				}
				for range 2 {
					if err := <-errorsReceived; err == nil || err.Error() != want {
						t.Fatalf("got %v, want %q", err, want)
					}
				}
				retry := newBuildQueueTestContext(path, func() (*BuildMeta, error) { return &BuildMeta{}, nil })
				if _, err := q.Build(context.Background(), retry); err != nil {
					t.Fatalf("retry failed: %v", err)
				}
			})
		})
	}
}

func TestBuildQueuePathChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := NewBuildQueue(2, time.Minute)
		aliasPath, canonicalPath := "/"+t.Name()+"/index", "/"+t.Name()+"/root"
		releaseAlias, releaseCanonical := make(chan struct{}), make(chan struct{})
		alias := newBuildQueueTestContext(aliasPath, nil)
		alias.esmPath.SubPath = "index"
		alias.metaDB = NewBuildMetaDB(buildQueueTestStorage{get: func() (*BuildMeta, error) {
			alias.path = canonicalPath
			alias.esmPath.SubPath = ""
			alias.status.Store("install")
			<-releaseAlias
			return &BuildMeta{}, nil
		}})
		canonical := newBuildQueueTestContext(canonicalPath, func() (*BuildMeta, error) {
			<-releaseCanonical
			return &BuildMeta{}, nil
		})
		for _, build := range []*BuildContext{alias, canonical} {
			go func() {
				if _, err := q.Build(context.Background(), build); err != nil {
					t.Error(err)
				}
			}()
		}
		synctest.Wait()
		duplicate := &BuildContext{path: aliasPath, esmPath: EsmPath{SubPath: "index"}}
		go func() {
			if _, err := q.Build(context.Background(), duplicate); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		for _, item := range q.Snapshot() {
			if item["path"] == aliasPath && (item["status"] != "install" || item["waitClients"] != 2) {
				t.Fatalf("unexpected alias task: %+v", item)
			}
		}
		close(releaseAlias)
		synctest.Wait()
		if duplicate.Path() != canonicalPath || duplicate.esmPath.SubPath != "" {
			t.Fatalf("joined request did not receive the resolved path: %+v", duplicate)
		}
		items := q.Snapshot()
		if len(items) != 1 || items[0]["path"] != canonicalPath {
			t.Fatalf("completion removed the wrong task: %+v", items)
		}
		go func() {
			if _, err := q.Build(context.Background(), &BuildContext{path: canonicalPath}); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		items = q.Snapshot()
		if len(items) != 1 || items[0]["waitClients"] != 2 {
			t.Fatalf("canonical build lost deduplication: %+v", items)
		}
		close(releaseCanonical)
		synctest.Wait()
	})
}

func TestBuildQueueSnapshotDuringBuild(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := NewBuildQueue(1, time.Minute)
		start, release := make(chan struct{}), make(chan struct{})
		path := "/" + t.Name()
		build := newBuildQueueTestContext(path, nil)
		build.metaDB = NewBuildMetaDB(buildQueueTestStorage{get: func() (*BuildMeta, error) {
			<-start
			for range 1000 {
				build.path = path + "/resolved"
				build.status.Store("install")
				build.status.Store("build")
			}
			<-release
			return &BuildMeta{}, nil
		}})
		go func() {
			if _, err := q.Build(context.Background(), build); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		close(start)
		for range 1000 {
			items := q.Snapshot()
			if len(items) != 1 || items[0]["path"] != path || (items[0]["status"] != "install" && items[0]["status"] != "build") {
				t.Fatalf("unexpected snapshot: %+v", items)
			}
		}
		close(release)
		synctest.Wait()
	})
}
