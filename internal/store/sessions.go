package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// CreateSession inserts a session.
func (s *Store) CreateSession(ctx context.Context, x *Session) error {
	_, err := s.exec(ctx, `INSERT INTO app_sessions (id, user_id, csrf_token, security_stamp, created_at, last_seen, expires_at, client_ip, user_agent)
		VALUES (?,?,?,?,?,?,?,?,?)`, x.ID, x.UserID, x.CSRFToken, x.SecurityStamp, x.CreatedAt, x.LastSeen, x.ExpiresAt, x.ClientIP, truncate(x.UserAgent, 512))
	return err
}

// GetSession loads a session by its hashed id.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var x Session
	var ip, ua sql.NullString
	err := s.row(ctx, `SELECT id, user_id, csrf_token, security_stamp, created_at, last_seen, expires_at, client_ip, user_agent
		FROM app_sessions WHERE id=?`, id).Scan(&x.ID, &x.UserID, &x.CSRFToken, &x.SecurityStamp, &x.CreatedAt, &x.LastSeen, &x.ExpiresAt, &ip, &ua)
	if err != nil {
		return nil, mapErr(err)
	}
	x.ClientIP, x.UserAgent = ip.String, ua.String
	return &x, nil
}

// SetSessionStamp updates the password version a session is bound to.
func (s *Store) SetSessionStamp(ctx context.Context, id, stamp string) error {
	_, err := s.exec(ctx, `UPDATE app_sessions SET security_stamp=? WHERE id=?`, stamp, id)
	return err
}

// TouchSession updates last_seen.
func (s *Store) TouchSession(ctx context.Context, id string, at int64) error {
	_, err := s.exec(ctx, `UPDATE app_sessions SET last_seen=? WHERE id=?`, at, id)
	return err
}

// DeleteSession removes one session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM app_sessions WHERE id=?`, id)
	return err
}

// DeleteUserSessions removes all sessions of a user, except keep (may be empty).
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keep string) ([]string, error) {
	var ids []string
	rows, err := s.query(ctx, `SELECT id FROM app_sessions WHERE user_id=? AND id<>?`, userID, keep)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	_, err = s.exec(ctx, `DELETE FROM app_sessions WHERE user_id=? AND id<>?`, userID, keep)
	return ids, err
}

// DeleteExpiredSessions removes sessions past their absolute or idle expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now, idleCutoff int64) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM app_sessions WHERE expires_at < ? OR last_seen < ?`, now, idleCutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetPreferences returns user preferences (zero values when unset).
func (s *Store) GetPreferences(ctx context.Context, userID int64) (*Preferences, error) {
	var p Preferences
	var theme, locale, effects, term sql.NullString
	err := s.row(ctx, `SELECT theme, locale, sidebar_collapsed, effects, terminal_scheme FROM preferences WHERE user_id=?`, userID).
		Scan(&theme, &locale, &p.SidebarCollapsed, &effects, &term)
	if err == sql.ErrNoRows {
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	p.Theme, p.Locale, p.Effects, p.TerminalScheme = theme.String, locale.String, effects.String, term.String
	return &p, nil
}

// PutPreferences upserts user preferences.
func (s *Store) PutPreferences(ctx context.Context, userID int64, p *Preferences) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.exec(ctx, `DELETE FROM preferences WHERE user_id=?`, userID); err != nil {
			return err
		}
		_, err := tx.exec(ctx, `INSERT INTO preferences (user_id, theme, locale, sidebar_collapsed, effects, terminal_scheme) VALUES (?,?,?,?,?,?)`,
			userID, p.Theme, p.Locale, p.SidebarCollapsed, nullStr(&p.Effects), nullStr(&p.TerminalScheme))
		return err
	})
}

