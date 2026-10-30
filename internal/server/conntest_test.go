package server

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/jniltinho/go-guacamole/internal/guac"
)

// scriptedGuacd answers like guacd 1.6 does for each case observed against the lab targets; the hostname picks
// the script: ok (frames after login), early (RDP's empty sync before it connects), badpass (drawn, then error
// 0x0301), required (asks for the password), and anything else hangs up.
func scriptedGuacd(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	scripts := map[string][]guac.Instruction{
		"ok":       {guac.New("size", "0", "1024", "768"), guac.New("img", "1", "14", "0", "image/png", "0", "0"), guac.New("sync", "1", "0")},
		"early":    {guac.New("sync", "1", "0"), guac.New("size", "-1", "32", "32"), guac.New("sync", "2", "0")},
		"badpass":  {guac.New("size", "0", "1024", "768"), guac.New("rect", "0", "0", "0", "1024", "768"), guac.New("error", "Aborted. See logs.", "769")},
		"required": {guac.New("required", "password")},
		"tls":      {guac.New("sync", "1", "0"), guac.New("error", "SSL/TLS connection failed (untrusted/self-signed certificate?)", "519")},
	}
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
				_, _ = c.Write(guac.New("args", "VERSION_1_5_0", "hostname", "username", "password", "command").Bytes())
				var host string
				for {
					ins, err := rd.Read()
					if err != nil {
						return
					}
					if ins.Opcode == "connect" {
						host = ins.Args[1]
						if ins.Args[4] != "" { // a test must never run the connection's command
							host = "command"
						}
						break
					}
				}
				_, _ = c.Write(guac.New("ready", "$fake").Bytes())
				script, ok := scripts[host]
				if !ok {
					return
				}
				for _, ins := range script {
					_, _ = c.Write(ins.Bytes())
				}
				_, _ = rd.Read() // until the test disconnects
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestConnectionTest(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Cfg.Guacd.Addr = scriptedGuacd(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	mkUser(t, a.Store, "bob", "BobPass#12", false)
	admin := login(t, srv.URL, "root", "RootPass#1")
	try := func(c *tclient, host string, extra map[string]any) (int, map[string]any) {
		body := map[string]any{"name": "t", "protocol": "ssh",
			"parameters": map[string]any{"hostname": host, "username": "lab", "password": "secret"}}
		for k, v := range extra {
			body[k] = v
		}
		return c.do("POST", "/api/v1/connections/test", body)
	}
	for host, want := range map[string]struct {
		ok      bool
		status  float64
		code    string
		message string
	}{
		"ok":       {ok: true},
		"early":    {ok: true},
		"badpass":  {status: 0x0301},
		"required": {status: 0x0301, code: "CREDENTIALS_REQUIRED", message: "password"},
		"tls":      {status: 0x0207, message: "SSL/TLS connection failed (untrusted/self-signed certificate?)"},
		"gone":     {status: float64(guac.StatusUpstreamError)},
	} {
		st, out := try(admin, host, map[string]any{"parameters": map[string]any{"hostname": host, "username": "lab",
			"password": "secret", "command": "touch /tmp/pwned"}})
		got := struct {
			ok      bool
			status  float64
			code    string
			message string
		}{out["ok"] == true, num(out["status"]), str(out["code"]), str(out["message"])}
		if st != 200 || got != want {
			t.Errorf("%s: %d %v", host, st, out)
		}
	}

	// SSH without a password: guacd would ask in the terminal, so the login is not verified
	st, out := admin.do("POST", "/api/v1/connections/test", map[string]any{"name": "t", "protocol": "ssh",
		"parameters": map[string]any{"hostname": "ok", "username": "lab"}})
	if st != 200 || out["ok"] != true || out["code"] != "UNVERIFIED" {
		t.Fatalf("no password: %d %v", st, out)
	}

	// an encrypted key: without its passphrase guacd would ask in the terminal; a wrong one fails before connecting
	_, pk, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(pk, "", []byte("right"))
	if err != nil {
		t.Fatal(err)
	}
	key := string(pem.EncodeToMemory(block))
	// the same key under a cipher x/crypto does not read (guacd may): not verified, never "wrong passphrase"
	block.Bytes = bytes.Replace(block.Bytes, []byte("aes256-ctr"), []byte("aes128-ctr"), 1)
	odd := string(pem.EncodeToMemory(block))
	for name, c := range map[string]struct {
		key, pass string
		ok        bool
		code      any
	}{
		"no passphrase":  {key, "", true, "UNVERIFIED"},
		"wrong":          {key, "wrong", false, "KEY_PASSPHRASE"},
		"right":          {key, "right", true, nil},
		"unknown cipher": {odd, "right", true, "UNVERIFIED"},
	} {
		_, out := admin.do("POST", "/api/v1/connections/test", map[string]any{"name": "t", "protocol": "ssh",
			"parameters": map[string]any{"hostname": "ok", "username": "lab", "private-key": c.key, "passphrase": c.pass}})
		if out["ok"] != c.ok || out["code"] != c.code {
			t.Errorf("key %s: %v", name, out)
		}
	}

	// an edited connection uses its stored password when none is typed; bob may not test it
	_, created := admin.do("POST", "/api/v1/connections", map[string]any{"name": "srv", "protocol": "ssh",
		"parameters": map[string]any{"hostname": "ok", "username": "lab", "password": "secret"}})
	id := created["id"].(string)
	st, out = try(admin, "ok", map[string]any{"id": id, "parameters": map[string]any{"hostname": "ok", "username": "lab"}})
	if st != 200 || out["ok"] != true || out["code"] != nil {
		t.Fatalf("stored secret: %d %v", st, out)
	}
	user := login(t, srv.URL, "bob", "BobPass#12")
	if st, _ := try(user, "ok", map[string]any{"id": id}); st != 404 {
		t.Fatalf("bob tests a hidden connection: %d", st)
	}
	if st, _ := try(user, "ok", nil); st != 403 {
		t.Fatalf("bob tests a new connection: %d", st)
	}
}

// A disabled connection keeps its settings, is listed as disabled and cannot be launched.
func TestDisabledConnection(t *testing.T) {
	a, srv, _ := newTestApp(t)
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	_, created := admin.do("POST", "/api/v1/connections", map[string]any{"name": "srv", "protocol": "ssh", "disabled": true,
		"parameters": map[string]any{"hostname": "10.0.0.9"}})
	id := created["id"].(string)
	if created["disabled"] != true || created["status"] != "disabled" {
		t.Fatalf("create: %v", created)
	}
	if st, out := admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id}); st != 409 || out["code"] != "CONNECTION_DISABLED" {
		t.Fatalf("launch disabled: %d %v", st, out)
	}
	_, out := admin.do("PUT", "/api/v1/connections/"+id, map[string]any{"name": "srv", "protocol": "ssh", "disabled": false,
		"parameters": map[string]any{"hostname": "10.0.0.9"}})
	if out["disabled"] != false {
		t.Fatalf("enable: %v", out)
	}
	if st, out := admin.do("POST", "/api/v1/sessions", map[string]any{"connectionId": id}); st != 201 {
		t.Fatalf("launch enabled: %d %v", st, out)
	}
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
