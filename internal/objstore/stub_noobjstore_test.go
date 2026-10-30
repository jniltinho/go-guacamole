//go:build noobjstore

package objstore

import (
	"context"
	"errors"
	"testing"
)

func TestBuiltWithoutObjectStorage(t *testing.T) {
	if Available {
		t.Fatal("noobjstore build reports object storage")
	}
	if _, err := Connect(context.Background(), Target{Provider: S3, Bucket: "b", Endpoint: "https://x"}, Policy{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("connect: %v", err)
	}
}
