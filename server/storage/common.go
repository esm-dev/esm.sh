package storage

import (
	"errors"
	"net/url"
	"time"

	"github.com/ije/gox/utils"
)

var (
	ErrExpired  = errors.New("record is expired")
	ErrNotFound = errors.New("record not found")
	// ErrIO       = errors.New("io error")
)

func parseConfigUrl(configUrl string) (root string, options url.Values, err error) {
	root, query := utils.SplitByFirstByte(configUrl, '?')
	if query != "" {
		options, err = url.ParseQuery(query)
		if err != nil {
			return root, nil, err
		}
	}
	return root, options, nil
}

func parseDurationValue(str string, defaultValue time.Duration) (time.Duration, error) {
	if str != "" {
		return time.ParseDuration(str)
	}
	return defaultValue, nil
}
