# hippius

S3 client preset for the [Hippius](https://docs.hippius.com) decentralized
storage gateway.

Hippius is S3-compatible (SigV4, path-style addressing, ListObjectsV2,
multipart, presigned URLs up to 7 days). There is no official Go SDK and
none is needed — Hippius is a drop-in replacement for AWS S3 when the
client is configured correctly. This package returns a `*s3.Client` from
`aws-sdk-go-v2` with the right defaults applied.

## Install

```
go get github.com/omniaura/go-files/hippius
```

## Usage

```go
import (
    "context"

    "github.com/omniaura/go-files/hippius"
)

client, err := hippius.NewClient(ctx, hippius.Options{
    AccessKeyID:     os.Getenv("HIPPIUS_ACCESS_KEY"),
    SecretAccessKey: os.Getenv("HIPPIUS_SECRET_KEY"),
})
```

The returned client is a standard `github.com/aws/aws-sdk-go-v2/service/s3.Client`
with `BaseEndpoint`, `Region` (`decentralized`), and `UsePathStyle` already set.

## Endpoints

| Constant            | URL                                |
| ------------------- | ---------------------------------- |
| `EndpointDefault`   | `https://s3.hippius.com`           |
| `EndpointEUCentral1`| `https://eu-central-1.hippius.com` |
| `EndpointUSEast1`   | `https://us-east-1.hippius.com`    |

All endpoints serve the same data; regional endpoints are lower-latency caches.

## Quirks vs AWS S3

- **Path-style only.** Virtual-hosted-style URLs (`bucket.s3.hippius.com`) break.
- **One region.** Always `decentralized`. `GetBucketLocation` returns this literal string.
- **No versioning.** Objects are overwritten in place.
- **No S3 Select, no Object Lock, no Lambda notifications.**
- **Presigned URLs.** GET/PUT/DELETE supported, max 7-day expiry.
- **S4 extension.** Hippius offers atomic O(delta) `append` with CAS — see
  the [Hippius S3 docs](https://github.com/thenervelab/hippius-s3/blob/main/docs/s4.md).
  Not exposed by this package (use the underlying HTTP client directly).

## Credentials

1. Sign up at https://console.hippius.com (Google or GitHub OAuth).
2. Console → Files → S3 Storage → Create Master Token.
3. Copy the `hip_…` Access Key ID and the Secret Access Key (shown once).

For per-bucket scoping, create **Sub Tokens** with Read-Only or Read+Write
permissions on specific buckets.
