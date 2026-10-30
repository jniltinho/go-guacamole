package server

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/store"
)

func rawGet(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func TestJobDashboardAccess(t *testing.T) {
	a, srv, _ := newTestApp(t)
	a.Start(t.Context())
	mkUser(t, a.Store, "root", "RootPass#1", true)
	aud := mkUser(t, a.Store, "aud", "AudPass#1", false)
	_ = a.Store.GrantSystem(context.Background(), store.SubjectUser, aud.ID, store.PermAudit)

	// anonymous: to sign-in, then back to the dashboard
	jar, _ := cookiejar.New(nil)
	anon := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res := rawGet(t, anon, srv.URL+"/asynqmon"); res.StatusCode != http.StatusFound ||
		!strings.HasPrefix(res.Header.Get("Location"), "/login?next=%2Fasynqmon") {
		t.Fatalf("anonymous: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// auditor: forbidden
	if res := rawGet(t, login(t, srv.URL, "aud", "AudPass#1").c, srv.URL+"/asynqmon"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("auditor: %d", res.StatusCode)
	}
	admin := login(t, srv.URL, "root", "RootPass#1")
	// no Redis: unavailable, and not offered in the menu
	if res := rawGet(t, admin.c, srv.URL+"/asynqmon"); res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("admin without queue: %d", res.StatusCode)
	}
	if _, me := admin.do("GET", "/api/v1/me", nil); me["jobsDashboard"] != false {
		t.Fatalf("menu link without queue: %v", me["jobsDashboard"])
	}
	// state-changing request from another origin
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/asynqmon/api/queues/goguacamole:default:pause", nil)
	req.Header.Set("Origin", "https://evil.example")
	res, err := admin.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin action: %d", res.StatusCode)
	}
}

// With Redis the administrator gets the dashboard and the menu link.
func TestJobDashboardWithRedis(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 6})
	_ = rdb.FlushDB(context.Background()).Err()
	_ = rdb.Close()
	a, srv, _ := newTestApp(t)
	a.Cfg.Redis = config.RedisConfig{Addr: addr, DB: 6}
	a.Cfg.Jobs = config.JobsConfig{Worker: true, Concurrency: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	defer a.Shutdown()
	for i := 0; i < 100 && a.queueHealth().State != jobs.Up; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	mkUser(t, a.Store, "root", "RootPass#1", true)
	admin := login(t, srv.URL, "root", "RootPass#1")
	res, err := admin.c.Get(srv.URL + "/asynqmon")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Security-Policy"), "'unsafe-inline'") {
		t.Fatalf("dashboard: %d %q", res.StatusCode, res.Header.Get("Content-Security-Policy"))
	}
	if _, me := admin.do("GET", "/api/v1/me", nil); me["jobsDashboard"] != true {
		t.Fatalf("menu link with queue: %v", me["jobsDashboard"])
	}
}
