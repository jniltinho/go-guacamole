package server

import (
	"context"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/store"
)

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

func pathID(c *echo.Context, name string) (int64, error) { return parseID(c.Param(name)) }

func optID(s *string) (*int64, error) {
	if s == nil || *s == "" || *s == "ROOT" {
		return nil, nil
	}
	id, err := strconv.ParseInt(*s, 10, 64)
	if err != nil || id <= 0 {
		return nil, validation("parentId", "invalid parent id")
	}
	return &id, nil
}

func page(c *echo.Context) store.Page {
	l, _ := strconv.Atoi(c.QueryParam("limit"))
	o, _ := strconv.Atoi(c.QueryParam("offset"))
	return store.Page{Limit: l, Offset: o}.Normalize()
}

func queryInt64(c *echo.Context, name string) int64 {
	v, _ := strconv.ParseInt(c.QueryParam(name), 10, 64)
	return v
}

// bg returns a context detached from the request for writes that must
// complete even if the client goes away.
func (a *App) bg() context.Context { return context.Background() }

// audit writes an audit event; failures are logged, never returned.
func (a *App) audit(c *echo.Context, actorID *int64, actor, action, targetType, targetID, targetName, outcome string, meta map[string]any) {
	ip := ""
	if c != nil {
		ip = c.RealIP()
	}
	e := &store.AuditEvent{ActorID: actorID, ActorName: actor, ClientIP: ip, Action: action, TargetType: targetType,
		TargetID: targetID, TargetName: targetName, Outcome: outcome, Metadata: meta}
	if err := a.Store.InsertAudit(a.bg(), e); err != nil {
		a.Log.Error("audit write failed", "err", err, "action", action)
	}
}

// auditBy records an action performed by the current user.
func (a *App) auditBy(c *echo.Context, action, targetType, targetID, targetName string, meta map[string]any) {
	u := userOf(c)
	a.audit(c, &u.ID, u.Username, action, targetType, targetID, targetName, "success", meta)
}

type list[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func newList[T any](items []T, total int) list[T] {
	if items == nil {
		items = []T{}
	}
	return list[T]{Items: items, Total: total}
}
