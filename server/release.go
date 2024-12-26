//go:build !debug

package server

const (
	DEBUG = false
)

// may be changed by `-ldflags`
var VERSION = "v136"
