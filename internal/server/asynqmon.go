package server

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/jniltinho/asynqmon"
	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/jobs"
)

// The job dashboard (jniltinho/asynqmon, the asynqmon UI used by AD Manager) at /asynqmon, for administrators only,
// while the job queue is up (openspec add-async-jobs-object-storage, design D12).

const asynqmonRoot = "/asynqmon"

type monitor struct {
	once sync.Once
	h    *asynqmon.HTTPHandler
}

func (a *App) asynqmonRoutes() {
	h := a.requireAdminPage(a.serveAsynqmon)
	a.Echo.Any(asynqmonRoot, h)
	a.Echo.Any(asynqmonRoot+"/*", h)
}

// requireAdminPage protects a page opened directly in the browser: without a session it redirects to sign-in (and
// back), without ADMINISTER it answers 403; state-changing requests must come from this origin and are audited.
func (a *App) requireAdminPage(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		_, u, err := a.loadSession(c)
		if err != nil {
			if r.Method != http.MethodGet {
				return errUnauthorized
			}
			return c.Redirect(http.StatusFound, "/login?next="+url.QueryEscape(asynqmonRoot))
		}
		eff, err := access.Resolve(r.Context(), a.Store, u.ID)
		if err != nil {
			return err
		}
		if !eff.IsAdmin() {
			return errForbidden
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !a.originAllowed(r) {
				return errForbidden
			}
			c.Set(string(keyUser), u)
			defer a.auditBy(c, "jobs."+strings.ToLower(r.Method), "jobs", "", strings.TrimPrefix(r.URL.Path, asynqmonRoot), nil)
		}
		return next(c)
	}
}

func (a *App) serveAsynqmon(c *echo.Context) error {
	if a.queueHealth().State != jobs.Up {
		return apiErr(http.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "the job queue is not running")
	}
	a.mon.once.Do(func() {
		a.mon.h = asynqmon.New(asynqmon.Options{RootPath: asynqmonRoot, RedisConnOpt: a.Jobs.RedisOpt(),
			Theme: func() (string, bool) { return "classic-blue", false }}) // the XP look, like the default UI theme
	})
	// the asynqmon SPA uses inline scripts and Google fonts: a CSP for these paths only
	c.Response().Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
			"font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data:; frame-ancestors 'none'")
	a.mon.h.ServeHTTP(c.Response(), c.Request())
	return nil
}
