package server

import (
	"strings"
	"testing"
)

// Creating a user needs the basics (a valid username and a password of 8+ characters); the optional fields are
// checked when given.
func TestCreateUserValidation(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	ok := map[string]any{"username": "carla", "password": "Carla-Pass#2026"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range ok {
			m[kk] = vv
		}
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	for name, body := range map[string]map[string]any{
		"no username":       with("username", nil),
		"blank username":    with("username", "   "),
		"bad username":      with("username", "carla mendes"),
		"no password":       with("password", nil),
		"short password":    with("password", "short"),
		"long password":     with("password", strings.Repeat("x", 73)),
		"bad e-mail":        with("email", "carla@"),
		"e-mail no domain":  with("email", "carla@localhost"),
		"half window":       with("accessWindowStart", "08:00"),
		"bad date":          with("validFrom", "2026-13-01"),
		"bad time zone":     with("timezone", "Mars/Olympus"),
		"long organization": with("organization", strings.Repeat("x", 257)),
		"until before from": merge(ok, map[string]any{"validFrom": "2026-12-01", "validUntil": "2026-11-01"}),
	} {
		if st, out := admin.do("POST", "/api/v1/users", body); st != 422 {
			t.Errorf("%s: %d %v", name, st, out)
		}
	}
	full := merge(ok, map[string]any{"fullName": "Carla Mendes", "email": "carla@example.com", "validFrom": "2026-11-01",
		"validUntil": "2027-01-31", "accessWindowStart": "08:00", "accessWindowEnd": "18:00", "timezone": "America/Sao_Paulo",
		"organization": " ACME ", "organizationalRole": "NOC"})
	if st, out := admin.do("POST", "/api/v1/users", full); st != 201 || out["organization"] != "ACME" || out["organizationalRole"] != "NOC" {
		t.Fatalf("valid user: %d %v", st, out)
	}
	if st, _ := admin.do("POST", "/api/v1/users", ok); st != 409 {
		t.Fatalf("duplicate username: %d", st)
	}
}

func merge(a, b map[string]any) map[string]any {
	m := map[string]any{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}
