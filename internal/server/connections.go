package server

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/secrets"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

// Protocols supported for new connections.
var Protocols = map[string]bool{"ssh": true, "rdp": true, "vnc": true}

var paramName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// enumParams restricts values of well-known parameters.
var enumParams = map[string][]string{
	"security":              {"any", "nla", "nla-ext", "tls", "vmconnect", "rdp"},
	"ignore-cert":           {"true", "false"},
	"enable-sftp":           {"true", "false"},
	"color-depth":           {"8", "16", "24", "32"},
	"resize-method":         {"display-update", "reconnect"},
	"server-layout":         {"en-us-qwerty", "en-gb-qwerty", "de-de-qwertz", "fr-fr-azerty", "it-it-qwerty", "ja-jp-qwerty", "pt-br-qwerty", "es-es-qwerty", "es-latam-qwerty", "sv-se-qwerty", "tr-tr-qwerty", "hu-hu-qwertz", "de-ch-qwertz", "fr-ch-qwertz", "fr-be-azerty", "no-no-qwerty", "da-dk-qwerty", "failsafe"},
	"disable-copy":          {"true", "false"},
	"disable-paste":         {"true", "false"},
	"read-only":             {"true", "false"},
	"enable-wallpaper":      {"true", "false"},
	"enable-font-smoothing": {"true", "false"},
	"backspace":             {"127", "8"},
	"terminal-type":         {"ansi", "linux", "vt100", "vt220", "xterm", "xterm-256color"},
	"sftp-disable-download": {"true", "false"},
	"sftp-disable-upload":   {"true", "false"},
	"disable-download":      {"true", "false"},
	"disable-upload":        {"true", "false"},
}

// colorScheme accepts the guacd presets or a custom palette
// ("foreground: rgb:00/00/ff; background: color0; color9: rgb:ff/00/00").
var colorScheme = regexp.MustCompile(`^(black-white|gray-black|green-black|white-black|` +
	`\s*(foreground|background|color\d{1,3})\s*:\s*(rgb:[0-9a-fA-F]{2}/[0-9a-fA-F]{2}/[0-9a-fA-F]{2}|color\d{1,3})\s*` +
	`(;\s*(foreground|background|color\d{1,3})\s*:\s*(rgb:[0-9a-fA-F]{2}/[0-9a-fA-F]{2}/[0-9a-fA-F]{2}|color\d{1,3})\s*)*;?\s*)$`)

var numericParams = map[string][2]int{
	"port": {1, 65535}, "sftp-port": {1, 65535}, "font-size": {4, 96}, "scrollback": {0, 1000000},
	"server-alive-interval": {0, 3600}, "dpi": {48, 600}, "width": {100, 8192}, "height": {100, 8192},
}

type connectionView struct {
	*store.Connection
	Hostname   string          `json:"hostname"`
	Port       string          `json:"port"`
	GroupPath  []string        `json:"groupPath"`
	Favorite   bool            `json:"favorite"`
	Status     string          `json:"status"`
	Active     int             `json:"activeSessions"`
	CanEdit    bool            `json:"canEdit"`
	Parameters map[string]any  `json:"parameters,omitempty"`
	Sources    []access.Source `json:"sources,omitempty"`
	Recording  *recordingView  `json:"recording,omitempty"`
}

func (a *App) groupPath(eff *access.Effective, parent *int64) []string {
	out := []string{}
	for _, id := range eff.Ancestors(parent) {
		out = append(out, eff.FolderName(id))
	}
	return out
}

func (a *App) view(eff *access.Effective, c *store.Connection, favs map[int64]bool) connectionView {
	return connectionView{
		Connection: c, Hostname: c.Parameters["hostname"], Port: c.Parameters["port"],
		GroupPath: a.groupPath(eff, c.ParentID), Favorite: favs[c.ID], Status: a.Prober.Status(c.ID),
		Active:  a.Registry.CountActive(func(t *tunnel.Tunnel) bool { return t.ConnectionID == c.ID }),
		CanEdit: eff.CanEditConnection(c),
	}
}

