package server

import (
	"context"
	"time"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

// EnforceInterval is how often sessions, restrictions and cross-process
// changes are re-checked for open tunnels.
const EnforceInterval = 5 * time.Second

// Enforcer re-evaluates open tunnels whenever access may have changed.
type Enforcer struct {
	a      *App
	epoch  string
	stamps map[int64]string
}

// NewEnforcer creates an enforcer.
func NewEnforcer(a *App) *Enforcer { return &Enforcer{a: a, stamps: map[int64]string{}} }

// RevokeSession closes everything bound to a login session.
func (e *Enforcer) RevokeSession(sessionID string, st guac.Status, msg string) {
	e.a.Registry.Revoke(func(t *tunnel.Tunnel) bool { return t.SessionID == sessionID }, st, msg)
}

// RevokeUser closes all tunnels and pending tickets of a user.
func (e *Enforcer) RevokeUser(userID int64, st guac.Status, msg string) {
	e.a.Registry.Revoke(func(t *tunnel.Tunnel) bool { return t.UserID == userID }, st, msg)
}

// RevokeUserEverywhere deletes all sessions of a user and closes its tunnels.
func (e *Enforcer) RevokeUserEverywhere(ctx context.Context, userID int64, msg string) {
	_, _ = e.a.Store.DeleteUserSessions(ctx, userID, "")
	e.RevokeUser(userID, guac.StatusClientForbidden, msg)
}

// Reevaluate re-checks READ for the given users' tunnels (all users when none given).
func (e *Enforcer) Reevaluate(ctx context.Context, userIDs ...int64) {
	want := map[int64]bool{}
	for _, id := range userIDs {
		want[id] = true
	}
	ts := e.a.Registry.Snapshot(func(t *tunnel.Tunnel) bool { return len(want) == 0 || want[t.UserID] })
	effs := map[int64]*access.Effective{}
	for _, t := range ts {
		eff, ok := effs[t.UserID]
		if !ok {
			var err error
			if eff, err = access.Resolve(ctx, e.a.Store, t.UserID); err != nil {
				continue
			}
			effs[t.UserID] = eff
		}
		conn, err := e.a.Store.GetConnection(ctx, t.ConnectionID)
		if err != nil || !eff.CanRead(conn) {
			id := t.ID
			e.a.Registry.Revoke(func(x *tunnel.Tunnel) bool { return x.ID == id }, guac.StatusClientForbidden, "access revoked")
		}
	}
}

// ReevaluateConnection re-checks every tunnel to one connection.
func (e *Enforcer) ReevaluateConnection(ctx context.Context, connID int64) {
	var users []int64
	for _, t := range e.a.Registry.Snapshot(func(t *tunnel.Tunnel) bool { return t.ConnectionID == connID }) {
		users = append(users, t.UserID)
	}
	if len(users) > 0 {
		e.Reevaluate(ctx, users...)
	}
}

// Run periodically enforces session expiry, account restrictions and
// changes made by other processes (security stamps and the access epoch).
func (e *Enforcer) Run(ctx context.Context) {
	e.epoch, _ = e.a.Store.AccessEpoch(ctx)
	t := time.NewTicker(EnforceInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

func (e *Enforcer) tick(ctx context.Context) {
	a := e.a
	now := time.Now()
	ts := a.Registry.Snapshot(nil)
	if len(ts) == 0 {
		e.epoch, _ = a.Store.AccessEpoch(ctx)
		return
	}
	var reeval []int64
	seenUser := map[int64]bool{}
	for _, t := range ts {
		id := t.ID
		revoke := func(st guac.Status, msg string) {
			a.Registry.Revoke(func(x *tunnel.Tunnel) bool { return x.ID == id }, st, msg)
		}
		sess, err := a.Store.GetSession(ctx, t.SessionID)
		if err != nil {
			revoke(guac.StatusSessionClosed, "session ended")
			continue
		}
		if now.UnixMilli() > sess.ExpiresAt || now.Add(-a.Cfg.App.SessionIdleTimeout).UnixMilli() > sess.LastSeen {
			_ = a.Store.DeleteSession(ctx, sess.ID)
			revoke(guac.StatusSessionTimeout, "session expired")
			continue
		}
		u, err := a.Store.GetUser(ctx, t.UserID)
		if err != nil {
			revoke(guac.StatusClientForbidden, "user removed")
			continue
		}
		if err := auth.CheckRestrictions(u, now); err != nil {
			_, _ = a.Store.DeleteUserSessions(ctx, u.ID, "")
			st := guac.StatusClientForbidden
			if err == auth.ErrAccessWindow {
				st = guac.StatusSessionTimeout
			}
			revoke(st, "account restricted")
			continue
		}
		if !seenUser[u.ID] {
			seenUser[u.ID] = true
			if old, ok := e.stamps[u.ID]; ok && old != u.SecurityStamp {
				reeval = append(reeval, u.ID)
			}
			e.stamps[u.ID] = u.SecurityStamp
		}
	}
	epoch, err := a.Store.AccessEpoch(ctx)
	if err == nil && epoch != e.epoch {
		e.epoch = epoch
		e.Reevaluate(ctx)
		return
	}
	if len(reeval) > 0 {
		e.Reevaluate(ctx, reeval...)
	}
}
