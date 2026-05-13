// Package hippius configures an AWS S3 client for the Hippius decentralized
// storage gateway (https://docs.hippius.com).
//
// Hippius is S3-compatible (SigV4, ListObjectsV2, multipart, presigned URLs),
// but requires path-style addressing and a fixed region string. There is no
// official Go SDK; this package returns a *s3.Client from aws-sdk-go-v2 with
// the correct defaults applied.
package hippius

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Endpoints exposed by the Hippius S3 gateway. All point at the same data;
// the regional endpoints offer lower-latency caches.
const (
	EndpointDefault    = "https://s3.hippius.com"
	EndpointEUCentral1 = "https://eu-central-1.hippius.com"
	EndpointUSEast1    = "https://us-east-1.hippius.com"
)

// Region is the fixed region string Hippius requires for SigV4. There is only
// one logical region ("decentralized"); GetBucketLocation always returns this.
const Region = "decentralized"

// Options configures a Hippius S3 client.
type Options struct {
	// AccessKeyID is the Hippius access key (typically prefixed "hip_").
	AccessKeyID string
	// SecretAccessKey is the secret shown once at token creation.
	SecretAccessKey string
	// Endpoint overrides the default gateway. Leave empty for EndpointDefault.
	Endpoint string
}

// NewClient returns an S3 client configured for Hippius:
//   - BaseEndpoint set to the Hippius gateway
//   - Region "decentralized"
//   - UsePathStyle = true (virtual-hosted-style is unsupported)
//   - Static credentials from the supplied access key
func NewClient(ctx context.Context, opts Options) (*s3.Client, error) {
	if opts.AccessKeyID == "" || opts.SecretAccessKey == "" {
		return nil, fmt.Errorf("hippius: AccessKeyID and SecretAccessKey are required")
	}
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = EndpointDefault
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			opts.AccessKeyID, opts.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("hippius: load aws config: %w", err)
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}
