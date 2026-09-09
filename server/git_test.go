package server

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ije/gox/crypto/rand"
)

type ghTestTransport func(*http.Request) (*http.Response, error)

func (f ghTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestListRepoRefs(t *testing.T) {
	refs, err := listGhRepoRefs("https://github.com/esm-dev/esm.sh")
	if err != nil {
		t.Fatal(err)
	}
	var headSha string
	for _, ref := range refs {
		if ref.Ref == "HEAD" {
			headSha = ref.Sha
			break
		}
	}
	if headSha == "" {
		t.Fatal("HEAD not found")
	}
}

func TestGhInstall(t *testing.T) {
	dir := filepath.Join(os.TempDir(), rand.Hex.String(8))
	defer os.RemoveAll(dir)
	err := ghInstall(dir, "esm-dev/esm.sh", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !existsFile(path.Join(dir, "node_modules/esm-dev/esm.sh/README.md")) {
		t.Fatal("README.md not found")
	}
}

func TestGhInstallLimits(t *testing.T) {
	workDir, transport := config.WorkDir, http.DefaultTransport
	config.WorkDir = t.TempDir()
	t.Cleanup(func() {
		config.WorkDir, http.DefaultTransport = workDir, transport
	})
	for _, test := range []struct {
		name     string
		tooLarge bool
	}{
		{"content-length", true},
		{"download", true},
		{"unpacked", true},
		{"valid", false},
		{"invalid", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				res := &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: -1}
				if test.name == "content-length" {
					res.ContentLength = maxPackageTarballSize + 1
					res.Body = io.NopCloser(strings.NewReader(""))
				} else if test.name == "invalid" {
					res.Body = io.NopCloser(strings.NewReader("invalid gzip"))
				} else {
					pr, pw := io.Pipe()
					res.Body = pr
					go func() {
						defer pw.Close()
						level := gzip.BestSpeed
						if test.name == "download" {
							level = gzip.NoCompression
						}
						gz, _ := gzip.NewWriterLevel(pw, level)
						defer gz.Close()
						tw := tar.NewWriter(gz)
						defer tw.Close()
						if err := tw.WriteHeader(&tar.Header{Name: "repo/package.json", Mode: 0644, Size: 2}); err != nil {
							return
						}
						if _, err := tw.Write([]byte("{}")); err != nil {
							return
						}
						if test.tooLarge {
							if err := tw.WriteHeader(&tar.Header{Name: "repo/large.bin", Mode: 0644, Size: maxPackageTarballSize + 1}); err != nil {
								return
							}
							io.CopyN(tw, zeroReader{}, maxPackageTarballSize+1)
						}
					}()
				}
				return res, nil
			})
			wd := filepath.Join(t.TempDir(), "install")
			repo := "owner/" + test.name
			err := ghInstall(wd, repo, "main")
			if test.tooLarge {
				if err != errRepoTooLarge || err.Error() != "repo is too large" {
					t.Fatalf("expected repo size error, got %v", err)
				}
				if err := ghInstall(wd, strings.ToUpper(repo), "another-tag"); err != errRepoTooLarge {
					t.Fatalf("expected recorded repo size error, got %v", err)
				}
				if requests != 1 {
					t.Fatalf("recorded repo was fetched again: %d requests", requests)
				}
			} else if test.name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if !existsFile(filepath.Join(wd, "node_modules", repo, "package.json")) {
					t.Fatal("package.json not found")
				}
			} else if err == nil || errors.Is(err, errRepoTooLarge) {
				t.Fatalf("expected invalid archive error, got %v", err)
			}
			if err != nil {
				if _, statErr := os.Stat(wd); !os.IsNotExist(statErr) {
					t.Fatalf("partial installation remains: %v", statErr)
				}
			}
		})
	}
	files, err := os.ReadDir(filepath.Join(config.WorkDir, "gh-too-large"))
	if err != nil || len(files) != 3 {
		t.Fatalf("expected three persistent repo records, got %d: %v", len(files), err)
	}
}

func TestGhInstallTimeout(t *testing.T) {
	workDir, transport := config.WorkDir, http.DefaultTransport
	config.WorkDir = t.TempDir()
	t.Cleanup(func() {
		config.WorkDir, http.DefaultTransport = workDir, transport
	})
	for _, phase := range []string{"request", "body", "canceled", "parent-deadline"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				http.DefaultTransport = ghTestTransport(func(r *http.Request) (*http.Response, error) {
					if phase == "canceled" {
						t.Fatal("canceled install made a request")
					}
					if phase != "body" {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					pr, pw := io.Pipe()
					go func() {
						gz := gzip.NewWriter(pw)
						tw := tar.NewWriter(gz)
						tw.WriteHeader(&tar.Header{Name: "repo/package.json", Mode: 0644, Size: 2})
						tw.Write([]byte("{}"))
						tw.Flush()
						gz.Flush()
						<-r.Context().Done()
						pw.CloseWithError(r.Context().Err())
					}()
					return &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: -1, Body: pr}, nil
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if phase == "canceled" {
					cancel()
				} else if phase == "parent-deadline" {
					var stop context.CancelFunc
					ctx, stop = context.WithTimeout(ctx, time.Second)
					defer stop()
				}
				wd := filepath.Join(t.TempDir(), "install")
				start := time.Now()
				err := ghInstallContext(ctx, wd, "owner/timeout", "main")
				if phase == "canceled" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("expected cancellation, got %v", err)
					}
				} else if phase == "parent-deadline" {
					if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != time.Second {
						t.Fatalf("expected caller deadline, got %v after %v", err, time.Since(start))
					}
				} else if err == nil || err.Error() != "github: install timeout after 30 seconds" || time.Since(start) != ghInstallTimeout {
					t.Fatalf("expected install timeout, got %v after %v", err, time.Since(start))
				}
				if _, err := os.Stat(wd); !os.IsNotExist(err) {
					t.Fatalf("partial installation remains: %v", err)
				}
			})
		})
	}
	if existsDir(filepath.Join(config.WorkDir, "gh-too-large")) {
		t.Fatal("timed out repo was recorded as too large")
	}
}
