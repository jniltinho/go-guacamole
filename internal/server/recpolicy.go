package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

// Recording policy: global defaults (admin UI, falling back to RECORDING_*),
// per-connection overrides, one snapshot per launch, fail-closed availability,
// history-driven retention with legal hold. See openspec change
// add-recording-policy-per-connection.

const (
	settingRecPolicy     = "recording.policy"
	settingRecExceptions = "recording.exceptions"
	settingRecState      = "recording.state"
	keepForever          = -1
	day                  = int64(24 * time.Hour / time.Millisecond)
)

type recDefaults struct {
	Saved              bool `json:"saved"`
	Video              bool `json:"video"`
	Text               bool `json:"text"`
	Keys               bool `json:"keys"`
	RetentionVideoDays int  `json:"retentionVideoDays"`
	RetentionTextDays  int  `json:"retentionTextDays"`
	// HideNotice stops telling users that their session is recorded (connecting screen and session
	// menu). Off by default, so settings saved before it existed keep the notice.
	HideNotice bool `json:"hideNotice"`
}

type recException struct {
	Until  int64  `json:"until"`
	Reason string `json:"reason"`
	By     string `json:"by"`
}

type recExceptions struct {
	Global      *recException           `json:"global,omitempty"`
	Connections map[string]recException `json:"connections"`
}

type kindTotals struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type purgeSummary struct {
	At      int64      `json:"at"`
	Video   kindTotals `json:"video"`
	Text    kindTotals `json:"text"`
	Orphans kindTotals `json:"orphans"`
	Missing int        `json:"missing"`
	Errors  int        `json:"errors"`
	// object storage: versions deleted, and sets whose remote deletion was refused (retried by later purges)
	Remote        int `json:"remote"`
	RemotePending int `json:"remotePending"`
}

type usageConn struct {
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Bytes          int64  `json:"bytes"`
}

type recUsage struct {
	At    int64       `json:"at"`
	Video kindTotals  `json:"video"`
	Text  kindTotals  `json:"text"`
	Top   []usageConn `json:"top"`
}

type recState struct {
	LastPurge   *purgeSummary         `json:"lastPurge,omitempty"`
	Usage       *recUsage             `json:"usage,omitempty"`
	Inventory   []store.InventoryItem `json:"inventory,omitempty"`
	InventoryAt int64                 `json:"inventoryAt,omitempty"`
}

// recSnapshot is the effective policy of one launch.
type recSnapshot struct {
	Video     bool `json:"video"`
	Text      bool `json:"text"`
	Keys      bool `json:"keys"`
	Exception bool `json:"exception"`
}

// recordingDefaults returns the saved defaults, or the RECORDING_* configuration until saved.
func (a *App) recordingDefaults(ctx context.Context) (recDefaults, error) {
	var d recDefaults
	if err := a.Store.GetSetting(ctx, settingRecPolicy, &d); err != nil {
		return d, err
	}
	if d.Saved {
		return d, nil
	}
	rc := a.Cfg.Recording
	days := rc.RetentionDays
	if days <= 0 {
		days = 90
	}
	return recDefaults{Video: rc.Enabled && rc.Screen, Text: rc.Enabled && rc.Typescript, Keys: rc.Enabled && rc.IncludeKeys,
		RetentionVideoDays: days, RetentionTextDays: days}, nil
}

func pick(override *string, def bool) bool {
	if override != nil {
		return *override == "on"
	}
	return def
}

// effectiveRecording resolves a connection's policy: text only for SSH, keys only with video.
func effectiveRecording(conn *store.Connection, d recDefaults) recSnapshot {
	s := recSnapshot{Video: pick(conn.RecVideo, d.Video)}
	s.Text = conn.Protocol == "ssh" && pick(conn.RecText, d.Text)
	s.Keys = s.Video && pick(conn.RecKeys, d.Keys)
	return s
}

func retentionOr(override *int, def int) int {
	if override != nil {
		return *override
	}
	return def
}

// longer reports whether retention a keeps recordings longer than b (-1 = forever).
func longer(a, b int) bool {
	if a == b {
		return false
	}
	return a == keepForever || (b != keepForever && a > b)
}

func validRetention(v int) bool { return v == keepForever || (v >= 1 && v <= 3650) }

// ---- availability and exceptions ----

type recAvailability struct {
	mu        sync.Mutex
	ok        bool
	reason    string
	checkedAt time.Time
}

// recordingAvailable reports whether the recordings directory can be written (cached for a minute).
func (a *App) recordingAvailable() (bool, string) {
	ra := &a.recAvail
	ra.mu.Lock()
	defer ra.mu.Unlock()
	if time.Since(ra.checkedAt) < time.Minute && !ra.checkedAt.IsZero() {
		return ra.ok, ra.reason
	}
	ra.ok, ra.reason, ra.checkedAt = true, "", time.Now()
	dir := a.Cfg.Recording.Dir()
	if a.Cfg.Recording.Path == "" || dir == "" {
		ra.ok, ra.reason = false, "recordings directory not configured"
		return ra.ok, ra.reason
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		ra.ok, ra.reason = false, "recordings directory not writable"
		return ra.ok, ra.reason
	}
	f.Close()
	_ = os.Remove(f.Name())
	return ra.ok, ra.reason
}

