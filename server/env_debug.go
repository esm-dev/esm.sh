//go:build debug

package server

import (
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"strings"
)

// debug mode
const DEBUG = true

// always "DEV" in debug mode
const VERSION = "DEV"

type MockEmbedFS struct{}

func (fs MockEmbedFS) ReadFile(name string) ([]byte, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path.Join(cwd, "server", name))
}

// mock embed.FS reads files from the current working directory in DEBUG mode
var embedFS MockEmbedFS

// pprof middleware
func pprofRouter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/pprof/cmdline":
			pprof.Cmdline(w, r)
		case "/debug/pprof/profile":
			pprof.Profile(w, r)
		case "/debug/pprof/symbol":
			pprof.Symbol(w, r)
		case "/debug/pprof/trace":
			pprof.Trace(w, r)
		default:
			if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
				pprof.Index(w, r)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}
