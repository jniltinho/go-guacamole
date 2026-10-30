//go:build !noobjstore

package objstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/rclone/rclone/backend/memory"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/object"
)

func memRemote(t *testing.T) *Remote {
	t.Helper()
	Setup(nil)
	f, err := fs.NewFs(context.Background(), ":memory:bucket/"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	return &Remote{f: f}
}

func localFile(t *testing.T, content string) string {
	p := filepath.Join(t.TempDir(), "rec")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPutVerifyOpenList(t *testing.T) {
	r, ctx := memRemote(t), context.Background()
	st, err := r.Put(ctx, "inst/2026/10/30/4812/abc", localFile(t, "screen recording bytes"))
	if err != nil || st.Size != 22 || len(st.SHA256) != 64 {
		t.Fatalf("put: %+v %v", st, err)
	}
	rc, n, err := r.Open(ctx, st.Key)
	if err != nil || n != st.Size {
		t.Fatalf("open: %d %v", n, err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "screen recording bytes" {
		t.Fatalf("content %q", b)
	}
	es, err := r.List(ctx, "inst")
	if err != nil || len(es) != 1 || es[0].Key != st.Key {
		t.Fatalf("list: %+v %v", es, err)
	}
	// the object changes in storage after the upload: verification catches it
	info := object.NewStaticObjectInfo(st.Key, time.Now(), st.Size, true, nil, r.f)
	if _, err := r.f.Put(ctx, bytes.NewReader([]byte("tampered bytes of 22..")), info); err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(ctx, st.Key, st.Size, st.SHA256); !errors.Is(err, ErrMismatch) {
		t.Fatalf("tampered object verified: %v", err)
	}
	if n, err := r.DeleteAll(ctx, "inst/2026/10/30/4812"); err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if es, _ := r.List(ctx, "inst"); len(es) != 0 {
		t.Fatalf("left after delete: %+v", es)
	}
}
