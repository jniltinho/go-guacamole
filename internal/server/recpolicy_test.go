package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

func recordingApp(t *testing.T) (*App, *tclient, string, string) {
	a, srv, _ := newTestApp(t)
	dir := t.TempDir()
	a.Cfg.Recording = config.RecordingConfig{Enabled: true, Path: dir, Screen: true, Typescript: true, RetentionDays: 90}
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	st, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "srv", "protocol": "ssh",
		"parameters": map[string]any{"hostname": "10.0.0.9"}})
	if st != 201 {
		t.Fatalf("create: %d %v", st, out)
	}
	return a, admin, srv.URL, out["id"].(string)
}

func TestRecordingLaunchPolicy(t *testing.T) {
	a, admin, base, id := recordingApp(t)
	ctx := context.Background()

	st, out := admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id})
	rec, _ := out["recording"].(map[string]any)
	if st != 201 || rec["video"] != true || rec["text"] != true || rec["keys"] != false {
		t.Fatalf("launch snapshot: %d %v", st, out)
	}
	hs, _, _ := a.Store.ListHistory(ctx, store.HistoryFilter{}, store.Page{Limit: 10})
	if len(hs) != 1 || hs[0].VideoCapture != store.CapturePending || hs[0].Recording == "" || hs[0].EndAt != nil {
		t.Fatalf("history at launch: %+v", hs)
	}

	// keystrokes exist only inside a video recording
	if st, out := admin.do("PUT", "/api/v1/connections/"+id+"/recording", map[string]any{"video": "off", "text": "inherit", "keys": "on", "version": 0}); st != 422 {
		t.Fatalf("keys without video: %d %v", st, out)
	}
	// stale version
	if st, _ := admin.do("PUT", "/api/v1/connections/"+id+"/recording", map[string]any{"video": "on", "version": 7}); st != 409 {
		t.Fatalf("version conflict: %d", st)
	}
	// shortening a retention needs a confirmed preview
	body := map[string]any{"video": "on", "text": "off", "keys": "inherit", "retentionVideoDays": 30, "version": 0}
	st, out = admin.do("PUT", "/api/v1/connections/"+id+"/recording", body)
	if st != 409 || out["code"] != "RETENTION_PREVIEW" {
		t.Fatalf("preview required: %d %v", st, out)
	}
	body["previewToken"] = out["data"].(map[string]any)["token"]
	if st, out = admin.do("PUT", "/api/v1/connections/"+id+"/recording", body); st != 200 || out["text"] != "off" || out["version"].(float64) != 1 {
		t.Fatalf("policy update: %d %v", st, out)
	}
	if st, _ = admin.do("PUT", "/api/v1/connections/"+id+"/recording", body); st != 409 {
		t.Fatal("preview token reused")
	}
	st, out = admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id})
	if rec := out["recording"].(map[string]any); st != 201 || rec["text"] != false || rec["video"] != true {
		t.Fatalf("override applied: %d %v", st, out)
	}
	ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "connection.recording.update"}, store.Page{Limit: 5})
	if len(ev) != 1 || ev[0].Metadata["text"] != "inherit → off" {
		t.Fatalf("audit: %+v", ev)
	}

	// an editor who is not an auditor cannot change recording nor the guacd endpoint
	mkUser(t, a.Store, "ed", "EdPass#123", false)
	edUser, _ := a.Store.GetUserByName(ctx, "ed")
	_ = a.Store.GrantSystem(ctx, store.SubjectUser, edUser.ID, store.PermCreateConnection)
	ed := login(t, base, "ed", "EdPass#123")
	st, out = ed.do("POST", "/api/v1/connections", map[string]any{"name": "mine", "protocol": "ssh", "parameters": map[string]any{"hostname": "h"}})
	if st != 201 {
		t.Fatalf("editor create: %d %v", st, out)
	}
	mine := out["id"].(string)
	if st, _ := ed.do("PUT", "/api/v1/connections/"+mine+"/recording", map[string]any{"video": "off", "version": 0}); st != 403 {
		t.Fatalf("editor changed recording: %d", st)
	}
	if st, _ := ed.do("PUT", "/api/v1/connections/"+mine, map[string]any{"name": "mine", "protocol": "ssh", "guacdHost": "evil",
		"parameters": map[string]any{"hostname": "h"}}); st != 403 {
		t.Fatalf("editor changed guacd host: %d", st)
	}
	if st, out := ed.do("PUT", "/api/v1/connections/"+mine, map[string]any{"name": "mine", "protocol": "ssh",
		"parameters": map[string]any{"hostname": "h", "recording-path": "/tmp"}}); st != 422 {
		t.Fatalf("recording parameter accepted: %d %v", st, out)
	}
}

