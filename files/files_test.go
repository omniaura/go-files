package files

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// memStorage is an in-memory Storage used to exercise the Client facade.
type memStorage struct {
	objs map[string][]byte
	meta map[string]map[string]string
}

func newMem() *memStorage {
	return &memStorage{objs: map[string][]byte{}, meta: map[string]map[string]string{}}
}

func (m *memStorage) Upload(_ context.Context, key string, r io.Reader, meta map[string]string) (*Object, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	m.objs[key] = b
	m.meta[key] = meta
	return &Object{Key: key, Size: int64(len(b)), Metadata: meta, LastModified: time.Now()}, nil
}

func (m *memStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memStorage) Head(_ context.Context, key string) (*Object, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return &Object{Key: key, Size: int64(len(b)), Metadata: m.meta[key]}, nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	delete(m.objs, key)
	delete(m.meta, key)
	return nil
}

func (m *memStorage) List(_ context.Context, prefix string) ([]*Object, error) {
	var out []*Object
	for k, v := range m.objs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, &Object{Key: k, Size: int64(len(v))})
		}
	}
	return out, nil
}

func (m *memStorage) Presign(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", ErrUnsupported
}

func (m *memStorage) PresignUpload(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", ErrUnsupported
}

// Compile-time assertion that memStorage satisfies Storage.
var _ Storage = (*memStorage)(nil)

func TestClientDelegates(t *testing.T) {
	t.Parallel()
	c := New(newMem())
	ctx := context.Background()

	if _, err := c.Upload(ctx, "k", strings.NewReader("hello"), map[string]string{"k": "v"}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	rc, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "hello" {
		t.Errorf("body = %q, want %q", body, "hello")
	}

	h, err := c.Head(ctx, "k")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if h.Size != 5 {
		t.Errorf("Size = %d, want 5", h.Size)
	}

	objs, err := c.List(ctx, "k")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objs) != 1 {
		t.Errorf("List len = %d, want 1", len(objs))
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete err = %v, want ErrNotFound", err)
	}
}

func TestPresignSentinels(t *testing.T) {
	t.Parallel()
	c := New(newMem())
	if _, err := c.Presign(context.Background(), "k", time.Minute); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Presign err = %v, want ErrUnsupported", err)
	}
	if _, err := c.PresignUpload(context.Background(), "k", "text/plain", time.Minute); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PresignUpload err = %v, want ErrUnsupported", err)
	}
}

func TestClientStorageAccessor(t *testing.T) {
	t.Parallel()
	m := newMem()
	c := New(m)
	if c.Storage() == nil {
		t.Fatal("Storage() returned nil")
	}
	if _, ok := c.Storage().(*memStorage); !ok {
		t.Errorf("Storage() type = %T, want *memStorage", c.Storage())
	}
}
