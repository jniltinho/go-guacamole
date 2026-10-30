package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// Administrators rename the application; everyone (signed in or not) gets the name; the change is audited.
func TestBranding(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	ctx := context.Background()
	mkUser(t, a.Store, "root", "RootPass#1", true)
	mkUser(t, a.Store, "bob", "BobPass#1", false)
	admin := login(t, srv.URL, "root", "RootPass#1")
	bob := login(t, srv.URL, "bob", "BobPass#1")

	if st, _ := bob.do("PUT", "/api/v1/settings/branding", map[string]any{"title": "Mine"}); st != 403 {
		t.Fatalf("non-admin: %d", st)
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/branding", map[string]any{"title": strings.Repeat("x", 41)}); st != 422 {
		t.Fatalf("too long: %d", st)
	}
	if st, _ := admin.do("PUT", "/api/v1/settings/branding", map[string]any{"title": "bad\nname"}); st != 422 {
		t.Fatalf("control character: %d", st)
	}
	if st, out := admin.do("PUT", "/api/v1/settings/branding", map[string]any{"title": "  ACME Remote Access  "}); st != 200 || out["title"] != "ACME Remote Access" {
		t.Fatalf("save: %d %v", st, out)
	}
	if _, out := bob.do("GET", "/api/v1/me", nil); out["appTitle"] != "ACME Remote Access" {
		t.Fatalf("me: %v", out["appTitle"])
	}
	anon := &tclient{t: t, base: srv.URL, c: &http.Client{}}
	if _, out := anon.do("GET", "/api/v1/auth/methods", nil); out["appTitle"] != "ACME Remote Access" {
		t.Fatalf("sign-in page: %v", out["appTitle"])
	}
	ev, _, _ := a.Store.ListAudit(ctx, store.AuditFilter{Action: "settings.branding.update"}, store.Page{Limit: 5})
	if len(ev) != 1 || ev[0].ActorName != "root" || ev[0].Metadata["old"] != "" || ev[0].Metadata["new"] != "ACME Remote Access" {
		t.Fatalf("audit: %+v", ev)
	}
	// back to the default
	if st, out := admin.do("PUT", "/api/v1/settings/branding", map[string]any{"title": ""}); st != 200 || out["title"] != "" {
		t.Fatalf("reset: %d %v", st, out)
	}
	if _, out := bob.do("GET", "/api/v1/me", nil); out["appTitle"] != "" {
		t.Fatalf("me after reset: %v", out["appTitle"])
	}
	if _, out := anon.do("GET", "/api/v1/auth/methods", nil); out["appTitle"] != "" {
		t.Fatalf("sign-in page after reset: %v", out["appTitle"])
	}
}
