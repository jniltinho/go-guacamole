package store

import (
	"context"
	"database/sql"
	"strings"
)

// Recording file and capture states stored per kind in connection_history.
const (
	CapturePending    = "pending"
	CaptureOK         = "ok"
	CaptureFailed     = "failed"
	CaptureException  = "exception"
	CaptureNotStarted = "not_started"

	FilesPresent = "present"
	FilesPurging = "purging"
	FilesPurged  = "purged"
	FilesMissing = "missing"
	FilesError   = "error"
)

// RecordingPolicy is a connection's recording override set; nil fields inherit.
type RecordingPolicy struct {
	Video, Text, Keys                     *string
	RetentionVideoDays, RetentionTextDays *int
}

// SetConnectionRecording replaces a connection's recording overrides if its
// policy version is still version; false means another change won the race.
func (s *Store) SetConnectionRecording(ctx context.Context, id int64, p RecordingPolicy, version int) (bool, error) {
	res, err := s.exec(ctx, `UPDATE connections SET rec_video=?, rec_text=?, rec_keys=?, retention_video_days=?, retention_text_days=?,
		rec_version=rec_version+1, updated_at=? WHERE id=? AND rec_version=?`,
		nullStr(p.Video), nullStr(p.Text), nullStr(p.Keys), nullInt(p.RetentionVideoDays), nullInt(p.RetentionTextDays), s.Now(), id, version)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// FreezeHistoryRetention stores the retention in effect for a connection being
// deleted in its history entries, so deletion never shortens it.
func (s *Store) FreezeHistoryRetention(ctx context.Context, connID int64, videoDays, textDays int) error {
	_, err := s.exec(ctx, `UPDATE connection_history SET retention_video_days=?, retention_text_days=? WHERE connection_id=?`,
		videoDays, textDays, connID)
	return err
}

// ActivateHistory sets the real start of a session once its tunnel is active.
func (s *Store) ActivateHistory(ctx context.Context, id, at int64) error {
	_, err := s.exec(ctx, `UPDATE connection_history SET start_at=? WHERE id=?`, at, id)
	return err
}

func kindCols(video bool) (capture, files, purgedAt string) {
	if video {
		return "video_capture", "video_files", "video_purged_at"
	}
	return "text_capture", "text_files", "text_purged_at"
}

// SetCapture records the capture result of one kind; files is the file set state ("" leaves it).
func (s *Store) SetCapture(ctx context.Context, id int64, video bool, capture, files string) error {
	c, f, _ := kindCols(video)
	if files == "" {
		_, err := s.exec(ctx, `UPDATE connection_history SET `+c+`=? WHERE id=?`, capture, id)
		return err
	}
	_, err := s.exec(ctx, `UPDATE connection_history SET `+c+`=?, `+f+`=? WHERE id=?`, capture, files, id)
	return err
}

// BeginPurge marks a file set as being deleted and takes its claim (token, until) for the purge, unless the entry is
// held, the set is not deletable or another claim (upload, local removal, purge) is valid; false means it must not
// be deleted now.
func (s *Store) BeginPurge(ctx context.Context, id int64, video bool, token string, until int64) (bool, error) {
	_, f, _ := kindCols(video)
	k := storagePrefix(video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+f+`=?, `+k+`claim_owner=?, `+k+`claim_token=?, `+k+`claim_until=?
		WHERE id=? AND hold_at IS NULL AND end_at IS NOT NULL AND `+f+` IN (?,?)
		AND (`+k+`claim_until IS NULL OR `+k+`claim_until < ?)`,
		FilesPurging, "purge", token, until, id, FilesPresent, FilesError, s.Now())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RecoverPurging turns sets left in "purging" by a purge whose claim expired (a crash, a lost lease) back into
// "error" so the next purge retries them; a purge still holding its claim keeps its sets.
func (s *Store) RecoverPurging(ctx context.Context) error {
	for _, video := range []bool{true, false} {
		_, f, _ := kindCols(video)
		k := storagePrefix(video)
		if _, err := s.exec(ctx, `UPDATE connection_history SET `+f+`=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
			WHERE `+f+`=? AND (`+k+`claim_until IS NULL OR `+k+`claim_until < ?)`, FilesError, FilesPurging, s.Now()); err != nil {
			return err
		}
	}
	return nil
}

// EndPurge records the outcome of a deletion started with BeginPurge and releases its claim (fenced by the token).
func (s *Store) EndPurge(ctx context.Context, id int64, video bool, files, token string) error {
	_, f, p := kindCols(video)
	k := storagePrefix(video)
	var at any
	if files == FilesPurged {
		at = s.Now()
	}
	_, err := s.exec(ctx, `UPDATE connection_history SET `+f+`=?, `+p+`=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+f+`=? AND `+k+`claim_token=?`, files, at, id, FilesPurging, token)
	return err
}

// SetHold places a legal hold; false when the entry is already held or a file
// set is being or has been deleted.
func (s *Store) SetHold(ctx context.Context, id int64, reason, by string) (bool, error) {
	now := s.Now()
	// refused while a purge or a storage claim (upload, local removal) works on the recording
	res, err := s.exec(ctx, `UPDATE connection_history SET hold_reason=?, hold_by=?, hold_at=? WHERE id=? AND hold_at IS NULL
		AND COALESCE(video_files,'') NOT IN (?,?) AND COALESCE(text_files,'') NOT IN (?,?)
		AND (video_claim_until IS NULL OR video_claim_until < ?) AND (text_claim_until IS NULL OR text_claim_until < ?)`,
		reason, by, now, id, FilesPurging, FilesPurged, FilesPurging, FilesPurged, now, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// ReleaseHold removes a legal hold; false when the entry was not held.
func (s *Store) ReleaseHold(ctx context.Context, id int64) (bool, error) {
	res, err := s.exec(ctx, `UPDATE connection_history SET hold_reason=NULL, hold_by=NULL, hold_at=NULL WHERE id=? AND hold_at IS NOT NULL`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// PurgeCandidates returns finished, unheld entries with a deletable file set.
func (s *Store) PurgeCandidates(ctx context.Context) ([]*HistoryEntry, error) {
	return s.historyWhere(ctx, `end_at IS NOT NULL AND hold_at IS NULL AND (video_files IN (?,?,?) OR text_files IN (?,?,?))`,
		FilesPresent, FilesError, FilesPurging, FilesPresent, FilesError, FilesPurging)
}

// UndiscoveredHistory returns finished entries whose file sets were never checked
// (sessions from before the recording policy, or interrupted by a crash).
// With missing false only sets never checked are returned (sessions ended by another instance, a crash).
func (s *Store) UndiscoveredHistory(ctx context.Context, missing bool) ([]*HistoryEntry, error) {
	again := FilesMissing
	if !missing {
		again = "" // matches no state
	}
	return s.historyWhere(ctx, `end_at IS NOT NULL AND ((recording IS NOT NULL AND (video_files IS NULL OR video_files=?))
		OR (typescript IS NOT NULL AND (text_files IS NULL OR text_files=?)))`, again, again)
}

// SetFiles records a discovered file set state.
func (s *Store) SetFiles(ctx context.Context, id int64, video bool, files string) error {
	_, f, _ := kindCols(video)
	_, err := s.exec(ctx, `UPDATE connection_history SET `+f+`=? WHERE id=?`, files, id)
	return err
}

// HeldHistory returns entries under legal hold.
func (s *Store) HeldHistory(ctx context.Context) ([]*HistoryEntry, error) {
	return s.historyWhere(ctx, `hold_at IS NOT NULL`)
}

// OpenHistory returns entries still open (session running, ticket pending or abandoned).
func (s *Store) OpenHistory(ctx context.Context) ([]*HistoryEntry, error) {
	return s.historyWhere(ctx, `end_at IS NULL`)
}

// FinishNotStarted closes an entry whose ticket was never used.
func (s *Store) FinishNotStarted(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `UPDATE connection_history SET end_at=start_at, close_reason='not_started',
		video_capture=CASE WHEN video_capture IS NULL THEN NULL ELSE ? END,
		text_capture=CASE WHEN text_capture IS NULL THEN NULL ELSE ? END WHERE id=? AND end_at IS NULL`,
		CaptureNotStarted, CaptureNotStarted, id)
	return err
}

// HistoryByTunnel maps tunnel ids to their history entries (missing ids are omitted).
func (s *Store) HistoryByTunnel(ctx context.Context, tunnelIDs []string) (map[string]*HistoryEntry, error) {
	out := map[string]*HistoryEntry{}
	for len(tunnelIDs) > 0 {
		n := min(len(tunnelIDs), 500)
		args := make([]any, n)
		for i, v := range tunnelIDs[:n] {
			args[i] = v
		}
		hs, err := s.historyWhere(ctx, `tunnel_id IN (`+placeholders(n)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for _, h := range hs {
			out[h.TunnelID] = h
		}
		tunnelIDs = tunnelIDs[n:]
	}
	return out, nil
}

func (s *Store) historyWhere(ctx context.Context, where string, args ...any) ([]*HistoryEntry, error) {
	rows, err := s.query(ctx, `SELECT `+historyCols+` FROM connection_history WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HistoryEntry
	for rows.Next() {
		h, err := scanHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// PurgeAuditKeeping deletes audit events older than before except those whose
// target is listed in keep (target type → ids) or whose metadata names one of
// the tunnels, e.g. evidence of held sessions.
func (s *Store) PurgeAuditKeeping(ctx context.Context, before int64, keep map[string][]string, tunnels []string) (int64, error) {
	q, args := `DELETE FROM audit_events WHERE at < ?`, []any{before}
	for _, t := range tunnels {
		q += ` AND (metadata IS NULL OR metadata NOT LIKE ?)`
		args = append(args, `%"tunnel":"`+t+`"%`)
	}
	for typ, ids := range keep {
		if len(ids) == 0 {
			continue
		}
		q += ` AND NOT (target_type = ? AND target_id IN (` + placeholders(len(ids)) + `))`
		args = append(args, typ)
		for _, id := range ids {
			args = append(args, id)
		}
	}
	res, err := s.exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecordingParam reports whether a connection parameter is a guacd recording
// setting, which the recording policy controls instead.
func RecordingParam(name string) bool {
	return strings.HasPrefix(name, "recording-") || strings.HasPrefix(name, "typescript-") ||
		name == "create-recording-path" || name == "create-typescript-path"
}

// InventoryItem describes a connection migrated from recording parameters to overrides.
type InventoryItem struct {
	ConnectionID   int64    `json:"connectionId,string"`
	ConnectionName string   `json:"connectionName"`
	Video          bool     `json:"video"`
	Text           bool     `json:"text"`
	Keys           bool     `json:"keys"`
	Removed        []string `json:"removed"`
}

// ApplyRecordingInventory turns recording parameters still stored on connections
// into explicit overrides (video, text for SSH, keys only with video) and removes
// them. It is idempotent: connections without such parameters are untouched.
func (s *Store) ApplyRecordingInventory(ctx context.Context) ([]InventoryItem, error) {
	rows, err := s.query(ctx, `SELECT connection_id, name, value FROM connection_parameters WHERE name LIKE 'recording-%'
		OR name LIKE 'typescript-%' OR name IN ('create-recording-path','create-typescript-path')`)
	if err != nil {
		return nil, err
	}
	params := map[int64]map[string]string{}
	for rows.Next() {
		var id int64
		var n string
		var v sql.NullString
		if err := rows.Scan(&id, &n, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if params[id] == nil {
			params[id] = map[string]string{}
		}
		params[id][n] = v.String
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err // never change a connection from a partial read
	}
	var out []InventoryItem
	for id, p := range params {
		err := s.Tx(ctx, func(tx *Store) error {
			c, err := tx.GetConnection(ctx, id)
			if err != nil {
				return err
			}
			it := InventoryItem{ConnectionID: id, ConnectionName: c.Name}
			on := "on"
			pol := RecordingPolicy{Video: c.RecVideo, Text: c.RecText, Keys: c.RecKeys,
				RetentionVideoDays: c.RetentionVideoDays, RetentionTextDays: c.RetentionTextDays}
			if p["recording-path"] != "" && pol.Video == nil {
				pol.Video, it.Video = &on, true
			}
			if p["typescript-path"] != "" && c.Protocol == "ssh" && pol.Text == nil {
				pol.Text, it.Text = &on, true
			}
			if p["recording-include-keys"] == "true" && pol.Video != nil && *pol.Video == "on" && pol.Keys == nil {
				pol.Keys, it.Keys = &on, true
			}
			for n := range p {
				it.Removed = append(it.Removed, n)
			}
			if _, err := tx.exec(ctx, `DELETE FROM connection_parameters WHERE connection_id=? AND (name LIKE 'recording-%'
				OR name LIKE 'typescript-%' OR name IN ('create-recording-path','create-typescript-path'))`, id); err != nil {
				return err
			}
			if _, err := tx.SetConnectionRecording(ctx, id, pol, c.RecVersion); err != nil {
				return err
			}
			out = append(out, it)
			return nil
		})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