func (a *App) resetAvailability() {
	a.recAvail.mu.Lock()
	a.recAvail.checkedAt = time.Time{}
	a.recAvail.mu.Unlock()
}

func (a *App) recordingExceptions(ctx context.Context) (recExceptions, error) {
	return readExceptions(ctx, a.Store)
}

func readExceptions(ctx context.Context, st *store.Store) (recExceptions, error) {
	var e recExceptions
	err := st.GetSetting(ctx, settingRecExceptions, &e)
	if e.Connections == nil {
		e.Connections = map[string]recException{}
	}
	return e, err
}

// activeException returns the exception allowing connID to run unrecorded, if any.
func (a *App) activeException(ctx context.Context, connID int64) (*recException, error) {
	e, err := a.recordingExceptions(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if x, ok := e.Connections[idStr(connID)]; ok && x.Until > now {
		return &x, nil
	}
	if e.Global != nil && e.Global.Until > now {
		return e.Global, nil
	}
	return nil, nil
}

// ---- files ----

var (
	uuidRe    = `([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`
	videoFile = regexp.MustCompile(`^` + uuidRe + `(\.\d{1,3})?$`)
	textFile  = regexp.MustCompile(`^` + uuidRe + `\.typescript(\.\d{1,3})?(\.timing)?$`)
)

// fileSet returns the files of one kind written for a tunnel (numbered variants and .timing included).
func (a *App) fileSet(tunnelID string, video bool) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(a.Cfg.Recording.Dir(), tunnelID+"*"))
	if err != nil {
		return nil, err
	}
	re := textFile
	if video {
		re = videoFile
	}
	var out []string
	for _, m := range matches {
		if g := re.FindStringSubmatch(filepath.Base(m)); g != nil && g[1] == tunnelID {
			out = append(out, m)
		}
	}
	return out, nil
}

// captured reports whether a file set really holds a recording: a non-empty
// screen recording, or a typescript data file (not only its .timing).
func captured(files []string, video bool) bool {
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if video && st.Size() > 0 {
			return true
		}
		if !video && !strings.HasSuffix(f, ".timing") {
			return true
		}
	}
	return false
}

// dirReadable tells a missing file from an unreachable recordings volume.
func (a *App) dirReadable() bool {
	_, err := os.ReadDir(a.Cfg.Recording.Dir())
	return err == nil
}

// recordingParams returns the connection parameters for guacd with every
// hand-typed recording parameter removed and the snapshot's kinds added.
func (a *App) recordingParams(conn *store.Connection, t *tunnel.Tunnel) map[string]string {
	p := make(map[string]string, len(conn.Parameters)+8)
	for k, v := range conn.Parameters {
		if !store.RecordingParam(k) {
			p[k] = v
		}
	}
	if t.RecException {
		return p
	}
	path := a.Cfg.Recording.Path
	if t.RecVideo {
		p["recording-path"], p["recording-name"], p["create-recording-path"] = path, t.ID, "true"
		if t.RecKeys {
			p["recording-include-keys"] = "true"
		}
	}
	if t.RecText {
		p["typescript-path"], p["typescript-name"], p["create-typescript-path"] = path, t.ID+".typescript", "true"
	}
	return p
}

// verifyCaptureSoon closes the session when guacd did not create a required
// file shortly after connecting (guacd 1.6 may keep running unrecorded).
func (a *App) verifyCaptureSoon(t *tunnel.Tunnel) {
	if t.RecException || (!t.RecVideo && !t.RecText) {
		return
	}
	select {
	case <-t.Context().Done():
		return
	case <-time.After(a.captureGrace):
	}
	var missing []string
	for _, k := range []struct {
		on    bool
		video bool
		name  string
	}{{t.RecVideo, true, "video"}, {t.RecText, false, "text"}} {
		if !k.on {
			continue
		}
		if files, err := a.fileSet(t.ID, k.video); err != nil || !captured(files, k.video) {
			missing = append(missing, k.name)
			_ = a.Store.SetCapture(a.bg(), t.HistoryID, k.video, store.CaptureFailed, "")
		}
	}
	if len(missing) == 0 {
		return
	}
	a.resetAvailability()
	a.audit(nil, &t.UserID, t.Username, "recording.failed", "connection", idStr(t.ConnectionID), t.ConnectionName, "failure",
		map[string]any{"tunnel": t.ID, "missing": missing})
	a.Registry.Revoke(func(x *tunnel.Tunnel) bool { return x.ID == t.ID }, guac.StatusServerError, "RECORDING_FAILED")
}

