package store

import "context"

// Recording storage states of a set (video, or transcript with timing).
const (
	StorageLocal         = "local"
	StorageOwed          = "owed"
	StorageUploading     = "uploading"
	StorageRemote        = "remote"
	StorageFailed        = "failed"
	StorageDeletePending = "delete_pending"
	StorageDeleted       = "deleted"
)

// storagePrefix is the column prefix of a set ("video_" or "text_"; never user input).
func storagePrefix(video bool) string {
	if video {
		return "video_"
	}
	return "text_"
}

// SetRef names one recording set.
type SetRef struct {
	ID    int64
	Video bool
}

// MarkUploadsOwed records an upload owed for every ended set whose files are present and that is not in object
// storage yet (called by the reconciliation and right after a session's files are found).
func (s *Store) MarkUploadsOwed(ctx context.Context) (int64, error) {
	var total int64
	for _, video := range []bool{true, false} {
		k := storagePrefix(video)
		_, f, _ := kindCols(video)
		res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=? WHERE end_at IS NOT NULL AND `+f+`=?
			AND (`+k+`storage IS NULL OR `+k+`storage=?) AND `+k+`local_removed=?`, StorageOwed, FilesPresent, StorageLocal, false)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// RetryFailedUploads gives failed sets (one history row, or all with id 0) a fresh count of attempts.
func (s *Store) RetryFailedUploads(ctx context.Context, id int64) (int64, error) {
	var total int64
	for _, video := range []bool{true, false} {
		k := storagePrefix(video)
		q := `UPDATE connection_history SET ` + k + `storage=?, ` + k + `attempts=0 WHERE ` + k + `storage=?`
		args := []any{StorageOwed, StorageFailed}
		if id != 0 {
			q += ` AND id=?`
			args = append(args, id)
		}
		res, err := s.exec(ctx, q, args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ResetExpiredUploads moves sets left "uploading" by a worker whose claim expired back to "owed".
func (s *Store) ResetExpiredUploads(ctx context.Context, now int64) error {
	for _, video := range []bool{true, false} {
		k := storagePrefix(video)
		if _, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
			WHERE `+k+`storage=? AND `+k+`claim_until < ?`, StorageOwed, StorageUploading, now); err != nil {
			return err
		}
	}
	return nil
}

