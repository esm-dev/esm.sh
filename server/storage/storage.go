package storage

import (
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExpired  = errors.New("record is expired")
)

type StorageOptions struct {
	Type            string `json:"type"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyID"`
	SecretAccessKey string `json:"secretAccessKey"`
}

type Storage interface {
	Stat(key string) (stat Stat, err error)
	List(prefix string) (keys []string, err error)
	Get(key string) (content io.ReadCloser, stat Stat, err error)
	Put(key string, r io.Reader) error
	Delete(keys ...string) error
	DeleteAll(prefix string) (deletedKeys []string, err error)
}

type Stat interface {
	Size() int64
	ModTime() time.Time
}

func New(options *StorageOptions) (storage Storage, err error) {
	switch options.Type {
	case "fs":
		return NewFSStorage(options)
	case "s3":
		return NewS3Storage(options)
	default:
		return nil, errors.New("unsupported storage type")
	}
}
