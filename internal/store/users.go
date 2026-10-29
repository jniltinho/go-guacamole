package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const userCols = `id, username, password_hash, password_scheme, password_salt, password_changed_at,
	must_change_password, disabled, valid_from, valid_until, access_window_start, access_window_end,
	timezone, full_name, email, security_stamp, created_at, updated_at, auth_source, totp_confirmed`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	var u User
	var salt, vf, vu, ws, we, tz, fn, em sql.NullString
	err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.PasswordScheme, &salt, &u.PasswordChangedAt,
		&u.MustChangePassword, &u.Disabled, &vf, &vu, &ws, &we, &tz, &fn, &em, &u.SecurityStamp, &u.CreatedAt, &u.UpdatedAt,
		&u.AuthSource, &u.TOTPEnrolled)
	if err != nil {
		return nil, mapErr(err)
	}
	u.PasswordSalt = salt.String
	u.ValidFrom, u.ValidUntil = strPtr(vf), strPtr(vu)
	u.AccessWindowStart, u.AccessWindowEnd = strPtr(ws), strPtr(we)
	u.Timezone, u.FullName, u.Email = strPtr(tz), strPtr(fn), strPtr(em)
	return &u, nil
}

// CreateUser inserts a user and returns it with its id.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	now := s.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	if u.PasswordChangedAt == 0 {
		u.PasswordChangedAt = now
	}
	if u.PasswordScheme == "" {
		u.PasswordScheme = SchemeBcrypt
	}
	if u.AuthSource == "" {
		u.AuthSource = SourceLocal
	}
	u.SecurityStamp = RandomHex(16)
	id, err := s.insert(ctx, `INSERT INTO users (username, password_hash, password_scheme, password_salt, password_changed_at,
		must_change_password, disabled, valid_from, valid_until, access_window_start, access_window_end, timezone,
		full_name, email, security_stamp, created_at, updated_at, auth_source) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.Username, u.PasswordHash, u.PasswordScheme, nullStr(&u.PasswordSalt), u.PasswordChangedAt,
		u.MustChangePassword, u.Disabled, nullStr(u.ValidFrom), nullStr(u.ValidUntil), nullStr(u.AccessWindowStart),
		nullStr(u.AccessWindowEnd), nullStr(u.Timezone), nullStr(u.FullName), nullStr(u.Email), u.SecurityStamp, now, now, u.AuthSource)
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

// GetUser loads a user by id.
func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.row(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// GetUserByName loads a user by username (exact match).
func (s *Store) GetUserByName(ctx context.Context, name string) (*User, error) {
	return scanUser(s.row(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, name))
}

// ListUsers lists users matching q (substring of username, name or e-mail).
func (s *Store) ListUsers(ctx context.Context, q string, p Page) ([]*User, int, error) {
	p = p.Normalize()
	where, args := "", []any{}
	if q = strings.TrimSpace(strings.ToLower(q)); q != "" {
		where = ` WHERE LOWER(username) LIKE ? OR LOWER(COALESCE(full_name,'')) LIKE ? OR LOWER(COALESCE(email,'')) LIKE ?`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int
	if err := s.row(ctx, `SELECT COUNT(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.query(ctx, `SELECT `+userCols+` FROM users`+where+` ORDER BY username LIMIT ? OFFSET ?`, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UpdateUser saves profile/restriction fields and rotates the security stamp.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	u.UpdatedAt = s.Now()
	u.SecurityStamp = RandomHex(16)
	res, err := s.exec(ctx, `UPDATE users SET username=?, must_change_password=?, disabled=?, valid_from=?, valid_until=?,
		access_window_start=?, access_window_end=?, timezone=?, full_name=?, email=?, security_stamp=?, updated_at=? WHERE id=?`,
		u.Username, u.MustChangePassword, u.Disabled, nullStr(u.ValidFrom), nullStr(u.ValidUntil), nullStr(u.AccessWindowStart),
		nullStr(u.AccessWindowEnd), nullStr(u.Timezone), nullStr(u.FullName), nullStr(u.Email), u.SecurityStamp, u.UpdatedAt, u.ID)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetPassword stores a new password hash and rotates the security stamp.
func (s *Store) SetPassword(ctx context.Context, id int64, hash, scheme, salt string, mustChange bool) error {
	now := s.Now()
	res, err := s.exec(ctx, `UPDATE users SET password_hash=?, password_scheme=?, password_salt=?, password_changed_at=?,
		must_change_password=?, security_stamp=?, updated_at=? WHERE id=?`,
		hash, scheme, nullStr(&salt), now, mustChange, RandomHex(16), now, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// UpgradeHash replaces the hash without rotating the security stamp
// (transparent legacy → bcrypt migration on login).
func (s *Store) UpgradeHash(ctx context.Context, id int64, hash string) error {
	_, err := s.exec(ctx, `UPDATE users SET password_hash=?, password_scheme=?, password_salt=NULL WHERE id=?`, hash, SchemeBcrypt, id)
	return err
}

// SetUserDisabled toggles the disabled flag.
func (s *Store) SetUserDisabled(ctx context.Context, id int64, disabled bool) error {
	res, err := s.exec(ctx, `UPDATE users SET disabled=?, security_stamp=?, updated_at=? WHERE id=?`, disabled, RandomHex(16), s.Now(), id)
	if err != nil {
		return err
	}
	return affected(res)
}

// DeleteUser removes a user and its grants.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.clearSubject(ctx, SubjectUser, id); err != nil {
			return err
		}
		res, err := tx.exec(ctx, `DELETE FROM users WHERE id=?`, id)
		if err != nil {
			return err
		}
		return affected(res)
	})
}

// CountUsers returns the number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.row(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// UserStamps returns the security stamp of each given user (missing users are omitted).
func (s *Store) UserStamps(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.query(ctx, `SELECT id, security_stamp FROM users WHERE id IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// TouchUserStamp rotates a user's security stamp (e.g. membership change).
func (s *Store) TouchUserStamp(ctx context.Context, ids ...int64) error {
	for _, id := range ids {
		if _, err := s.exec(ctx, `UPDATE users SET security_stamp=? WHERE id=?`, RandomHex(16), id); err != nil {
			return err
		}
	}
	return nil
}

// AdminUserIDs returns ids of enabled users holding ADMINISTER directly or
// through an enabled group.
func (s *Store) AdminUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.query(ctx, `SELECT u.id FROM users u WHERE u.disabled = ? AND (
		EXISTS (SELECT 1 FROM system_permissions sp WHERE sp.subject_type='user' AND sp.subject_id=u.id AND sp.permission='ADMINISTER')
		OR EXISTS (SELECT 1 FROM user_group_members m JOIN user_groups g ON g.id=m.group_id
			JOIN system_permissions sp ON sp.subject_type='group' AND sp.subject_id=g.id AND sp.permission='ADMINISTER'
			WHERE m.user_id=u.id AND g.disabled = ?))`, false, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// User sources: where a user's identity is verified.
const (
	SourceLocal  = "local"
	SourceLDAP   = "ldap"
	SourceOIDC   = "oidc"
	SourceSAML   = "saml"
	SourceHeader = "header"
	SourceJSON   = "json"
)

// TOTP returns the user's TOTP secret (unsealed; "" when none), whether it was
// confirmed by a first valid code, and the last accepted time step.
func (s *Store) TOTP(ctx context.Context, id int64) (secret string, confirmed bool, lastStep int64, err error) {
	var sealed sql.NullString
	if err = s.row(ctx, `SELECT totp_secret, totp_confirmed, totp_last_step FROM users WHERE id=?`, id).Scan(&sealed, &confirmed, &lastStep); err != nil {
		return "", false, 0, mapErr(err)
	}
	if sealed.String != "" {
		secret, err = s.box.Open(sealed.String)
	}
	return secret, confirmed, lastStep, err
}

// SetTOTPSecret stores a new, unconfirmed TOTP secret ("" removes it).
func (s *Store) SetTOTPSecret(ctx context.Context, id int64, secret string) error {
	var val any
	if secret != "" {
		sealed, err := s.box.Seal(secret)
		if err != nil {
			return err
		}
		val = sealed
	}
	res, err := s.exec(ctx, `UPDATE users SET totp_secret=?, totp_confirmed=?, totp_last_step=0, updated_at=? WHERE id=?`, val, false, s.Now(), id)
	if err != nil {
		return err
	}
	return affected(res)
}

// UseTOTPStep confirms the secret and records step as used. It returns false
// when step is not newer than the last accepted one (code replay).
func (s *Store) UseTOTPStep(ctx context.Context, id, step int64) (bool, error) {
	res, err := s.exec(ctx, `UPDATE users SET totp_confirmed=?, totp_last_step=? WHERE id=? AND totp_last_step < ?`, true, step, id, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// BindExternalID records the user's identity at its provider on first use and
// reports false when the user is already bound to another identity.
func (s *Store) BindExternalID(ctx context.Context, id int64, ext string) (bool, error) {
	var cur sql.NullString
	if err := s.row(ctx, `SELECT external_id FROM users WHERE id=?`, id).Scan(&cur); err != nil {
		return false, mapErr(err)
	}
	if cur.String != "" {
		return cur.String == ext, nil
	}
	_, err := s.exec(ctx, `UPDATE users SET external_id=? WHERE id=?`, ext, id)
	return err == nil, err
}

// UseToken consumes a single-use token; false means it was already used.
func (s *Store) UseToken(ctx context.Context, token string, expiresAt int64) (bool, error) {
	if _, err := s.exec(ctx, `DELETE FROM used_tokens WHERE expires_at < ?`, s.Now()); err != nil {
		return false, err
	}
	_, err := s.exec(ctx, `INSERT INTO used_tokens (token, expires_at) VALUES (?,?)`, token, expiresAt)
	if errors.Is(err, ErrConflict) {
		return false, nil
	}
	return err == nil, err
}

// SetUserGroups replaces the user's memberships (groups owned by an external
// identity provider) and rotates the security stamp when they change.
func (s *Store) SetUserGroups(ctx context.Context, userID int64, groupIDs []int64) error {
	return s.Tx(ctx, func(tx *Store) error {
		cur, err := tx.ids(ctx, `SELECT group_id FROM user_group_members WHERE user_id=?`, userID)
		if err != nil {
			return err
		}
		want := uniq(groupIDs)
		if sameIDs(cur, want) {
			return nil
		}
		if _, err := tx.exec(ctx, `DELETE FROM user_group_members WHERE user_id=?`, userID); err != nil {
			return err
		}
		for _, g := range want {
			if _, err := tx.exec(ctx, `INSERT INTO user_group_members (group_id, user_id) VALUES (?,?)`, g, userID); err != nil {
				return err
			}
		}
		return tx.TouchUserStamp(ctx, userID)
	})
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[int64]bool{}
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if !set[v] {
			return false
		}
	}
	return true
}