func (a *App) visibleConnections(ctx context.Context, eff *access.Effective) ([]*store.Connection, error) {
	all, err := a.Store.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	var out []*store.Connection
	for _, c := range all {
		if eff.CanRead(c) || eff.CanEditConnection(c) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (a *App) favSet(ctx context.Context, userID int64) map[int64]bool {
	ids, _ := a.Store.Favorites(ctx, userID)
	m := map[int64]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func (a *App) listConnections(c *echo.Context) error {
	ctx := c.Request().Context()
	eff := effOf(c)
	conns, err := a.visibleConnections(ctx, eff)
	if err != nil {
		return err
	}
	favs := a.favSet(ctx, eff.UserID)
	q := strings.ToLower(strings.TrimSpace(c.QueryParam("q")))
	proto := c.QueryParam("protocol")
	groupID := queryInt64(c, "groupId")
	onlyFav := c.QueryParam("favorite") == "true"
	var items []connectionView
	for _, conn := range conns {
		if proto != "" && conn.Protocol != proto {
			continue
		}
		if onlyFav && !favs[conn.ID] {
			continue
		}
		if groupID > 0 && (conn.ParentID == nil || !eff.IsDescendant(*conn.ParentID, groupID)) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(conn.Name), q) && !strings.Contains(strings.ToLower(conn.Parameters["hostname"]), q) {
			continue
		}
		items = append(items, a.view(eff, conn, favs))
	}
	sortKey := c.QueryParam("sort")
	desc := strings.HasPrefix(sortKey, "-")
	sortKey = strings.TrimPrefix(sortKey, "-")
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch sortKey {
		case "protocol":
			less = items[i].Protocol < items[j].Protocol
		case "group":
			less = strings.Join(items[i].GroupPath, "/") < strings.Join(items[j].GroupPath, "/")
		case "status":
			less = items[i].Status < items[j].Status
		default:
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if desc {
			return !less
		}
		return less
	})
	p := page(c)
	total := len(items)
	end := p.Offset + p.Limit
	if p.Offset > total {
		p.Offset = total
	}
	if end > total {
		end = total
	}
	return c.JSON(http.StatusOK, newList(items[p.Offset:end], total))
}

type connectionInput struct {
	Name                  string             `json:"name"`
	Protocol              string             `json:"protocol"`
	ParentID              *string            `json:"parentId"`
	GuacdHost             *string            `json:"guacdHost"`
	GuacdPort             *int               `json:"guacdPort"`
	GuacdEncryption       *string            `json:"guacdEncryption"`
	MaxConnections        *int               `json:"maxConnections"`
	MaxConnectionsPerUser *int               `json:"maxConnectionsPerUser"`
	Parameters            map[string]*string `json:"parameters"`
}

// apply validates the input and merges it into conn. existing holds the
// current parameters for secret-preserving updates (nil on create).
func (in *connectionInput) apply(conn *store.Connection, existing map[string]string) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 || strings.ContainsAny(in.Name, "/\\") {
		return validation("name", "name is required (max 128 characters, no slashes)")
	}
	if !Protocols[in.Protocol] {
		return validation("protocol", "protocol must be ssh, rdp or vnc")
	}
	parent, err := optID(in.ParentID)
	if err != nil {
		return err
	}
	if in.GuacdEncryption != nil && *in.GuacdEncryption != "" && *in.GuacdEncryption != "NONE" && *in.GuacdEncryption != "SSL" {
		return validation("guacdEncryption", "must be NONE or SSL")
	}
	if in.GuacdPort != nil && (*in.GuacdPort < 1 || *in.GuacdPort > 65535) {
		return validation("guacdPort", "invalid port")
	}
	for _, p := range []*int{in.MaxConnections, in.MaxConnectionsPerUser} {
		if p != nil && *p < 0 {
			return validation("maxConnections", "limits must be >= 0")
		}
	}
	params := map[string]string{}
	for name, v := range in.Parameters {
		if !paramName.MatchString(name) {
			return validation("parameters."+name, "invalid parameter name")
		}
		if store.RecordingParam(name) {
			return validation("parameters."+name, "recording is set by the connection's recording policy")
		}
		if v == nil {
			continue // explicit null clears
		}
		val := *v
		if len(val) > 64<<10 {
			return validation("parameters."+name, "value too large")
		}
		if allowed, ok := enumParams[name]; ok && val != "" && !containsStr(allowed, val) {
			return validation("parameters."+name, "invalid value")
		}
		if name == "color-scheme" && val != "" && !colorScheme.MatchString(val) {
			return validation("parameters.color-scheme", "use a preset or entries like \"foreground: rgb:ff/ff/ff; background: color0\"")
		}
		if name == "timezone" && val != "" {
			if _, err := time.LoadLocation(val); err != nil {
				return validation("parameters.timezone", "unknown time zone")
			}
		}
		if rng, ok := numericParams[name]; ok && val != "" {
			n, err := strconv.Atoi(val)
			if err != nil || n < rng[0] || n > rng[1] {
				return validation("parameters."+name, "must be a number between "+strconv.Itoa(rng[0])+" and "+strconv.Itoa(rng[1]))
			}
		}
		if val != "" {
			params[name] = val
		}
	}
	// secrets omitted on update keep their stored value
	for name, val := range existing {
		if _, given := in.Parameters[name]; !given && secrets.IsSecret(in.Protocol, name) {
			params[name] = val
		}
	}
	if strings.TrimSpace(params["hostname"]) == "" {
		return validation("parameters.hostname", "hostname is required")
	}
	conn.Name, conn.Protocol, conn.ParentID = in.Name, in.Protocol, parent
	conn.GuacdHost, conn.GuacdPort, conn.GuacdEncryption = in.GuacdHost, in.GuacdPort, in.GuacdEncryption
	conn.MaxConnections, conn.MaxConnectionsPerUser = in.MaxConnections, in.MaxConnectionsPerUser
	conn.Parameters = params
	return nil
}

