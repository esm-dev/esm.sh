//go:build !debug

package server

import (
	"embed"
	"net/http"
)

// production mode
const DEBUG = false

//go:embed embed
var embedFS embed.FS

// defined by `-ldflags`
var VERSION = "PROD"

// pprof is disabled in production build
func pprofRouter(next http.Handler) http.Handler {
	return next
}
