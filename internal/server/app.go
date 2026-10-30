// Package server wires the Echo v5 HTTP application: middleware, REST API,
// WebSocket tunnel endpoint and the embedded SPA.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

// App is the HTTP application.
type App struct {
	Cfg      *config.Config
	Store    *store.Store
	Registry *tunnel.Registry
	Log      *slog.Logger
	Echo     *echo.Echo
	Prober   *Prober
	Enforcer *Enforcer
	dist     fs.FS
	// Instance identifies this process among replicas (history rows it serves, heartbeats, leases)
	Instance string
	// Jobs is the background job queue (asynq on Redis; disabled without REDIS_ADDR)
	Jobs    *jobs.Queue
	mon     monitor
	remotes remoteCache

	loginIP   *auth.Limiter
	loginUser *auth.Limiter
	launches  *auth.Limiter

	// Build describes the running binary (shown in Settings → About).
	Build BuildInfo

	allowedOrigins map[string]bool
	touchMu        sync.Mutex
	touched        map[string]time.Time

	// sign-in state shared by the external methods (single replica, like the tunnel registry)
	authMu     sync.Mutex
	duoPending map[string]duoPending
	oidc       *oidcClient
	saml       *samlClient

	// recording policy runtime state
	recAvail     recAvailability
	recStateMu   sync.Mutex
	recExcMu     sync.Mutex
	activated    sync.Map // history ids of sessions that really started (not "not_started")
	purgeMu      sync.Mutex
	previews     map[string]previewEntry
	captureGrace time.Duration // how long guacd gets to create recording files
}

// BuildInfo is version metadata injected at build time.
type BuildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
}