func containsStr(l []string, v string) bool {
	for _, s := range l {
		if s == v {
			return true
		}
	}
	return false
}

func maskedParams(protocol string, params map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range params {
		if secrets.IsSecret(protocol, k) {
			out[k] = map[string]bool{"set": true}
		} else {
			out[k] = v
		}
	}
	return out
}

// changedKeys lists parameter names whose value differs (values never logged).
func changedKeys(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if before[k] != v {
			out = append(out, "parameters."+k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			out = append(out, "parameters."+k)
		}
	}
	sort.Strings(out)
	return out
}

func (a *App) folderVisible(ctx context.Context, eff *access.Effective) func(int64) bool {
	visible := map[int64]bool{}
	if conns, err := a.visibleConnections(ctx, eff); err == nil {
		for _, c := range conns {
			for _, id := range eff.Ancestors(c.ParentID) {
				visible[id] = true
			}
		}
	}
	return func(id int64) bool {
		if eff.CanReadFolder(id) || visible[id] {
			return true
		}
		g, err := a.Store.GetConnectionGroup(ctx, id)
		return err == nil && eff.CanEditFolder(g)
	}
}

func (a *App) createConnection(c *echo.Context) error {
	var in connectionInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	eff := effOf(c)
	conn := &store.Connection{}
	if err := in.apply(conn, nil); err != nil {
		return err
	}
	if guacdChanged(&store.Connection{Protocol: conn.Protocol}, conn) && !eff.IsAdmin() {
		return apiErr(http.StatusForbidden, "FORBIDDEN", "only administrators can set the guacd endpoint")
	}
	if conn.ParentID != nil {
		if _, err := a.Store.GetConnectionGroup(ctx, *conn.ParentID); err != nil {
			return validation("parentId", "parent group does not exist")
		}
	}
	if !eff.CanCreateIn(conn.ParentID, a.folderVisible(ctx, eff)) {
		return errForbidden
	}
	uid := eff.UserID
	conn.CreatedBy = &uid
	if err := a.Store.CreateConnection(ctx, conn); err != nil {
		return err
	}
	a.auditBy(c, "connection.create", "connection", idStr(conn.ID), conn.Name, map[string]any{"protocol": conn.Protocol})
	return c.JSON(http.StatusCreated, a.detail(eff, conn, a.favSet(ctx, uid)))
}

