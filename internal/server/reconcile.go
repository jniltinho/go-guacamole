package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// storage:reconcile (every 5 minutes): the database is the source of truth, the queue only carries work.
func (a *App) reconcileJob() jobs.Periodic {
	return jobs.Periodic{Name: "storage:reconcile", Queue: jobs.Low, Every: 5 * time.Minute, Lease: 5 * time.Minute,
		Run: func(ctx context.Context, _ string) error { return a.reconcileStorage(ctx) }}
}

func (a *App) reconcileStorage(ctx context.Context) error {
	now := a.Store.Now()
	// 1. uploads left by a crashed worker
	if err := a.Store.ResetExpiredUploads(ctx, now); err != nil {
		return err
	}
	set, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	if !set.Enabled || set.TargetID == 0 || a.queueHealth().State != jobs.Up {
		return nil
	}
	// 2–3. sets on disk not stored remotely (also a crash between ending a session and recording its files), then
	// owed and retryable sets without a valid claim
	if _, err := a.queueOwed(ctx, now); err != nil {
		return err
	}
	// 4. versions a stale worker may have written next to recent uploads
	a.pruneRecent(ctx, now)
	// 5. local copies once they are safely in storage
	if !set.KeepLocal {
		a.removeLocalCopies(ctx)
	}
	return nil
}

// queueOwed marks every set on disk not stored remotely as owed and enqueues the owed sets nobody holds.
func (a *App) queueOwed(ctx context.Context, now int64) (int, error) {
	if _, err := a.Store.MarkUploadsOwed(ctx); err != nil {
		return 0, err
	}
	refs, err := a.Store.UploadQueue(ctx, now, maxUploadTries, 500)
	if err != nil {
		return 0, err
	}
	for _, r := range refs {
		if err := a.enqueueUpload(ctx, r, 0); err != nil {
			return 0, err
		}
	}
	return len(refs), nil
}

func (a *App) pruneRecent(ctx context.Context, now int64) {
	hs, err := a.Store.RecentUploads(ctx, now-2*time.Hour.Milliseconds())
	if err != nil {
		return
	}
	for _, h := range hs {
		dir, err := a.remoteDir(ctx, h)
		if err != nil {
			return
		}
		for _, tid := range []*int64{h.VideoTargetID, h.TextTargetID} {
			if tid == nil {
				continue
			}
			if r, err := a.remote(ctx, *tid); err == nil {
				_, _ = r.PruneVersions(ctx, dir)
			}
		}
	}
}

// removeLocalCopies deletes local files of sets verified in storage, after the safety window, never under legal
// hold, and only when the files still match what was uploaded (otherwise they are uploaded again).
func (a *App) removeLocalCopies(ctx context.Context) {
	cutoff := time.Now().Add(-LocalRemovalWindow).UnixMilli()
	refs, err := a.Store.RemovalCandidates(ctx, cutoff, 200)
	if err != nil {
		a.Log.Warn("local removal candidates", "err", err)
		return
	}
	for _, ref := range refs {
		a.removeLocalSet(ctx, ref, cutoff)
	}
}

func (a *App) removeLocalSet(ctx context.Context, ref store.SetRef, cutoff int64) {
	tok, now := store.RandomHex(16), a.Store.Now()
	if ok, err := a.Store.ClaimLocalRemoval(ctx, ref, a.Instance, tok, now, now+claimTTL.Milliseconds(), cutoff); err != nil || !ok {
		return
	}
	h, err := a.Store.GetHistory(ctx, ref.ID)
	if err != nil || !localRemovalAllowed(h, time.Now()) {
		_ = a.Store.ReleaseLocalClaim(ctx, ref, tok)
		return
	}
	manifest := h.TextRemote
	if ref.Video {
		manifest = h.VideoRemote
	}
	var entries []manifestEntry
	_ = json.Unmarshal([]byte(manifest), &entries)
	byName := map[string]manifestEntry{}
	for _, e := range entries {
		byName[filepath.Base(e.Key)] = e
	}
	files, err := a.fileSet(h.TunnelID, ref.Video)
	if err != nil || len(files) == 0 {
		_ = a.Store.ReleaseLocalClaim(ctx, ref, tok)
		return
	}
	for _, f := range files {
		e, ok := byName[filepath.Base(f)]
		if sum, err := fileSHA256(f); err != nil || !ok || sum != e.SHA256 {
			// changed (or new) since the upload: send it again instead of removing it
			_ = a.Store.Reupload(ctx, ref, tok)
			a.Log.Info("recording changed after upload, uploading again", "history", ref.ID, "set", kindName(ref.Video))
			return
		}
	}
	// the claim is still held while unlinking, so a legal hold cannot slip in between
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			a.Log.Warn("remove local recording", "history", ref.ID, "err", objstore.Clean(err.Error()))
			_ = a.Store.ReleaseLocalClaim(ctx, ref, tok)
			return
		}
	}
	if ok, _ := a.Store.FinishLocalRemoval(ctx, ref, tok, a.Store.Now()); ok {
		a.Log.Debug("local recording removed after upload", "history", ref.ID, "set", kindName(ref.Video))
	}
}

