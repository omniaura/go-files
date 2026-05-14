// Package files defines a backend-agnostic interface for object storage.
//
// The Storage interface is implemented by every backend module in this
// repository (s3, b2, hippius via s3 preset). Consumers depend on
// *files.Client and swap backends underneath.
package files

import (
	"context"
	"errors"
	"io"
	"time"
)

// Object describes a stored object's metadata.
type Object struct {
	Key          string
	ContentType  string
	ETag         string
	Size         int64
	Metadata     map[string]string
	LastModified time.Time
}

// Storage is the contract every backend implements.
type Storage interface {
	Upload(ctx context.Context, key string, r io.Reader, meta map[string]string) (*Object, error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Head(ctx context.Context, key string) (*Object, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]*Object, error)
	// Presign returns a time-limited GET URL. Implementations that cannot
	// produce a presigned URL (e.g. native B2) must return ErrUnsupported.
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
	// PresignUpload returns a time-limited PUT URL. Same ErrUnsupported rule.
	PresignUpload(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
}

// Sentinel errors returned by Storage implementations and the Client facade.
// Backend implementations should wrap these with %w so callers can use
// errors.Is for control flow.
var (
	ErrNotFound    = errors.New("files: not found")
	ErrUnsupported = errors.New("files: operation unsupported")
	ErrPermission  = errors.New("files: permission denied")
)

// Client is a thin facade over any Storage backend. It exists so callers
// can pin to a single concrete type and swap backends without touching
// call sites.
type Client struct {
	s Storage
}

// New wraps s into a Client.
func New(s Storage) *Client { return &Client{s: s} }

// Storage returns the underlying backend.
func (c *Client) Storage() Storage { return c.s }

// Upload puts an object using the io.Reader as body.
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, meta map[string]string) (*Object, error) {
	return c.s.Upload(ctx, key, r, meta)
}

// Get returns the object body. Caller must Close.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.s.Get(ctx, key)
}

// Head returns metadata for a single object.
func (c *Client) Head(ctx context.Context, key string) (*Object, error) {
	return c.s.Head(ctx, key)
}

// Delete removes an object.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.s.Delete(ctx, key)
}

// List returns all objects with the given prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]*Object, error) {
	return c.s.List(ctx, prefix)
}

// Presign returns a time-limited GET URL.
func (c *Client) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return c.s.Presign(ctx, key, ttl)
}

// PresignUpload returns a time-limited PUT URL.
func (c *Client) PresignUpload(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	return c.s.PresignUpload(ctx, key, contentType, ttl)
}
