package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/store"
)

var (
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,128}$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	clockRe    = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2})?$`)

	errSelf      = apiErr(http.StatusBadRequest, "SELF_MODIFICATION", "you cannot do this to your own account")
	errLastAdmin = apiErr(http.StatusConflict, "LAST_ADMIN", "at least one enabled administrator must remain")
)

func requireAdmin(c *echo.Context) error {
	if !effOf(c).IsAdmin() {
		return errForbidden
	}
	return nil
}

func requirePerm(c *echo.Context, p string) error {
	if !effOf(c).Has(p) {
		return errForbidden
	}
	return nil
}

// ensureAdminsRemain runs fn in a transaction and rolls back if no enabled admin is left.
func (a *App) ensureAdminsRemain(ctx context.Context, fn func(tx *store.Store) error) error {
	return a.Store.Tx(ctx, func(tx *store.Store) error {
		if err := fn(tx); err != nil {
			return err
		}
		ids, err := tx.AdminUserIDs(ctx)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return errLastAdmin
		}
		return nil
	})
}

type userInput struct {
	Username           *string `json:"username"`
	Password           string  `json:"password"`
	FullName           *string `json:"fullName"`
	Email              *string `json:"email"`
	Disabled           *bool   `json:"disabled"`
	MustChangePassword *bool   `json:"mustChangePassword"`
	ValidFrom          *string `json:"validFrom"`
	ValidUntil         *string `json:"validUntil"`
	AccessWindowStart  *string `json:"accessWindowStart"`
	AccessWindowEnd    *string `json:"accessWindowEnd"`
	Timezone           *string `json:"timezone"`
}

func (in *userInput) apply(u *store.User) error {
	if in.Username != nil {
		name := strings.TrimSpace(*in.Username)
		if !usernameRe.MatchString(name) {
			return validation("username", "username must be 1-128 characters: letters, digits, . _ @ -")
		}
		u.Username = name
	}
	if in.FullName != nil {
		u.FullName = in.FullName
	}
	if in.Email != nil {
		if *in.Email != "" && !strings.Contains(*in.Email, "@") {
			return validation("email", "invalid e-mail")
		}
		u.Email = in.Email
	}
	if in.Disabled != nil {
		u.Disabled = *in.Disabled
	}
	if in.MustChangePassword != nil {
		u.MustChangePassword = *in.MustChangePassword
	}
	for field, p := range map[string]*string{"validFrom": in.ValidFrom, "validUntil": in.ValidUntil} {
		if p != nil && *p != "" && !dateRe.MatchString(*p) {
			return validation(field, "date must be YYYY-MM-DD")
		}
	}
	for field, p := range map[string]*string{"accessWindowStart": in.AccessWindowStart, "accessWindowEnd": in.AccessWindowEnd} {
		if p != nil && *p != "" && !clockRe.MatchString(*p) {
			return validation(field, "time must be HH:MM")
		}
	}
	if in.ValidFrom != nil {
		u.ValidFrom = in.ValidFrom
	}
	if in.ValidUntil != nil {
		u.ValidUntil = in.ValidUntil
	}
	if in.AccessWindowStart != nil {
		u.AccessWindowStart = in.AccessWindowStart
	}
	if in.AccessWindowEnd != nil {
		u.AccessWindowEnd = in.AccessWindowEnd
	}
	if in.Timezone != nil {
		if *in.Timezone != "" {
			if _, err := time.LoadLocation(*in.Timezone); err != nil {
				return validation("timezone", "unknown time zone")
			}
		}
		u.Timezone = in.Timezone
	}
	return nil
}

type userView struct {
	*store.User
	Groups []string `json:"groups"`
}

func (a *App) listUsers(c *echo.Context) error {
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermCreateUser) && !eff.Has(store.PermAudit) {
		return errForbidden
	}
	users, total, err := a.Store.ListUsers(c.Request().Context(), c.QueryParam("q"), page(c))
	if err != nil {
		return err
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		gs, _ := a.Store.GroupsOfUser(c.Request().Context(), u.ID)
		names := []string{}
		for _, g := range gs {
			names = append(names, g.Name)
		}
		out = append(out, userView{User: u, Groups: names})
	}
	return c.JSON(http.StatusOK, newList(out, total))
}

func (a *App) createUser(c *echo.Context) error {
	if err := requirePerm(c, store.PermCreateUser); err != nil {
		return err
	}
	var in userInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	if in.Username == nil {
		return validation("username", "username is required")
	}
	u := &store.User{MustChangePassword: true}
	if err := in.apply(u); err != nil {
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		if pe := passwordErr(err); pe != nil {
			return pe
		}
		return err
	}
	u.PasswordHash, u.PasswordScheme = hash, store.SchemeBcrypt
	if err := a.Store.CreateUser(c.Request().Context(), u); err != nil {
		return err
	}
	a.auditBy(c, "user.create", "user", idStr(u.ID), u.Username, nil)
	return c.JSON(http.StatusCreated, userView{User: u, Groups: []string{}})
}

func (a *App) loadUser(c *echo.Context) (*store.User, error) {
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermAudit) {
		return nil, errForbidden
	}
	id, err := pathID(c, "id")
	if err != nil {
		return nil, err
	}
	return a.Store.GetUser(c.Request().Context(), id)
}

func (a *App) getUser(c *echo.Context) error {
	u, err := a.loadUser(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	gs, _ := a.Store.GroupsOfUser(ctx, u.ID)
	names := []string{}
	for _, g := range gs {
		names = append(names, g.Name)
	}
	grants, err := a.Store.GetGrants(ctx, store.SubjectUser, u.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"user": u, "groups": names, "grants": grants})
}

func (a *App) updateUser(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	u, err := a.loadUser(c)
	if err != nil {
		return err
	}
	var in userInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	wasDisabled := u.Disabled
	if err := in.apply(u); err != nil {
		return err
	}
	me := userOf(c)
	if u.ID == me.ID && u.Disabled {
		return errSelf
	}
	ctx := c.Request().Context()
	if err := a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.UpdateUser(ctx, u) }); err != nil {
		return err
	}
	if u.Disabled && !wasDisabled {
		a.Enforcer.RevokeUserEverywhere(ctx, u.ID, "account disabled")
	} else {
		// restrictions may have changed: the next enforcer tick re-checks windows/validity
		a.Enforcer.Reevaluate(ctx, u.ID)
	}
	a.auditBy(c, "user.update", "user", idStr(u.ID), u.Username, map[string]any{"disabled": u.Disabled})
	return c.JSON(http.StatusOK, userView{User: u})
}

func (a *App) deleteUser(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	u, err := a.loadUser(c)
	if err != nil {
		return err
	}
	if u.ID == userOf(c).ID {
		return errSelf
	}
	ctx := c.Request().Context()
	if err := a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.DeleteUser(ctx, u.ID) }); err != nil {
		return err
	}
	a.Enforcer.RevokeUser(u.ID, guac.StatusClientForbidden, "account deleted")
	a.auditBy(c, "user.delete", "user", idStr(u.ID), u.Username, nil)
	return c.NoContent(http.StatusNoContent)
}

func (a *App) resetPassword(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	u, err := a.loadUser(c)
	if err != nil {
		return err
	}
	var in struct {
		Password           string `json:"password"`
		MustChangePassword *bool  `json:"mustChangePassword"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		if pe := passwordErr(err); pe != nil {
			return pe
		}
		return err
	}
	must := true
	if in.MustChangePassword != nil {
		must = *in.MustChangePassword
	}
	ctx := c.Request().Context()
	if err := a.Store.SetPassword(ctx, u.ID, hash, store.SchemeBcrypt, "", must); err != nil {
		return err
	}
	keep := ""
	if u.ID == userOf(c).ID {
		keep = sessionOf(c).ID
	}
	ids, _ := a.Store.DeleteUserSessions(ctx, u.ID, keep)
	for _, id := range ids {
		a.Enforcer.RevokeSession(id, guac.StatusClientForbidden, "password reset")
	}
	a.auditBy(c, "user.password.reset", "user", idStr(u.ID), u.Username, nil)
	return c.NoContent(http.StatusNoContent)
}