func (a *App) detail(eff *access.Effective, conn *store.Connection, favs map[int64]bool) connectionView {
	v := a.view(eff, conn, favs)
	if v.CanEdit {
		v.Parameters = maskedParams(conn.Protocol, conn.Parameters)
	}
	v.Sources = eff.ConnectionSources(conn.ID, conn.ParentID)
	return v
}

// loadVisibleConnection returns 404 when the caller cannot see the connection.
func (a *App) loadVisibleConnection(c *echo.Context) (*store.Connection, error) {
	id, err := pathID(c, "id")
	if err != nil {
		return nil, err
	}
	conn, err := a.Store.GetConnection(c.Request().Context(), id)
	if err != nil {
		return nil, errNotFound
	}
	eff := effOf(c)
	if !eff.CanRead(conn) && !eff.CanEditConnection(conn) {
		return nil, errNotFound
	}
	return conn, nil
}

func (a *App) getConnection(c *echo.Context) error {
	conn, err := a.loadVisibleConnection(c)
	if err != nil {
		return err
	}
	eff := effOf(c)
	v := a.detail(eff, conn, a.favSet(c.Request().Context(), eff.UserID))
	if v.CanEdit || eff.Has(store.PermAudit) {
		v.Recording = a.recordingViewOf(c.Request().Context(), c, conn)
	}
	return c.JSON(http.StatusOK, v)
}