func fileSHA256(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// purgeRemoteSet deletes every version of a set's objects (none for sets never stored remotely).
func (a *App) purgeRemoteSet(ctx context.Context, h *store.HistoryEntry, video bool) (int, error) {
	tid, entries := remoteSet(h, video)
	if len(entries) == 0 {
		return 0, nil
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	r, err := a.remote(ctx, tid)
	if err != nil {
		return 0, err
	}
	return r.DeleteSet(ctx, keys)
}

// remoteSet is the target version and manifest of a set stored remotely (none otherwise).
func remoteSet(h *store.HistoryEntry, video bool) (int64, []manifestEntry) {
	storage, tid, manifest := h.TextStorage, h.TextTargetID, h.TextRemote
	if video {
		storage, tid, manifest = h.VideoStorage, h.VideoTargetID, h.VideoRemote
	}
	if (storage != store.StorageRemote && storage != store.StorageDeletePending) || tid == nil {
		return 0, nil
	}
	var entries []manifestEntry
	if json.Unmarshal([]byte(manifest), &entries) != nil {
		return 0, nil
	}
	return *tid, entries
}

// remoteKey is the key layout under the installation namespace: yyyy/mm/dd/<history id>/<recording file>.
var remoteKey = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}/(\d+)/(` + uuidRe + `(\.\d{1,3}|\.typescript(\.\d{1,3})?(\.timing)?)?)$`)

// remoteOrphans deletes objects of the current target that no history row owns, with the grace period of local orphans.
// Only keys in this installation's namespace and layout are considered; anything else under the prefix is left alone.
// shortcut: only the current target is swept, sweep retired targets too if bucket moves leave orphans behind.
func (a *App) remoteOrphans(ctx context.Context, d recDefaults, sum *purgeSummary) {
	if d.RetentionVideoDays == keepForever || d.RetentionTextDays == keepForever {
		return
	}
	set, err := a.storageSettings(ctx)
	if err != nil || set.TargetID == 0 {
		return
	}
	inst, err := a.Store.InstallationID(ctx)
	if err != nil {
		return
	}
	r, err := a.remote(ctx, set.TargetID)
	if err != nil {
		return
	}
	objs, err := r.List(ctx, inst)
	if err != nil {
		return
	}
	type group struct {
		id     string
		keys   []string
		bytes  int64
		newest time.Time
	}
	dirs := map[string]*group{}
	var tunnels []string
	for _, o := range objs {
		m := remoteKey.FindStringSubmatch(strings.TrimPrefix(o.Key, inst+"/"))
		if m == nil || !strings.HasPrefix(o.Key, inst+"/") {
			continue
		}
		dir := path.Dir(o.Key)
		g := dirs[dir]
		if g == nil {
			g = &group{id: m[1]}
			dirs[dir] = g
			tunnels = append(tunnels, m[3])
		}
		g.keys, g.bytes = append(g.keys, o.Key), g.bytes+o.Size
		if o.ModTime.After(g.newest) {
			g.newest = o.ModTime
		}
	}
	known, err := a.Store.HistoryByTunnel(ctx, tunnels)
	if err != nil {
		return // absence must be confirmed by the database
	}
	owned := map[string]bool{}
	for _, h := range known {
		owned[idStr(h.ID)] = true
	}
	cutoff := time.Now().AddDate(0, 0, -(max(d.RetentionVideoDays, d.RetentionTextDays) + 7))
	for _, g := range dirs {
		if owned[g.id] || g.newest.After(cutoff) {
			continue
		}
		if _, err := r.DeleteSet(ctx, g.keys); err == nil {
			sum.Orphans.Files += len(g.keys)
			sum.Orphans.Bytes += g.bytes
		}
	}
}
