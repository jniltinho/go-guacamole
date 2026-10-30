package store

import (
	"context"
	"encoding/json"
)

// Object storage targets (openspec add-async-jobs-object-storage): each version of a target's location is a row whose
// configuration, credentials included, is sealed with the application secret. Recording sets reference the row that
// holds them, so a target stays readable after the administrator moves uploads elsewhere.

// CreateStorageTarget stores a new target version and returns its id.
func (s *Store) CreateStorageTarget(ctx context.Context, cfg any) (int64, error) {
	sealed, err := s.sealJSON(cfg)
	if err != nil {
		return 0, err
	}
	return s.insert(ctx, `INSERT INTO storage_targets (config, created_at) VALUES (?, ?)`, sealed, s.Now())
}

// UpdateStorageTarget replaces a target's configuration in place (credentials, encryption: same location).
func (s *Store) UpdateStorageTarget(ctx context.Context, id int64, cfg any) error {
	sealed, err := s.sealJSON(cfg)
	if err != nil {
		return err
	}
	res, err := s.exec(ctx, `UPDATE storage_targets SET config=? WHERE id=?`, sealed, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// GetStorageTarget loads a target's configuration into cfg.
func (s *Store) GetStorageTarget(ctx context.Context, id int64, cfg any) error {
	var sealed string
	if err := s.row(ctx, `SELECT config FROM storage_targets WHERE id=?`, id).Scan(&sealed); err != nil {
		return mapErr(err)
	}
	raw, err := s.box.Open(sealed)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), cfg)
}

// RetireStorageTarget marks a target version as no longer used for new uploads (it stays readable).
func (s *Store) RetireStorageTarget(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `UPDATE storage_targets SET retired_at=? WHERE id=? AND retired_at IS NULL`, s.Now(), id)
	return err
}

func (s *Store) sealJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return s.box.Seal(string(raw))
}
