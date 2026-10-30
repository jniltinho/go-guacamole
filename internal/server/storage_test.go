package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rclone/gofakes3"
	"github.com/rclone/gofakes3/s3mem"

	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
)

func TestStorageSettings(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	aud := mkUser(t, a.Store, "aud", "AudPass#1", false)
	_ = a.Store.GrantSystem(ctx, store.SubjectUser, aud.ID, store.PermAudit)
	admin := login(t, srv.URL, "root", "RootPass#1")

	st, out := admin.do("GET", "/api/v1/settings/recording/storage", nil)
	if st != 200 || out["keepLocal"] != true || out["available"] != false || !strings.Contains(out["unavailableReason"].(string), "job queue") {
		t.Fatalf("defaults: %d %v", st, out)
	}
	target := map[string]any{"provider": "s3", "endpoint": "https://s3.example.com", "bucket": "rec", "prefix": "/gg/",
		"accessKey": "AKIAEXAMPLE", "secretKey": "s3cr3t-value-123"}
	// uploads cannot be enabled without the job queue
	if st, out := admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"enabled": true, "target": target}); st != 409 {
		t.Fatalf("enable without queue: %d %v", st, out)
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"target": map[string]any{"provider": "s3", "bucket": "rec"}}); st != 422 {
		t.Fatalf("missing endpoint: %d", st)
	}
	if st, _ := login(t, srv.URL, "aud", "AudPass#1").do("PUT", "/api/v1/settings/recording/storage", map[string]any{"target": target}); st != 403 {
		t.Fatalf("auditor: %d", st)
	}
	st, out = admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"keepLocal": true, "target": target})
	first, _ := out["targetId"].(string)
	tv, _ := out["target"].(map[string]any)
	set, _ := out["secretsSet"].(map[string]any)
	if st != 200 || first == "" || first == "0" || (tv["secretKey"] != nil && tv["secretKey"] != "") || set["secretKey"] != true || tv["prefix"] != "gg" {
		t.Fatalf("save: %d %v", st, out)
	}
	// new credentials or encryption, same location: same target version; the omitted secret is kept
	target["secretKey"], target["sse"] = "", "AES256"
	_, out = admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"target": target})
	if out["targetId"] != first {
		t.Fatalf("same location changed version: %v", out["targetId"])
	}
	var saved objstore.Target
	id, _ := parseID(first)
	if err := a.Store.GetStorageTarget(ctx, id, &saved); err != nil || saved.SecretKey != "s3cr3t-value-123" || saved.SSE != "AES256" {
		t.Fatalf("kept secret: %+v %v", saved, err)
	}
	// moving to another bucket: a new version, the old one stays readable
	target["bucket"] = "rec2"
	_, out = admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"target": target})
	if out["targetId"] == first {
		t.Fatal("new location reused the target version")
	}
	if err := a.Store.GetStorageTarget(ctx, id, &saved); err != nil || saved.Bucket != "rec" {
		t.Fatalf("previous version: %+v %v", saved, err)
	}
	// local only: no target, previous kept
	_, out = admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"keepLocal": true})
	if out["targetId"] != "0" || out["target"] != nil {
		t.Fatalf("local only: %v", out)
	}
	// audit has no secret
	ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "settings.recording.storage.update"}, store.Page{Limit: 10})
	for _, e := range ev {
		if strings.Contains(strings.Join([]string{e.TargetName, toJSON(e.Metadata)}, " "), "s3cr3t") {
			t.Fatalf("secret in audit: %v", e.Metadata)
		}
	}
	if len(ev) != 4 {
		t.Fatalf("audit events: %d", len(ev))
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// fakeBucket serves an in-memory S3 (signature checked, versioning on) and lets the app reach it.
func fakeBucket(t *testing.T, a *App) string {
	t.Helper()
	mem := s3mem.New()
	if err := mem.CreateBucket(context.Background(), "gg"); err != nil {
		t.Fatal(err)
	}
	_ = mem.SetVersioningConfiguration("gg", gofakes3.VersioningConfiguration{Status: gofakes3.VersioningEnabled})
	srv := httptest.NewServer(gofakes3.New(mem, gofakes3.WithV4Auth(map[string]string{"ggkey": "ggsecret-right"})).Server())
	t.Cleanup(srv.Close)
	a.Cfg.ObjStore.AllowHTTP, a.Cfg.ObjStore.AllowedEndpoints = true, []string{"127.0.0.1"}
	return srv.URL
}

func TestStorageConnectionTest(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	endpoint := fakeBucket(t, a)
	try := func(target map[string]any) map[string]any {
		st, out := admin.do("POST", "/api/v1/settings/recording/storage/test", map[string]any{"target": target})
		if st != 200 {
			t.Fatalf("test: %d %v", st, out)
		}
		return out
	}
	good := map[string]any{"provider": "s3", "endpoint": endpoint, "pathStyle": true, "bucket": "gg", "prefix": "rec",
		"accessKey": "ggkey", "secretKey": "ggsecret-right"}
	if out := try(good); out["ok"] != true || out["versioned"] != true {
		t.Fatalf("right credentials: %v", out)
	}
	good["secretKey"] = "wrong-secret-value"
	if out := try(good); out["ok"] != false || strings.Contains(toJSON(out), "wrong-secret-value") || out["message"] == "" {
		t.Fatalf("wrong secret: %v", out)
	}
	meta := map[string]any{"provider": "s3", "endpoint": "https://169.254.169.254", "bucket": "gg", "accessKey": "k", "secretKey": "s"}
	if out := try(meta); out["ok"] != false || !strings.Contains(out["message"].(string), "not allowed") {
		t.Fatalf("metadata endpoint: %v", out)
	}
}
