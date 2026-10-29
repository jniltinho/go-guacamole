package server

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-guacamole/internal/auth"
)

func totpNow(t *testing.T, secret string, at time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[off:off+4])&0x7fffffff)%1_000_000)
}

func TestAuthSettingsAndTOTP(t *testing.T) {
	a, srv, _ := newTestApp(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	bob := mkUser(t, a.Store, "bob", "BobPass#12", false)
	admin := login(t, srv.URL, "root", "RootPass#1")
	if st, _ := login(t, srv.URL, "bob", "BobPass#12").do("GET", "/api/v1/settings/auth", nil); st != 403 {
		t.Fatalf("non-admin read auth settings: %d", st)
	}

	key := "4c0b569e4c96df157eee1b65dd0e4d41"
	st, out := admin.do("PUT", "/api/v1/settings/auth", map[string]any{"totp": map[string]any{"enabled": true},
		"json": map[string]any{"enabled": true, "secretKey": key}})
	if st != 200 {
		t.Fatalf("save: %d %v", st, out)
	}
	if out["json"].(map[string]any)["secretKey"] != "" || out["secretsSet"].(map[string]any)["json.secretKey"] != true {
		t.Fatalf("secret not masked: %v", out)
	}
	// saving again without the secret keeps it
	if st, _ := admin.do("PUT", "/api/v1/settings/auth", map[string]any{"totp": map[string]any{"enabled": true},
		"json": map[string]any{"enabled": true}}); st != 200 {
		t.Fatalf("resave: %d", st)
	}
	if s, _ := a.authSettings(t.Context()); s.JSON.SecretKey != key || !s.TOTP.Enabled {
		t.Fatalf("stored settings: %+v", s)
	}

	jar, _ := cookiejar.New(nil)
	tc := &tclient{t: t, base: srv.URL, c: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
	creds := map[string]any{"username": "bob", "password": "BobPass#12"}
	st, out = tc.do("POST", "/api/v1/auth/login", creds)
	if st != 403 || out["code"] != "TOTP_ENROLL" {
		t.Fatalf("expected enrollment: %d %v", st, out)
	}
	data := out["data"].(map[string]any)
	secret := data["secret"].(string)
	if !strings.HasPrefix(data["qr"].(string), "data:image/png;base64,") {
		t.Fatalf("no QR code: %v", data)
	}
	creds["totpCode"] = "000000"
	if st, out = tc.do("POST", "/api/v1/auth/login", creds); st != 401 || out["code"] != "TOTP_INVALID" {
		t.Fatalf("wrong code: %d %v", st, out)
	}
	code := totpNow(t, secret, time.Now())
	creds["totpCode"] = code
	if st, out = tc.do("POST", "/api/v1/auth/login", creds); st != 200 {
		t.Fatalf("enroll login: %d %v", st, out)
	}
	if u, _ := a.Store.GetUser(t.Context(), bob.ID); !u.TOTPEnrolled {
		t.Fatal("not enrolled")
	}
	// a code cannot be used twice
	if st, out = tc.do("POST", "/api/v1/auth/login", creds); st != 401 || out["code"] != "TOTP_INVALID" {
		t.Fatalf("replayed code: %d %v", st, out)
	}
	delete(creds, "totpCode")
	if st, out = tc.do("POST", "/api/v1/auth/login", creds); st != 403 || out["code"] != "TOTP_REQUIRED" {
		t.Fatalf("enrolled user without code: %d %v", st, out)
	}
	// admin reset → enrollment again (admin itself must pass TOTP too: use the store directly)
	if err := a.Store.SetTOTPSecret(t.Context(), bob.ID, ""); err != nil {
		t.Fatal(err)
	}
	if st, out = tc.do("POST", "/api/v1/auth/login", creds); st != 403 || out["code"] != "TOTP_ENROLL" {
		t.Fatalf("after reset: %d %v", st, out)
	}
}

func noRedirect(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestJSONAndHeaderLogin(t *testing.T) {
	a, srv, _ := newTestApp(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	key := "00112233445566778899aabbccddeeff"

	// header auth needs trusted proxies
	if st, out := admin.do("PUT", "/api/v1/settings/auth", map[string]any{"header": map[string]any{"enabled": true, "name": "X-User"}}); st != 422 {
		t.Fatalf("header without trusted proxies: %d %v", st, out)
	}
	a.Cfg.App.TrustedProxies = []string{"127.0.0.1"}
	if st, out := admin.do("PUT", "/api/v1/settings/auth", map[string]any{"header": map[string]any{"enabled": true, "name": "X-User"},
		"json": map[string]any{"enabled": true, "secretKey": key}}); st != 200 {
		t.Fatalf("save: %d %v", st, out)
	}

	data, _ := auth.EncodeJSONAuth(key, &auth.JSONPayload{Username: "ext", Expires: time.Now().Add(time.Minute).UnixMilli(),
		Connections: map[string]auth.JSONConnection{"web": {Protocol: "ssh", Parameters: map[string]string{"hostname": "10.1.1.1", "password": "s3cret"}}}})
	c := noRedirect(t)
	resp, err := c.Get(srv.URL + "/api/v1/auth/json?data=" + url.QueryEscape(data))
	if err != nil || resp.StatusCode != 302 || resp.Header.Get("Location") != "/" {
		t.Fatalf("json login: %v %v", err, resp)
	}
	tc := &tclient{t: t, base: srv.URL, c: c}
	st, out := tc.do("GET", "/api/v1/connections", nil)
	items, _ := out["items"].([]any)
	if st != 200 || len(items) != 1 || items[0].(map[string]any)["name"] != "web" {
		t.Fatalf("json connections: %d %v", st, out)
	}
	// single use
	resp, _ = noRedirect(t).Get(srv.URL + "/api/v1/auth/json?data=" + url.QueryEscape(data))
	if !strings.HasPrefix(resp.Header.Get("Location"), "/login?error=") {
		t.Fatalf("replayed JSON accepted: %v", resp.Header)
	}

	header := func(user string) string {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/auth/header", nil)
		req.Header.Set("X-User", user)
		resp, err := noRedirect(t).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.Header.Get("Location")
	}
	if loc := header("carol"); loc != "/" {
		t.Fatalf("header login: %s", loc)
	}
	// an external identity never takes over a local account unless linking is enabled
	if loc := header("root"); loc != "/login?error=USER_CONFLICT" {
		t.Fatalf("local takeover: %s", loc)
	}
	if loc := header(""); loc != "/login?error=HEADER_MISSING" {
		t.Fatalf("missing header: %s", loc)
	}
}

func TestProvisionBindsExternalIDAndJSONFolderOwnership(t *testing.T) {
	a, srv, _ := newTestApp(t)
	ctx := t.Context()
	s := auth.DefaultSettings()
	if _, err := a.provision(ctx, &s, "oidc", &auth.Identity{Username: "ana", ExternalID: "https://idp|111"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.provision(ctx, &s, "oidc", &auth.Identity{Username: "ana", ExternalID: "https://idp|111"}); err != nil {
		t.Fatalf("same identity refused: %v", err)
	}
	// a new IdP account reusing the user name does not inherit the old one
	if _, err := a.provision(ctx, &s, "oidc", &auth.Identity{Username: "ana", ExternalID: "https://idp|222"}); err == nil {
		t.Fatal("different subject accepted")
	}

	// a user-created "JSON" folder is never adopted by JSON sign-in
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	if st, out := admin.do("POST", "/api/v1/connection-groups", map[string]any{"name": "JSON"}); st != 201 {
		t.Fatalf("folder: %d %v", st, out)
	}
	u, _ := a.Store.GetUserByName(ctx, "ana")
	if err := a.syncJSONConnections(ctx, u, nil); err == nil {
		t.Fatal("user-created JSON folder adopted")
	}
}
