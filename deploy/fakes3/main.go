// Command fakes3 is an in-memory S3 (gofakes3, versioning on, signatures checked) for the lab and e2e when no MinIO
// image is available: go run ./deploy/fakes3 -addr :19001 -bucket gg -key ggkey -secret ggsecret
package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/rclone/gofakes3"
	"github.com/rclone/gofakes3/s3mem"
)

func main() {
	addr := flag.String("addr", ":19001", "listen address")
	bucket := flag.String("bucket", "gg", "bucket to create (versioning on)")
	key := flag.String("key", "ggkey", "access key")
	secret := flag.String("secret", "ggsecret", "secret key")
	flag.Parse()
	mem := s3mem.New()
	if err := mem.CreateBucket(context.Background(), *bucket); err != nil {
		log.Fatal(err)
	}
	if err := mem.SetVersioningConfiguration(*bucket, gofakes3.VersioningConfiguration{Status: gofakes3.VersioningEnabled}); err != nil {
		log.Fatal(err)
	}
	log.Printf("fake S3 on %s, bucket %q (versioned)", *addr, *bucket)
	log.Fatal(http.ListenAndServe(*addr, gofakes3.New(mem, gofakes3.WithV4Auth(map[string]string{*key: *secret})).Server()))
}
