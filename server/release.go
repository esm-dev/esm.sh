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

// may be changed by `-ldflags`
var VERSION = "v136"

// pprof is disabled in production build
func pprofRouter() rex.Handle {
	return func(ctx *rex.Context) any {
		return rex.Next()
	}
}