// guacdChanged reports a change that could move a session away from the
// audited guacd/recordings volume or drop its text recording (ADMINISTER only).
func guacdChanged(old, cur *store.Connection) bool {
	eqS := func(a, b *string) bool {
		return (a == nil || *a == "") == (b == nil || *b == "") && (a == nil || b == nil || *a == *b)
	}
	eqI := func(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	return old.Protocol != cur.Protocol || !eqS(old.GuacdHost, cur.GuacdHost) || !eqI(old.GuacdPort, cur.GuacdPort) ||
		!eqS(old.GuacdEncryption, cur.GuacdEncryption)
}

func (a *App) updateConnection(c *echo.Context) error {
	conn, err := a.loadVisibleConnection(c)
	if err != nil {
		return err
	}
	eff := effOf(c)
	if !eff.CanEditConnection(conn) {
		return errForbidden
	}
	var in connectionInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	before := conn.Parameters
	oldParent := conn.ParentID
	updated := *conn
	if err := in.apply(&updated, before); err != nil {
		return err
	}
	if guacdChanged(conn, &updated) && !eff.IsAdmin() {
		return apiErr(http.StatusForbidden, "FORBIDDEN", "only administrators can change the protocol or the guacd endpoint")
	}
	if !sameParent(oldParent, updated.ParentID) {
		if updated.ParentID != nil {
			if _, err := a.Store.GetConnectionGroup(ctx, *updated.ParentID); err != nil {
				return validation("parentId", "parent group does not exist")
			}
		}
		if !eff.CanCreateIn(updated.ParentID, a.folderVisible(ctx, eff)) {
			return errForbidden
		}
	}
	save := a.Store.UpdateConnection
	if !eff.IsAdmin() {
		save = a.Store.UpdateConnectionUnprivileged // protocol/guacd can never be restored by a stale copy
	}
	if err := save(ctx, &updated); err != nil {
		return err
	}
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.ReevaluateConnection(ctx, updated.ID)
	a.auditBy(c, "connection.update", "connection", idStr(updated.ID), updated.Name, map[string]any{"changed": changedKeys(before, updated.Parameters)})
	return c.JSON(http.StatusOK, a.detail(eff, &updated, a.favSet(ctx, eff.UserID)))
}

func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (a *App) deleteConnection(c *echo.Context) error {
	conn, err := a.loadVisibleConnection(c)
	if err != nil {
		return err
	}
	if !effOf(c).CanEditConnection(conn) {
		return errForbidden
	}
	ctx := c.Request().Context()
	// deleting a connection never shortens the retention of its recordings
	err = a.deleteConnectionsKeepingRetention(ctx, []*store.Connection{conn}, func(tx *store.Store) error {
		return tx.DeleteConnection(ctx, conn.ID)
	})
	if err != nil {
		return err
	}
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Registry.Revoke(func(t *tunnel.Tunnel) bool { return t.ConnectionID == conn.ID }, guac.StatusClientForbidden, "connection deleted")
	a.auditBy(c, "connection.delete", "connection", idStr(conn.ID), conn.Name, nil)
	return c.NoContent(http.StatusNoContent)
}

// ---- connection groups ----

type groupNode struct {
	ID          int64            `json:"id,string"`
	Name        string           `json:"name"`
	ParentID    *int64           `json:"parentId,string"`
	CanEdit     bool             `json:"canEdit"`
	Inheritable bool             `json:"inheritable"`
	Groups      []*groupNode     `json:"groups"`
	Connections []connectionView `json:"connections"`
}

func (a *App) listConnectionGroups(c *echo.Context) error {
	ctx := c.Request().Context()
	eff := effOf(c)
	all, err := a.Store.ListConnectionGroups(ctx)
	if err != nil {
		return err
	}
	vis := a.folderVisible(ctx, eff)
	var out []groupNode
	for _, g := range all {
		if eff.IsAdmin() || eff.Has(store.PermCreateConnection) || vis(g.ID) {
			out = append(out, groupNode{ID: g.ID, Name: g.Name, ParentID: g.ParentID, CanEdit: eff.CanEditFolder(g)})
		}
	}
	return c.JSON(http.StatusOK, newList(out, len(out)))
}

func (a *App) connectionTree(c *echo.Context) error {
	ctx := c.Request().Context()
	eff := effOf(c)
	groups, err := a.Store.ListConnectionGroups(ctx)
	if err != nil {
		return err
	}
	conns, err := a.visibleConnections(ctx, eff)
	if err != nil {
		return err
	}
	favs := a.favSet(ctx, eff.UserID)
	nodes := map[int64]*groupNode{}
	for _, g := range groups {
		nodes[g.ID] = &groupNode{ID: g.ID, Name: g.Name, ParentID: g.ParentID, CanEdit: eff.CanEditFolder(g),
			Inheritable: eff.CanReadFolder(g.ID), Groups: []*groupNode{}, Connections: []connectionView{}}
	}
	root := &groupNode{Name: "ROOT", Groups: []*groupNode{}, Connections: []connectionView{}}
	keep := map[int64]bool{}
	for _, conn := range conns {
		target := root
		if conn.ParentID != nil && nodes[*conn.ParentID] != nil {
			target = nodes[*conn.ParentID]
		}
		target.Connections = append(target.Connections, a.view(eff, conn, favs))
		for _, id := range eff.Ancestors(conn.ParentID) {
			keep[id] = true
		}
	}
	showAll := eff.IsAdmin() || eff.Has(store.PermCreateConnection)
	for _, g := range groups {
		if !showAll && !keep[g.ID] && !eff.CanEditFolder(g) {
			continue
		}
		parent := root
		if g.ParentID != nil && nodes[*g.ParentID] != nil {
			parent = nodes[*g.ParentID]
		}
		parent.Groups = append(parent.Groups, nodes[g.ID])
	}
	return c.JSON(http.StatusOK, root)
}

type groupInput struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
}