// APIError is the uniform JSON error.
type APIError struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Field   string         `json:"field,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func apiErr(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

var (
	errNotFound     = apiErr(http.StatusNotFound, "NOT_FOUND", "resource not found")
	errForbidden    = apiErr(http.StatusForbidden, "FORBIDDEN", "permission denied")
	errUnauthorized = apiErr(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	errCSRF         = apiErr(http.StatusForbidden, "CSRF_INVALID", "missing or invalid CSRF protection")
	errBadRequest   = apiErr(http.StatusBadRequest, "BAD_REQUEST", "invalid request")
)

func validation(field, msg string) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED", Message: msg, Field: field}
}

// New builds the application.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, dist fs.FS) *App {
	a := &App{
		Cfg:          cfg,
		Store:        st,
		Registry:     tunnel.NewRegistry(),
		Log:          log,
		loginIP:      auth.NewLimiter(10, 5*time.Minute),
		loginUser:    auth.NewLimiter(10, 5*time.Minute),
		launches:     auth.NewLimiter(30, time.Minute),
		touched:      map[string]time.Time{},
		duoPending:   map[string]duoPending{},
		previews:     map[string]previewEntry{},
		captureGrace: 10 * time.Second,
		dist:         dist,
		Instance:     instanceID(),
	}
	a.allowedOrigins = map[string]bool{}
	for _, o := range append([]string{cfg.App.BaseURL}, cfg.App.AllowedOrigins...) {
		if n := normalizeOrigin(o); n != "" {
			a.allowedOrigins[n] = true
		}
	}
	a.Prober = NewProber(st, cfg.App.ProbeInterval, log)
	a.Enforcer = NewEnforcer(a)

	e := echo.NewWithConfig(echo.Config{
		Logger:           log,
		HTTPErrorHandler: a.errorHandler,
		IPExtractor:      ipExtractor(cfg.App.TrustedProxies),
	})
	a.Echo = e
	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())
	e.Use(a.requestLogger())
	e.Use(a.securityHeaders)
	e.Use(middleware.BodyLimit(2 << 20))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{Skipper: func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().URL.Path, "/api/v1/tunnel")
	}}))
	a.routes()
	a.asynqmonRoutes()
	return a
}

func ipExtractor(trusted []string) echo.IPExtractor {
	if len(trusted) == 0 {
		return echo.ExtractIPDirect()
	}
	opts := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
	for _, p := range trusted {
		if !strings.Contains(p, "/") {
			if strings.Contains(p, ":") {
				p += "/128"
			} else {
				p += "/32"
			}
		}
		if _, n, err := net.ParseCIDR(p); err == nil {
			opts = append(opts, echo.TrustIPRange(n))
		}
	}
	return echo.ExtractIPFromXFFHeader(opts...)
}

func (a *App) requestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogMethod: true, LogURIPath: true, LogLatency: true, LogRemoteIP: true, LogRequestID: true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			lvl := slog.LevelInfo
			if v.Status >= 500 {
				lvl = slog.LevelError
			} else if v.URIPath == "/health" || v.URIPath == "/ready" {
				lvl = slog.LevelDebug
			}
			a.Log.LogAttrs(context.Background(), lvl, "request",
				slog.String("request_id", v.RequestID), slog.String("method", v.Method), slog.String("path", v.URIPath),
				slog.Int("status", v.Status), slog.Duration("latency", v.Latency), slog.String("ip", v.RemoteIP))
			return nil
		},
	})
}

func (a *App) securityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		h := c.Response().Header()
		host := c.Request().Host
		h.Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' blob:; worker-src 'self' blob:; "+
			"connect-src 'self' ws://%s wss://%s; frame-ancestors 'none'; base-uri 'self'; form-action 'self'", host, host))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if a.isHTTPS(c) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		return next(c)
	}
}

func (a *App) isHTTPS(c *echo.Context) bool {
	if c.Request().TLS != nil {
		return true
	}
	return a.Cfg.App.CookieSecure && strings.EqualFold(c.Request().Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) errorHandler(c *echo.Context, err error) {
	if resp, uErr := echo.UnwrapResponse(c.Response()); uErr == nil && resp.Committed {
		return
	}
	var ae *APIError
	var sc echo.HTTPStatusCoder
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, store.ErrNotFound):
		ae = errNotFound
	case errors.Is(err, store.ErrConflict):
		ae = apiErr(http.StatusConflict, "NAME_CONFLICT", "an object with this name already exists")
	case errors.As(err, &sc):
		code := sc.StatusCode()
		ae = apiErr(code, strings.ToUpper(strings.ReplaceAll(http.StatusText(code), " ", "_")), http.StatusText(code))
		if code == http.StatusRequestEntityTooLarge {
			ae.Code = "PAYLOAD_TOO_LARGE"
		}
	default:
		a.Log.Error("unhandled error", "err", err, "path", c.Request().URL.Path)
		ae = apiErr(http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
	_ = c.JSON(ae.Status, ae)
}

// normalizeOrigin returns scheme://host[:port] in lower case with default ports removed.
func normalizeOrigin(o string) string {
	u, err := url.Parse(strings.TrimSpace(o))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host
}

// originAllowed reports whether the request's Origin (scheme, host and port) is
// the server's own origin or one of the configured origins (APP_BASE_URL,
// APP_ALLOWED_ORIGINS).
func (a *App) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	n := normalizeOrigin(origin)
	if n == "" {
		return false
	}
	if a.allowedOrigins[n] {
		return true
	}
	scheme := "http"
	if r.TLS != nil || (len(a.Cfg.App.TrustedProxies) > 0 && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		scheme = "https"
	}
	return n == normalizeOrigin(scheme+"://"+r.Host)
}

func (a *App) routes() {
	e := a.Echo
	e.GET("/health", a.health)
	e.GET("/ready", a.ready)

	api := e.Group("/api/v1")
	api.POST("/auth/login", a.login)
	api.GET("/auth/methods", a.authMethods)
	api.GET("/auth/header", a.headerLogin)
	api.GET("/auth/json", a.jsonLogin)
	api.POST("/auth/json", a.jsonLogin)
	api.GET("/auth/oidc/login", a.oidcLogin)
	api.GET("/auth/oidc/callback", a.oidcCallback)
	api.GET("/auth/saml/login", a.samlLogin)
	api.GET("/auth/saml/metadata", a.samlMetadata)
	api.POST("/auth/saml/acs", a.samlACS)
	api.GET("/auth/duo/callback", a.duoCallback)

	authed := api.Group("", a.requireSession, a.csrf)
	authed.POST("/auth/logout", a.logout)
	authed.GET("/me", a.me)
	authed.PUT("/me/password", a.changeOwnPassword)
	authed.GET("/me/preferences", a.getPreferences)
	authed.PUT("/me/preferences", a.putPreferences)

	authed.GET("/dashboard", a.dashboard)
	authed.GET("/terminal-schemes", a.listTerminalSchemes)
	authed.GET("/connections", a.listConnections)
	authed.POST("/connections", a.createConnection)
	authed.POST("/connections/test", a.testConnection)
	authed.GET("/connections/:id", a.getConnection)
	authed.PUT("/connections/:id", a.updateConnection)
	authed.DELETE("/connections/:id", a.deleteConnection)
	authed.GET("/connection-groups", a.listConnectionGroups)
	authed.GET("/connection-groups/tree", a.connectionTree)
	authed.POST("/connection-groups", a.createConnectionGroup)
	authed.PUT("/connection-groups/:id", a.updateConnectionGroup)
	authed.DELETE("/connection-groups/:id", a.deleteConnectionGroup)
	authed.GET("/favorites", a.listFavorites)
	authed.POST("/favorites", a.addFavorite)
	authed.DELETE("/favorites/:connectionId", a.removeFavorite)

	authed.POST("/sessions", a.launch)
	authed.GET("/sessions", a.listSessions)
	authed.DELETE("/sessions/:id", a.killSession)
	authed.GET("/tunnel", a.tunnel)

	authed.GET("/users", a.listUsers)
	authed.POST("/users", a.createUser)
	authed.GET("/users/:id", a.getUser)
	authed.PUT("/users/:id", a.updateUser)
	authed.DELETE("/users/:id", a.deleteUser)
	authed.PUT("/users/:id/password", a.resetPassword)
	authed.DELETE("/users/:id/totp", a.resetTOTP)
	authed.GET("/settings/auth", a.getAuthSettings)
	authed.PUT("/settings/auth", a.putAuthSettings)
	authed.POST("/settings/auth/test/:method", a.testAuthSettings)
	authed.GET("/user-groups", a.listUserGroups)
	authed.POST("/user-groups", a.createUserGroup)
	authed.GET("/user-groups/:id", a.getUserGroup)
	authed.PUT("/user-groups/:id", a.updateUserGroup)
	authed.DELETE("/user-groups/:id", a.deleteUserGroup)
	authed.PUT("/user-groups/:id/members", a.setMembers)
	authed.GET("/permissions", a.getPermissions)
	authed.PUT("/permissions", a.putPermissions)
	authed.GET("/history", a.listHistory)
	authed.GET("/history/stats", a.accessStats)
	authed.GET("/history/:id/recording", a.getRecording)
	authed.GET("/history/:id/typescript", a.getTypescript)
	authed.PUT("/history/:id/hold", a.putHold)
	authed.DELETE("/history/:id/hold", a.deleteHold)
	authed.POST("/history/:id/storage/retry", a.retryStorage)
	authed.PUT("/connections/:id/recording", a.putConnectionRecording)
	authed.GET("/settings/recording/storage", a.getStorageSettings)
	authed.PUT("/settings/recording/storage", a.putStorageSettings)
	authed.POST("/settings/recording/storage/test", a.testStorage)
	authed.POST("/settings/recording/storage/backfill", a.backfillStorage)
	authed.GET("/settings/branding", a.getBranding)
	authed.PUT("/settings/branding", a.putBranding)
	authed.GET("/settings/recording", a.getRecordingSettings)
	authed.PUT("/settings/recording", a.putRecordingSettings)
	authed.PUT("/settings/recording/exception", a.putRecordingException)
	authed.POST("/recordings/purge", a.runPurgeNow)
	authed.POST("/recordings/usage", a.refreshUsage)
	authed.GET("/audit", a.listAudit)

	api.Any("/*", func(c *echo.Context) error { return errNotFound })
	a.mountSPA()
}

// Start runs background jobs until ctx is cancelled.
func (a *App) Start(ctx context.Context) {
	a.beat(ctx)
	inst, err := a.Store.InstallationID(ctx)
	if err != nil {
		a.Log.Error("installation id", "err", err)
	}
	objstore.Setup(a.Log)
	a.Jobs = jobs.New(a.Cfg.Redis, a.Cfg.Jobs, inst, a.Log)
	a.Jobs.Schedule(a.Store, a.Instance, a.upkeepJobs())
	a.Jobs.Register(uploadTask, a.handleUpload)
	a.Jobs.Start(ctx)
	go a.Jobs.RunScheduler(ctx)
	a.recordingStartup(ctx)
	go a.Prober.Run(ctx)
	go a.Enforcer.Run(ctx)
	go a.janitor(ctx)
}

// Shutdown closes all tunnels.
func (a *App) Shutdown() {
	a.Registry.CloseAll("server shutting down")
	if a.Jobs != nil {
		a.Jobs.Shutdown()
	}
}

// janitor does the work that belongs to this instance; shared upkeep runs as periodic jobs (upkeep.go).
func (a *App) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		a.beat(ctx)
		a.finishAbandoned(ctx)
		a.resetAvailability()
	}
}

func (a *App) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()
	out := map[string]string{"db": "up", "guacd": "up"}
	status := http.StatusOK
	if err := a.Store.Ping(ctx); err != nil {
		out["db"], status = "down", http.StatusServiceUnavailable
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	if conn, err := d.DialContext(ctx, "tcp", a.Cfg.Guacd.Addr); err != nil {
		out["guacd"], status = "down", http.StatusServiceUnavailable
	} else {
		conn.Close()
	}
	// the job queue is optional: reported, never a reason to be unready
	out["queue"] = string(a.queueHealth().State)
	return c.JSON(status, out)
}
