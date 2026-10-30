package server

import (
	"context"
	"os"
	"time"

	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// queueHealth reports the job queue state (disabled before Start or without Redis).
func (a *App) queueHealth() jobs.Health {
	if a.Jobs == nil {
		return jobs.Health{State: jobs.Disabled}
	}
	return a.Jobs.Health()
}

// InstanceTTL is how long an instance may go without a heartbeat before its open history rows are ended.
const InstanceTTL = 3 * time.Minute

// instanceID is unique per process: <hostname>-<random>, readable in logs and leases.
func instanceID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "goguacamole"
	}
	return host + "-" + store.RandomHex(4)
}

// beat records this instance as alive and ends sessions left open by instances that stopped beating
// (a crash, a restart: the new process has a new id), never those of live replicas.
func (a *App) beat(ctx context.Context) {
	now := a.Store.Now()
	if err := a.Store.Heartbeat(ctx, a.Instance, now); err != nil {
		a.Log.Warn("heartbeat", "err", err)
		return
	}
	stale := now - InstanceTTL.Milliseconds()
	if n, err := a.Store.CloseOrphanedHistory(ctx, now, stale, now-24*time.Hour.Milliseconds()); err != nil {
		a.Log.Warn("close orphaned history", "err", err)
	} else if n > 0 {
		a.Log.Info("sessions of stopped instances ended", "count", n)
	}
}