// finishCapture records, after a session, which file sets exist.
func (a *App) finishCapture(t *tunnel.Tunnel) {
	if t.RecException {
		return
	}
	ctx := a.bg()
	h, err := a.Store.GetHistory(ctx, t.HistoryID)
	if err != nil || !a.dirReadable() {
		return // left undiscovered: reconciled at the next startup
	}
	for _, k := range []struct {
		on, video bool
		capture   string
	}{{t.RecVideo, true, h.VideoCapture}, {t.RecText, false, h.TextCapture}} {
		if !k.on {
			continue
		}
		files, _ := a.fileSet(t.ID, k.video)
		state := store.FilesMissing
		if len(files) > 0 {
			state = store.FilesPresent
		}
		capture := store.CaptureOK
		if k.capture == store.CaptureFailed || !captured(files, k.video) {
			capture = store.CaptureFailed
		}
		_ = a.Store.SetCapture(ctx, t.HistoryID, k.video, capture, state)
		if capture == store.CaptureFailed && k.capture != store.CaptureFailed {
			a.audit(nil, &t.UserID, t.Username, "recording.failed", "connection", idStr(t.ConnectionID), t.ConnectionName, "failure",
				map[string]any{"tunnel": t.ID, "video": k.video})
		}
	}
}

// ---- startup and janitor ----

// recordingStartup converts legacy recording parameters into overrides and
// discovers the file sets of sessions that were never checked.
func (a *App) recordingStartup(ctx context.Context) {
	items, err := a.Store.ApplyRecordingInventory(ctx)
	if err != nil {
		a.Log.Error("recording inventory", "err", err)
	}
	if len(items) > 0 {
		a.Log.Warn("recording parameters converted to recording overrides", "connections", len(items))
		_ = a.updateRecState(ctx, func(s *recState) { s.Inventory, s.InventoryAt = append(s.Inventory, items...), a.Store.Now() })
	}
	a.discoverFiles(ctx, true)
}

// discoverFiles records which recording files exist for ended sessions not checked yet (with missing, also those
// found missing before).
func (a *App) discoverFiles(ctx context.Context, missing bool) {
	hs, err := a.Store.UndiscoveredHistory(ctx, missing)
	if err != nil || !a.dirReadable() {
		return
	}
	for _, h := range hs {
		for _, k := range []struct {
			name  string
			video bool
			state string
		}{{h.Recording, true, h.VideoFiles}, {h.Typescript, false, h.TextFiles}} {
			if k.name == "" || (k.state != "" && k.state != store.FilesMissing) {
				continue
			}
			files, _ := a.fileSet(h.TunnelID, k.video)
			state := store.FilesMissing
			if len(files) > 0 {
				state = store.FilesPresent
			}
			_ = a.Store.SetFiles(ctx, h.ID, k.video, state)
		}
	}
}

// finishAbandoned closes history rows of tickets that were never used.
func (a *App) finishAbandoned(ctx context.Context) {
	a.Registry.ExpireTickets()
	open, err := a.Store.OpenHistory(ctx)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-2 * tunnel.TicketTTL).UnixMilli()
	for _, h := range open {
		if h.OwnerInstance != a.Instance {
			continue // another replica's (or an earlier run's) launch: beat() closes it once that owner is gone
		}
		if _, started := a.activated.Load(h.ID); started {
			continue // a real session, still closing
		}
		if h.StartAt < cutoff && a.Registry.Get(h.TunnelID) == nil {
			_ = a.Store.FinishNotStarted(ctx, h.ID)
		}
	}
}

func (a *App) updateRecState(ctx context.Context, fn func(*recState)) error {
	a.recStateMu.Lock()
	defer a.recStateMu.Unlock()
	var s recState
	if err := a.Store.GetSetting(ctx, settingRecState, &s); err != nil {
		return err
	}
	fn(&s)
	return a.Store.PutSetting(ctx, settingRecState, &s)
}

// purgeAuditKeepingHolds applies audit retention without deleting the evidence of held sessions.
func (a *App) purgeAuditKeepingHolds(ctx context.Context, before int64) (int64, error) {
	a.purgeMu.Lock() // holds are placed under the same lock: none appears mid-purge
	defer a.purgeMu.Unlock()
	held, err := a.Store.HeldHistory(ctx)
	if err != nil {
		return 0, err
	}
	keep := map[string][]string{}
	var tunnels []string
	for _, h := range held {
		keep["history"] = append(keep["history"], idStr(h.ID))
		if h.ConnectionID != nil {
			keep["connection"] = append(keep["connection"], idStr(*h.ConnectionID))
		}
		tunnels = append(tunnels, h.TunnelID) // survives connection deletion
	}
	return a.Store.PurgeAuditKeeping(ctx, before, keep, tunnels)
}

// ---- retention ----

