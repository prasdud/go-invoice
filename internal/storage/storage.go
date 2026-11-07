package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// ErrNotFound is returned when the requested object does not exist.
var ErrNotFound = errors.New("storage: object not found")

type Client struct {
	s3     *s3.Client
	bucket string
	region string
}

type Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

func New(cfg Config) (*Client, error) {
	var opts []func(*config.LoadOptions) error

	if cfg.Endpoint != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), append(opts, config.WithRetryMaxAttempts(1))...)
	if err != nil {
		return nil, fmt.Errorf("storage: load config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = cfg.UsePathStyle
		}
	})

	return &Client{s3: client, bucket: cfg.Bucket, region: cfg.Region}, nil
}

// EnsureBucket checks the configured bucket and creates it when missing.
func (c *Client) EnsureBucket(ctx context.Context) error {
	_, err := c.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("storage: head bucket: %w", err)
	}
	switch apiErr.ErrorCode() {
	case "", "NotFound", "NoSuchBucket", "404":
	default:
		return fmt.Errorf("storage: head bucket: %w", err)
	}

	input := &s3.CreateBucketInput{Bucket: aws.String(c.bucket)}
	if c.region != "" && c.region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(c.region),
		}
	}
	if _, err := c.s3.CreateBucket(ctx, input); err != nil {
		return fmt.Errorf("storage: create bucket %s: %w", c.bucket, err)
	}
	return nil
}

// uploadBackoff is the delay before each retry; the SDK's own retries are
// disabled so this policy is the only one.
var uploadBackoff = []time.Duration{50 * time.Millisecond, 250 * time.Millisecond}

func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	var err error
	for attempt := 0; ; attempt++ {
		_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(c.bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(contentType),
		})
		if err == nil {
			return nil
		}
		if attempt >= len(uploadBackoff) {
			break
		}
		select {
		case <-time.After(uploadBackoff[attempt]):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("storage: upload %s: %w", key, err)
}

// Download returns the object body and its size (-1 when unknown).
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("storage: download %s: %w", key, err)
	}
	size := int64(-1)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

// Key is the artifact location for an invoice id. Flat by design: the key is a
// pure function of the id, so downloads need no lookup store.
func Key(id string) string {
	return "invoices/" + id + ".pdf"
}
