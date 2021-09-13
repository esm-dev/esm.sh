package storage

import (
	logx "github.com/ije/gox/log"
)

var (
	log   *logx.Logger
	isDev bool
)

func SetLogger(logger *logx.Logger) {
	log = logger
}

func SetIsDev(isDevValue bool) {
	isDev = isDevValue
}

func init() {
	log = &logx.Logger{}
	isDev = false
}
