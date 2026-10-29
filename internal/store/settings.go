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
