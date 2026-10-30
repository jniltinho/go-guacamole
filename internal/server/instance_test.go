package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// Two replicas share one database: a restart of B never ends A's live session, while rows of a stopped instance
// (and rows written before ownership existed) are ended, and B's launch cleanup leaves A's tickets alone.
func TestReplicaOwnership(t *testing.T) {
	a, _, _ := newTestApp(t)
	ctx := context.Background()
	b := New(a.Cfg, a.Store, slog.New(slog.NewTextHandler(io.Discard, nil)), a.dist)
	if a.Instance == b.Instance {
		t.Fatal("instances share an id")
	}
	now := a.Store.Now()
	open := func(owner string, start int64) *store.HistoryEntry {
		h := &store.HistoryEntry{Username: "u", ConnectionName: "c", Protocol: "ssh", TunnelID: store.RandomHex(8), StartAt: start, OwnerInstance: owner}
		if err := a.Store.StartHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	a.beat(ctx)
	live := open(a.Instance, now)
	pendingA := open(a.Instance, now-int64(10*60*1000)) // A's unused ticket, old enough for A's own cleanup
	if err := a.Store.Heartbeat(ctx, "gone-1", now-InstanceTTL.Milliseconds()-60_000); err != nil {
		t.Fatal(err)
	}
	dead := open("gone-1", now)
	legacy := open("", now)

	b.beat(ctx) // B starts while A serves its session
	b.finishAbandoned(ctx)

	ended := func(h *store.HistoryEntry) bool {
		g, err := a.Store.GetHistory(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		return g.EndAt != nil
	}
	if ended(live) || ended(pendingA) {
		t.Fatal("B ended a row of the live replica A")
	}
	if !ended(dead) || !ended(legacy) {
		t.Fatal("rows of a stopped instance or without owner stay open")
	}
	a.finishAbandoned(ctx) // A cleans its own unused ticket
	if !ended(pendingA) || ended(live) {
		t.Fatal("A's own cleanup")
	}
}

// Without REDIS_ADDR the queue is reported as disabled and the instance stays ready.
func TestReadyReportsQueue(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	res, err := http.Get(srv.URL + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]string
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["queue"] != "disabled" || out["db"] != "up" {
		t.Fatalf("ready: %d %v", res.StatusCode, out)
	}
}
