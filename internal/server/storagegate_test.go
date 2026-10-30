package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-guacamole/internal/store"
)

func TestUploadWaitsForQuietFiles(t *testing.T) {
	dir := t.TempDir()
	rec, timing := filepath.Join(dir, "t1"), filepath.Join(dir, "t1.typescript.timing")
	for _, f := range []string{rec, timing} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	old := now.Add(-3 * time.Minute)
	ended := old.UnixMilli()
	h := &store.HistoryEntry{EndAt: &ended}
	_ = os.Chtimes(rec, old, old)
	// guacd is still flushing the timing file after the session ended
	if err := uploadReady(h, []string{rec, timing}, now); err != errSetChanging {
		t.Fatalf("a growing file must hold the upload: %v", err)
	}
	_ = os.Chtimes(timing, old, old)
	if err := uploadReady(h, []string{rec, timing}, now); err != nil {
		t.Fatalf("quiet set: %v", err)
	}
	if err := uploadReady(&store.HistoryEntry{}, []string{rec}, now); err == nil {
		t.Fatal("an active session is never uploaded")
	}
	if err := uploadReady(h, []string{filepath.Join(dir, "missing")}, now); err == nil {
		t.Fatal("a missing file is not quiet")
	}
}

func TestLocalRemovalWindow(t *testing.T) {
	now := time.Now()
	recent, longAgo := now.Add(-10*time.Minute).UnixMilli(), now.Add(-2*time.Hour).UnixMilli()
	held := now.UnixMilli()
	cases := []struct {
		h    store.HistoryEntry
		want bool
	}{
		{store.HistoryEntry{EndAt: &recent}, false},                 // inside the window
		{store.HistoryEntry{EndAt: &longAgo}, true},                 // after the window
		{store.HistoryEntry{EndAt: &longAgo, HoldAt: &held}, false}, // legal hold
		{store.HistoryEntry{}, false},                               // still active
	}
	for i, c := range cases {
		if got := localRemovalAllowed(&c.h, now); got != c.want {
			t.Fatalf("case %d: %v", i, got)
		}
	}
}
