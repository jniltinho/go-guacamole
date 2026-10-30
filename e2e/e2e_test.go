//go:build e2e

// Package e2e runs API and tunnel tests against a running GoGuacamole with a
// real guacd and an SSH lab target. Run through `make e2e`, or manually:
//
//	GG_E2E_URL=http://127.0.0.1:8080 GG_E2E_ADMIN_PASSWORD=... go test -tags e2e ./e2e/...
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	baseURL   = env("GG_E2E_URL", "http://127.0.0.1:8080")
	adminUser = env("GG_E2E_ADMIN_USER", "admin")
	adminPass = env("GG_E2E_ADMIN_PASSWORD", "adminadmin1")
	sshHost   = env("GG_E2E_SSH_HOST", "ssh-target")
	sshPort   = env("GG_E2E_SSH_PORT", "2222")
	sshUser   = env("GG_E2E_SSH_USER", "lab")
	sshPass   = env("GG_E2E_SSH_PASSWORD", "labpass123")
	newAdmin  = "E2e-Admin#2026"
)

type client struct {
	t    *testing.T
	http *http.Client
	csrf string
}

func newClient(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "goguacamole")
	req.Header.Set("Origin", baseURL)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (c *client) raw(method, path string) (int, string) {
	c.t.Helper()
	req, _ := http.NewRequest(method, baseURL+path, nil)
	req.Header.Set("Origin", baseURL)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (c *client) login(user, pass string, newPass string) (int, map[string]any) {
	body := map[string]any{"username": user, "password": pass}
	if newPass != "" {
		body["newPassword"] = newPass
	}
	st, out := c.do("POST", "/api/v1/auth/login", body)
	if st == 200 {
		c.csrf, _ = out["csrfToken"].(string)
	}
	return st, out
}

func must(t *testing.T, st, want int, out map[string]any, what string) {
	t.Helper()
	if st != want {
		t.Fatalf("%s: status %d (want %d): %v", what, st, want, out)
	}
}

func (c *client) wsURL(ticket string) string {
	u, _ := url.Parse(baseURL)
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	u.Path = "/api/v1/tunnel"
	q := url.Values{"ticket": {ticket}, "width": {"1024"}, "height": {"768"}, "dpi": {"96"}, "image": {"image/png"}, "timezone": {"America/Sao_Paulo"}}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *client) dial(ticket, origin string) (*websocket.Conn, *http.Response, error) {
	h := http.Header{"Origin": {origin}}
	u, _ := url.Parse(baseURL)
	for _, ck := range c.http.Jar.Cookies(u) {
		h.Add("Cookie", ck.Name+"="+ck.Value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return websocket.Dial(ctx, c.wsURL(ticket), &websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{"guacamole"}})
}

func (c *client) launch(connID string) (int, map[string]any) {
	return c.do("POST", "/api/v1/sessions", map[string]any{"connectionId": connID})
}

func read(t *testing.T, ws *websocket.Conn, d time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	_, b, err := ws.Read(ctx)
	return string(b), err
}

func TestEndToEnd(t *testing.T) {
	admin := newClient(t)
	st, out := admin.login(adminUser, adminPass, "")
	if st == 403 && out["code"] == "PASSWORD_CHANGE_REQUIRED" {
		st, out = admin.login(adminUser, adminPass, newAdmin)
	}
	if st == 401 { // already changed by a previous run
		st, out = admin.login(adminUser, newAdmin, "")
	}
	must(t, st, 200, out, "admin login")

	// cross-site login is refused
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", strings.NewReader(`{"username":"x","password":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "goguacamole")
	req.Header.Set("Origin", "https://evil.example")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 403 {
		t.Fatalf("cross-site login not refused: %v %v", resp, err)
	}

	suffix := fmt.Sprint(time.Now().UnixNano() % 1e9)
	st, out = admin.do("POST", "/api/v1/connections", map[string]any{"name": "lab-ssh-" + suffix, "protocol": "ssh",
		"parameters": map[string]any{"hostname": sshHost, "port": sshPort, "username": sshUser, "password": sshPass}})
	must(t, st, 201, out, "create connection")
	connID := out["id"].(string)

	mkUser := func(name string) string {
		st, out := admin.do("POST", "/api/v1/users", map[string]any{"username": name, "password": "UserPass#123", "mustChangePassword": false})
		must(t, st, 201, out, "create user "+name)
		return out["id"].(string)
	}
	bobID := mkUser("bob-" + suffix)
	mkUser("eve-" + suffix)
	st, out = admin.do("POST", "/api/v1/user-groups", map[string]any{"name": "ops-" + suffix})
	must(t, st, 201, out, "create group")
	groupID := out["id"].(string)
	st, out = admin.do("PUT", "/api/v1/user-groups/"+groupID+"/members", map[string]any{"members": []string{bobID}})
	must(t, st, 204, out, "members")
	st, out = admin.do("PUT", "/api/v1/permissions", map[string]any{"subject": "group:" + groupID, "connections": []string{connID}})
	must(t, st, 204, out, "grant group")

	bob := newClient(t)
	st, out = bob.login("bob-"+suffix, "UserPass#123", "")
	must(t, st, 200, out, "bob login")
	eve := newClient(t)
	st, out = eve.login("eve-"+suffix, "UserPass#123", "")
	must(t, st, 200, out, "eve login")

	// eve has no permission: 404, not 403
	st, out = eve.launch(connID)
	must(t, st, 404, out, "eve launch")
	st, out = eve.do("GET", "/api/v1/connections/"+connID, nil)
	must(t, st, 404, out, "eve get connection")

	// mutation without CSRF token
	noCSRF := *bob
	noCSRF.csrf = ""
	st, out = noCSRF.do("POST", "/api/v1/favorites", map[string]any{"connectionId": connID})
	must(t, st, 403, out, "missing csrf")

	// foreign origin WebSocket is refused before any guacd connection
	st, out = bob.launch(connID)
	must(t, st, 201, out, "bob launch")
	if _, resp, err := bob.dial(out["ticket"].(string), "https://evil.example"); err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("foreign origin accepted: %v %v", resp, err)
	}

	// real tunnel
	st, out = bob.launch(connID)
	must(t, st, 201, out, "bob launch 2")
	ticket := out["ticket"].(string)
	ws, _, err := bob.dial(ticket, baseURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()
	first, err := read(t, ws, 20*time.Second)
	if err != nil || !strings.HasPrefix(first, "0.,36.") {
		t.Fatalf("expected tunnel uuid instruction, got %q %v", first, err)
	}
	sawDisplay := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && !sawDisplay {
		msg, err := read(t, ws, 10*time.Second)
		if err != nil {
			t.Fatalf("read display: %v", err)
		}
		if strings.Contains(msg, "3.img,") || strings.Contains(msg, "4.size,") || strings.Contains(msg, "4.blob,") {
			sawDisplay = true
		}
		if strings.Contains(msg, "5.error,") {
			t.Fatalf("guacd error: %s", msg)
		}
	}
	if !sawDisplay {
		t.Fatal("no display instructions received")
	}

	// type a command in the SSH session (recorded in the typescript)
	time.Sleep(time.Second)
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	for _, r := range "echo GGREC-OK" {
		for _, pressed := range []string{"1", "0"} {
			ks := fmt.Sprint(int(r))
			ins := fmt.Sprintf("3.key,%d.%s,1.%s;", len(ks), ks, pressed)
			if err := ws.Write(wctx, websocket.MessageText, []byte(ins)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, pressed := range []string{"1", "0"} {
		_ = ws.Write(wctx, websocket.MessageText, []byte("3.key,5.65293,1."+pressed+";"))
	}
	wcancel()
	time.Sleep(1500 * time.Millisecond)

	// ticket reuse is refused
	if _, resp, err := bob.dial(ticket, baseURL); err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("ticket reuse accepted: %v %v", resp, err)
	}

	// session is listed for the admin
	st, out = admin.do("GET", "/api/v1/sessions", nil)
	must(t, st, 200, out, "sessions")
	if out["total"].(float64) < 1 {
		t.Fatal("active session not listed")
	}

	// revoke while connected → tunnel closed with 771
	st, out = admin.do("PUT", "/api/v1/permissions", map[string]any{"subject": "group:" + groupID, "connections": []string{}})
	must(t, st, 204, out, "revoke")
	closed := false
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := read(t, ws, 5*time.Second)
		if err != nil {
			if websocket.CloseStatus(err) == 1008 {
				closed = true
			}
			if strings.Contains(err.Error(), "771") {
				closed = true
			}
			break
		}
		if strings.Contains(msg, "5.error,") && strings.Contains(msg, "3.771;") {
			closed = true
		}
	}
	if !closed {
		t.Fatal("tunnel not closed with 771 after revocation")
	}
	st, out = bob.launch(connID)
	must(t, st, 404, out, "launch after revoke")

	// history and audit (no secrets)
	time.Sleep(500 * time.Millisecond)
	st, out = admin.do("GET", "/api/v1/history?q=lab-ssh-"+suffix, nil)
	must(t, st, 200, out, "history")
	items := out["items"].([]any)
	if len(items) == 0 || items[0].(map[string]any)["endAt"] == nil {
		t.Fatalf("history row missing/unfinished: %v", out)
	}
	hist := items[0].(map[string]any)
	if os.Getenv("GG_E2E_RECORDING") != "0" {
		if hist["hasRecording"] != true || hist["hasTypescript"] != true {
			t.Fatalf("history row without recordings: %v", hist)
		}
		hid := hist["id"].(string)
		code, body := admin.raw("GET", "/api/v1/history/"+hid+"/typescript")
		if code != 200 || !strings.Contains(body, "echo GGREC-OK") {
			t.Fatalf("typescript: %d %q", code, body)
		}
		code, body = admin.raw("GET", "/api/v1/history/"+hid+"/recording")
		if code != 200 || len(body) < 100 || !strings.Contains(body, ".size,") {
			t.Fatalf("screen recording: %d len=%d", code, len(body))
		}
		if code, _ := bob.raw("GET", "/api/v1/history/"+hid+"/recording"); code != 403 {
			t.Fatalf("recording visible without AUDIT: %d", code)
		}
	}

	st, out = admin.do("GET", "/api/v1/audit?limit=200", nil)
	must(t, st, 200, out, "audit")
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte(sshPass)) || bytes.Contains(raw, []byte("UserPass#123")) {
		t.Fatal("secret found in audit log")
	}
	for _, want := range []string{"tunnel.open", "tunnel.close", "permissions.update", "auth.login.success"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("audit missing %s", want)
		}
	}
	if os.Getenv("GG_E2E_S3") != "" {
		objectStorage(t, admin, "lab-ssh-"+suffix, hist)
	}
}