func (a *App) createConnectionGroup(c *echo.Context) error {
	var in groupInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	eff := effOf(c)
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 || strings.ContainsAny(in.Name, "/\\") {
		return validation("name", "name is required (max 128 characters, no slashes)")
	}
	parent, err := optID(in.ParentID)
	if err != nil {
		return err
	}
	if parent != nil {
		if _, err := a.Store.GetConnectionGroup(ctx, *parent); err != nil {
			return validation("parentId", "parent group does not exist")
		}
	}
	if !eff.CanCreateIn(parent, a.folderVisible(ctx, eff)) {
		return errForbidden
	}
	uid := eff.UserID
	g := &store.ConnectionGroup{Name: in.Name, ParentID: parent, CreatedBy: &uid}
	if err := a.Store.CreateConnectionGroup(ctx, g); err != nil {
		return err
	}
	a.auditBy(c, "connection_group.create", "connection_group", idStr(g.ID), g.Name, nil)
	return c.JSON(http.StatusCreated, groupNode{ID: g.ID, Name: g.Name, ParentID: g.ParentID, CanEdit: true})
}

func (a *App) loadGroup(c *echo.Context) (*store.ConnectionGroup, error) {
	id, err := pathID(c, "id")
	if err != nil {
		return nil, err
	}
	g, err := a.Store.GetConnectionGroup(c.Request().Context(), id)
	if err != nil {
		return nil, errNotFound
	}
	eff := effOf(c)
	if !eff.IsAdmin() && !eff.Has(store.PermCreateConnection) && !a.folderVisible(c.Request().Context(), eff)(g.ID) {
		return nil, errNotFound
	}
	return g, nil
}

func (a *App) updateConnectionGroup(c *echo.Context) error {
	g, err := a.loadGroup(c)
	if err != nil {
		return err
	}
	eff := effOf(c)
	if !eff.CanEditFolder(g) {
		return errForbidden
	}
	var in groupInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 || strings.ContainsAny(in.Name, "/\\") {
		return validation("name", "name is required (max 128 characters, no slashes)")
	}
	parent, err := optID(in.ParentID)
	if err != nil {
		return err
	}
	if !sameParent(parent, g.ParentID) {
		if parent != nil {
			if _, err := a.Store.GetConnectionGroup(ctx, *parent); err != nil {
				return validation("parentId", "parent group does not exist")
			}
			if eff.IsDescendant(*parent, g.ID) {
				return apiErr(http.StatusBadRequest, "GROUP_CYCLE", "a group cannot be moved under itself or a descendant")
			}
		}
		empty, err := a.Store.GroupIsEmpty(ctx, g.ID)
		if err != nil {
			return err
		}
		if !empty && !eff.IsAdmin() {
			return errForbidden
		}
		if !eff.CanCreateIn(parent, a.folderVisible(ctx, eff)) {
			return errForbidden
		}
	}
	g.Name, g.ParentID = in.Name, parent
	if err := a.Store.UpdateConnectionGroup(ctx, g); err != nil {
		return err
	}
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.Reevaluate(ctx)
	a.auditBy(c, "connection_group.update", "connection_group", idStr(g.ID), g.Name, nil)
	return c.JSON(http.StatusOK, groupNode{ID: g.ID, Name: g.Name, ParentID: g.ParentID, CanEdit: true})
}