// ---- user groups ----

func (a *App) listUserGroups(c *echo.Context) error {
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermAudit) {
		return errForbidden
	}
	gs, err := a.Store.ListUserGroups(c.Request().Context(), c.QueryParam("q"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, newList(gs, len(gs)))
}

func (a *App) loadUserGroup(c *echo.Context) (*store.UserGroup, error) {
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermAudit) {
		return nil, errForbidden
	}
	id, err := pathID(c, "id")
	if err != nil {
		return nil, err
	}
	return a.Store.GetUserGroup(c.Request().Context(), id)
}

func (a *App) getUserGroup(c *echo.Context) error {
	g, err := a.loadUserGroup(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	members, err := a.Store.GroupMembers(ctx, g.ID)
	if err != nil {
		return err
	}
	grants, err := a.Store.GetGrants(ctx, store.SubjectGroup, g.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, idStr(m))
	}
	return c.JSON(http.StatusOK, map[string]any{"group": g, "members": ids, "grants": grants})
}

type userGroupInput struct {
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

func (in *userGroupInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 {
		return validation("name", "name is required (max 128 characters)")
	}
	return nil
}

func (a *App) createUserGroup(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in userGroupInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	if err := in.validate(); err != nil {
		return err
	}
	g := &store.UserGroup{Name: in.Name, Disabled: in.Disabled}
	if err := a.Store.CreateUserGroup(c.Request().Context(), g); err != nil {
		return err
	}
	a.auditBy(c, "user_group.create", "user_group", idStr(g.ID), g.Name, nil)
	return c.JSON(http.StatusCreated, g)
}

func (a *App) groupChanged(ctx context.Context, groupID int64, extra ...int64) {
	members, _ := a.Store.GroupMembers(ctx, groupID)
	members = append(members, extra...)
	_ = a.Store.TouchUserStamp(ctx, members...)
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.Reevaluate(ctx, members...)
}

func (a *App) updateUserGroup(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	g, err := a.loadUserGroup(c)
	if err != nil {
		return err
	}
	var in userGroupInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	if err := in.validate(); err != nil {
		return err
	}
	g.Name, g.Disabled = in.Name, in.Disabled
	ctx := c.Request().Context()
	if err := a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.UpdateUserGroup(ctx, g) }); err != nil {
		return err
	}
	a.groupChanged(ctx, g.ID)
	a.auditBy(c, "user_group.update", "user_group", idStr(g.ID), g.Name, map[string]any{"disabled": g.Disabled})
	return c.JSON(http.StatusOK, g)
}

