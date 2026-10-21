package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestCanceledMetadataWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		previous := http.DefaultTransport
		defer func() { http.DefaultTransport = previous }()
		calls := 0
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				close(started)
			}
			<-release
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"versions":{"1.0.0":{"name":"cancel-shared-test","version":"1.0.0"}}}`))}, nil
		})
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: "http://shared.invalid/"}}}
		leader := make(chan error, 1)
		go func() {
			_, _, err := rc.fetchPackageMetadataContext(context.Background(), "cancel-shared-test", "^1", false)
			leader <- err
		}()
		<-started
		ctx, cancel := context.WithCancel(context.Background())
		waiter := make(chan error, 1)
		go func() {
			_, _, err := rc.fetchPackageMetadataContext(ctx, "cancel-shared-test", "^1", false)
			waiter <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-waiter; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(release)
		if err := <-leader; err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if calls != 1 {
			t.Fatalf("registry calls = %d, want 1", calls)
		}
	})
}

func TestCanceledCacheWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var locks keyedMutex
		unlock := locks.Lock("metadata")
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			release, err := locks.LockContext(ctx, "metadata")
			if release != nil {
				release()
			}
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		acquired := make(chan struct{})
		go func() { release := locks.Lock("metadata"); close(acquired); release() }()
		synctest.Wait()
		select {
		case <-acquired:
			t.Fatal("canceled waiter released another owner")
		default:
		}
		unlock()
		<-acquired
	})
}

func TestNpmResolutionRequestDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := http.DefaultTransport
		defer func() { http.DefaultTransport = previous }()
		var calls atomic.Int32
		http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		rc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: "http://deadline.invalid/"}}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _, _, _, _, err := parseEsmPathContext(ctx, rc, "/cancel-deadline-test@^1")
		synctest.Wait()
		if !errors.Is(err, context.DeadlineExceeded) || calls.Load() == 0 {
			t.Fatalf("resolution: %v, calls: %d", err, calls.Load())
		}
		time.Sleep(npmQueryTimeout + time.Second)
		synctest.Wait()
	})
}