func (a *App) deleteConnectionGroup(c *echo.Context) error {
	g, err := a.loadGroup(c)
	if err != nil {
		return err
	}
	eff := effOf(c)
	ctx := c.Request().Context()
	empty, err := a.Store.GroupIsEmpty(ctx, g.ID)
	if err != nil {
		return err
	}
	if empty {
		if !eff.CanEditFolder(g) {
			return errForbidden
		}
	} else {
		if c.QueryParam("recursive") != "true" {
			return apiErr(http.StatusConflict, "GROUP_NOT_EMPTY", "group is not empty; use recursive=true")
		}
		if !eff.IsAdmin() {
			return errForbidden
		}
	}
	conns, err := a.connectionsUnder(ctx, g.ID)
	if err != nil {
		return err
	}
	if err := a.deleteConnectionsKeepingRetention(ctx, conns, func(tx *store.Store) error {
		return tx.DeleteConnectionGroup(ctx, g.ID)
	}); err != nil {
		return err
	}
	_ = a.Store.BumpAccessEpoch(ctx)
	a.Enforcer.Reevaluate(ctx)
	a.auditBy(c, "connection_group.delete", "connection_group", idStr(g.ID), g.Name, map[string]any{"recursive": !empty})
	return c.NoContent(http.StatusNoContent)
}

// ---- favorites ----

func (a *App) listFavorites(c *echo.Context) error {
	ctx := c.Request().Context()
	eff := effOf(c)
	conns, err := a.visibleConnections(ctx, eff)
	if err != nil {
		return err
	}
	favs := a.favSet(ctx, eff.UserID)
	var out []connectionView
	for _, conn := range conns {
		if favs[conn.ID] && eff.CanRead(conn) {
			out = append(out, a.view(eff, conn, favs))
		}
	}
	return c.JSON(http.StatusOK, newList(out, len(out)))
}

func (a *App) addFavorite(c *echo.Context) error {
	var in struct {
		ConnectionID string `json:"connectionId"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	id, err := parseID(in.ConnectionID)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	conn, err := a.Store.GetConnection(ctx, id)
	if err != nil || !effOf(c).CanRead(conn) {
		return errNotFound
	}
	if err := a.Store.AddFavorite(ctx, effOf(c).UserID, id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) removeFavorite(c *echo.Context) error {
	id, err := pathID(c, "connectionId")
	if err != nil {
		return err
	}
	if err := a.Store.RemoveFavorite(c.Request().Context(), effOf(c).UserID, id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ---- dashboard ----

func (a *App) dashboard(c *echo.Context) error {
	ctx := c.Request().Context()
	eff := effOf(c)
	u := userOf(c)
	conns, err := a.visibleConnections(ctx, eff)
	if err != nil {
		return err
	}
	favs := a.favSet(ctx, u.ID)
	var readable []connectionView
	for _, conn := range conns {
		if eff.CanRead(conn) {
			readable = append(readable, a.view(eff, conn, favs))
		}
	}
	sort.SliceStable(readable, func(i, j int) bool {
		if readable[i].Favorite != readable[j].Favorite {
			return readable[i].Favorite
		}
		return strings.ToLower(readable[i].Name) < strings.ToLower(readable[j].Name)
	})
	auditor := eff.Has(store.PermAudit)
	sessFilter := func(t *tunnel.Tunnel) bool { return auditor || t.UserID == u.ID }
	active := a.Registry.List(sessFilter)
	out := map[string]any{}
	counts := map[string]int{"connections": len(readable), "activeSessions": len(active)}
	af := store.AuditFilter{}
	if !auditor {
		af.ActorID = &u.ID
	}
	recent, _, err := a.Store.ListAudit(ctx, af, store.Page{Limit: 8})
	if err != nil {
		return err
	}
	if eff.IsAdmin() || auditor {
		users, groups, err := a.Store.Counts(ctx)
		if err != nil {
			return err
		}
		counts["users"], counts["groups"] = users, groups
	}
	if eff.IsAdmin() {
		po, err := a.Store.PermissionOverview(ctx, 10)
		if err != nil {
			return err
		}
		out["permissionOverview"] = po
	}
	if len(readable) > 10 {
		readable = readable[:10]
	}
	if len(active) > 8 {
		active = active[:8]
	}
	out["counts"] = counts
	out["connections"] = orEmpty(readable)
	out["recentActivity"] = orEmpty(recent)
	out["activeSessions"] = orEmpty(active)
	return c.JSON(http.StatusOK, out)
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
