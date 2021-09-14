package storage

import (
	"errors"
)

var (
	ErrExpired  = errors.New("expired")
	ErrNotFound = errors.New("not found")
	ErrIO       = errors.New("io error")
)