// purgeRecordings deletes expired file sets (history driven) and old orphans.
func (a *App) purgeRecordings(ctx context.Context) (*purgeSummary, error) {
	a.purgeMu.Lock()
	defer a.purgeMu.Unlock()
	if !a.dirReadable() {
		return nil, errors.New("recordings directory cannot be read")
	}
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.Store.RecoverPurging(ctx); err != nil {
		return nil, err
	}
	conns, err := a.Store.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*store.Connection{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	cands, err := a.Store.PurgeCandidates(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	sum := &purgeSummary{At: now}
	for _, h := range cands {
		for _, video := range []bool{true, false} {
			state, frozen, def := h.TextFiles, h.RetentionTextDays, d.RetentionTextDays
			if video {
				state, frozen, def = h.VideoFiles, h.RetentionVideoDays, d.RetentionVideoDays
			}
			if state != store.FilesPresent && state != store.FilesError && state != store.FilesPurging {
				continue
			}
			days := retentionOr(frozen, def)
			if h.ConnectionID != nil && byID[*h.ConnectionID] != nil {
				c := byID[*h.ConnectionID]
				days = retentionOr(c.RetentionTextDays, def)
				if video {
					days = retentionOr(c.RetentionVideoDays, def)
				}
			}
			if days == keepForever || h.EndAt == nil || *h.EndAt+int64(days)*day > now {
				continue
			}
			tok, ref := store.RandomHex(16), store.SetRef{ID: h.ID, Video: video}
			if ok, err := a.Store.BeginPurge(ctx, h.ID, video, tok, a.Store.Now()+claimTTL.Milliseconds()); err != nil || !ok {
				continue
			}
			// the set is ours now: decide from its current state (an upload may have finished since the candidates
			// were listed)
			h, err := a.Store.GetHistory(ctx, h.ID)
			if err != nil {
				continue // left purging: the next purge recovers it once the claim expires
			}
			// remote copies first: a refusal (object lock, permissions) keeps the set pending, local files included
			if n, err := a.purgeRemoteSet(ctx, h, video); err != nil {
				_ = a.Store.SetStorageState(ctx, h.ID, video, store.StorageDeletePending, objstore.Clean(err.Error()))
				_ = a.Store.EndPurge(ctx, h.ID, video, store.FilesError, tok)
				sum.RemotePending++
				sum.Errors++
				continue
			} else {
				sum.Remote += n
			}
			localRemoved, storage := h.TextLocalRemoved, h.TextStorage
			if video {
				localRemoved, storage = h.VideoLocalRemoved, h.VideoStorage
			}
			if storage != "" {
				_ = a.Store.SetStorageState(ctx, h.ID, video, store.StorageDeleted, "")
			}
			files, err := a.fileSet(h.TunnelID, video)
			result := store.FilesPurged
			// still ours right before unlinking: a purge that lost its claim (and so its set) leaves the files
			if ok, _ := a.Store.RenewClaim(ctx, ref, tok, a.Store.Now(), a.Store.Now()+claimTTL.Milliseconds()); !ok {
				continue
			}
			switch {
			case err != nil:
				result = store.FilesError
			case len(files) == 0 && !localRemoved: // removed after upload: nothing local is expected
				result = store.FilesMissing
				sum.Missing++
			}
			for _, f := range files {
				st, _ := os.Stat(f)
				if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
					result = store.FilesError
					continue
				}
				t := &sum.Text
				if video {
					t = &sum.Video
				}
				t.Files++
				if st != nil {
					t.Bytes += st.Size()
				}
			}
			if result == store.FilesError {
				sum.Errors++
			}
			_ = a.Store.EndPurge(ctx, h.ID, video, result, tok)
		}
	}
	a.purgeOrphans(ctx, d, sum)
	a.audit(nil, nil, "system", "recording.purge", "recording", "", "retention", "success", map[string]any{
		"video": sum.Video, "text": sum.Text, "orphans": sum.Orphans, "missing": sum.Missing, "errors": sum.Errors,
		"remote": sum.Remote, "remotePending": sum.RemotePending,
		"period": jobs.PeriodOf(ctx)})
	_ = a.updateRecState(ctx, func(s *recState) { s.LastPurge = sum })
	return sum, nil
}

func (a *App) purgeOrphans(ctx context.Context, d recDefaults, sum *purgeSummary) {
	a.orphans(ctx, d, sum, false)
	a.remoteOrphans(ctx, d, sum)
}

// orphans deletes (or with dry only counts) recording sets without history.
func (a *App) orphans(ctx context.Context, d recDefaults, sum *purgeSummary, dry bool) {
	if d.RetentionVideoDays == keepForever || d.RetentionTextDays == keepForever {
		return
	}
	entries, err := os.ReadDir(a.Cfg.Recording.Dir())
	if err != nil {
		return
	}
	newest := map[string]time.Time{}
	files := map[string][]os.DirEntry{}
	for _, e := range entries {
		g := videoFile.FindStringSubmatch(e.Name())
		if g == nil {
			g = textFile.FindStringSubmatch(e.Name())
		}
		if g == nil || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest[g[1]]) {
			newest[g[1]] = info.ModTime()
		}
		files[g[1]] = append(files[g[1]], e)
	}
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	known, err := a.Store.HistoryByTunnel(ctx, ids)
	if err != nil {
		return // absence must be confirmed by the database
	}
	days := max(d.RetentionVideoDays, d.RetentionTextDays) + 7
	cutoff := time.Now().AddDate(0, 0, -days)
	for id, fs := range files {
		if known[id] != nil || newest[id].After(cutoff) {
			continue
		}
		for _, e := range fs {
			info, _ := e.Info()
			if dry || os.Remove(filepath.Join(a.Cfg.Recording.Dir(), e.Name())) == nil {
				sum.Orphans.Files++
				if info != nil {
					sum.Orphans.Bytes += info.Size()
				}
			}
		}
	}
}

