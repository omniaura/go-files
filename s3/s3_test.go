package s3

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/omniaura/go-files/files"
)

func TestNewClient_RequiresRegion(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(context.Background(), Options{}); err == nil {
		t.Fatal("expected error when region missing, got nil")
	}
}

func TestNewClient_CredentialPairing(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(context.Background(), Options{Region: "us-east-1", AccessKeyID: "key"}); err == nil {
		t.Fatal("expected error when only AccessKeyID set, got nil")
	}
	if _, err := NewClient(context.Background(), Options{Region: "us-east-1", SecretAccessKey: "secret"}); err == nil {
		t.Fatal("expected error when only SecretAccessKey set, got nil")
	}
}

func TestNewClient_DefaultsApplied(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{
		Bucket:          "bkt",
		Region:          "us-east-1",
		Endpoint:        "https://example.invalid",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	opts := cl.SDK().Options()
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != "https://example.invalid" {
		t.Errorf("BaseEndpoint = %v, want https://example.invalid", opts.BaseEndpoint)
	}
	if !opts.UsePathStyle {
		t.Error("UsePathStyle = false, want true")
	}
	if cl.Bucket() != "bkt" {
		t.Errorf("Bucket() = %q, want bkt", cl.Bucket())
	}
}

func TestRequireBucket(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := cl.Get(context.Background(), "k"); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Errorf("Get without bucket err = %v, want bucket-not-configured", err)
	}
	bound := cl.WithBucket("after-the-fact")
	if bound.Bucket() != "after-the-fact" {
		t.Errorf("WithBucket().Bucket() = %q, want after-the-fact", bound.Bucket())
	}
	if cl.Bucket() != "" {
		t.Errorf("WithBucket mutated original; Bucket() = %q, want empty", cl.Bucket())
	}
}

func TestPresign_TTLValidation(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{
		Bucket:          "bkt",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := cl.Presign(context.Background(), "k", 0); err == nil {
		t.Error("Presign(ttl=0) err = nil, want non-nil")
	}
	if _, err := cl.PresignUpload(context.Background(), "k", "text/plain", -time.Second); err == nil {
		t.Error("PresignUpload(ttl<0) err = nil, want non-nil")
	}
}

func TestPresign_ClampsToMax(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{
		Bucket:          "bkt",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Beyond AWS's 7-day max — should still succeed (clamped, not rejected).
	url, err := cl.Presign(context.Background(), "k", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Presign 30d: %v", err)
	}
	if url == "" {
		t.Error("Presign returned empty URL")
	}
}

// fakeAPIErr lets us exercise mapErr without round-tripping S3.
type fakeAPIErr struct {
	code, msg string
}

func (e *fakeAPIErr) ErrorCode() string             { return e.code }
func (e *fakeAPIErr) ErrorMessage() string          { return e.msg }
func (e *fakeAPIErr) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }
func (e *fakeAPIErr) Error() string                 { return e.code + ": " + e.msg }

var _ smithy.APIError = (*fakeAPIErr)(nil)

func TestMapErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"nil", nil, nil},
		{"NoSuchKey typed", &types.NoSuchKey{}, files.ErrNotFound},
		{"NotFound typed", &types.NotFound{}, files.ErrNotFound},
		{"AccessDenied code", &fakeAPIErr{code: "AccessDenied", msg: "denied"}, files.ErrPermission},
		{"Forbidden code", &fakeAPIErr{code: "Forbidden", msg: "denied"}, files.ErrPermission},
		{"NoSuchBucket code", &fakeAPIErr{code: "NoSuchBucket", msg: "missing"}, files.ErrNotFound},
		{"unknown passthrough", errors.New("boom"), nil},
	}
	for _, tc := range cases {
		got := mapErr(tc.in)
		if tc.want == nil {
			if tc.in == nil && got != nil {
				t.Errorf("%s: nil-in got = %v", tc.name, got)
			}
			continue
		}
		if !errors.Is(got, tc.want) {
			t.Errorf("%s: got = %v, want errors.Is %v", tc.name, got, tc.want)
		}
	}
}
