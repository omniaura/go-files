package hippius

import (
	"context"
	"strings"
	"testing"
)

func TestNewClient_RequiresCredentials(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(context.Background(), Options{}); err == nil {
		t.Fatal("expected error when credentials are missing, got nil")
	}
	if _, err := NewClient(context.Background(), Options{AccessKeyID: "hip_x"}); err == nil {
		t.Fatal("expected error when secret is missing, got nil")
	}
}

func TestNewClient_DefaultsApplied(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{
		AccessKeyID:     "hip_test",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	opts := cl.Options()
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != EndpointDefault {
		t.Errorf("BaseEndpoint = %v, want %s", opts.BaseEndpoint, EndpointDefault)
	}
	if !opts.UsePathStyle {
		t.Error("UsePathStyle = false, want true (virtual-hosted-style is unsupported)")
	}
	if opts.Region != Region {
		t.Errorf("Region = %q, want %q", opts.Region, Region)
	}
}

func TestNewClient_CustomEndpoint(t *testing.T) {
	t.Parallel()
	cl, err := NewClient(context.Background(), Options{
		AccessKeyID:     "hip_test",
		SecretAccessKey: "secret",
		Endpoint:        EndpointEUCentral1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := cl.Options().BaseEndpoint; got == nil || !strings.Contains(*got, "eu-central-1") {
		t.Errorf("BaseEndpoint = %v, want eu-central-1 host", got)
	}
}
