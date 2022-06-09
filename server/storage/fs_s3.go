package storage

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"time"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
)

type s3FS struct{}

func getBackingFS(options url.Values) (FS, error) {
	url := options.Get("backingFS")
	if url != "" {
		return OpenFS(url)
	}
	return nil, nil
}

func (fs *s3FS) Open(bucket string, options url.Values) (FS, error) {
	backingFS, err := getBackingFS(options)
	if err != nil {
		return nil, err
	}
	accountId := options.Get("accountId")
	region := options.Get("region")
	s3Client, err := NewS3Client(&SimpleS3ClientConfig{
		Bucket:    &bucket,
		AccountId: &accountId,
		Region:    &region,
		Log:       log,
	})
	if err != nil {
		return nil, err
	}
	return &s3FSLayer{backingFS, s3Client}, nil
}

type s3FSLayer struct {
	backingFS FS
	s3Client  SimpleS3Client
}

func (fs *s3FSLayer) Exists(name string) (bool, int64, time.Time, error) {
	var modtime time.Time
	if fs.backingFS != nil {
		found, size, modtime, err := fs.backingFS.Exists(name)
		if found && err == nil {
			return true, size, modtime, nil
		}
	}
	result, err := fs.s3Client.Head(&name)
	if err != nil {
		// https://aws.github.io/aws-sdk-go-v2/docs/handling-errors/#retrieving-request-identifiers
		var rspErr *awshttp.ResponseError
		if errors.As(err, &rspErr) && rspErr.HTTPStatusCode() == 404 {
			return false, 0, modtime, nil
		}
		return false, 0, modtime, err
	}
	modtime = *result.LastModified
	return true, result.ContentLength, modtime, nil
}

func (fs *s3FSLayer) ReadFile(name string, size int64) (io.ReadSeekCloser, error) {
	if fs.backingFS != nil {
		file, err := fs.backingFS.ReadFile(name, size)
		if file != nil && err == nil {
			return file, err
		}
	}
	result, err := fs.s3Client.Download(&name, size)
	if err != nil {
		go fs.s3Client.Delete(&name)
		return nil, err
	}
	if fs.backingFS != nil {
		fs.backingFS.WriteFile(name, result)
	}
	return result, nil
}

func (fs *s3FSLayer) WriteFile(name string, content io.Reader) (int64, error) {
	err := fs.s3Client.Upload(&name, content)
	if err != nil {
		return 0, err
	}
	result, err := fs.s3Client.Head(&name)
	if err != nil {
		return 0, err
	}
	if fs.backingFS != nil {
		go fs.backingFS.WriteFile(name, content)
	}
	return result.ContentLength, nil
}

func (fs *s3FSLayer) WriteData(name string, data []byte) error {
	content := bytes.NewReader(data)
	err := fs.s3Client.Upload(&name, content)
	if err != nil {
		return err
	}
	if fs.backingFS != nil {
		go fs.backingFS.WriteData(name, data)
	}
	return nil
}

func init() {
	RegisterFS("s3", &s3FS{})
}