func TestRecordingFailClosedAndException(t *testing.T) {
	a, admin, _, id := recordingApp(t)
	a.Cfg.Recording.Path = filepath.Join(t.TempDir(), "missing", "dir")
	a.resetAvailability()
	st, out := admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id})
	if st != 503 || out["code"] != "RECORDING_UNAVAILABLE" {
		t.Fatalf("fail closed: %d %v", st, out)
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/recording/exception", map[string]any{"minutes": 60}); st != 422 {
		t.Fatal("exception without reason accepted")
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/recording/exception", map[string]any{"connectionId": id, "minutes": 60, "reason": "storage incident"}); st != 204 {
		t.Fatalf("exception: %d", st)
	}
	st, out = admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id})
	if rec := out["recording"].(map[string]any); st != 201 || rec["exception"] != true {
		t.Fatalf("launch under exception: %d %v", st, out)
	}
	hs, _, _ := a.Store.ListHistory(context.Background(), store.HistoryFilter{}, store.Page{Limit: 5})
	if len(hs) != 1 || !hs[0].RecException || hs[0].Recording != "" || hs[0].VideoCapture != store.CaptureException {
		t.Fatalf("exception history: %+v", hs[0])
	}
}

func TestRecordingDefaultsAdminOnly(t *testing.T) {
	a, admin, base, _ := recordingApp(t)
	ctx := context.Background()
	mkUser(t, a.Store, "aud", "AudPass#12", false)
	u, _ := a.Store.GetUserByName(ctx, "aud")
	_ = a.Store.GrantSystem(ctx, store.SubjectUser, u.ID, store.PermAudit)
	aud := login(t, base, "aud", "AudPass#12")
	body := map[string]any{"video": true, "text": false, "keys": false, "retentionVideoDays": 90, "retentionTextDays": 90}
	if st, _ := aud.do("PUT", "/api/v1/settings/recording", body); st != 403 {
		t.Fatalf("auditor changed defaults: %d", st)
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/recording", map[string]any{"video": true, "retentionVideoDays": 0, "retentionTextDays": 90}); st != 422 {
		t.Fatal("retention 0 accepted")
	}
	st, out := admin.do("PUT", "/api/v1/settings/recording", body)
	if st != 200 || out["defaults"].(map[string]any)["text"] != false || out["fromSettings"] != true {
		t.Fatalf("save defaults: %d %v", st, out)
	}
	// background jobs never create or change the saved policy
	if _, err := a.purgeRecordings(ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ := a.recordingDefaults(ctx); !d.Saved || d.Text {
		t.Fatalf("defaults after purge: %+v", d)
	}
}

func TestRecordingRetentionPurgeAndHold(t *testing.T) {
	a, admin, _, id := recordingApp(t)
	ctx := context.Background()
	dir := a.Cfg.Recording.Path
	connID, _ := parseID(id)
	old := time.Now().AddDate(0, 0, -100).UnixMilli()
	mk := func(tunnelID string, cid *int64) *store.HistoryEntry {
		yes := true
		h := &store.HistoryEntry{Username: "root", ConnectionID: cid, ConnectionName: "srv", Protocol: "ssh", TunnelID: tunnelID,
			StartAt: old, Recording: tunnelID, Typescript: tunnelID + ".typescript", RecVideo: &yes, RecText: &yes,
			VideoCapture: store.CaptureOK, TextCapture: store.CaptureOK}
		if err := a.Store.StartHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
		_ = a.Store.EndHistory(ctx, h.ID, old+1000, "done")
		for _, f := range []string{tunnelID, tunnelID + ".typescript", tunnelID + ".typescript.timing"} {
			_ = os.WriteFile(filepath.Join(dir, f), []byte("data"), 0o600)
		}
		_ = a.Store.SetFiles(ctx, h.ID, true, store.FilesPresent)
		_ = a.Store.SetFiles(ctx, h.ID, false, store.FilesPresent)
		return h
	}
	expired := mk("11111111-1111-1111-1111-111111111111", &connID)
	held := mk("22222222-2222-2222-2222-222222222222", &connID)
	if st, _ := admin.do("PUT", "/api/v1/history/"+idStr(held.ID)+"/hold", map[string]any{"reason": " "}); st != 422 {
		t.Fatal("hold without reason accepted")
	}
	if st, _ := admin.do("PUT", "/api/v1/history/"+idStr(held.ID)+"/hold", map[string]any{"reason": "incident 2026-114"}); st != 204 {
		t.Fatal("hold failed")
	}
	orphan := "33333333-3333-3333-3333-333333333333"
	_ = os.WriteFile(filepath.Join(dir, orphan), []byte("x"), 0o600)
	_ = os.Chtimes(filepath.Join(dir, orphan), time.Now().AddDate(0, 0, -200), time.Now().AddDate(0, 0, -200))
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o600)

	sum, err := a.purgeRecordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Video.Files != 1 || sum.Text.Files != 2 || sum.Orphans.Files != 1 {
		t.Fatalf("summary: %+v", sum)
	}
	h, _ := a.Store.GetHistory(ctx, expired.ID)
	if h.VideoFiles != store.FilesPurged || h.TextFiles != store.FilesPurged || h.HasRecording || h.VideoPurgedAt == nil {
		t.Fatalf("expired entry: %+v", h)
	}
	if _, err := os.Stat(filepath.Join(dir, held.TunnelID+".typescript.timing")); err != nil {
		t.Fatal("held recording deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal("unrelated file deleted")
	}
	// a hold is refused once deletion started
	if ok, _ := a.Store.SetHold(ctx, expired.ID, "late", "root"); ok {
		t.Fatal("hold placed on purged entry")
	}

	// keep forever on the connection, then delete the connection: never purged
	other := mk("44444444-4444-4444-4444-444444444444", &connID)
	conn, _ := a.Store.GetConnection(ctx, connID)
	forever := keepForever
	if ok, err := a.Store.SetConnectionRecording(ctx, connID, store.RecordingPolicy{RetentionVideoDays: &forever, RetentionTextDays: &forever}, conn.RecVersion); !ok || err != nil {
		t.Fatalf("set forever: %v %v", ok, err)
	}
	if st, _ := admin.do("DELETE", "/api/v1/connections/"+id, nil); st != 204 {
		t.Fatal("delete connection")
	}
	if _, err := a.purgeRecordings(ctx); err != nil {
		t.Fatal(err)
	}
	if h, _ := a.Store.GetHistory(ctx, other.ID); h.VideoFiles != store.FilesPresent || h.RetentionVideoDays == nil || *h.RetentionVideoDays != keepForever {
		t.Fatalf("retention after deletion: %+v", h)
	}
}

func TestRecordingCaptureVerification(t *testing.T) {
	a, _, _, id := recordingApp(t)
	ctx := context.Background()
	a.captureGrace = 10 * time.Millisecond
	connID, _ := parseID(id)
	yes := true
	tn := &tunnel.Tunnel{UserID: 1, Username: "root", ConnectionID: connID, ConnectionName: "srv", RecVideo: true}
	if _, err := a.Registry.Issue(tn); err != nil {
		t.Fatal(err)
	}
	h := &store.HistoryEntry{Username: "root", ConnectionID: &connID, ConnectionName: "srv", Protocol: "ssh", TunnelID: tn.ID,
		StartAt: time.Now().UnixMilli(), Recording: tn.ID, RecVideo: &yes, VideoCapture: store.CapturePending}
	_ = a.Store.StartHistory(ctx, h)
	tn.HistoryID = h.ID
	a.verifyCaptureSoon(tn) // guacd wrote nothing
	if got, _ := a.Store.GetHistory(ctx, h.ID); got.VideoCapture != store.CaptureFailed {
		t.Fatalf("capture not failed: %+v", got)
	}
	if tn.Context().Err() == nil {
		t.Fatal("unrecorded session not closed")
	}
	ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "recording.failed"}, store.Page{Limit: 5})
	if len(ev) != 1 {
		t.Fatalf("audit: %+v", ev)
	}
}