// computeUsage sums the recordings directory by kind and connection.
func (a *App) computeUsage(ctx context.Context) (*recUsage, error) {
	entries, err := os.ReadDir(a.Cfg.Recording.Dir())
	if err != nil {
		return nil, err
	}
	u := &recUsage{At: time.Now().UnixMilli(), Top: []usageConn{}}
	perTunnel := map[string]int64{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		if g := videoFile.FindStringSubmatch(e.Name()); g != nil {
			u.Video.Files++
			u.Video.Bytes += info.Size()
			perTunnel[g[1]] += info.Size()
		} else if g := textFile.FindStringSubmatch(e.Name()); g != nil {
			u.Text.Files++
			u.Text.Bytes += info.Size()
			perTunnel[g[1]] += info.Size()
		}
	}
	ids := make([]string, 0, len(perTunnel))
	for id := range perTunnel {
		ids = append(ids, id)
	}
	hs, err := a.Store.HistoryByTunnel(ctx, ids)
	if err != nil {
		return nil, err
	}
	byConn := map[string]*usageConn{}
	for id, b := range perTunnel {
		key, name := "", "(orphan)"
		if h := hs[id]; h != nil {
			name = h.ConnectionName
			if h.ConnectionID != nil {
				key = idStr(*h.ConnectionID)
			} else {
				key = "deleted:" + h.ConnectionName
			}
		}
		if byConn[key] == nil {
			byConn[key] = &usageConn{ConnectionID: key, ConnectionName: name}
		}
		byConn[key].Bytes += b
	}
	for _, c := range byConn {
		u.Top = append(u.Top, *c)
	}
	sort.Slice(u.Top, func(i, j int) bool { return u.Top[i].Bytes > u.Top[j].Bytes })
	if len(u.Top) > 10 {
		u.Top = u.Top[:10]
	}
	return u, nil
}

// ---- preview tokens (shortening a retention needs a confirmed preview) ----

type previewEntry struct {
	key     string
	expires time.Time
}

func (a *App) previewToken(key string) string {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	now := time.Now()
	for k, p := range a.previews {
		if now.After(p.expires) {
			delete(a.previews, k)
		}
	}
	tok := store.RandomHex(16)
	a.previews[tok] = previewEntry{key: key, expires: now.Add(10 * time.Minute)}
	return tok
}

func (a *App) usePreview(tok, key string) bool {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	p, ok := a.previews[tok]
	delete(a.previews, tok)
	return ok && p.key == key && time.Now().Before(p.expires)
}

// previewRetention counts what a purge would delete under the proposed
// retentions (connID 0 = connections inheriting the global default).
func (a *App) previewRetention(ctx context.Context, connID int64, videoDays, textDays int) (int, int64, error) {
	cands, err := a.Store.PurgeCandidates(ctx)
	if err != nil {
		return 0, 0, err
	}
	conns, err := a.Store.ListConnections(ctx)
	if err != nil {
		return 0, 0, err
	}
	byID := map[int64]*store.Connection{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	now := time.Now().UnixMilli()
	n, bytes := 0, int64(0)
	for _, h := range cands {
		if h.EndAt == nil {
			continue
		}
		c := (*store.Connection)(nil)
		if h.ConnectionID != nil {
			c = byID[*h.ConnectionID]
		}
		for _, video := range []bool{true, false} {
			days, own, frozen, state := textDays, (*int)(nil), h.RetentionTextDays, h.TextFiles
			if video {
				days, frozen, state = videoDays, h.RetentionVideoDays, h.VideoFiles
			}
			if state != store.FilesPresent && state != store.FilesError && state != store.FilesPurging {
				continue
			}
			if c != nil {
				own = c.RetentionTextDays
				if video {
					own = c.RetentionVideoDays
				}
			}
			switch {
			case connID != 0 && (c == nil || c.ID != connID):
				continue
			case connID == 0 && (c == nil && frozen != nil || c != nil && own != nil):
				continue // not inheriting the global default
			}
			if days == keepForever || *h.EndAt+int64(days)*day > now {
				continue
			}
			files, _ := a.fileSet(h.TunnelID, video)
			n++
			for _, f := range files {
				if st, err := os.Stat(f); err == nil {
					bytes += st.Size()
				}
			}
		}
	}
	if connID == 0 {
		var orph purgeSummary
		a.orphans(ctx, recDefaults{RetentionVideoDays: videoDays, RetentionTextDays: textDays}, &orph, true)
		n += orph.Orphans.Files
		bytes += orph.Orphans.Bytes
	}
	return n, bytes, nil
}

// ---- API ----

type recordingSettingsView struct {
	Defaults     recDefaults   `json:"defaults"`
	Exceptions   recExceptions `json:"exceptions"`
	State        recState      `json:"state"`
	Available    bool          `json:"available"`
	Unavailable  string        `json:"unavailableReason,omitempty"`
	Directory    string        `json:"directory"`
	FromSettings bool          `json:"fromSettings"`
	Queue        jobs.Health   `json:"queue"`
}

func (a *App) getRecordingSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return err
	}
	ex, err := a.recordingExceptions(ctx)
	if err != nil {
		return err
	}
	var st recState
	if err := a.Store.GetSetting(ctx, settingRecState, &st); err != nil {
		return err
	}
	ok, why := a.recordingAvailable()
	return c.JSON(http.StatusOK, recordingSettingsView{Defaults: d, Exceptions: ex, State: st, Available: ok, Unavailable: why,
		Directory: a.Cfg.Recording.Path, FromSettings: d.Saved, Queue: a.queueHealth()})
}

