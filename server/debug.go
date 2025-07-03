//go:build debug

package server

import (
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"strings"

	"github.com/ije/rex"
)

// debug mode
const DEBUG = true

// always "DEV" in DEBUG mode
const VERSION = "DEV"

// mock embed.FS reads files from the current working directory in DEBUG mode
var embedFS MockEmbedFS

type MockEmbedFS struct{}

func (fs MockEmbedFS) ReadFile(name string) ([]byte, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path.Join(cwd, "server", name))
}

func pprofRouter() rex.Handle {
	return func(ctx *rex.Context) any {
		switch ctx.R.URL.Path {
		case "/debug/pprof/cmdline":
			return http.HandlerFunc(pprof.Cmdline)
		case "/debug/pprof/profile":
			return http.HandlerFunc(pprof.Profile)
		case "/debug/pprof/symbol":
			return http.HandlerFunc(pprof.Symbol)
		case "/debug/pprof/trace":
			return http.HandlerFunc(pprof.Trace)
		default:
			if strings.HasPrefix(ctx.R.URL.Path, "/debug/pprof/") {
				return http.HandlerFunc(pprof.Index)
			}
			return rex.Next()
		}
	}
}
