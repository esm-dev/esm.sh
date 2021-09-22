package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	logx "github.com/ije/gox/log"
)

type SimpleS3Client interface {
	Head(key *string) (*s3.HeadObjectOutput, error)
	Get(key *string) (*s3.GetObjectOutput, error)
	Put(key *string, body io.ReadSeeker) (*s3.PutObjectOutput, error)
}

type SimpleS3ClientConfig struct {
	AccountId *string
	Bucket    *string
	Region    *string
	Log       *logx.Logger
}

func NewS3Client(config *SimpleS3ClientConfig) (SimpleS3Client, error) {

	if config.AccountId == nil || *config.AccountId == "" {
		S3_ACCOUNT_ID, found := os.LookupEnv("S3_ACCOUNT_ID")
		if !found {
			S3_ACCOUNT_ID, found = os.LookupEnv("AWS_ACCOUNT_ID")
		}
		if !found {
			S3_ACCOUNT_ID, found = os.LookupEnv("EC2_OWNER_ID")
		}
		if found {
			config.AccountId = aws.String(S3_ACCOUNT_ID)
		} else {
			return nil, errors.New("S3ClientConfig.AccountId not provided and cannot not be derived by environment")
		}
	}

	if config.Bucket == nil || *config.Bucket == "" {
		S3_BUCKET, found := os.LookupEnv("S3_BUCKET")
		if found {
			config.Bucket = aws.String(S3_BUCKET)
		} else {
			return nil, errors.New("S3ClientConfig.Bucket not provided and cannot not be derived by environment")
		}
	}

	if config.Region == nil || *config.Region == "" {
		S3_REGION, found := os.LookupEnv("S3_REGION")
		if !found {
			S3_REGION, found = os.LookupEnv("AWS_REGION")
		}
		if !found {
			S3_REGION, found = os.LookupEnv("EC2_REGION")
		}
		if found {
			config.Region = aws.String(S3_REGION)
		} else {
			return nil, errors.New("S3ClientConfig.Region not provided and cannot not be derived by environment")
		}
	}

	awsSession := session.Must(session.NewSession(&aws.Config{
		MaxRetries:                    aws.Int(2),
		CredentialsChainVerboseErrors: aws.Bool(true),
		HTTPClient:                    &http.Client{Timeout: 10 * time.Second},
		Region:                        config.Region,
	}))
	s3Client := s3.New(awsSession)

	config.Log.Debugf("NewS3Client HeadBucket request: %s, ExpectedBucketOwner: %s", *config.Bucket, *config.AccountId)

	output, err := s3Client.HeadBucket(&s3.HeadBucketInput{
		Bucket:              config.Bucket,
		ExpectedBucketOwner: config.AccountId,
	})
	if err != nil {
		return nil, fmt.Errorf("NewS3Client HeadBucket err: %v", err)
	}
	config.Log.Debugf("NewS3Client HeadBucket output: %v", output)

	return &simpleS3ClientImpl{
		config:   config,
		s3Client: s3Client,
	}, nil
}

type simpleS3ClientImpl struct {
	config   *SimpleS3ClientConfig
	s3Client *s3.S3
}

func (c *simpleS3ClientImpl) Head(key *string) (*s3.HeadObjectOutput, error) {
	return c.s3Client.HeadObject(&s3.HeadObjectInput{
		Bucket:              c.config.Bucket,
		Key:                 key,
		ExpectedBucketOwner: c.config.AccountId,
	})
}

func (c *simpleS3ClientImpl) Get(key *string) (*s3.GetObjectOutput, error) {
	return c.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
	})
}

func (c *simpleS3ClientImpl) Put(key *string, body io.ReadSeeker) (*s3.PutObjectOutput, error) {
	return c.s3Client.PutObject(&s3.PutObjectInput{
		Bucket: c.config.Bucket,
		Key:    key,
		Body:   body,
	})
}
