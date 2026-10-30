package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/hibiken/asynq"

	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// Recording upload (openspec add-async-jobs-object-storage, design D4–D6): a set owed an upload is claimed (fenced
// token, renewed while working), every file is streamed and verified by reading it back, and only then the set is
// recorded as stored remotely. The claim keeps purge, local removal and legal holds away meanwhile.

const (
	uploadTask     = "recording:upload"
	claimTTL       = 5 * time.Minute
	maxUploadTries = 10
)

type uploadPayload struct {
	ID    int64 `json:"id,string"`
	Video bool  `json:"video"`
}

func kindName(video bool) string {
	if video {
		return "video"
	}
	return "text"
}

// enqueueUpload queues one set; the TaskID changes every 5 minutes so the reconciliation can re-enqueue later.
func (a *App) enqueueUpload(ctx context.Context, ref store.SetRef, delay time.Duration) error {
	payload, _ := json.Marshal(uploadPayload{ID: ref.ID, Video: ref.Video})
	slot := time.Now().Unix() / 300
	err := a.Jobs.Enqueue(ctx, jobs.Default, uploadTask, payload,
		asynq.TaskID(fmt.Sprintf("upload:%d:%s:%d", ref.ID, kindName(ref.Video), slot)),
		asynq.ProcessIn(delay), asynq.MaxRetry(maxUploadTries), asynq.Timeout(30*time.Minute), asynq.Retention(24*time.Hour))
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// remoteDir is where a history row's sets live under the target: <installation>/<yyyy>/<mm>/<dd>/<history id>.
func (a *App) remoteDir(ctx context.Context, h *store.HistoryEntry) (string, error) {
	inst, err := a.Store.InstallationID(ctx)
	if err != nil {
		return "", err
	}
	day := time.UnixMilli(h.StartAt).UTC().Format("2006/01/02")
	return path.Join(inst, day, idStr(h.ID)), nil
}

// manifestEntry records one uploaded file of a set.
type manifestEntry struct {
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (a *App) handleUpload(ctx context.Context, payload []byte) error {
	var p uploadPayload
	if err := json.Unmarshal(payload, &p); err != nil || p.ID == 0 {
		return errors.Join(errors.New("bad upload payload"), asynq.SkipRetry)
	}
	ref := store.SetRef{ID: p.ID, Video: p.Video}
	set, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	if !set.Enabled || set.TargetID == 0 {
		return nil // uploads off: the set stays owed for when they are turned on
	}
	h, err := a.Store.GetHistory(ctx, p.ID)
	if errors.Is(err, store.ErrNotFound) {
		return errors.Join(err, asynq.SkipRetry)
	}
	if err != nil {
		return err
	}
	files, err := a.fileSet(h.TunnelID, p.Video)
	if err != nil {
		return err
	}
	if err := uploadReady(h, files, time.Now()); err != nil {
		return err // retried with backoff; the reconciliation re-enqueues after the retries
	}
	tok := store.RandomHex(16)
	now := a.Store.Now()
	if ok, err := a.Store.ClaimUpload(ctx, ref, a.Instance, tok, now, now+claimTTL.Milliseconds()); err != nil || !ok {
		return err // someone else works on it, or it is not owed any more
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(claimTTL / 3)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				n := a.Store.Now()
				if ok, err := a.Store.RenewClaim(context.WithoutCancel(ctx), ref, tok, n, n+claimTTL.Milliseconds()); err != nil || !ok {
					cancel() // the claim is gone: stop writing
					return
				}
			}
		}
	}()
	fail := func(err error) error {
		msg := objstore.Clean(err.Error())
		_, _ = a.Store.FailUpload(context.WithoutCancel(ctx), ref, tok, a.Store.Now(), msg)
		return errors.New(msg)
	}
	remote, err := a.remote(work, set.TargetID)
	if err != nil {
		return fail(err)
	}
	dir, err := a.remoteDir(work, h)
	if err != nil {
		return fail(err)
	}
	plan := make([]manifestEntry, 0, len(files))
	for _, f := range files {
		plan = append(plan, manifestEntry{Key: path.Join(dir, filepath.Base(f))})
	}
	raw, _ := json.Marshal(plan)
	if ok, err := a.Store.PlanUpload(work, ref, tok, a.Store.Now(), set.TargetID, string(raw)); err != nil || !ok {
		return err // the claim is gone
	}
	var manifest []manifestEntry
	for _, f := range files {
		st, err := remote.Put(work, path.Join(dir, filepath.Base(f)), f)
		if err != nil {
			return fail(err)
		}
		manifest = append(manifest, manifestEntry{Key: st.Key, Size: st.Size, SHA256: st.SHA256})
	}
	if _, err := remote.PruneVersions(work, dir); err != nil {
		a.Log.Warn("prune earlier upload versions", "history", p.ID, "err", objstore.Clean(err.Error()))
	}
	raw, _ = json.Marshal(manifest)
	ok, err := a.Store.FinishUpload(context.WithoutCancel(ctx), ref, tok, a.Store.Now(), set.TargetID, string(raw))
	if err != nil {
		return err
	}
	if !ok {
		// lost the claim, or the set was purged meanwhile: what this run wrote must not outlive the purge
		if g, err := a.Store.GetHistory(context.WithoutCancel(ctx), p.ID); err == nil {
			state := g.TextFiles
			if p.Video {
				state = g.VideoFiles
			}
			if state == store.FilesPurged { // only this set's keys: the sibling set shares the directory
				keys := make([]string, 0, len(manifest))
				for _, e := range manifest {
					keys = append(keys, e.Key)
				}
				if _, err := remote.DeleteSet(context.WithoutCancel(ctx), keys); err != nil {
					a.Log.Warn("delete upload of a purged recording", "history", p.ID, "err", objstore.Clean(err.Error()))
				}
			}
		}
		return nil
	}
	a.Log.Debug("recording uploaded", "history", p.ID, "set", kindName(p.Video), "files", len(manifest))
	if !set.KeepLocal { // sessions that ended long ago (a backfill) need not wait for the reconciliation
		a.removeLocalSet(context.WithoutCancel(ctx), ref, time.Now().Add(-LocalRemovalWindow).UnixMilli())
	}
	return nil
}

// uploadSoon marks this row's sets owed and queues them for after the quiet period (the fast path at session end;
// the reconciliation catches anything missed).
func (a *App) uploadSoon(id int64) {
	set, err := a.storageSettings(a.bg())
	if err != nil || !set.Enabled || a.queueHealth().State != jobs.Up {
		return
	}
	if _, err := a.Store.MarkUploadsOwed(a.bg()); err != nil {
		return
	}
	for _, video := range []bool{true, false} {
		_ = a.enqueueUpload(a.bg(), store.SetRef{ID: id, Video: video}, QuietPeriod+30*time.Second)
	}
}