func (a *App) deleteUserGroup(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	g, err := a.loadUserGroup(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	members, _ := a.Store.GroupMembers(ctx, g.ID)
	if err := a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.DeleteUserGroup(ctx, g.ID) }); err != nil {
		return err
	}
	_ = a.Store.TouchUserStamp(ctx, members...)
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.Reevaluate(ctx, members...)
	a.auditBy(c, "user_group.delete", "user_group", idStr(g.ID), g.Name, nil)
	return c.NoContent(http.StatusNoContent)
}

func (a *App) setMembers(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	g, err := a.loadUserGroup(c)
	if err != nil {
		return err
	}
	var in struct {
		Members []string `json:"members"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	ids := make([]int64, 0, len(in.Members))
	for _, s := range in.Members {
		id, err := parseID(s)
		if err != nil {
			return validation("members", "invalid user id")
		}
		if _, err := a.Store.GetUser(ctx, id); err != nil {
			return validation("members", "unknown user "+s)
		}
		ids = append(ids, id)
	}
	old, _ := a.Store.GroupMembers(ctx, g.ID)
	if err := a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.SetGroupMembers(ctx, g.ID, ids) }); err != nil {
		return err
	}
	a.groupChanged(ctx, g.ID, old...)
	a.auditBy(c, "user_group.members", "user_group", idStr(g.ID), g.Name, map[string]any{"count": len(ids)})
	return c.NoContent(http.StatusNoContent)
}

// ---- permissions ----

func parseSubject(s string) (string, int64, error) {
	typ, id, ok := strings.Cut(s, ":")
	if !ok || (typ != store.SubjectUser && typ != store.SubjectGroup) {
		return "", 0, validation("subject", "subject must be user:<id> or group:<id>")
	}
	n, err := parseID(id)
	if err != nil {
		return "", 0, validation("subject", "invalid subject id")
	}
	return typ, n, nil
}

func (a *App) getPermissions(c *echo.Context) error {
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermAudit) {
		return errForbidden
	}
	typ, id, err := parseSubject(c.QueryParam("subject"))
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	direct, err := a.Store.GetGrants(ctx, typ, id)
	if err != nil {
		return err
	}
	out := map[string]any{"subject": c.QueryParam("subject"), "direct": direct}
	if typ == store.SubjectUser {
		if _, err := a.Store.GetUser(ctx, id); err != nil {
			return err
		}
		ue, err := access.Resolve(ctx, a.Store, id)
		if err != nil {
			return err
		}
		conns, err := a.Store.ListConnections(ctx)
		if err != nil {
			return err
		}
		type effConn struct {
			ID      int64           `json:"id,string"`
			Name    string          `json:"name"`
			Sources []access.Source `json:"sources"`
		}
		var ec []effConn
		for _, conn := range conns {
			if src := ue.ConnectionSources(conn.ID, conn.ParentID); len(src) > 0 {
				ec = append(ec, effConn{conn.ID, conn.Name, src})
			}
		}
		sys := map[string][]access.Source{}
		for p, s := range ue.System {
			sys[p] = s
		}
		out["effective"] = map[string]any{"system": sys, "connections": orEmpty(ec), "isAdmin": ue.IsAdmin()}
	} else if _, err := a.Store.GetUserGroup(ctx, id); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

func (a *App) putPermissions(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in struct {
		Subject          string   `json:"subject"`
		System           []string `json:"system"`
		Connections      []string `json:"connections"`
		ConnectionGroups []string `json:"connectionGroups"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	typ, id, err := parseSubject(in.Subject)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	g := &store.Grants{}
	for _, p := range in.System {
		if !containsStr(store.SystemPermissions, p) {
			return validation("system", "unknown permission "+p)
		}
		g.System = append(g.System, p)
	}
	for _, s := range in.Connections {
		cid, err := parseID(s)
		if err != nil {
			return validation("connections", "invalid id")
		}
		if _, err := a.Store.GetConnection(ctx, cid); err != nil {
			return validation("connections", "unknown connection "+s)
		}
		g.Connections = append(g.Connections, cid)
	}
	for _, s := range in.ConnectionGroups {
		gid, err := parseID(s)
		if err != nil {
			return validation("connectionGroups", "invalid id")
		}
		if _, err := a.Store.GetConnectionGroup(ctx, gid); err != nil {
			return validation("connectionGroups", "unknown group "+s)
		}
		g.Groups = append(g.Groups, gid)
	}
	me := userOf(c)
	if typ == store.SubjectUser && id == me.ID && !containsStr(g.System, store.PermAdminister) {
		cur, _ := a.Store.GetGrants(ctx, typ, id)
		if containsStr(cur.System, store.PermAdminister) {
			return errSelf
		}
	}
	var affected []int64
	var subjectName string
	if typ == store.SubjectUser {
		u, err := a.Store.GetUser(ctx, id)
		if err != nil {
			return err
		}
		affected, subjectName = []int64{id}, u.Username
	} else {
		g, err := a.Store.GetUserGroup(ctx, id)
		if err != nil {
			return err
		}
		affected, _ = a.Store.GroupMembers(ctx, id)
		subjectName = g.Name
	}
	err = a.ensureAdminsRemain(ctx, func(tx *store.Store) error { return tx.SetGrants(ctx, typ, id, g) })
	if errors.Is(err, errLastAdmin) {
		return errLastAdmin
	}
	if err != nil {
		return err
	}
	_ = a.Store.TouchUserStamp(ctx, affected...)
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.Reevaluate(ctx, affected...)
	a.auditBy(c, "permissions.update", typ, idStr(id), subjectName, map[string]any{
		"system": g.System, "connections": len(g.Connections), "connectionGroups": len(g.Groups)})
	return c.NoContent(http.StatusNoContent)
}

// ---- history & audit ----

func (a *App) listHistory(c *echo.Context) error {
	eff, u := effOf(c), userOf(c)
	f := store.HistoryFilter{Protocol: c.QueryParam("protocol"), From: queryInt64(c, "from"), To: queryInt64(c, "to"), Q: c.QueryParam("q")}
	if !eff.Has(store.PermAudit) {
		f.UserID = &u.ID
	} else if v := queryInt64(c, "userId"); v > 0 {
		f.UserID = &v
	}
	if v := queryInt64(c, "connectionId"); v > 0 {
		f.ConnectionID = &v
	}
	items, total, err := a.Store.ListHistory(c.Request().Context(), f, page(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, newList(items, total))
}

func (a *App) listAudit(c *echo.Context) error {
	if err := requirePerm(c, store.PermAudit); err != nil {
		return err
	}
	f := store.AuditFilter{Actor: c.QueryParam("actor"), Action: c.QueryParam("action"), TargetType: c.QueryParam("targetType"),
		TargetID: c.QueryParam("targetId"), From: queryInt64(c, "from"), To: queryInt64(c, "to")}
	items, total, err := a.Store.ListAudit(c.Request().Context(), f, page(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, newList(items, total))
}
