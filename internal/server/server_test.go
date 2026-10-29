package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/store/storetest"
)

// fakeGuacd accepts connections, completes the handshake and then sends a
// sync instruction every 100 ms, recording the connect arguments it received.
func fakeGuacd(t *testing.T) (addr string, connects chan []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	connects = make(chan []string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				rd := guac.NewReader(c, 0)
				if ins, err := rd.Read(); err != nil || ins.Opcode != "select" {
					return
				}
				_, _ = c.Write(guac.New("args", "VERSION_1_5_0", "hostname", "password").Bytes())
				for {
					ins, err := rd.Read()
					if err != nil {
						return
					}
					if ins.Opcode == "connect" {
						connects <- ins.Args
						break
					}
				}
				_, _ = c.Write(guac.New("ready", "$fake").Bytes())
				go func() { _, _ = io.Copy(io.Discard, c) }()
				for i := 0; ; i++ {
					if _, err := c.Write(guac.New("sync", "1").Bytes()); err != nil {
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			}(c)
		}
	}()
	return ln.Addr().String(), connects
}

type tclient struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
}

func (tc *tclient) do(method, path string, body any) (int, map[string]any) {
	tc.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, tc.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "goguacamole")
	req.Header.Set("Origin", tc.base)
	if tc.csrf != "" {
		req.Header.Set("X-CSRF-Token", tc.csrf)
	}
	resp, err := tc.c.Do(req)
	if err != nil {
		tc.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func newTestApp(t *testing.T) (*App, *httptest.Server, chan []string) {
	t.Helper()
	gaddr, connects := fakeGuacd(t)
	st := storetest.Open(t, "sqlite", filepath.Join(t.TempDir(), "srv.db"))
	cfg := &config.Config{}
	cfg.App = config.AppConfig{Env: "test", Addr: ":0", SessionIdleTimeout: 30 * time.Minute, SessionTTL: 8 * time.Hour, LogLevel: "error"}
	cfg.Guacd = config.GuacdConfig{Addr: gaddr}
	cfg.Audit.RetentionDays = 90
	dist := fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app-123.js": {Data: []byte("js")}}
	a := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), dist)
	srv := httptest.NewServer(a.Echo)
	t.Cleanup(srv.Close)
	return a, srv, connects
}

