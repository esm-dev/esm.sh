package storage

import (
	"errors"

	logx "github.com/ije/gox/log"
)

var (
	log   *logx.Logger
	isDev bool
)

var (
	ErrExpired  = errors.New("record is expired")
	ErrNotFound = errors.New("record not found")
	ErrIO       = errors.New("io error")
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
