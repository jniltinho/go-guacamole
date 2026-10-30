package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/jobs"
)

// upkeepJobs are the periodic jobs shared by all replicas (each period runs once, under a database lease). Work that
// belongs to one instance — heartbeats, its own unused tickets, recording availability — stays in janitor.
func (a *App) upkeepJobs() []jobs.Periodic {
	return []jobs.Periodic{
		{Name: "sessions:expired", Queue: jobs.Low, Every: time.Minute, Lease: time.Minute,
			Run: func(ctx context.Context, _ string) error {
				now := time.Now()
				n, err := a.Store.DeleteExpiredSessions(ctx, now.UnixMilli(), now.Add(-a.Cfg.App.SessionIdleTimeout).UnixMilli())
				if err == nil && n > 0 {
					a.Log.Debug("expired sessions removed", "count", n)
				}
				return err
			}},
		{Name: "audit:purge", Queue: jobs.Low, Every: 24 * time.Hour, Lease: 5 * time.Minute,
			Run: func(ctx context.Context, _ string) error {
				before := time.Now().AddDate(0, 0, -a.Cfg.Audit.RetentionDays).UnixMilli()
				n, err := a.purgeAuditKeepingHolds(ctx, before)
				if err == nil && n > 0 {
					a.Log.Info("audit events purged", "count", n)
				}
				return err
			}},
		a.recordingPurgeJob(nil),
		a.reconcileJob(),
		{Name: "recording:usage", Queue: jobs.Low, Every: 24 * time.Hour, Lease: 5 * time.Minute,
			Run: func(ctx context.Context, _ string) error {
				if !a.Cfg.Recording.Enabled {
					return nil
				}
				u, err := a.computeUsage(ctx)
				if err != nil {
					return err
				}
				return a.updateRecState(ctx, func(s *recState) { s.Usage = u })
			}},
	}
}

// recordingPurgeJob is the daily recording purge; out, when set, receives the summary (manual runs).
func (a *App) recordingPurgeJob(out **purgeSummary) jobs.Periodic {
	return jobs.Periodic{Name: "recording:purge", Queue: jobs.Critical, Every: 24 * time.Hour, Lease: 5 * time.Minute,
		Run: func(ctx context.Context, _ string) error {
			if !a.Cfg.Recording.Enabled {
				return nil
			}
			sum, err := a.purgeRecordings(ctx)
			if out != nil {
				*out = sum
			}
			return err
		}}
}

func (a *App) runPurgeNow(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var sum *purgeSummary
	err := jobs.RunNow(c.Request().Context(), a.Store, a.Instance, a.recordingPurgeJob(&sum))
	if errors.Is(err, jobs.ErrBusy) {
		return apiErr(http.StatusConflict, "PURGE_RUNNING", "a purge is already running; try again in a few minutes")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, sum)
}