func login(t *testing.T, base, user, pass string) *tclient {
	jar, _ := cookiejar.New(nil)
	tc := &tclient{t: t, base: base, c: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
	st, out := tc.do("POST", "/api/v1/auth/login", map[string]any{"username": user, "password": pass})
	if st != 200 {
		t.Fatalf("login %s: %d %v", user, st, out)
	}
	tc.csrf = out["csrfToken"].(string)
	return tc
}

func mkUser(t *testing.T, st *store.Store, name, pass string, admin bool) *store.User {
	hash, _ := auth.HashPassword(pass)
	u := &store.User{Username: name, PasswordHash: hash, PasswordScheme: store.SchemeBcrypt}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if admin {
		_ = st.GrantSystem(context.Background(), store.SubjectUser, u.ID, store.PermAdminister)
	}
	return u
}

func TestTunnelLifecycleAndCrossProcessRevocation(t *testing.T) {
	a, srv, connects := newTestApp(t)
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	bob := mkUser(t, a.Store, "bob", "BobPass#12", false)

	admin := login(t, srv.URL, "root", "RootPass#1")
	st, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "srv", "protocol": "ssh",
		"parameters": map[string]any{"hostname": "10.0.0.9", "password": "Pa55w0rd", "disable-paste": "true"}})
	if st != 201 {
		t.Fatalf("create: %d %v", st, out)
	}
	connID := out["id"].(string)
	if p := out["parameters"].(map[string]any)["password"]; p.(map[string]any)["set"] != true {
		t.Fatalf("secret not masked: %v", p)
	}
	// missing CSRF token
	noCSRF := *admin
	noCSRF.csrf = ""
	if st, _ := noCSRF.do("POST", "/api/v1/connection-groups", map[string]any{"name": "x"}); st != 403 {
		t.Fatalf("missing CSRF accepted: %d", st)
	}
	if st, _ := admin.do("PUT", "/api/v1/permissions", map[string]any{"subject": "user:" + idStr(bob.ID), "connections": []string{connID}}); st != 204 {
		t.Fatalf("grant: %d", st)
	}

	user := login(t, srv.URL, "bob", "BobPass#12")
	st, out = user.do("POST", "/api/v1/sessions", map[string]any{"connectionId": connID})
	if st != 201 {
		t.Fatalf("launch: %d %v", st, out)
	}
	if out["disablePaste"] != true || out["disableCopy"] != false {
		t.Fatalf("launch clipboard flags: %v", out)
	}
	ticket := out["ticket"].(string)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/tunnel?ticket=" + ticket + "&width=800&height=600"
	h := http.Header{"Origin": {srv.URL}}
	for _, c := range user.c.Jar.Cookies(mustURL(srv.URL)) {
		h.Add("Cookie", c.Name+"="+c.Value)
	}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(dctx, wsURL, &websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{"guacamole"}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	_, first, err := ws.Read(dctx)
	if err != nil || !strings.HasPrefix(string(first), "0.,36.") {
		t.Fatalf("uuid instruction: %q %v", first, err)
	}
	args := <-connects
	if strings.Join(args, "|") != "VERSION_1_5_0|10.0.0.9|Pa55w0rd" {
		t.Fatalf("connect args: %v", args)
	}
	// internal ping is echoed, not forwarded
	if err := ws.Write(dctx, websocket.MessageText, guac.New("", "ping", "42").Bytes()); err != nil {
		t.Fatal(err)
	}
	gotPing := false
	for i := 0; i < 20 && !gotPing; i++ {
		_, msg, err := ws.Read(dctx)
		if err != nil {
			t.Fatal(err)
		}
		gotPing = strings.Contains(string(msg), "4.ping,2.42;")
	}
	if !gotPing {
		t.Fatal("ping not echoed")
	}
	// ticket reuse refused
	if _, resp, err := websocket.Dial(dctx, wsURL, &websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{"guacamole"}}); err == nil || resp.StatusCode != 403 {
		t.Fatalf("ticket reuse: %v", err)
	}
	if a.Registry.CountActive(nil) != 1 {
		t.Fatal("tunnel not active in registry")
	}

	// simulate `goguacamole user disable bob` from another process
	if err := a.Store.SetUserDisabled(ctx, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	_, _ = a.Store.DeleteUserSessions(ctx, bob.ID, "")
	a.Enforcer.tick(ctx)

	rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
	defer rcancel()
	var closeErr error
	for {
		if _, _, closeErr = ws.Read(rctx); closeErr != nil {
			break
		}
	}
	if !strings.Contains(closeErr.Error(), "523") && !strings.Contains(closeErr.Error(), "771") {
		t.Fatalf("tunnel not closed with a revocation status: %v", closeErr)
	}
	time.Sleep(200 * time.Millisecond)
	hist, _, _ := a.Store.ListHistory(ctx, store.HistoryFilter{}, store.Page{})
	if len(hist) != 1 || hist[0].EndAt == nil {
		t.Fatalf("history not finalized: %+v", hist)
	}
	if st, _ := user.do("GET", "/api/v1/me", nil); st != 401 {
		t.Fatalf("disabled user still authenticated: %d", st)
	}
}

func TestAuthorizationMatrix(t *testing.T) {
	a, srv, _ := newTestApp(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	mkUser(t, a.Store, "eve", "EvePass#12", false)
	admin := login(t, srv.URL, "root", "RootPass#1")
	_, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "srv", "protocol": "rdp",
		"parameters": map[string]any{"hostname": "10.0.0.9", "port": "3389"}})
	connID := out["id"].(string)
	eve := login(t, srv.URL, "eve", "EvePass#12")

	cases := []struct {
		method, path string
		body         any
		want         int
	}{
		{"GET", "/api/v1/connections/" + connID, nil, 404},
		{"POST", "/api/v1/sessions", map[string]any{"connectionId": connID}, 404},
		{"PUT", "/api/v1/connections/" + connID, map[string]any{"name": "x", "protocol": "rdp", "parameters": map[string]any{"hostname": "h"}}, 404},
		{"GET", "/api/v1/users", nil, 403},
		{"GET", "/api/v1/audit", nil, 403},
		{"POST", "/api/v1/connections", map[string]any{"name": "mine", "protocol": "ssh", "parameters": map[string]any{"hostname": "h"}}, 403},
		{"PUT", "/api/v1/permissions", map[string]any{"subject": "user:1", "system": []string{"ADMINISTER"}}, 403},
	}
	for _, c := range cases {
		if st, out := eve.do(c.method, c.path, c.body); st != c.want {
			t.Errorf("%s %s: got %d want %d (%v)", c.method, c.path, st, c.want, out)
		}
	}
	// validation and last-admin protection
	if st, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "bad", "protocol": "ssh", "parameters": map[string]any{"hostname": "h", "port": "abc"}}); st != 422 || out["field"] != "parameters.port" {
		t.Errorf("invalid port: %d %v", st, out)
	}
	for name, val := range map[string]string{"color-scheme": "pink", "backspace": "9", "terminal-type": "dumb", "timezone": "Mars/Base", "sftp-disable-upload": "yes", "font-size": "200"} {
		if st, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "bad", "protocol": "ssh", "parameters": map[string]any{"hostname": "h", name: val}}); st != 422 || out["field"] != "parameters."+name {
			t.Errorf("invalid %s: %d %v", name, st, out)
		}
	}
	if st, out := admin.do("POST", "/api/v1/connections", map[string]any{"name": "palette", "protocol": "ssh", "parameters": map[string]any{"hostname": "h",
		"color-scheme": "foreground: rgb:00/00/ff; background: color0"}}); st != 201 {
		t.Errorf("custom color-scheme: %d %v", st, out)
	}
	if st, out := admin.do("PUT", "/api/v1/permissions", map[string]any{"subject": "user:1", "system": []string{}}); st != 400 || out["code"] != "SELF_MODIFICATION" {
		t.Errorf("self lockout: %d %v", st, out)
	}
	// SPA fallback and cache headers
	resp, _ := http.Get(srv.URL + "/connections")
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-cache" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("spa fallback: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = http.Get(srv.URL + "/assets/app-123.js")
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("asset cache: %v", resp.Header.Get("Cache-Control"))
	}
	resp, _ = http.Get(srv.URL + "/api/v1/nope")
	if resp.StatusCode != 404 {
		t.Errorf("unknown api route: %d", resp.StatusCode)
	}
}