func TestRecordingUsageAuditKeepAndVisibility(t *testing.T) {
	a, admin, base, id := recordingApp(t)
	ctx := context.Background()
	connID, _ := parseID(id)
	dir := a.Cfg.Recording.Path
	tid := "55555555-5555-5555-5555-555555555555"
	_ = os.WriteFile(filepath.Join(dir, tid), make([]byte, 1000), 0o600)
	_ = os.WriteFile(filepath.Join(dir, tid+".typescript"), make([]byte, 300), 0o600)
	h := &store.HistoryEntry{Username: "root", ConnectionID: &connID, ConnectionName: "srv", Protocol: "ssh", TunnelID: tid, StartAt: 1}
	_ = a.Store.StartHistory(ctx, h)
	_ = a.Store.EndHistory(ctx, h.ID, 2, "done")
	u, err := a.computeUsage(ctx)
	if err != nil || u.Video.Bytes != 1000 || u.Text.Bytes != 300 || len(u.Top) != 1 || u.Top[0].Bytes != 1300 {
		t.Fatalf("usage: %+v %v", u, err)
	}

	// audit retention keeps the events of a held entry's connection
	_ = a.Store.InsertAudit(ctx, &store.AuditEvent{At: 10, ActorName: "x", Action: "tunnel.open", TargetType: "connection", TargetID: id, Outcome: "success"})
	_ = a.Store.InsertAudit(ctx, &store.AuditEvent{At: 10, ActorName: "x", Action: "tunnel.open", TargetType: "connection", TargetID: "999", Outcome: "success"})
	if ok, _ := a.Store.SetHold(ctx, h.ID, "case 7", "root"); !ok {
		t.Fatal("hold")
	}
	if n, err := a.purgeAuditKeepingHolds(ctx, 100); err != nil || n != 1 {
		t.Fatalf("audit purge removed %d (%v), want only the unrelated event", n, err)
	}

	// a pure auditor (no edit right) changes the policy but not the connection; reasons hidden from others
	mkUser(t, a.Store, "aud", "AudPass#12", false)
	au, _ := a.Store.GetUserByName(ctx, "aud")
	_ = a.Store.GrantSystem(ctx, store.SubjectUser, au.ID, store.PermAudit)
	aud := login(t, base, "aud", "AudPass#12")
	if st, out := aud.do("PUT", "/api/v1/connections/"+id+"/recording", map[string]any{"video": "on", "text": "on", "keys": "on", "version": 0}); st != 200 {
		t.Fatalf("auditor policy: %d %v", st, out)
	}
	if st, _ := aud.do("PUT", "/api/v1/connections/"+id, map[string]any{"name": "renamed", "protocol": "ssh", "parameters": map[string]any{"hostname": "h"}}); st != 403 && st != 404 {
		t.Fatalf("auditor edited the connection: %d", st)
	}
	if _, out := aud.do("GET", "/api/v1/history?connectionId="+id, nil); out["items"].([]any)[0].(map[string]any)["holdReason"] != "case 7" {
		t.Fatal("auditor cannot see the hold reason")
	}
	mkUser(t, a.Store, "plain", "PlainPass#1", false)
	pu, _ := a.Store.GetUserByName(ctx, "plain")
	h2 := &store.HistoryEntry{UserID: &pu.ID, Username: "plain", ConnectionName: "srv", Protocol: "ssh", TunnelID: "t-plain", StartAt: 5}
	_ = a.Store.StartHistory(ctx, h2)
	_ = a.Store.EndHistory(ctx, h2.ID, 6, "done")
	_, _ = a.Store.SetHold(ctx, h2.ID, "secret case", "root")
	_, out := login(t, base, "plain", "PlainPass#1").do("GET", "/api/v1/history", nil)
	if r, ok := out["items"].([]any)[0].(map[string]any)["holdReason"]; ok && r != nil {
		t.Fatalf("hold reason leaked to the user: %v", r)
	}
	_ = admin
}
