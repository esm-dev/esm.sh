//go:build !debug

package server

import (
	"embed"
)

// production mode
const DEBUG = false

// may be changed by `-ldflags`
var VERSION = "v136"

//go:embed embed
var embedFS embed.FS
