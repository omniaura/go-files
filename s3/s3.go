// Package s3 provides a generic S3-compatible storage backend implementing
// files.Storage. It works against any S3 API surface: AWS, MinIO, Backblaze
// B2 (S3-compat endpoint), and Hippius (via the hippius/ preset).
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/omniaura/go-files/files"
)

// MaxPresignTTL is the AWS hard limit for SigV4 presigned URLs (7 days).
// Calls with a larger TTL are clamped down silently.
const MaxPresignTTL = 7 * 24 * time.Hour

// Options configures a generic S3 client.
type Options struct {
	// Bucket is the target bucket. Required for Storage method calls but
	// optional at construction time so callers can build a client and
	// rebind buckets via WithBucket.
	Bucket string

	// Region is the AWS-style region string (or backend-specific region,
	// e.g. "decentralized" for Hippius). Required.
	Region string

	// Endpoint optionally overrides the SDK's default endpoint resolver.
	// Set this for non-AWS backends (MinIO, Hippius, B2 S3-compat).
	Endpoint string

	// AccessKeyID / SecretAccessKey / SessionToken supply static
	// credentials. Leave empty to use the default credentials chain
	// (env, IMDS, profile, etc.).
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// UsePathStyle forces path-style URLs (bucket-in-path). Required for
	// MinIO and Hippius; AWS S3 prefers virtual-hosted-style.
	UsePathStyle bool
}

// Client implements files.Storage backed by aws-sdk-go-v2.
type Client struct {
	sdk    *awss3.Client
	bucket string
}

// Compile-time assertion.
var _ files.Storage = (*Client)(nil)

// NewClient builds an S3 client.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	if opts.Region == "" {
		return nil, errors.New("s3: Region is required")
	}
	if (opts.AccessKeyID == "") != (opts.SecretAccessKey == "") {
		return nil, errors.New("s3: AccessKeyID and SecretAccessKey must be set together")
	}

	loadOpts := []func(*config.LoadOptions) error{config.WithRegion(opts.Region)}
	if opts.AccessKeyID != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				opts.AccessKeyID, opts.SecretAccessKey, opts.SessionToken,
			),
		))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3: load aws config: %w", err)
	}

	sdk := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
		}
		o.UsePathStyle = opts.UsePathStyle
	})
	return &Client{sdk: sdk, bucket: opts.Bucket}, nil
}

// NewFromSDK wraps an existing aws-sdk-go-v2 s3.Client. Useful for tests
// and for callers that need fine-grained SDK configuration.
func NewFromSDK(sdk *awss3.Client, bucket string) *Client {
	return &Client{sdk: sdk, bucket: bucket}
}

// SDK returns the underlying aws-sdk-go-v2 s3.Client for advanced calls
// not covered by the Storage interface.
func (c *Client) SDK() *awss3.Client { return c.sdk }

// Bucket returns the configured bucket.
func (c *Client) Bucket() string { return c.bucket }

// WithBucket returns a new Client targeting a different bucket while
// reusing the underlying SDK client.
func (c *Client) WithBucket(bucket string) *Client {
	return &Client{sdk: c.sdk, bucket: bucket}
}

func (c *Client) requireBucket() error {
	if c.bucket == "" {
		return errors.New("s3: bucket is not configured (set Options.Bucket or call WithBucket)")
	}
	return nil
}

// Upload puts an object using r as the body.
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, meta map[string]string) (*files.Object, error) {
	if err := c.requireBucket(); err != nil {
		return nil, err
	}
	in := &awss3.PutObjectInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		Body:     r,
		Metadata: meta,
	}
	if ct, ok := meta["Content-Type"]; ok {
		in.ContentType = aws.String(ct)
	}
	out, err := c.sdk.PutObject(ctx, in)
	if err != nil {
		return nil, mapErr(err)
	}
	obj := &files.Object{Key: key, Metadata: meta}
	if out.ETag != nil {
		obj.ETag = *out.ETag
	}
	if ct := in.ContentType; ct != nil {
		obj.ContentType = *ct
	}
	return obj, nil
}

// Get returns the object body. Caller must Close.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := c.requireBucket(); err != nil {
		return nil, err
	}
	out, err := c.sdk.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out.Body, nil
}

// Head returns metadata for a single object.
func (c *Client) Head(ctx context.Context, key string) (*files.Object, error) {
	if err := c.requireBucket(); err != nil {
		return nil, err
	}
	out, err := c.sdk.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	obj := &files.Object{Key: key, Metadata: out.Metadata}
	if out.ContentLength != nil {
		obj.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		obj.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		obj.ETag = *out.ETag
	}
	if out.LastModified != nil {
		obj.LastModified = *out.LastModified
	}
	return obj, nil
}

// Delete removes an object. Idempotent at the S3 layer.
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.requireBucket(); err != nil {
		return err
	}
	_, err := c.sdk.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// List returns all objects with the given prefix (paginated automatically).
func (c *Client) List(ctx context.Context, prefix string) ([]*files.Object, error) {
	if err := c.requireBucket(); err != nil {
		return nil, err
	}
	var out []*files.Object
	in := &awss3.ListObjectsV2Input{Bucket: aws.String(c.bucket)}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	p := awss3.NewListObjectsV2Paginator(c.sdk, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, mapErr(err)
		}
		for _, o := range page.Contents {
			obj := &files.Object{}
			if o.Key != nil {
				obj.Key = *o.Key
			}
			if o.Size != nil {
				obj.Size = *o.Size
			}
			if o.ETag != nil {
				obj.ETag = *o.ETag
			}
			if o.LastModified != nil {
				obj.LastModified = *o.LastModified
			}
			out = append(out, obj)
		}
	}
	return out, nil
}

// Presign returns a time-limited GET URL.
func (c *Client) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := c.requireBucket(); err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", errors.New("s3: ttl must be > 0")
	}
	if ttl > MaxPresignTTL {
		ttl = MaxPresignTTL
	}
	ps := awss3.NewPresignClient(c.sdk)
	req, err := ps.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, awss3.WithPresignExpires(ttl))
	if err != nil {
		return "", mapErr(err)
	}
	return req.URL, nil
}

// PresignUpload returns a time-limited PUT URL.
func (c *Client) PresignUpload(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	if err := c.requireBucket(); err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", errors.New("s3: ttl must be > 0")
	}
	if ttl > MaxPresignTTL {
		ttl = MaxPresignTTL
	}
	in := &awss3.PutObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	ps := awss3.NewPresignClient(c.sdk)
	req, err := ps.PresignPutObject(ctx, in, awss3.WithPresignExpires(ttl))
	if err != nil {
		return "", mapErr(err)
	}
	return req.URL, nil
}

// mapErr maps AWS API errors to files.* sentinel errors when possible.
// The original error is wrapped with %w so callers retain the SDK detail.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return fmt.Errorf("%w: %v", files.ErrNotFound, err)
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return fmt.Errorf("%w: %v", files.ErrNotFound, err)
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NoSuchBucket", "NotFound":
			return fmt.Errorf("%w: %v", files.ErrNotFound, err)
		case "AccessDenied", "Forbidden":
			return fmt.Errorf("%w: %v", files.ErrPermission, err)
		}
	}
	return err
}
