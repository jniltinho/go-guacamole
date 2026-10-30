package store

import "context"

// Heartbeat records that an instance is alive; history rows it owns stay open while it beats.
func (s *Store) Heartbeat(ctx context.Context, instance string, at int64) error {
	res, err := s.exec(ctx, `UPDATE instance_heartbeats SET seen_at=? WHERE instance=?`, at, instance)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.exec(ctx, `INSERT INTO instance_heartbeats (instance, seen_at) VALUES (?, ?)`, instance, at)
	return err
}

// CloseOrphanedHistory ends open rows whose instance stopped beating before aliveSince (or that have no owner,
// rows written before ownership existed), and forgets heartbeats older than forgetBefore. Rows of live instances,
// including other replicas, are never touched.
func (s *Store) CloseOrphanedHistory(ctx context.Context, at, aliveSince, forgetBefore int64) (int64, error) {
	res, err := s.exec(ctx, `UPDATE connection_history SET end_at=?, close_reason='server_restart'
		WHERE end_at IS NULL AND (owner_instance IS NULL
			OR owner_instance NOT IN (SELECT instance FROM instance_heartbeats WHERE seen_at >= ?))`, at, aliveSince)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_, err = s.exec(ctx, `DELETE FROM instance_heartbeats WHERE seen_at < ?`, forgetBefore)
	return n, err
}