func TestLoginProtections(t *testing.T) {
	a, srv, _ := newTestApp(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	// cross-site login
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/login", strings.NewReader(`{"username":"root","password":"RootPass#1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "goguacamole")
	req.Header.Set("Origin", "https://evil.example")
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 403 {
		t.Fatalf("cross-site login: %d", resp.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	tc := &tclient{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	for i := 0; i < 10; i++ {
		if st, _ := tc.do("POST", "/api/v1/auth/login", map[string]any{"username": "root", "password": "wrong"}); st != 401 {
			t.Fatalf("attempt %d: %d", i, st)
		}
	}
	if st, out := tc.do("POST", "/api/v1/auth/login", map[string]any{"username": "root", "password": "RootPass#1"}); st != 429 {
		t.Fatalf("rate limit: %d %v", st, out)
	}
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func TestCodexRegressions(t *testing.T) {
	a, srv, _ := newTestApp(t)
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	bob := mkUser(t, a.Store, "bob", "BobPass#12", false)
	admin := login(t, srv.URL, "root", "RootPass#1")
	user := login(t, srv.URL, "bob", "BobPass#12")

	// password changed by another process (importer / CLI) invalidates existing sessions
	hash, _ := auth.HashPassword("Changed#123")
	if err := a.Store.SetPassword(ctx, bob.ID, hash, store.SchemeBcrypt, "", false); err != nil {
		t.Fatal(err)
	}
	if st, _ := user.do("GET", "/api/v1/me", nil); st != 401 {
		t.Fatalf("stale session still valid after out-of-process password change: %d", st)
	}
	// self-service change keeps the current session but ends the others
	other := login(t, srv.URL, "root", "RootPass#1")
	if st, out := admin.do("PUT", "/api/v1/me/password", map[string]any{"currentPassword": "RootPass#1", "newPassword": "RootPass#2"}); st != 204 {
		t.Fatalf("self change: %d %v", st, out)
	}
	if st, _ := admin.do("GET", "/api/v1/me", nil); st != 200 {
		t.Fatalf("current session lost after self change: %d", st)
	}
	if st, _ := other.do("GET", "/api/v1/me", nil); st != 401 {
		t.Fatalf("other session kept after self change: %d", st)
	}

	// invalid access windows are rejected
	for _, body := range []map[string]any{
		{"accessWindowStart": "99:00", "accessWindowEnd": "18:00"},
		{"accessWindowStart": "08:00"},
		{"validFrom": "2026-02-30"},
	} {
		if st, out := admin.do("PUT", "/api/v1/users/"+idStr(bob.ID), body); st != 422 {
			t.Errorf("invalid restriction %v accepted: %d %v", body, st, out)
		}
	}

	// origin comparison includes the scheme
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/login", strings.NewReader(`{"username":"root","password":"RootPass#2"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "goguacamole")
	req.Header.Set("Origin", strings.Replace(srv.URL, "http://", "https://", 1))
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 403 {
		t.Fatalf("origin with different scheme accepted: %d", resp.StatusCode)
	}
}
