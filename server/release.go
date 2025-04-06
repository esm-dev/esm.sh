//go:build !debug

package server

import (
	"embed"
)

// production mode
const DEBUG = false

//go:embed embed
var embedFS embed.FS

// may be changed by `-ldflags`
var VERSION = "v136"