// previewRequired answers 409 with the deletion count and a token to confirm.
func (a *App) previewRequired(c *echo.Context, connID int64, videoDays, textDays int, key string) error {
	n, bytes, err := a.previewRetention(c.Request().Context(), connID, videoDays, textDays)
	if err != nil {
		return err
	}
	return &APIError{Status: http.StatusConflict, Code: "RETENTION_PREVIEW", Message: "confirm the recordings that will be deleted",
		Data: map[string]any{"recordings": n, "bytes": bytes, "token": a.previewToken(key)}}
}

func (a *App) putRecordingSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in struct {
		recDefaults
		PreviewToken string `json:"previewToken"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	if !validRetention(in.RetentionVideoDays) {
		return validation("retentionVideoDays", "1 to 3650 days, or keep forever")
	}
	if !validRetention(in.RetentionTextDays) {
		return validation("retentionTextDays", "1 to 3650 days, or keep forever")
	}
	ctx := c.Request().Context()
	old, err := a.recordingDefaults(ctx)
	if err != nil {
		return err
	}
	nd := in.recDefaults
	nd.Saved = true
	if longer(old.RetentionVideoDays, nd.RetentionVideoDays) || longer(old.RetentionTextDays, nd.RetentionTextDays) {
		key := "global:" + idStr(int64(nd.RetentionVideoDays)) + ":" + idStr(int64(nd.RetentionTextDays))
		if in.PreviewToken == "" || !a.usePreview(in.PreviewToken, key) {
			return a.previewRequired(c, 0, nd.RetentionVideoDays, nd.RetentionTextDays, key)
		}
	}
	u := userOf(c)
	a.purgeMu.Lock() // never change retention while a purge is deciding with the old one
	defer a.purgeMu.Unlock()
	err = a.Store.Tx(ctx, func(tx *store.Store) error {
		if err := tx.PutSetting(ctx, settingRecPolicy, &nd); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: u.Username, ClientIP: c.RealIP(),
			Action: "settings.recording.update", TargetType: "settings", TargetID: settingRecPolicy, TargetName: "recording",
			Outcome: "success", Metadata: map[string]any{"old": old, "new": nd}})
	})
	if err != nil {
		return err
	}
	return a.getRecordingSettings(c)
}

// putRecordingException grants (minutes > 0) or clears an exception.
func (a *App) putRecordingException(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in struct {
		ConnectionID string `json:"connectionId"`
		Minutes      int    `json:"minutes"`
		Reason       string `json:"reason"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Minutes < 0 || in.Minutes > 1440 {
		return validation("minutes", "0 (clear) to 1440 minutes")
	}
	if in.Minutes > 0 && (in.Reason == "" || len(in.Reason) > 256) {
		return validation("reason", "a reason (1-256 characters) is required")
	}
	ctx := c.Request().Context()
	u := userOf(c)
	target := "global"
	if in.ConnectionID != "" {
		id, err := parseID(in.ConnectionID)
		if err != nil {
			return err
		}
		if _, err := a.Store.GetConnection(ctx, id); err != nil {
			return validation("connectionId", "connection not found")
		}
		target = idStr(id)
	}
	a.recExcMu.Lock() // read-modify-write of the exceptions document
	defer a.recExcMu.Unlock()
	err := a.Store.Tx(ctx, func(tx *store.Store) error {
		ex, err := readExceptions(ctx, tx)
		if err != nil {
			return err
		}
		var x *recException
		if in.Minutes > 0 {
			x = &recException{Until: time.Now().Add(time.Duration(in.Minutes) * time.Minute).UnixMilli(), Reason: in.Reason, By: u.Username}
		}
		switch {
		case target == "global":
			ex.Global = x
		case x == nil:
			delete(ex.Connections, target)
		default:
			ex.Connections[target] = *x
		}
		if err := tx.PutSetting(ctx, settingRecExceptions, &ex); err != nil {
			return err
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: u.Username, ClientIP: c.RealIP(),
			Action: "recording.exception", TargetType: "recording", TargetID: target, TargetName: "exception",
			Outcome: "success", Metadata: map[string]any{"minutes": in.Minutes, "reason": in.Reason}})
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) refreshUsage(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	u, err := a.computeUsage(ctx)
	if err != nil {
		return apiErr(http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "recordings directory cannot be read")
	}
	if err := a.updateRecState(ctx, func(s *recState) { s.Usage = u }); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, u)
}

