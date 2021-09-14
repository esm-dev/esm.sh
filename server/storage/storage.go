package storage

import (
	"errors"
	"net/url"
	"time"

	logx "github.com/ije/gox/log"
	"github.com/ije/gox/utils"
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

func parseConfigUrl(configUrl string) (path string, query url.Values, err error) {
	parsed, err := url.Parse(configUrl)
	if err != nil {
		return "", nil, err
	}
	return parsed.Path, parsed.Query(), nil
}

func parseBytesValue(str string, defaultValue int64) (int64, error) {
	if str != "" {
		return utils.ParseBytes(str)
	}
	return defaultValue, nil
}

func parseDurationValue(str string, defaultValue time.Duration) (time.Duration, error) {
	if str != "" {
		return time.ParseDuration(str)
	}
	return defaultValue, nil
}

func init() {
	log = &logx.Logger{}
	isDev = false
}
