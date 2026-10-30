package store

import (
	"context"
	"errors"
)

// Database side of the job queue: leases make a periodic job (or a manual run of it) exclusive across replicas, and
// job_runs records completed periods so a re-enqueued or retried run of the same period has no effect.

// PeriodDone reports whether a periodic job already completed a period.
func (s *Store) PeriodDone(ctx context.Context, job, period string) (bool, error) {
	var n int
	err := s.row(ctx, `SELECT COUNT(*) FROM job_runs WHERE job=? AND period=?`, job, period).Scan(&n)
	return n > 0, err
}

// AcquireLease takes the job's lease for a run identified by token until `until`, if it is free or expired.
func (s *Store) AcquireLease(ctx context.Context, job, owner, token string, now, until int64) (bool, error) {
	res, err := s.exec(ctx, `UPDATE job_leases SET owner=?, run_token=?, expires_at=? WHERE job=? AND expires_at < ?`,
		owner, token, until, job, now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	_, err = s.exec(ctx, `INSERT INTO job_leases (job, owner, run_token, expires_at) VALUES (?, ?, ?, ?)`, job, owner, token, until)
	if err = mapErr(err); errors.Is(err, ErrConflict) {
		return false, nil // held by a live run
	}
	return err == nil, err
}

// leaseValid reports whether the run identified by token still holds an unexpired lease.
func (s *Store) leaseValid(ctx context.Context, job, token string, now int64) (bool, error) {
	var n int
	err := s.row(ctx, `SELECT COUNT(*) FROM job_leases WHERE job=? AND run_token=? AND expires_at >= ?`, job, token, now).Scan(&n)
	return n > 0, err
}

// RenewLease extends the lease of the run identified by token; false when it was lost (expired or taken over).
func (s *Store) RenewLease(ctx context.Context, job, token string, now, until int64) (bool, error) {
	ok, err := s.leaseValid(ctx, job, token, now)
	if err != nil || !ok {
		return false, err
	}
	_, err = s.exec(ctx, `UPDATE job_leases SET expires_at=? WHERE job=? AND run_token=?`, until, job, token)
	return err == nil, err
}

// CompletePeriod records the period as done and releases the lease, only for the run that still holds it.
func (s *Store) CompletePeriod(ctx context.Context, job, period, token string, now int64) (bool, error) {
	done := false
	err := s.Tx(ctx, func(tx *Store) error {
		ok, err := tx.leaseValid(ctx, job, token, now)
		if err != nil || !ok {
			return err
		}
		if _, err := tx.exec(ctx, `INSERT INTO job_runs (job, period, completed_at) VALUES (?, ?, ?)`, job, period, now); err != nil {
			return mapErr(err)
		}
		if _, err := tx.exec(ctx, `DELETE FROM job_leases WHERE job=? AND run_token=?`, job, token); err != nil {
			return err
		}
		// completed periods are only needed while a period can still be re-enqueued
		_, err = tx.exec(ctx, `DELETE FROM job_runs WHERE completed_at < ?`, now-7*24*3600*1000)
		done = err == nil
		return err
	})
	return done, err
}

// ReleaseLease frees the lease of the run identified by token (no effect when another run took it over).
func (s *Store) ReleaseLease(ctx context.Context, job, token string) error {
	_, err := s.exec(ctx, `DELETE FROM job_leases WHERE job=? AND run_token=?`, job, token)
	return err
}
