// Package hippius is a thin S3 preset for the Hippius decentralized storage
// gateway (https://docs.hippius.com).
//
// Hippius is S3-compatible (SigV4, ListObjectsV2, multipart, presigned URLs),
// but requires path-style addressing and a fixed region string. This package
// returns a *s3.Client (from omniaura/go-files/s3) preconfigured with those
// defaults; the returned client implements files.Storage.
package hippius

import (
	"context"
	"fmt"

	"github.com/omniaura/go-files/s3"
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
	// Bucket is the target bucket. Optional at construction; can be bound
	// later via (*s3.Client).WithBucket.
	Bucket string
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
	return s3.NewClient(ctx, s3.Options{
		Bucket:          opts.Bucket,
		Region:          Region,
		Endpoint:        endpoint,
		AccessKeyID:     opts.AccessKeyID,
		SecretAccessKey: opts.SecretAccessKey,
		UsePathStyle:    true,
	})
}