// ---- per-connection policy ----

type recordingView struct {
	Video              string      `json:"video"`
	Text               string      `json:"text"`
	Keys               string      `json:"keys"`
	RetentionVideoDays *int        `json:"retentionVideoDays"`
	RetentionTextDays  *int        `json:"retentionTextDays"`
	Version            int         `json:"version"`
	Effective          recSnapshot `json:"effective"`
	EffectiveVideoDays int         `json:"effectiveVideoDays"`
	EffectiveTextDays  int         `json:"effectiveTextDays"`
	CanEdit            bool        `json:"canEdit"`
	Defaults           recDefaults `json:"defaults"`
}

func tri(p *string) string {
	if p == nil {
		return "inherit"
	}
	return *p
}

func (a *App) recordingViewOf(ctx context.Context, c *echo.Context, conn *store.Connection) *recordingView {
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return nil
	}
	return &recordingView{Video: tri(conn.RecVideo), Text: tri(conn.RecText), Keys: tri(conn.RecKeys),
		RetentionVideoDays: conn.RetentionVideoDays, RetentionTextDays: conn.RetentionTextDays, Version: conn.RecVersion,
		Effective:          effectiveRecording(conn, d),
		EffectiveVideoDays: retentionOr(conn.RetentionVideoDays, d.RetentionVideoDays),
		EffectiveTextDays:  retentionOr(conn.RetentionTextDays, d.RetentionTextDays),
		CanEdit:            effOf(c).Has(store.PermAudit), Defaults: d}
}

func parseTri(v, field string) (*string, error) {
	switch v {
	case "inherit", "":
		return nil, nil
	case "on", "off":
		return &v, nil
	}
	return nil, validation(field, "inherit, on or off")
}

