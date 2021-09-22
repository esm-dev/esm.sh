package storage

import (
	"bytes"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
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

func (fs *s3FSLayer) Exists(name string) (bool, time.Time, error) {
	var modtime time.Time
	if fs.backingFS != nil {
		found, modtime, err := fs.backingFS.Exists(name)
		if found && err == nil {
			return true, modtime, nil
		}
	}
	result, err := fs.s3Client.Head(&name)
	if err != nil {
		// http://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html
		// https://github.com/awsdocs/aws-doc-sdk-examples/blob/master/go/example_code/extending_sdk/handleServiceErrorCodes.go
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == s3.ErrCodeNoSuchKey {
				return false, modtime, nil
			}
		}
		return false, modtime, err
	}
	modtime = *result.LastModified
	return true, modtime, nil
}

func (fs *s3FSLayer) ReadFile(name string) (io.ReadSeekCloser, error) {
	if fs.backingFS != nil {
		if found, _, _ := fs.backingFS.Exists(name); found {
			file, err := fs.backingFS.ReadFile(name)
			if file != nil && err == nil {
				return file, err
			}
		}
	}
	result, err := fs.s3Client.Get(&name)
	if err != nil {
		return nil, err
	}
	if fs.backingFS != nil {
		fs.backingFS.WriteFile(name, result.Body)
		return fs.backingFS.ReadFile(name)
	}
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	return aws.ReadSeekCloser(bytes.NewReader(data)), nil
}

func (fs *s3FSLayer) WriteFile(name string, content io.Reader) (int64, error) {
	_, err := fs.s3Client.Put(&name, aws.ReadSeekCloser(content))
	if err != nil {
		return 0, err
	}
	if fs.backingFS != nil {
		go fs.backingFS.WriteFile(name, content)
	}
	result, err := fs.s3Client.Head(&name)
	if err != nil {
		return 0, err
	}
	return aws.Int64Value(result.ContentLength), nil
}

func (fs *s3FSLayer) WriteData(name string, data []byte) error {
	content := bytes.NewReader(data)
	_, err := fs.s3Client.Put(&name, content)
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