// StartHistory records the start of a tunnel.
func (s *Store) StartHistory(ctx context.Context, h *HistoryEntry) error {
	id, err := s.insert(ctx, `INSERT INTO connection_history (user_id, username, connection_id, connection_name, protocol, client_ip, tunnel_id, start_at, recording, typescript,
		rec_video, rec_text, rec_keys, rec_exception, video_capture, text_capture, owner_instance)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, nullInt64(h.UserID), h.Username, nullInt64(h.ConnectionID), h.ConnectionName, h.Protocol, h.ClientIP, h.TunnelID, h.StartAt,
		nullStr(&h.Recording), nullStr(&h.Typescript), nullBool(h.RecVideo), nullBool(h.RecText), nullBool(h.RecKeys), h.RecException,
		nullStr(&h.VideoCapture), nullStr(&h.TextCapture), nullStr(&h.OwnerInstance))
	if err != nil {
		return err
	}
	h.ID = id
	return nil
}

// EndHistory records the end of a tunnel.
func (s *Store) EndHistory(ctx context.Context, id, at int64, reason string) error {
	_, err := s.exec(ctx, `UPDATE connection_history SET end_at=?, close_reason=? WHERE id=? AND end_at IS NULL`, at, reason, id)
	return err
}

// HistoryFilter filters history queries.
type HistoryFilter struct {
	UserID       *int64
	ConnectionID *int64
	Protocol     string
	From, To     int64
	Q            string
}

// ListHistory lists history newest first.
func (s *Store) ListHistory(ctx context.Context, f HistoryFilter, p Page) ([]*HistoryEntry, int, error) {
	p = p.Normalize()
	// launches whose ticket was never used are not sessions
	conds := []string{"(close_reason IS NULL OR close_reason <> 'not_started')"}
	var args []any
	if f.UserID != nil {
		conds, args = append(conds, "user_id=?"), append(args, *f.UserID)
	}
	if f.ConnectionID != nil {
		conds, args = append(conds, "connection_id=?"), append(args, *f.ConnectionID)
	}
	if f.Protocol != "" {
		conds, args = append(conds, "protocol=?"), append(args, f.Protocol)
	}
	if f.From > 0 {
		conds, args = append(conds, "start_at>=?"), append(args, f.From)
	}
	if f.To > 0 {
		conds, args = append(conds, "start_at<=?"), append(args, f.To)
	}
	if q := strings.ToLower(strings.TrimSpace(f.Q)); q != "" {
		conds, args = append(conds, "(LOWER(username) LIKE ? OR LOWER(connection_name) LIKE ?)"), append(args, "%"+q+"%", "%"+q+"%")
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := s.row(ctx, `SELECT COUNT(*) FROM connection_history`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.query(ctx, `SELECT `+historyCols+` FROM connection_history`+where+` ORDER BY start_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*HistoryEntry
	for rows.Next() {
		h, err := scanHistory(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}

const historyCols = `id, user_id, username, connection_id, connection_name, protocol, client_ip, tunnel_id, start_at, end_at, close_reason, recording, typescript,
	rec_video, rec_text, rec_keys, rec_exception, video_capture, text_capture, video_files, text_files, video_purged_at, text_purged_at,
	retention_video_days, retention_text_days, hold_reason, hold_by, hold_at, owner_instance`

func scanHistory(sc interface{ Scan(...any) error }) (*HistoryEntry, error) {
	var h HistoryEntry
	var uid, cid, end, vpa, tpa, rv, rt, hat sql.NullInt64
	var ip, reason, rec, ts, vc, tc, vf, tf, hr, hb, owner sql.NullString
	var recV, recT, recK sql.NullBool
	if err := sc.Scan(&h.ID, &uid, &h.Username, &cid, &h.ConnectionName, &h.Protocol, &ip, &h.TunnelID, &h.StartAt, &end, &reason, &rec, &ts,
		&recV, &recT, &recK, &h.RecException, &vc, &tc, &vf, &tf, &vpa, &tpa, &rv, &rt, &hr, &hb, &hat, &owner); err != nil {
		return nil, mapErr(err)
	}
	h.UserID, h.ConnectionID, h.EndAt, h.CloseReason, h.ClientIP = int64Ptr(uid), int64Ptr(cid), int64Ptr(end), strPtr(reason), ip.String
	h.Recording, h.Typescript = rec.String, ts.String
	h.RecVideo, h.RecText, h.RecKeys = boolPtr(recV), boolPtr(recT), boolPtr(recK)
	h.VideoCapture, h.TextCapture, h.VideoFiles, h.TextFiles = vc.String, tc.String, vf.String, tf.String
	h.VideoPurgedAt, h.TextPurgedAt, h.HoldAt = int64Ptr(vpa), int64Ptr(tpa), int64Ptr(hat)
	h.RetentionVideoDays, h.RetentionTextDays = intPtr(rv), intPtr(rt)
	h.HoldReason, h.HoldBy, h.OwnerInstance = strPtr(hr), strPtr(hb), owner.String
	// a recording can be played only while its files are there (unknown state = sessions before the policy)
	h.HasRecording = h.Recording != "" && (h.VideoFiles == "" || h.VideoFiles == "present")
	h.HasTypescript = h.Typescript != "" && (h.TextFiles == "" || h.TextFiles == "present")
	return &h, nil
}

// GetHistory loads one history row.
func (s *Store) GetHistory(ctx context.Context, id int64) (*HistoryEntry, error) {
	return scanHistory(s.row(ctx, `SELECT `+historyCols+` FROM connection_history WHERE id=?`, id))
}

// AccessStat aggregates connection history for one server or one user.
type AccessStat struct {
	Key         string `json:"key"`
	Sessions    int64  `json:"sessions"`
	TotalMs     int64  `json:"totalMs"`
	Distinct    int64  `json:"distinct"` // users per server, servers per user
	LastAccess  int64  `json:"lastAccess"`
	ActiveCount int64  `json:"active"`
}

// AccessStats groups history since `since` by "connection" or "user"; open
// sessions count up to `now`.
func (s *Store) AccessStats(ctx context.Context, by string, since, now int64, limit int) ([]AccessStat, error) {
	key, other := "connection_name", "username"
	if by == "user" {
		key, other = "username", "connection_name"
	}
	d := s.db.D
	q := `SELECT ` + key + `, COUNT(*), ` + d.CastInt(`SUM(COALESCE(end_at, ?) - start_at)`) + `, COUNT(DISTINCT ` + other + `), MAX(start_at), ` +
		d.CastInt(`SUM(CASE WHEN end_at IS NULL THEN 1 ELSE 0 END)`) + `
		FROM connection_history WHERE start_at >= ? AND (close_reason IS NULL OR close_reason <> 'not_started') GROUP BY ` + key + ` ORDER BY 3 DESC LIMIT ?`
	rows, err := s.query(ctx, q, now, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccessStat{}
	for rows.Next() {
		var a AccessStat
		if err := rows.Scan(&a.Key, &a.Sessions, &a.TotalMs, &a.Distinct, &a.LastAccess, &a.ActiveCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// InsertAudit writes an audit event.
func (s *Store) InsertAudit(ctx context.Context, e *AuditEvent) error {
	var meta any
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return err
		}
		meta = string(b)
	}
	if e.At == 0 {
		e.At = s.Now()
	}
	id, err := s.insert(ctx, `INSERT INTO audit_events (at, actor_id, actor_name, client_ip, action, target_type, target_id, target_name, outcome, metadata)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, e.At, nullInt64(e.ActorID), e.ActorName, e.ClientIP, e.Action, e.TargetType, e.TargetID, truncate(e.TargetName, 256), e.Outcome, meta)
	if err != nil {
		return err
	}
	e.ID = id
	return nil
}

// AuditFilter filters audit queries.
type AuditFilter struct {
	ActorID    *int64
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	From, To   int64
}

// ListAudit lists audit events newest first.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter, p Page) ([]*AuditEvent, int, error) {
	p = p.Normalize()
	var conds []string
	var args []any
	if f.ActorID != nil {
		conds, args = append(conds, "actor_id=?"), append(args, *f.ActorID)
	}
	if f.Actor != "" {
		conds, args = append(conds, "LOWER(actor_name) LIKE ?"), append(args, "%"+strings.ToLower(f.Actor)+"%")
	}
	if f.Action != "" {
		conds, args = append(conds, "action LIKE ?"), append(args, f.Action+"%")
	}
	if f.TargetType != "" {
		conds, args = append(conds, "target_type=?"), append(args, f.TargetType)
	}
	if f.TargetID != "" {
		conds, args = append(conds, "target_id=?"), append(args, f.TargetID)
	}
	if f.From > 0 {
		conds, args = append(conds, "at>=?"), append(args, f.From)
	}
	if f.To > 0 {
		conds, args = append(conds, "at<=?"), append(args, f.To)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := s.row(ctx, `SELECT COUNT(*) FROM audit_events`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.query(ctx, `SELECT id, at, actor_id, actor_name, client_ip, action, target_type, target_id, target_name, outcome, metadata
		FROM audit_events`+where+` ORDER BY at DESC, id DESC LIMIT ? OFFSET ?`, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*AuditEvent
	for rows.Next() {
		var e AuditEvent
		var actor sql.NullInt64
		var an, ip, tt, ti, tn, meta sql.NullString
		if err := rows.Scan(&e.ID, &e.At, &actor, &an, &ip, &e.Action, &tt, &ti, &tn, &e.Outcome, &meta); err != nil {
			return nil, 0, err
		}
		e.ActorID, e.ActorName, e.ClientIP = int64Ptr(actor), an.String, ip.String
		e.TargetType, e.TargetID, e.TargetName = tt.String, ti.String, tn.String
		if meta.Valid && meta.String != "" {
			_ = json.Unmarshal([]byte(meta.String), &e.Metadata)
		}
		out = append(out, &e)
	}
	return out, total, rows.Err()
}

// PurgeAudit deletes events older than before.
func (s *Store) PurgeAudit(ctx context.Context, before int64) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM audit_events WHERE at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Counts returns totals for the dashboard.
func (s *Store) Counts(ctx context.Context) (users, groups int, err error) {
	if err = s.row(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return
	}
	err = s.row(ctx, `SELECT COUNT(*) FROM user_groups`).Scan(&groups)
	return
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