// UploadQueue lists sets owed an upload, or failed with fewer than maxAttempts, with no valid claim.
func (s *Store) UploadQueue(ctx context.Context, now int64, maxAttempts, limit int) ([]SetRef, error) {
	var out []SetRef
	for _, video := range []bool{true, false} {
		k := storagePrefix(video)
		rows, err := s.db.QueryContext(ctx, s.db.D.Rebind(`SELECT id FROM connection_history WHERE (`+k+`storage=? OR (`+k+`storage=? AND `+k+`attempts < ?))
			AND (`+k+`claim_until IS NULL OR `+k+`claim_until < ?) ORDER BY id LIMIT ?`), StorageOwed, StorageFailed, maxAttempts, now, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			r := SetRef{Video: video}
			if err := rows.Scan(&r.ID); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ClaimUpload takes a set owed (or failed) an upload for the worker identified by token until `until`.
func (s *Store) ClaimUpload(ctx context.Context, ref SetRef, owner, token string, now, until int64) (bool, error) {
	k := storagePrefix(ref.Video)
	_, f, _ := kindCols(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`claim_owner=?, `+k+`claim_token=?, `+k+`claim_until=?
		WHERE id=? AND `+k+`storage IN (?,?) AND `+f+`=? AND (`+k+`claim_until IS NULL OR `+k+`claim_until < ?)`,
		StorageUploading, owner, token, until, ref.ID, StorageOwed, StorageFailed, FilesPresent, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RenewClaim extends a valid claim held by token.
func (s *Store) RenewClaim(ctx context.Context, ref SetRef, token string, now, until int64) (bool, error) {
	k := storagePrefix(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`claim_until=? WHERE id=? AND `+k+`claim_token=? AND `+k+`claim_until >= ?`,
		until, ref.ID, token, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// PlanUpload records where a claimed upload is about to write (target version and keys), so the objects of an attempt
// that fails half-way are still known to the retention purge.
func (s *Store) PlanUpload(ctx context.Context, ref SetRef, token string, now, targetID int64, manifest string) (bool, error) {
	k := storagePrefix(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`target_id=?, `+k+`remote=? WHERE id=? AND `+k+`claim_token=? AND `+k+`claim_until >= ?`,
		targetID, manifest, ref.ID, token, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// FinishUpload records a verified upload (fenced: only the holder of a valid claim, on a set not purged meanwhile).
func (s *Store) FinishUpload(ctx context.Context, ref SetRef, token string, now, targetID int64, manifest string) (bool, error) {
	k := storagePrefix(ref.Video)
	_, f, _ := kindCols(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`target_id=?, `+k+`remote=?, `+k+`uploaded_at=?, `+k+`storage_error=NULL,
		`+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+k+`claim_token=? AND `+k+`claim_until >= ? AND `+f+`=?`,
		StorageRemote, targetID, manifest, now, ref.ID, token, now, FilesPresent)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// FailUpload records a failed attempt (fenced) with a sanitized error.
func (s *Store) FailUpload(ctx context.Context, ref SetRef, token string, now int64, msg string) (bool, error) {
	k := storagePrefix(ref.Video)
	if len(msg) > 500 {
		msg = msg[:500]
	}
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`attempts=`+k+`attempts+1, `+k+`storage_error=?,
		`+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL WHERE id=? AND `+k+`claim_token=? AND `+k+`claim_until >= ?`,
		StorageFailed, msg, ref.ID, token, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// ReleaseClaim gives a set back to "owed" without counting an attempt (a set not ready yet, a shutdown).
func (s *Store) ReleaseClaim(ctx context.Context, ref SetRef, token string) error {
	k := storagePrefix(ref.Video)
	_, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+k+`claim_token=? AND `+k+`storage=?`, StorageOwed, ref.ID, token, StorageUploading)
	return err
}

// RemovalCandidates lists sets stored remotely whose local files may be removed: ended before cutoff, not held,
// files present and not removed yet.
func (s *Store) RemovalCandidates(ctx context.Context, cutoff int64, limit int) ([]SetRef, error) {
	var out []SetRef
	for _, video := range []bool{true, false} {
		k := storagePrefix(video)
		_, f, _ := kindCols(video)
		rows, err := s.db.QueryContext(ctx, s.db.D.Rebind(`SELECT id FROM connection_history WHERE `+k+`storage=? AND `+k+`local_removed=?
			AND `+f+`=? AND hold_at IS NULL AND end_at IS NOT NULL AND end_at <= ? ORDER BY id LIMIT ?`), StorageRemote, false, FilesPresent, cutoff, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			r := SetRef{Video: video}
			if err := rows.Scan(&r.ID); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ClaimLocalRemoval takes a remote set for local removal (not held, still ended before cutoff, claim free).
func (s *Store) ClaimLocalRemoval(ctx context.Context, ref SetRef, owner, token string, now, until, cutoff int64) (bool, error) {
	k := storagePrefix(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`claim_owner=?, `+k+`claim_token=?, `+k+`claim_until=?
		WHERE id=? AND `+k+`storage=? AND `+k+`local_removed=? AND hold_at IS NULL AND end_at <= ?
		AND (`+k+`claim_until IS NULL OR `+k+`claim_until < ?)`, owner, token, until, ref.ID, StorageRemote, false, cutoff, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// FinishLocalRemoval records that the local files were removed (fenced) and releases the claim.
func (s *Store) FinishLocalRemoval(ctx context.Context, ref SetRef, token string, now int64) (bool, error) {
	k := storagePrefix(ref.Video)
	res, err := s.exec(ctx, `UPDATE connection_history SET `+k+`local_removed=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+k+`claim_token=? AND `+k+`claim_until >= ?`, true, ref.ID, token, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Reupload sends a remote set back to "owed" (its local files changed after the upload), fenced.
func (s *Store) Reupload(ctx context.Context, ref SetRef, token string) error {
	k := storagePrefix(ref.Video)
	_, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+k+`claim_token=?`, StorageOwed, ref.ID, token)
	return err
}

// ReleaseLocalClaim frees a local-removal claim without changes.
func (s *Store) ReleaseLocalClaim(ctx context.Context, ref SetRef, token string) error {
	k := storagePrefix(ref.Video)
	_, err := s.exec(ctx, `UPDATE connection_history SET `+k+`claim_owner=NULL, `+k+`claim_token=NULL, `+k+`claim_until=NULL
		WHERE id=? AND `+k+`claim_token=?`, ref.ID, token)
	return err
}

// RecentUploads lists sets uploaded since `since` (the reconciliation prunes stray versions a stale worker may leave).
func (s *Store) RecentUploads(ctx context.Context, since int64) ([]*HistoryEntry, error) {
	return s.historyWhere(ctx, `video_uploaded_at >= ? OR text_uploaded_at >= ?`, since, since)
}

// SetStorageState records a set's storage state during a purge (the purge holds the set exclusively).
func (s *Store) SetStorageState(ctx context.Context, id int64, video bool, state, msg string) error {
	k := storagePrefix(video)
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.exec(ctx, `UPDATE connection_history SET `+k+`storage=?, `+k+`storage_error=? WHERE id=?`, state, nullStr(&msg), id)
	return err
}
