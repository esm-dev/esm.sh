//go:build !debug

package server

import (
	"embed"

	"github.com/ije/rex"
)

// production mode
const DEBUG = false

//go:embed embed
var embedFS embed.FS

// defined by `-ldflags`
var VERSION = "PROD"

// pprof is disabled in production build
func pprofRouter() rex.Handle {
	return func(ctx *rex.Context) any {
		return rex.Next()
	}
}
