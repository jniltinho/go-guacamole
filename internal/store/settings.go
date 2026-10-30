package store

import (
	"context"
	"encoding/json"
	"errors"
)

// GetSetting decodes the named setting into v; v is left untouched when unset.
func (s *Store) GetSetting(ctx context.Context, name string, v any) error {
	var sealed string
	if err := s.row(ctx, `SELECT value FROM settings WHERE name=?`, name).Scan(&sealed); err != nil {
		if err = mapErr(err); errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	raw, err := s.box.Open(sealed)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// PutSetting stores v as the named setting (sealed: settings may hold secrets).
func (s *Store) PutSetting(ctx context.Context, name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sealed, err := s.box.Seal(string(raw))
	if err != nil {
		return err
	}
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.exec(ctx, `DELETE FROM settings WHERE name=?`, name); err != nil {
			return err
		}
		_, err := tx.exec(ctx, `INSERT INTO settings (name, value, updated_at) VALUES (?,?,?)`, name, sealed, tx.Now())
		return err
	})
}

// InstallationID returns the id generated once per database: it namespaces object keys and marks the job queue's
// Redis database. Concurrent first starts agree on the first id written.
func (s *Store) InstallationID(ctx context.Context) (string, error) {
	var id string
	if err := s.GetSetting(ctx, "installation", &id); err != nil || id != "" {
		return id, err
	}
	raw, err := json.Marshal(RandomHex(6))
	if err != nil {
		return "", err
	}
	sealed, err := s.box.Seal(string(raw))
	if err != nil {
		return "", err
	}
	// a plain INSERT: if another replica wrote it first, its id wins
	_, _ = s.exec(ctx, `INSERT INTO settings (name, value, updated_at) VALUES (?, ?, ?)`, "installation", sealed, s.Now())
	err = s.GetSetting(ctx, "installation", &id)
	if err == nil && id == "" {
		err = errors.New("installation id not stored")
	}
	return id, err
}
