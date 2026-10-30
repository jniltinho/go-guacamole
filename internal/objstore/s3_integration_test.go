//go:build !noobjstore

package objstore

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Against a real S3-compatible server (RustFS, MinIO...): GG_TEST_S3="http://127.0.0.1:19100 key secret".
func realS3(t *testing.T) (endpoint, key, secret string) {
	f := strings.Fields(os.Getenv("GG_TEST_S3"))
	if len(f) != 3 {
		t.Skip(`set GG_TEST_S3="<endpoint> <access key> <secret key>" to test against a real S3 server`)
	}
	return f[0], f[1], f[2]
}

// mkBucket creates a bucket with versioning, or with object lock (COMPLIANCE, 1 day) when locked.
func mkBucket(t *testing.T, endpoint, key, secret, name string, locked bool) {
	t.Helper()
	ctx := context.Background()
	c := s3.New(s3.Options{Region: "us-east-1", BaseEndpoint: aws.String(endpoint), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(key, secret, "")})
	_, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name), ObjectLockEnabledForBucket: aws.Bool(locked)})
	if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		t.Fatalf("create bucket %s: %v", name, err)
	}
	if _, err := c.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(name),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatalf("versioning %s: %v", name, err)
	}
	if locked {
		if _, err := c.PutObjectLockConfiguration(ctx, &s3.PutObjectLockConfigurationInput{Bucket: aws.String(name),
			ObjectLockConfiguration: &types.ObjectLockConfiguration{ObjectLockEnabled: types.ObjectLockEnabledEnabled,
				Rule: &types.ObjectLockRule{DefaultRetention: &types.DefaultRetention{Mode: types.ObjectLockRetentionModeCompliance, Days: aws.Int32(1)}}}}); err != nil {
			t.Fatalf("object lock %s: %v", name, err)
		}
	}
}

func realRemote(t *testing.T, bucket string, locked bool) *Remote {
	endpoint, key, secret := realS3(t)
	mkBucket(t, endpoint, key, secret, bucket, locked)
	r, err := Connect(context.Background(), Target{Provider: S3, Endpoint: endpoint, PathStyle: true, Bucket: bucket,
		Prefix: "it-" + t.Name(), AccessKey: key, SecretKey: secret}, Policy{AllowHTTP: true, Allowed: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRealS3Versioned(t *testing.T) {
	r, ctx := realRemote(t, "gg-ver", false), context.Background()
	key := "inst/2026/10/30/7/rec"
	if _, err := r.Put(ctx, key, write(t, "attempt one")); err != nil {
		t.Fatal(err)
	}
	second, err := r.Put(ctx, key, write(t, "attempt two, longer"))
	if err != nil {
		t.Fatal(err)
	}
	if all, _ := r.all(ctx, "inst"); len(all) != 2 {
		t.Fatalf("versions after two attempts: %d", len(all))
	}
	if n, err := r.PruneVersions(ctx, "inst/2026/10/30/7"); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	if err := r.Verify(ctx, key, second.Size, second.SHA256); err != nil {
		t.Fatal(err)
	}
	if n, err := r.DeleteAll(ctx, "inst/2026/10/30/7"); err != nil || n != 1 {
		t.Fatalf("delete all: %d %v", n, err)
	}
	if all, _ := r.all(ctx, "inst"); len(all) != 0 {
		t.Fatalf("left: %d", len(all))
	}
	if res, err := r.Probe(ctx, "inst/.test"); err != nil || !res.Versioned {
		t.Fatalf("probe: %+v %v", res, err)
	}
}

func TestRealS3ObjectLock(t *testing.T) {
	r, ctx := realRemote(t, "gg-lock", true), context.Background()
	if _, err := r.Put(ctx, "inst/2026/10/30/8/rec", write(t, "locked for a day")); err != nil {
		t.Fatal(err)
	}
	if n, err := r.DeleteAll(ctx, "inst/2026/10/30/8"); err == nil {
		t.Fatalf("object lock bypassed: %d deleted", n)
	}
	if res, err := r.Probe(ctx, "inst/.test"); err == nil || !res.DeleteRefused {
		t.Fatalf("probe on a locked bucket: %+v %v", res, err)
	}
}
