package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	logx "github.com/ije/gox/log"
)

type SimpleS3Client interface {
	Head(key *string) (*s3.HeadObjectOutput, error)
	Get(key *string) (*s3.GetObjectOutput, error)
	Put(key *string, body io.Reader) (*s3.PutObjectOutput, error)
	Delete(key *string) (*s3.DeleteObjectOutput, error)
	Download(key *string, size int64) (io.ReadSeekCloser, error)
	Upload(key *string, body io.Reader) error
}

type SimpleS3ClientConfig struct {
	AccountId *string
	Bucket    *string
	Region    *string
	Log       *logx.Logger
}

func NewS3Client(simpleConfig *SimpleS3ClientConfig) (SimpleS3Client, error) {

	if simpleConfig.AccountId == nil || *simpleConfig.AccountId == "" {
		S3_ACCOUNT_ID, found := os.LookupEnv("S3_ACCOUNT_ID")
		if !found {
			S3_ACCOUNT_ID, found = os.LookupEnv("AWS_ACCOUNT_ID")
		}
		if !found {
			S3_ACCOUNT_ID, found = os.LookupEnv("EC2_OWNER_ID")
		}
		if found {
			simpleConfig.AccountId = aws.String(S3_ACCOUNT_ID)
		} else {
			return nil, errors.New("S3ClientConfig.AccountId not provided and cannot not be derived by environment")
		}
	}

	if simpleConfig.Bucket == nil || *simpleConfig.Bucket == "" {
		S3_BUCKET, found := os.LookupEnv("S3_BUCKET")
		if found {
			simpleConfig.Bucket = aws.String(S3_BUCKET)
		} else {
			return nil, errors.New("S3ClientConfig.Bucket not provided and cannot not be derived by environment")
		}
	}

	if simpleConfig.Region == nil || *simpleConfig.Region == "" {
		S3_REGION, found := os.LookupEnv("S3_REGION")
		if !found {
			S3_REGION, found = os.LookupEnv("AWS_REGION")
		}
		if !found {
			S3_REGION, found = os.LookupEnv("EC2_REGION")
		}
		if found {
			simpleConfig.Region = aws.String(S3_REGION)
		} else {
			return nil, errors.New("S3ClientConfig.Region not provided and cannot not be derived by environment")
		}
	}

	ctx := context.TODO()

	config, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
		awsconfig.WithRegion(*simpleConfig.Region),
	)

	if err != nil {
		return nil, fmt.Errorf("NewS3Client LoadDefaultConfig err: %v", err)
	}

	s3Client := s3.NewFromConfig(config, func(o *s3.Options) {
		o.HTTPClient = &http.Client{Timeout: 10 * time.Second}
		o.Region = *simpleConfig.Region
	})

	simpleConfig.Log.Debugf("NewS3Client HeadBucket request: %s, ExpectedBucketOwner: %s", *simpleConfig.Bucket, *simpleConfig.AccountId)

	output, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket:              simpleConfig.Bucket,
		ExpectedBucketOwner: simpleConfig.AccountId,
	})
	if err != nil {
		return nil, fmt.Errorf("NewS3Client HeadBucket err: %v", err)
	}
	simpleConfig.Log.Debugf("NewS3Client HeadBucket output: %v", output)

	downloader := s3manager.NewDownloader(s3Client)
	uploader := s3manager.NewUploader(s3Client)

	return &simpleS3ClientImpl{
		context:    ctx,
		config:     simpleConfig,
		s3Client:   s3Client,
		downloader: downloader,
		uploader:   uploader,
	}, nil
}

type simpleS3ClientImpl struct {
	context    context.Context
	config     *SimpleS3ClientConfig
	s3Client   *s3.Client
	downloader *s3manager.Downloader
	uploader   *s3manager.Uploader
}

func (c *simpleS3ClientImpl) Head(key *string) (*s3.HeadObjectOutput, error) {
	return c.s3Client.HeadObject(c.context, &s3.HeadObjectInput{
		Bucket:              c.config.Bucket,
		Key:                 key,
		ExpectedBucketOwner: c.config.AccountId,
	})
}

func (c *simpleS3ClientImpl) Get(key *string) (*s3.GetObjectOutput, error) {
	return c.s3Client.GetObject(c.context, &s3.GetObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
	})
}

func (c *simpleS3ClientImpl) Put(key *string, body io.Reader) (*s3.PutObjectOutput, error) {
	return c.s3Client.PutObject(c.context, &s3.PutObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
		Body:   body,
	})
}

func (c *simpleS3ClientImpl) Delete(key *string) (*s3.DeleteObjectOutput, error) {
	return c.s3Client.DeleteObject(c.context, &s3.DeleteObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
	})
}

func (c *simpleS3ClientImpl) Download(key *string, size int64) (io.ReadSeekCloser, error) {
	// pre-allocate in memory buffer, where headObject type is *s3.HeadObjectOutput
	buf := make([]byte, int(size))
	// wrap with aws.WriteAtBuffer
	w := s3manager.NewWriteAtBuffer(buf)
	// download file into the memory
	_, err := c.downloader.Download(c.context, w, &s3.GetObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
	})
	bufReader := bytes.NewReader(w.Bytes())
	return s3manager.ReadSeekCloser(bufReader), err
}

func (c *simpleS3ClientImpl) Upload(key *string, body io.Reader) error {
	_, err := c.uploader.Upload(c.context, &s3.PutObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
		Body:   body,
	})
	return err
}
