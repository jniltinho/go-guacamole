//go:build !noobjstore

package objstore

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rclone/gofakes3"
	"github.com/rclone/gofakes3/s3mem"
)

// lockedBackend refuses deleting versions, like a bucket whose object lock (COMPLIANCE) still protects them.
type lockedBackend struct{ *s3mem.Backend }

func (b lockedBackend) DeleteObjectVersion(bucket, object string, v gofakes3.VersionID) (gofakes3.ObjectDeleteResult, error) {
	return gofakes3.ObjectDeleteResult{}, gofakes3.ErrorCode("AccessDenied")
}

// fakeS3 serves an in-memory S3 with versioning on bucket "gg" and returns a connected Remote.
func fakeS3(t *testing.T, locked bool) *Remote {
	t.Helper()
	mem := s3mem.New()
	ctx := context.Background()
	if err := mem.CreateBucket(ctx, "gg"); err != nil {
		t.Fatal(err)
	}
	if err := mem.SetVersioningConfiguration("gg", gofakes3.VersioningConfiguration{Status: gofakes3.VersioningEnabled}); err != nil {
		t.Fatal(err)
	}
	var backend gofakes3.Backend = mem
	if locked {
		backend = lockedBackend{mem}
	}
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(srv.Close)
	r, err := Connect(ctx, Target{Provider: S3, Endpoint: srv.URL, PathStyle: true, Bucket: "gg", Prefix: "rec",
		AccessKey: "ggkey", SecretKey: "ggsecret-value"}, Policy{AllowHTTP: true, Allowed: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, content string) string {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A retried upload leaves one version per key; deleting a set removes every version, not behind delete markers.
func TestVersionedBucket(t *testing.T) {
	r, ctx := fakeS3(t, false), context.Background()
	key := "inst/2026/10/30/4812/abc"
	if _, err := r.Put(ctx, key, write(t, "first attempt")); err != nil {
		t.Fatal(err)
	}
	second, err := r.Put(ctx, key, write(t, "second attempt"))
	if err != nil {
		t.Fatal(err)
	}
	if all, _ := r.all(ctx, "inst"); len(all) != 2 {
		t.Fatalf("versions after two attempts: %d", len(all))
	}
	if n, err := r.PruneVersions(ctx, "inst/2026/10/30/4812"); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	if err := r.Verify(ctx, key, second.Size, second.SHA256); err != nil {
		t.Fatalf("current version after prune: %v", err)
	}
	if n, err := r.DeleteAll(ctx, "inst/2026/10/30/4812"); err != nil || n != 1 {
		t.Fatalf("delete all: %d %v", n, err)
	}
	if all, _ := r.all(ctx, "inst"); len(all) != 0 {
		t.Fatalf("versions or delete markers left: %d", len(all))
	}
}

// A provider refusing deletion (object lock) is reported, never counted as deleted.
func TestLockedBucketRefusesDeletion(t *testing.T) {
	r, ctx := fakeS3(t, true), context.Background()
	if _, err := r.Put(ctx, "inst/2026/10/30/1/abc", write(t, "kept by the lock")); err != nil {
		t.Fatal(err)
	}
	n, err := r.DeleteAll(ctx, "inst/2026/10/30/1")
	if err == nil || n != 0 || strings.Contains(err.Error(), "ggsecret-value") {
		t.Fatalf("locked deletion: %d %v", n, err)
	}
	if all, _ := r.all(ctx, "inst"); len(all) != 1 {
		t.Fatalf("object gone despite the lock: %d", len(all))
	}
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	res, err := fakeS3(t, false).Probe(ctx, "inst/.test")
	if err != nil || !res.Versioned || res.DeleteRefused {
		t.Fatalf("versioned bucket probe: %+v %v", res, err)
	}
	res, err = fakeS3(t, true).Probe(ctx, "inst/.test")
	if err == nil || !res.DeleteRefused {
		t.Fatalf("locked bucket probe: %+v %v", res, err)
	}
}
