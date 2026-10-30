package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

func (a *App) launch(c *echo.Context) error {
	var in struct {
		ConnectionID string `json:"connectionId"`
		UITheme      string `json:"uiTheme"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	id, err := parseID(in.ConnectionID)
	if err != nil {
		return err
	}
	u, eff, sess := userOf(c), effOf(c), sessionOf(c)
	if !a.launches.Hit(idStr(u.ID)) {
		return apiErr(http.StatusTooManyRequests, "TOO_MANY_LAUNCHES", "too many session launches, try again later")
	}
	ctx := c.Request().Context()
	conn, err := a.Store.GetConnection(ctx, id)
	if err != nil || !eff.CanRead(conn) {
		return errNotFound
	}
	// recording snapshot: resolved once here, used for the notice, guacd and history
	d, err := a.recordingDefaults(ctx)
	if err != nil {
		return err
	}
	snap := effectiveRecording(conn, d)
	if snap.Video || snap.Text {
		if ok, why := a.recordingAvailable(); !ok {
			exc, err := a.activeException(ctx, conn.ID)
			if err != nil {
				return err
			}
			if exc == nil {
				a.audit(c, &u.ID, u.Username, "recording.unavailable", "connection", idStr(conn.ID), conn.Name, "denied", map[string]any{"reason": why})
				return apiErr(http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "this connection must be recorded and recording is unavailable")
			}
			snap.Exception = true
		}
	}
	t := &tunnel.Tunnel{SessionID: sess.ID, UserID: u.ID, Username: u.Username, ConnectionID: conn.ID,
		ConnectionName: conn.Name, Protocol: conn.Protocol, ClientIP: c.RealIP(),
		RecVideo: snap.Video, RecText: snap.Text, RecKeys: snap.Keys, RecException: snap.Exception}
	if _, ok := themeSchemes[in.UITheme]; ok {
		t.UITheme = in.UITheme
	}
	if conn.MaxConnections != nil {
		t.MaxConn = *conn.MaxConnections
	}
	if conn.MaxConnectionsPerUser != nil {
		t.MaxPerUser = *conn.MaxConnectionsPerUser
	}
	ticket, err := a.Registry.Issue(t)
	if errors.Is(err, tunnel.ErrLimit) {
		return apiErr(http.StatusConflict, "CONNECTION_LIMIT", "connection limit reached")
	}
	if err != nil {
		return err
	}
	// the history entry exists before the ticket can be used; without it the launch is refused
	h := &store.HistoryEntry{UserID: &u.ID, Username: u.Username, ConnectionID: &conn.ID, ConnectionName: conn.Name,
		Protocol: conn.Protocol, ClientIP: t.ClientIP, TunnelID: t.ID, StartAt: time.Now().UnixMilli(),
		RecVideo: &snap.Video, RecText: &snap.Text, RecKeys: &snap.Keys, RecException: snap.Exception,
		OwnerInstance: a.Instance}
	capture := store.CapturePending
	if snap.Exception {
		capture = store.CaptureException
	}
	if snap.Video {
		h.VideoCapture = capture
		if !snap.Exception {
			h.Recording = t.ID
		}
	}
	if snap.Text {
		h.TextCapture = capture
		if !snap.Exception {
			h.Typescript = t.ID + ".typescript"
		}
	}
	if err := a.Store.StartHistory(ctx, h); err != nil {
		a.Registry.Remove(t)
		return err
	}
	t.HistoryID = h.ID
	return c.JSON(http.StatusCreated, map[string]any{"ticket": ticket, "tunnelId": t.ID, "expiresAt": t.ExpiresAt.UnixMilli(),
		"protocol": conn.Protocol, "connectionName": conn.Name, "recording": snap, "recordingNotice": !d.HideNotice,
		// guacd enforces these; the client only uses them to hide copy/paste actions
		"disableCopy": conn.Parameters["disable-copy"] == "true", "disablePaste": conn.Parameters["disable-paste"] == "true"})
}

// authorizeTunnel re-checks session, account restrictions and READ.
func (a *App) authorizeTunnel(ctx context.Context, t *tunnel.Tunnel) (*store.Connection, *guac.Error) {
	sess, err := a.Store.GetSession(ctx, t.SessionID)
	if err != nil || time.Now().UnixMilli() > sess.ExpiresAt {
		return nil, guac.Errorf(guac.StatusClientUnauthorized, "session expired")
	}
	u, err := a.Store.GetUser(ctx, t.UserID)
	if err != nil {
		return nil, guac.Errorf(guac.StatusClientForbidden, "user removed")
	}
	if !sessionCurrent(sess, u) {
		return nil, guac.Errorf(guac.StatusSessionClosed, "password changed")
	}
	if err := auth.CheckRestrictions(u, time.Now()); err != nil {
		return nil, guac.Errorf(guac.StatusClientForbidden, "account restricted")
	}
	eff, err := access.Resolve(ctx, a.Store, u.ID)
	if err != nil {
		return nil, guac.Errorf(guac.StatusServerError, "permission lookup failed")
	}
	conn, err := a.Store.GetConnection(ctx, t.ConnectionID)
	if err != nil || !eff.CanRead(conn) {
		return nil, guac.Errorf(guac.StatusClientForbidden, "access revoked")
	}
	return conn, nil
}

func clientInfo(c *echo.Context) guac.ClientInfo {
	q := c.QueryParams()
	atoi := func(k string, def, lo, hi int) int {
		v, err := strconv.Atoi(q.Get(k))
		if err != nil || v < lo || v > hi {
			return def
		}
		return v
	}
	limit := func(vals []string) []string {
		var out []string
		for _, v := range vals {
			if len(v) <= 64 && len(out) < 16 {
				out = append(out, v)
			}
		}
		return out
	}
	tz := q.Get("timezone")
	if _, err := time.LoadLocation(tz); err != nil || len(tz) > 64 {
		tz = ""
	}
	return guac.ClientInfo{
		Width: atoi("width", 1024, 1, 8192), Height: atoi("height", 768, 1, 8192), DPI: atoi("dpi", 96, 48, 600),
		Audio: limit(q["audio"]), Video: limit(q["video"]), Image: limit(q["image"]), Timezone: tz,
	}
}

func (a *App) guacdDial(conn *store.Connection) (guac.DialOptions, error) {
	host, port, _ := net.SplitHostPort(a.Cfg.Guacd.Addr)
	if conn.GuacdHost != nil && *conn.GuacdHost != "" {
		host = *conn.GuacdHost
	}
	if conn.GuacdPort != nil && *conn.GuacdPort > 0 {
		port = strconv.Itoa(*conn.GuacdPort)
	}
	useTLS := a.Cfg.Guacd.TLS
	if conn.GuacdEncryption != nil {
		switch *conn.GuacdEncryption {
		case "SSL":
			useTLS = true
		case "NONE":
			useTLS = false
		}
	}
	opts := guac.DialOptions{Addr: net.JoinHostPort(host, port)}
	if useTLS {
		tc := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: a.Cfg.Guacd.TLSInsecure} //nolint:gosec // opt-in for labs
		if a.Cfg.Guacd.TLSServerName != "" {
			tc.ServerName = a.Cfg.Guacd.TLSServerName
		}
		if a.Cfg.Guacd.TLSCA != "" {
			pem, err := os.ReadFile(a.Cfg.Guacd.TLSCA)
			if err != nil {
				return opts, fmt.Errorf("read GUACD_TLS_CA: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return opts, errors.New("GUACD_TLS_CA contains no certificates")
			}
			tc.RootCAs = pool
		}
		opts.TLSConfig = tc
	}
	return opts, nil
}

func (a *App) tunnel(c *echo.Context) error {
	r := c.Request()
	if !a.originAllowed(r) {
		return errCSRF
	}
	sess := sessionOf(c)
	t, err := a.Registry.Redeem(c.QueryParam("ticket"), sess.ID)
	if err != nil {
		return apiErr(http.StatusForbidden, "TICKET_INVALID", "invalid or expired launch ticket")
	}
	ctx := r.Context()
	conn, gerr := a.authorizeTunnel(ctx, t)
	if gerr != nil {
		a.Registry.Remove(t)
		_ = a.Store.FinishNotStarted(a.bg(), t.HistoryID)
		return errForbidden
	}
	ws, err := websocket.Accept(c.Response(), r, &websocket.AcceptOptions{
		Subprotocols:       []string{"guacamole"},
		InsecureSkipVerify: true, // Origin already validated above
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		a.Registry.Remove(t)
		_ = a.Store.FinishNotStarted(a.bg(), t.HistoryID)
		return nil
	}
	a.runTunnel(ws, t, conn, clientInfo(c))
	return nil
}

func (a *App) runTunnel(ws *websocket.Conn, t *tunnel.Tunnel, conn *store.Connection, ci guac.ClientInfo) {
	tctx := t.Context()
	defer a.Registry.Remove(t)
	started := false
	defer func() {
		if !started {
			_ = a.Store.FinishNotStarted(a.bg(), t.HistoryID)
		}
	}()
	fail := func(e *guac.Error) {
		a.Log.Info("tunnel failed", "tunnel", t.ID, "user", t.Username, "connection", t.ConnectionName, "status", int(e.Status), "err", e.Message)
		tunnel.SendError(ws, e)
	}

	opts, err := a.guacdDial(conn)
	if err != nil {
		fail(guac.Errorf(guac.StatusServerError, "guacd TLS configuration error"))
		return
	}
	hctx, cancel := context.WithTimeout(tctx, guac.HandshakeTimeout)
	defer cancel()
	raw, err := guac.Dial(hctx, opts)
	if err != nil {
		fail(tunnel.AsGuacError(err))
		return
	}
	params := a.recordingParams(conn, t)
	if conn.Protocol == "ssh" || conn.Protocol == "telnet" {
		// the connection's scheme wins, then the user's preference (or the palette of their UI theme)
		prefs, _ := a.Store.GetPreferences(hctx, t.UserID)
		if prefs == nil {
			prefs = &store.Preferences{}
		}
		theme := t.UITheme // what the user is looking at; the saved theme otherwise
		if theme == "" {
			theme = prefs.Theme
		}
		params["color-scheme"] = terminalColorScheme(conn.Parameters["color-scheme"], prefs.TerminalScheme, theme)
	}
	g, err := guac.Handshake(hctx, raw, guac.Config{Protocol: conn.Protocol, Parameters: params, Client: ci})
	if err != nil {
		raw.Close()
		if cause := context.Cause(tctx); cause != nil {
			fail(tunnel.AsGuacError(cause))
			return
		}
		fail(tunnel.AsGuacError(err))
		return
	}
	// re-check immediately before activation
	if _, gerr := a.authorizeTunnel(tctx, t); gerr != nil {
		g.Close()
		fail(gerr)
		return
	}
	if err := a.Registry.Activate(t); err != nil {
		g.Close()
		fail(tunnel.AsGuacError(err))
		return
	}

	started = true
	a.activated.Store(t.HistoryID, true)
	uid, cid := t.UserID, t.ConnectionID
	_ = a.Store.ActivateHistory(a.bg(), t.HistoryID, time.Now().UnixMilli())
	go a.verifyCaptureSoon(t)
	a.audit(nil, &uid, t.Username, "tunnel.open", "connection", idStr(cid), t.ConnectionName, "success",
		map[string]any{"tunnel": t.ID, "protocol": t.Protocol, "ip": t.ClientIP})
	a.Log.Info("tunnel open", "tunnel", t.ID, "user", t.Username, "connection", t.ConnectionName, "guacd_id", g.ConnectionID)

	reason := tunnel.Relay(tctx, ws, g, t.ID, tunnel.RelayOptions{OnActivity: func() {
		_ = a.Store.TouchSession(a.bg(), t.SessionID, time.Now().UnixMilli())
	}})

	closeReason := reason.Message
	if reason.Status != guac.StatusSuccess {
		tunnel.SendError(ws, reason)
	} else {
		_ = ws.Close(websocket.StatusNormalClosure, "0")
	}
	if err := a.Store.EndHistory(a.bg(), t.HistoryID, time.Now().UnixMilli(), truncateStr(closeReason, 64)); err != nil {
		a.Log.Error("history end failed", "err", err, "tunnel", t.ID) // stays marked as started
	} else {
		a.activated.Delete(t.HistoryID)
	}
	a.finishCapture(t)
	a.audit(nil, &uid, t.Username, "tunnel.close", "connection", idStr(cid), t.ConnectionName, "success",
		map[string]any{"tunnel": t.ID, "reason": closeReason, "status": int(reason.Status)})
	a.Log.Info("tunnel closed", "tunnel", t.ID, "reason", closeReason, "status", int(reason.Status))
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (a *App) listSessions(c *echo.Context) error {
	eff, u := effOf(c), userOf(c)
	all := eff.Has(store.PermAudit)
	items := a.Registry.List(func(t *tunnel.Tunnel) bool { return all || t.UserID == u.ID })
	return c.JSON(http.StatusOK, newList(items, len(items)))
}

func (a *App) killSession(c *echo.Context) error {
	id := c.Param("id")
	t := a.Registry.Get(id)
	eff, u := effOf(c), userOf(c)
	if t == nil || (t.UserID != u.ID && !eff.Has(store.PermAudit)) {
		return errNotFound
	}
	if t.UserID != u.ID && !eff.IsAdmin() {
		return errForbidden
	}
	msg := "terminated by user"
	if t.UserID != u.ID {
		msg = "terminated by an administrator"
	}
	a.Registry.Revoke(func(x *tunnel.Tunnel) bool { return x.ID == id }, guac.StatusSessionClosed, msg)
	a.auditBy(c, "tunnel.kill", "tunnel", id, t.ConnectionName, map[string]any{"user": t.Username})
	return c.NoContent(http.StatusNoContent)
}
