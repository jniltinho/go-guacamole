package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rclone/gofakes3"
	"github.com/rclone/gofakes3/s3mem"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
)

type lockedS3 struct{ *s3mem.Backend }

func (b lockedS3) DeleteObjectVersion(string, string, gofakes3.VersionID) (gofakes3.ObjectDeleteResult, error) {
	return gofakes3.ObjectDeleteResult{}, gofakes3.ErrorCode("AccessDenied")
}

// storageApp: recording on, uploads enabled to an in-memory S3 (locked: deletions refused like an object lock).
func storageApp(t *testing.T, keepLocal, locked bool) (*App, *s3mem.Backend) {
	t.Helper()
	a, _, _ := newTestApp(t)
	a.Cfg.Recording = config.RecordingConfig{Enabled: true, Path: t.TempDir(), Screen: true, Typescript: true, RetentionDays: 1}
	ctx := context.Background()
	mem := s3mem.New()
	_ = mem.CreateBucket(ctx, "gg")
	_ = mem.SetVersioningConfiguration("gg", gofakes3.VersioningConfiguration{Status: gofakes3.VersioningEnabled})
	var backend gofakes3.Backend = mem
	if locked {
		backend = lockedS3{mem}
	}
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(srv.Close)
	a.Cfg.ObjStore = config.ObjStoreConfig{AllowHTTP: true, AllowedEndpoints: []string{"127.0.0.1"}}
	objstore.Setup(nil)
	tid, err := a.Store.CreateStorageTarget(ctx, objstore.Target{Provider: objstore.S3, Endpoint: srv.URL, PathStyle: true,
		Bucket: "gg", Prefix: "rec", AccessKey: "k", SecretKey: "s3cret-key-value"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSetting(ctx, settingStorage, storageSettings{Enabled: true, KeepLocal: keepLocal, TargetID: tid}); err != nil {
		t.Fatal(err)
	}
	return a, mem
}

// recorded writes an ended SSH session's video and transcript, quiet and ended `ago`.
func recorded(t *testing.T, a *App, ago time.Duration) *store.HistoryEntry {
	t.Helper()
	ctx := context.Background()
	tun := uuid.NewString()
	old := time.Now().Add(-ago)
	for _, name := range []string{tun, tun + ".typescript", tun + ".typescript.timing"} {
		p := filepath.Join(a.Cfg.Recording.Dir(), name)
		if err := os.WriteFile(p, []byte("content of "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, old, old)
	}
	yes := true
	h := &store.HistoryEntry{Username: "u", ConnectionName: "c", Protocol: "ssh", TunnelID: tun, StartAt: old.UnixMilli(),
		Recording: tun, Typescript: tun + ".typescript", RecVideo: &yes, RecText: &yes, OwnerInstance: a.Instance}
	if err := a.Store.StartHistory(ctx, h); err != nil {
		t.Fatal(err)
	}
	_ = a.Store.EndHistory(ctx, h.ID, old.UnixMilli(), "client_closed")
	_ = a.Store.SetCapture(ctx, h.ID, true, store.CaptureOK, store.FilesPresent)
	_ = a.Store.SetCapture(ctx, h.ID, false, store.CaptureOK, store.FilesPresent)
	return h
}

func upload(t *testing.T, a *App, id int64, video bool) error {
	t.Helper()
	return a.handleUpload(context.Background(), []byte(`{"id":"`+idStr(id)+`","video":`+map[bool]string{true: "true", false: "false"}[video]+`}`))
}

func get(t *testing.T, a *App, id int64) *store.HistoryEntry {
	h, err := a.Store.GetHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestUploadPipeline(t *testing.T) {
	a, mem := storageApp(t, true, false)
	ctx := context.Background()
	h := recorded(t, a, 3*time.Minute)
	if n, err := a.Store.MarkUploadsOwed(ctx); err != nil || n != 2 {
		t.Fatalf("owed: %d %v", n, err)
	}
	for _, video := range []bool{true, false} {
		if err := upload(t, a, h.ID, video); err != nil {
			t.Fatal(err)
		}
	}
	g := get(t, a, h.ID)
	if g.VideoStorage != store.StorageRemote || g.TextStorage != store.StorageRemote || !strings.Contains(g.TextRemote, ".typescript.timing") {
		t.Fatalf("after upload: %q %q %s", g.VideoStorage, g.TextStorage, g.TextRemote)
	}
	inst, _ := a.Store.InstallationID(ctx)
	day := time.UnixMilli(h.StartAt).UTC().Format("2006/01/02")
	if _, err := mem.HeadObject(context.Background(), "gg", "rec/"+inst+"/"+day+"/"+idStr(h.ID)+"/"+h.TunnelID); err != nil {
		t.Fatalf("object key layout: %v", err)
	}
	// a deleted row: no retry
	if err := upload(t, a, 999999, true); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("deleted row: %v", err)
	}
}

func TestUploadWaitsForQuietFilesAndActiveSessions(t *testing.T) {
	a, _ := storageApp(t, true, false)
	h := recorded(t, a, 30*time.Second) // files changed 30s ago
	_, _ = a.Store.MarkUploadsOwed(context.Background())
	if err := upload(t, a, h.ID, true); !errors.Is(err, errSetChanging) {
		t.Fatalf("recent files uploaded: %v", err)
	}
	if g := get(t, a, h.ID); g.VideoStorage != store.StorageOwed {
		t.Fatalf("state: %q", g.VideoStorage)
	}
}

func TestUploadFailureIsRecordedWithoutSecrets(t *testing.T) {
	a, _ := storageApp(t, true, false)
	ctx := context.Background()
	set, _ := a.storageSettings(ctx)
	// the provider is down: point the target at a closed server
	dead := httptest.NewServer(nil)
	dead.Close()
	_ = a.Store.UpdateStorageTarget(ctx, set.TargetID, objstore.Target{Provider: objstore.S3, Endpoint: dead.URL, PathStyle: true,
		Bucket: "gg", AccessKey: "k", SecretKey: "s3cret-key-value"})
	h := recorded(t, a, 3*time.Minute)
	_, _ = a.Store.MarkUploadsOwed(ctx)
	if err := upload(t, a, h.ID, true); err == nil {
		t.Fatal("upload to a dead endpoint succeeded")
	}
	g := get(t, a, h.ID)
	if g.VideoStorage != store.StorageFailed || g.VideoStorageError == "" || strings.Contains(g.VideoStorageError, "s3cret") {
		t.Fatalf("failure record: %q %q", g.VideoStorage, g.VideoStorageError)
	}
	// the local files stay
	if _, err := os.Stat(filepath.Join(a.Cfg.Recording.Dir(), h.TunnelID)); err != nil {
		t.Fatalf("local file after a failed upload: %v", err)
	}
}

// Fenced claims: a worker whose claim expired cannot record anything; the purge waits for a valid claim; a legal
// hold is refused during a claim.
func TestClaimsFenceWorkersPurgeAndHolds(t *testing.T) {
	a, _ := storageApp(t, true, false)
	ctx := context.Background()
	h := recorded(t, a, 3*time.Minute)
	_, _ = a.Store.MarkUploadsOwed(ctx)
	ref := store.SetRef{ID: h.ID, Video: true}
	now := a.Store.Now()
	if ok, _ := a.Store.ClaimUpload(ctx, ref, "w1", "tok-1", now, now+60_000); !ok {
		t.Fatal("claim")
	}
	if ok, _ := a.Store.BeginPurge(ctx, h.ID, true); ok {
		t.Fatal("purge started during an upload claim")
	}
	if ok, _ := a.Store.SetHold(ctx, h.ID, "incident", "auditor"); ok {
		t.Fatal("hold placed during a claim")
	}
	// the claim expires; the stale worker's bookkeeping is refused
	later := now + 120_000
	if ok, _ := a.Store.FinishUpload(ctx, ref, "tok-1", later, 1, "[]"); ok {
		t.Fatal("stale worker recorded an upload")
	}
	if err := a.Store.ResetExpiredUploads(ctx, later); err != nil {
		t.Fatal(err)
	}
	if g := get(t, a, h.ID); g.VideoStorage != store.StorageOwed {
		t.Fatalf("expired upload not owed again: %q", g.VideoStorage)
	}
}

// keepLocal off: after the window, verified sets lose their local files (not under hold; a changed file is uploaded
// again); the purge then deletes the remote copies, or keeps them pending when the bucket refuses.
func TestLocalRemovalAndRemotePurge(t *testing.T) {
	a, mem := storageApp(t, false, false)
	ctx := context.Background()
	h := recorded(t, a, 2*time.Hour)
	held := recorded(t, a, 2*time.Hour)
	changed := recorded(t, a, 2*time.Hour)
	_, _ = a.Store.MarkUploadsOwed(ctx)
	for _, x := range []*store.HistoryEntry{h, held, changed} {
		for _, video := range []bool{true, false} {
			if err := upload(t, a, x.ID, video); err != nil {
				t.Fatal(err)
			}
		}
	}
	if ok, _ := a.Store.SetHold(ctx, held.ID, "incident", "auditor"); !ok {
		t.Fatal("hold")
	}
	_ = os.WriteFile(filepath.Join(a.Cfg.Recording.Dir(), changed.TunnelID), []byte("appended after upload"), 0o600)
	a.removeLocalCopies(ctx)
	if g := get(t, a, h.ID); !g.VideoLocalRemoved || !g.TextLocalRemoved {
		t.Fatal("local copies kept")
	}
	if _, err := os.Stat(filepath.Join(a.Cfg.Recording.Dir(), h.TunnelID)); !os.IsNotExist(err) {
		t.Fatal("local file still there")
	}
	if g := get(t, a, held.ID); g.VideoLocalRemoved {
		t.Fatal("held recording removed")
	}
	if g := get(t, a, changed.ID); g.VideoLocalRemoved || g.VideoStorage != store.StorageOwed {
		t.Fatalf("changed file: removed=%v state=%q", g.VideoLocalRemoved, g.VideoStorage)
	}
	// retention (1 day) ends for h: remote copies deleted, nothing reported missing
	_ = a.Store.EndHistory(ctx, h.ID, 0, "")
	if _, err := a.Store.DB().ExecContext(ctx, a.Store.DB().D.Rebind(`UPDATE connection_history SET end_at=? WHERE id=?`), time.Now().Add(-48*time.Hour).UnixMilli(), h.ID); err != nil {
		t.Fatal(err)
	}
	sum, err := a.purgeRecordings(ctx)
	if err != nil || sum.Remote < 3 || sum.Missing != 0 {
		t.Fatalf("purge: %+v %v", sum, err)
	}
	g := get(t, a, h.ID)
	if g.VideoStorage != store.StorageDeleted || g.TextStorage != store.StorageDeleted {
		t.Fatalf("after purge: %q %q", g.VideoStorage, g.TextStorage)
	}
	inst, _ := a.Store.InstallationID(ctx)
	day := time.UnixMilli(h.StartAt).UTC().Format("2006/01/02")
	if _, err := mem.HeadObject(context.Background(), "gg", "rec/"+inst+"/"+day+"/"+idStr(h.ID)+"/"+h.TunnelID); err == nil {
		t.Fatal("remote object survived the purge")
	}
}

func TestRemotePurgeRefusedStaysPending(t *testing.T) {
	a, _ := storageApp(t, true, true)
	ctx := context.Background()
	h := recorded(t, a, 3*time.Minute)
	_, _ = a.Store.MarkUploadsOwed(ctx)
	if err := upload(t, a, h.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB().ExecContext(ctx, a.Store.DB().D.Rebind(`UPDATE connection_history SET end_at=? WHERE id=?`), time.Now().Add(-48*time.Hour).UnixMilli(), h.ID); err != nil {
		t.Fatal(err)
	}
	sum, err := a.purgeRecordings(ctx)
	if err != nil || sum.RemotePending != 1 {
		t.Fatalf("purge: %+v %v", sum, err)
	}
	g := get(t, a, h.ID)
	if g.VideoStorage != store.StorageDeletePending || g.VideoFiles != store.FilesError {
		t.Fatalf("pending: %q %q %q", g.VideoStorage, g.VideoFiles, g.VideoStorageError)
	}
	if _, err := os.Stat(filepath.Join(a.Cfg.Recording.Dir(), h.TunnelID)); err != nil {
		t.Fatal("local file removed although the remote deletion was refused")
	}
}

// The database is the source of truth: with the queue emptied (no task for an owed set) the reconciliation queues
// it again and the worker uploads it once.
func TestReconciliationUploadsThroughTheQueue(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	rdb := redisFlush(t, addr, 7)
	defer rdb()
	a, _ := storageApp(t, true, false)
	a.Cfg.Redis = config.RedisConfig{Addr: addr, DB: 7}
	a.Cfg.Jobs = config.JobsConfig{Worker: true, Concurrency: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	defer a.Shutdown()
	for i := 0; i < 100 && a.queueHealth().State != "up"; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	h := recorded(t, a, 3*time.Minute)
	if err := a.reconcileStorage(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if g := get(t, a, h.ID); g.VideoStorage == store.StorageRemote && g.TextStorage == store.StorageRemote {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	g := get(t, a, h.ID)
	t.Fatalf("not uploaded through the queue: %q %q %q", g.VideoStorage, g.TextStorage, g.VideoStorageError)
}

// Upload existing recordings: a set that ran out of attempts starts over and is uploaded at once; audited; only for
// administrators and only with uploads enabled.
func TestBackfillRetriesExhaustedSets(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	defer redisFlush(t, addr, 7)()
	a, _ := storageApp(t, true, false)
	srv := httptest.NewServer(a.Echo)
	defer srv.Close()
	a.Cfg.Redis = config.RedisConfig{Addr: addr, DB: 7}
	a.Cfg.Jobs = config.JobsConfig{Worker: true, Concurrency: 2}
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	mkUser(t, a.Store, "op", "OpPass#1", false)
	h := recorded(t, a, 3*time.Minute)
	ref := store.SetRef{ID: h.ID, Video: true}
	_, _ = a.Store.MarkUploadsOwed(ctx)
	for i := 0; i < maxUploadTries; i++ {
		now := a.Store.Now()
		if ok, err := a.Store.ClaimUpload(ctx, ref, "w", "tok", now, now+60_000); !ok || err != nil {
			t.Fatalf("claim %d: %v %v", i, ok, err)
		}
		_, _ = a.Store.FailUpload(ctx, ref, "tok", now, "provider down")
	}
	if g := get(t, a, h.ID); g.VideoStorage != store.StorageFailed {
		t.Fatalf("not exhausted: %q", g.VideoStorage)
	}
	if st, _ := login(t, srv.URL, "op", "OpPass#1").do("POST", "/api/v1/settings/recording/storage/backfill", nil); st != 403 {
		t.Fatalf("operator: %d", st)
	}
	admin := login(t, srv.URL, "root", "RootPass#1")
	if st, out := admin.do("POST", "/api/v1/settings/recording/storage/backfill", nil); st != 409 {
		t.Fatalf("queue down: %d %v", st, out)
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.Start(cctx)
	defer a.Shutdown()
	for i := 0; i < 100 && a.queueHealth().State != "up"; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	st, out := admin.do("POST", "/api/v1/settings/recording/storage/backfill", nil)
	if st != 200 || out["retried"] != float64(1) || out["queued"].(float64) < 1 {
		t.Fatalf("backfill: %d %v", st, out)
	}
	for i := 0; i < 200 && get(t, a, h.ID).VideoStorage != store.StorageRemote; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if g := get(t, a, h.ID); g.VideoStorage != store.StorageRemote {
		t.Fatalf("not uploaded: %q %q", g.VideoStorage, g.VideoStorageError)
	}
	if ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "settings.recording.storage.backfill"}, store.Page{Limit: 5}); len(ev) != 1 {
		t.Fatalf("audit events: %d", len(ev))
	}
}

// Playback and transcript come from the bucket once the local copy is gone, from the target version the set was
// uploaded to even after the settings moved to another bucket, with the same audit event.
func TestPlaybackFromStorage(t *testing.T) {
	a, _ := storageApp(t, false, false)
	srv := httptest.NewServer(a.Echo)
	defer srv.Close()
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	h := recorded(t, a, 3*time.Minute)
	_, _ = a.Store.MarkUploadsOwed(ctx)
	for _, v := range []bool{true, false} {
		if err := upload(t, a, h.ID, v); err != nil {
			t.Fatal(err)
		}
	}
	if g := get(t, a, h.ID); g.VideoStorage != store.StorageRemote || g.TextStorage != store.StorageRemote {
		t.Fatalf("not uploaded: %q %q %q", g.VideoStorage, g.TextStorage, g.TextRemote)
	}
	// uploads now go elsewhere
	tid, err := a.Store.CreateStorageTarget(ctx, objstore.Target{Provider: objstore.S3, Endpoint: "http://127.0.0.1:1", Bucket: "other"})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Store.PutSetting(ctx, settingStorage, storageSettings{Enabled: true, TargetID: tid})
	for _, name := range []string{h.Recording, h.Typescript, h.Typescript + ".timing"} {
		if err := os.Remove(filepath.Join(a.Cfg.Recording.Dir(), name)); err != nil {
			t.Fatal(err)
		}
	}
	admin := login(t, srv.URL, "root", "RootPass#1")
	for path, want := range map[string]string{
		"/recording":              "content of " + h.Recording,
		"/typescript?format=raw":  "content of " + h.Typescript,
		"/typescript?format=text": "content of " + h.Typescript + "\n",
	} {
		st, body := admin.raw("GET", "/api/v1/history/"+idStr(h.ID)+path)
		if st != 200 || body != want {
			t.Fatalf("%s: %d %q", path, st, body)
		}
	}
	if ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "recording.view"}, store.Page{Limit: 5}); len(ev) != 3 {
		t.Fatalf("audit events: %d", len(ev))
	}
}

// The orphan sweep deletes old sets no history row owns, only in this installation's namespace and key layout.
func TestRemoteOrphanSweep(t *testing.T) {
	a, _ := storageApp(t, true, false)
	ctx := context.Background()
	set, _ := a.storageSettings(ctx)
	h := recorded(t, a, 3*time.Minute)
	inst, _ := a.Store.InstallationID(ctx)
	r, err := a.remote(ctx, set.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(src, []byte("x"), 0o600)
	month := time.Now().AddDate(0, 0, -30) // rclone keeps the file's modification time
	_ = os.Chtimes(src, month, month)
	orphan := "44444444-4444-4444-4444-444444444444"
	keys := map[string]bool{ // key → kept
		inst + "/2026/01/02/999/" + orphan:                        false,
		inst + "/2026/01/02/999/" + orphan + ".typescript.timing": false,
		inst + "/2026/01/02/" + idStr(h.ID) + "/" + h.Recording:   true, // owned
		inst + "/2026/01/02/999/notes.txt":                        true, // not a recording file
		"other-installation/2026/01/02/999/" + orphan:             true,
		"backup/" + orphan: true,
	}
	for k := range keys {
		if _, err := r.Put(ctx, k, src); err != nil {
			t.Fatal(err)
		}
	}
	var sum purgeSummary
	a.remoteOrphans(ctx, recDefaults{RetentionVideoDays: 60, RetentionTextDays: 60}, &sum)
	if sum.Orphans.Files != 0 {
		t.Fatalf("inside the grace period: %+v", sum.Orphans)
	}
	a.remoteOrphans(ctx, recDefaults{RetentionVideoDays: 1, RetentionTextDays: 1}, &sum)
	if sum.Orphans.Files != 2 || sum.Orphans.Bytes != 2 {
		t.Fatalf("orphans: %+v", sum.Orphans)
	}
	for k, kept := range keys {
		if _, _, err := r.Open(ctx, k); (err == nil) != kept {
			t.Errorf("%s: kept=%v, want %v", k, err == nil, kept)
		}
	}
}

// An administrator retries one session's failed upload from the history.
func TestRetryOneSession(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	defer redisFlush(t, addr, 7)()
	a, _ := storageApp(t, true, false)
	srv := httptest.NewServer(a.Echo)
	defer srv.Close()
	a.Cfg.Redis = config.RedisConfig{Addr: addr, DB: 7}
	a.Cfg.Jobs = config.JobsConfig{Worker: true, Concurrency: 2}
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	aud := mkUser(t, a.Store, "aud", "AudPass#1", false)
	_ = a.Store.GrantSystem(ctx, store.SubjectUser, aud.ID, store.PermAudit)
	h := recorded(t, a, 3*time.Minute)
	ref := store.SetRef{ID: h.ID, Video: false}
	_, _ = a.Store.MarkUploadsOwed(ctx)
	now := a.Store.Now()
	_, _ = a.Store.ClaimUpload(ctx, ref, "w", "tok", now, now+60_000)
	_, _ = a.Store.FailUpload(ctx, ref, "tok", now, "provider down")
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.Start(cctx)
	defer a.Shutdown()
	for i := 0; i < 100 && a.queueHealth().State != "up"; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	url := "/api/v1/history/" + idStr(h.ID) + "/storage/retry"
	if st, _ := login(t, srv.URL, "aud", "AudPass#1").do("POST", url, nil); st != 403 {
		t.Fatalf("auditor: %d", st)
	}
	admin := login(t, srv.URL, "root", "RootPass#1")
	if st, out := admin.do("POST", url, nil); st != 204 {
		t.Fatalf("retry: %d %v", st, out)
	}
	for i := 0; i < 200 && get(t, a, h.ID).TextStorage != store.StorageRemote; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if g := get(t, a, h.ID); g.TextStorage != store.StorageRemote {
		t.Fatalf("not uploaded: %q %q", g.TextStorage, g.TextStorageError)
	}
	if st, _ := admin.do("POST", url, nil); st != 409 {
		t.Fatalf("nothing to retry: %d", st)
	}
	if ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "recording.storage.retry"}, store.Page{Limit: 5}); len(ev) != 1 {
		t.Fatalf("audit events: %d", len(ev))
	}
}