func (a *App) putConnectionRecording(c *echo.Context) error {
	if err := requirePerm(c, store.PermAudit); err != nil {
		return err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	conn, err := a.Store.GetConnection(ctx, id)
	if err != nil {
		return errNotFound
	}
	var in struct {
		Video              string `json:"video"`
		Text               string `json:"text"`
		Keys               string `json:"keys"`
		RetentionVideoDays *int   `json:"retentionVideoDays"`
		RetentionTextDays  *int   `json:"retentionTextDays"`
		Version            int    `json:"version"`
		PreviewToken       string `json:"previewToken"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	var p store.RecordingPolicy
	if p.Video, err = parseTri(in.Video, "video"); err != nil {
		return err
	}
	if p.Text, err = parseTri(in.Text, "text"); err != nil {
		return err
	}
	if p.Keys, err = parseTri(in.Keys, "keys"); err != nil {
		return err
	}
	for _, r := range []struct {
		v     *int
		field string
	}{{in.RetentionVideoDays, "retentionVideoDays"}, {in.RetentionTextDays, "retentionTextDays"}} {
		if r.v != nil && !validRetention(*r.v) {
			return validation(r.field, "1 to 3650 days, or keep forever")
		}
	}
	p.RetentionVideoDays, p.RetentionTextDays = in.RetentionVideoDays, in.RetentionTextDays
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return err
	}
	next := *conn
	next.RecVideo, next.RecText, next.RecKeys = p.Video, p.Text, p.Keys
	next.RetentionVideoDays, next.RetentionTextDays = p.RetentionVideoDays, p.RetentionTextDays
	if p.Keys != nil && *p.Keys == "on" && !effectiveRecording(&next, d).Video {
		return validation("keys", "keystrokes are captured inside the video recording: enable video")
	}
	oldV, oldT := retentionOr(conn.RetentionVideoDays, d.RetentionVideoDays), retentionOr(conn.RetentionTextDays, d.RetentionTextDays)
	newV, newT := retentionOr(next.RetentionVideoDays, d.RetentionVideoDays), retentionOr(next.RetentionTextDays, d.RetentionTextDays)
	if longer(oldV, newV) || longer(oldT, newT) {
		key := "conn:" + idStr(id) + ":" + idStr(int64(newV)) + ":" + idStr(int64(newT)) + ":" + idStr(int64(in.Version))
		if in.PreviewToken == "" || !a.usePreview(in.PreviewToken, key) {
			return a.previewRequired(c, id, newV, newT, key)
		}
	}
	u := userOf(c)
	oldView, newView := a.recordingViewOf(ctx, c, conn), a.recordingViewOf(ctx, c, &next)
	a.purgeMu.Lock()
	defer a.purgeMu.Unlock()
	err = a.Store.Tx(ctx, func(tx *store.Store) error {
		ok, err := tx.SetConnectionRecording(ctx, id, p, in.Version)
		if err != nil {
			return err
		}
		if !ok {
			return apiErr(http.StatusConflict, "VERSION_CONFLICT", "the recording policy was changed by someone else; reload")
		}
		return tx.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: u.Username, ClientIP: c.RealIP(),
			Action: "connection.recording.update", TargetType: "connection", TargetID: idStr(id), TargetName: conn.Name,
			Outcome: "success", Metadata: map[string]any{
				"video": oldView.Video + " → " + newView.Video, "text": oldView.Text + " → " + newView.Text,
				"keys": oldView.Keys + " → " + newView.Keys, "retentionVideoDays": [2]int{oldV, newV}, "retentionTextDays": [2]int{oldT, newT}}})
	})
	if err != nil {
		return err
	}
	conn, err = a.Store.GetConnection(ctx, id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, a.recordingViewOf(ctx, c, conn))
}

// ---- legal hold ----

func (a *App) putHold(c *echo.Context) error {
	if err := requirePerm(c, store.PermAudit); err != nil {
		return err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len(in.Reason) > 256 {
		return validation("reason", "a reason (1-256 characters) is required")
	}
	ctx := c.Request().Context()
	h, err := a.Store.GetHistory(ctx, id)
	if err != nil {
		return err
	}
	u := userOf(c)
	a.purgeMu.Lock()
	defer a.purgeMu.Unlock()
	err = a.Store.Tx(ctx, func(tx *store.Store) error {
		ok, err := tx.SetHold(ctx, id, in.Reason, u.Username)
		if err != nil {
			return err
		}
		if !ok {
			return apiErr(http.StatusConflict, "HOLD_NOT_POSSIBLE", "the entry is already held or its recordings are being or were deleted")
		}
		if err := tx.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: u.Username, ClientIP: c.RealIP(),
			Action: "recording.hold", TargetType: "history", TargetID: idStr(id), TargetName: h.ConnectionName,
			Outcome: "success", Metadata: map[string]any{"reason": in.Reason, "user": h.Username}}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) deleteHold(c *echo.Context) error {
	if err := requirePerm(c, store.PermAudit); err != nil {
		return err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	h, err := a.Store.GetHistory(ctx, id)
	if err != nil {
		return err
	}
	u := userOf(c)
	err = a.Store.Tx(ctx, func(tx *store.Store) error {
		ok, err := tx.ReleaseHold(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return apiErr(http.StatusConflict, "NOT_HELD", "the entry is not held")
		}
		if err := tx.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: u.Username, ClientIP: c.RealIP(),
			Action: "recording.release", TargetType: "history", TargetID: idStr(id), TargetName: h.ConnectionName,
			Outcome: "success", Metadata: map[string]any{"reason": h.HoldReason}}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// deleteConnectionsKeepingRetention freezes the effective retention of each
// connection into its history, then deletes it (or deleteFn, e.g. a folder),
// all under the purge lock so a running purge never sees a half state.
func (a *App) deleteConnectionsKeepingRetention(ctx context.Context, conns []*store.Connection, deleteFn func(tx *store.Store) error) error {
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return err
	}
	a.purgeMu.Lock()
	defer a.purgeMu.Unlock()
	return a.Store.Tx(ctx, func(tx *store.Store) error {
		for _, c := range conns {
			if err := tx.FreezeHistoryRetention(ctx, c.ID, retentionOr(c.RetentionVideoDays, d.RetentionVideoDays),
				retentionOr(c.RetentionTextDays, d.RetentionTextDays)); err != nil {
				return err
			}
		}
		return deleteFn(tx)
	})
}

// connectionsUnder returns the connections inside a folder subtree.
func (a *App) connectionsUnder(ctx context.Context, groupID int64) ([]*store.Connection, error) {
	groups, err := a.Store.ListConnectionGroups(ctx)
	if err != nil {
		return nil, err
	}
	parent := map[int64]*int64{}
	for _, g := range groups {
		parent[g.ID] = g.ParentID
	}
	inside := func(id *int64) bool {
		for seen := 0; id != nil && seen < len(groups)+1; seen++ {
			if *id == groupID {
				return true
			}
			id = parent[*id]
		}
		return false
	}
	all, err := a.Store.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	var out []*store.Connection
	for _, c := range all {
		if inside(c.ParentID) {
			out = append(out, c)
		}
	}
	return out, nil
}
