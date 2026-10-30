package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/store"
)

const cookieName = "gg_session"

type ctxKey string

const (
	keySession ctxKey = "session"
	keyUser    ctxKey = "user"
	keyEff     ctxKey = "effective"
)

// passwordVersion binds a session to the password it was created with: any
// password change (API, CLI or importer, any process) invalidates older sessions.
func passwordVersion(u *store.User) string { return strconv.FormatInt(u.PasswordChangedAt, 10) }

// sessionCurrent reports whether a session still matches the user's password version.
func sessionCurrent(s *store.Session, u *store.User) bool {
	return subtle.ConstantTimeCompare([]byte(s.SecurityStamp), []byte(passwordVersion(u))) == 1
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func sessionOf(c *echo.Context) *store.Session { return c.Get(string(keySession)).(*store.Session) }
func userOf(c *echo.Context) *store.User       { return c.Get(string(keyUser)).(*store.User) }
func effOf(c *echo.Context) *access.Effective  { return c.Get(string(keyEff)).(*access.Effective) }

func (a *App) clearCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: a.Cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
}

// loadSession validates the cookie and returns session and user.
func (a *App) loadSession(c *echo.Context) (*store.Session, *store.User, error) {
	ck, err := c.Cookie(cookieName)
	if err != nil || ck.Value == "" {
		return nil, nil, errUnauthorized
	}
	ctx := c.Request().Context()
	sess, err := a.Store.GetSession(ctx, hashToken(ck.Value))
	if err != nil {
		return nil, nil, errUnauthorized
	}
	now := time.Now()
	if now.UnixMilli() > sess.ExpiresAt || now.Add(-a.Cfg.App.SessionIdleTimeout).UnixMilli() > sess.LastSeen {
		_ = a.Store.DeleteSession(ctx, sess.ID)
		a.Enforcer.RevokeSession(sess.ID, guac.StatusSessionTimeout, "session expired")
		return nil, nil, errUnauthorized
	}
	u, err := a.Store.GetUser(ctx, sess.UserID)
	if err != nil {
		return nil, nil, errUnauthorized
	}
	if !sessionCurrent(sess, u) {
		_ = a.Store.DeleteSession(ctx, sess.ID)
		a.Enforcer.RevokeSession(sess.ID, guac.StatusSessionClosed, "password changed")
		return nil, nil, errUnauthorized
	}
	if err := auth.CheckRestrictions(u, now); err != nil {
		_ = a.Store.DeleteSession(ctx, sess.ID)
		a.Enforcer.RevokeSession(sess.ID, guac.StatusClientForbidden, "account restricted")
		return nil, nil, errUnauthorized
	}
	return sess, u, nil
}

// touch refreshes last_seen at most every 30 s per session.
func (a *App) touch(sessID string) {
	a.touchMu.Lock()
	last := a.touched[sessID]
	now := time.Now()
	if now.Sub(last) < 30*time.Second {
		a.touchMu.Unlock()
		return
	}
	a.touched[sessID] = now
	if len(a.touched) > 10000 {
		a.touched = map[string]time.Time{}
	}
	a.touchMu.Unlock()
	_ = a.Store.TouchSession(a.bg(), sessID, now.UnixMilli())
}

func (a *App) requireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		sess, u, err := a.loadSession(c)
		if err != nil {
			a.clearCookie(c)
			return err
		}
		eff, err := access.Resolve(c.Request().Context(), a.Store, u.ID)
		if err != nil {
			return err
		}
		c.Set(string(keySession), sess)
		c.Set(string(keyUser), u)
		c.Set(string(keyEff), eff)
		a.touch(sess.ID)
		return next(c)
	}
}

func (a *App) csrf(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}
		tok := c.Request().Header.Get("X-CSRF-Token")
		if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(sessionOf(c).CSRFToken)) != 1 {
			return errCSRF
		}
		return next(c)
	}
}

type loginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	NewPassword string `json:"newPassword"`
	TOTPCode    string `json:"totpCode"`
}

func passwordErr(err error) *APIError {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "PASSWORD_TOO_SHORT", Message: "password must have at least 8 characters", Field: "password"}
	case errors.Is(err, auth.ErrPasswordTooLong):
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "PASSWORD_TOO_LONG", Message: "password must have at most 72 bytes", Field: "password"}
	}
	return nil
}

// login is the password sign-in (local and LDAP users), followed by the
// optional second factors: TOTP (code in the same request) and Duo (redirect).
func (a *App) login(c *echo.Context) error {
	r := c.Request()
	if r.Header.Get("X-Requested-With") != "goguacamole" || !a.originAllowed(r) {
		return errCSRF
	}
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return errBadRequest
	}
	ip := c.RealIP()
	userKey := "u:" + req.Username
	if a.loginIP.Blocked(ip) || a.loginUser.Blocked(userKey) {
		return apiErr(http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many failed login attempts, try again later")
	}
	ctx := r.Context()
	s, err := a.authSettings(ctx)
	if err != nil {
		return err
	}
	u, res, err := a.passwordAuth(ctx, &s, req.Username, req.Password)
	if errors.Is(err, errExternalConflict) {
		res.OK = false
	} else if err != nil {
		return err
	}
	if !res.OK {
		a.loginIP.Hit(ip)
		a.loginUser.Hit(userKey)
		a.audit(c, nil, req.Username, "auth.login.failure", "user", "", req.Username, "failure", nil)
		return apiErr(http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
	}
	if err := auth.CheckRestrictions(u, time.Now()); err != nil {
		a.audit(c, &u.ID, u.Username, "auth.login.denied", "user", idStr(u.ID), u.Username, "denied", map[string]any{"reason": err.Error()})
		return apiErr(http.StatusForbidden, err.Error(), "login not allowed")
	}
	if s.TOTP.Enabled {
		if err := a.checkTOTP(c, &s, u, req.TOTPCode); err != nil {
			if ae, ok := err.(*APIError); ok && ae.Code == "TOTP_INVALID" {
				a.loginIP.Hit(ip)
				a.loginUser.Hit(userKey)
			}
			return err
		}
	}
	// password changes only apply to local accounts (directory passwords are managed there);
	// with Duo the new hash is stored only after Duo approves
	newHash := ""
	if u.AuthSource == store.SourceLocal && (u.MustChangePassword || res.MustReplace) {
		if req.NewPassword == "" {
			return apiErr(http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "a new password is required")
		}
		if req.NewPassword == req.Password {
			return &APIError{Status: http.StatusUnprocessableEntity, Code: "PASSWORD_REUSED", Message: "new password must differ", Field: "newPassword"}
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			if pe := passwordErr(err); pe != nil {
				pe.Field = "newPassword"
				return pe
			}
			return err
		}
		newHash = hash
		if !s.Duo.Enabled {
			if err := a.Store.SetPassword(ctx, u.ID, hash, store.SchemeBcrypt, "", false); err != nil {
				return err
			}
			a.audit(c, &u.ID, u.Username, "user.password.change", "user", idStr(u.ID), u.Username, "success", nil)
			if u, err = a.Store.GetUser(ctx, u.ID); err != nil {
				return err
			}
		}
	} else if res.NeedsUpgrade {
		if hash, err := auth.HashPassword(req.Password); err == nil {
			_ = a.Store.UpgradeHash(ctx, u.ID, hash)
		}
	}
	a.loginIP.Reset(ip)
	a.loginUser.Reset(userKey)
	if s.Duo.Enabled {
		return a.duoStart(c, &s, u, newHash)
	}
	method := "password"
	if u.AuthSource == store.SourceLDAP {
		method = "ldap"
	}
	sess, err := a.startSession(c, u, method)
	if err != nil {
		return err
	}
	eff, err := access.Resolve(ctx, a.Store, u.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, a.mePayload(c, u, sess, eff))
}

func (a *App) logout(c *echo.Context) error {
	sess, u := sessionOf(c), userOf(c)
	_ = a.Store.DeleteSession(c.Request().Context(), sess.ID)
	a.Enforcer.RevokeSession(sess.ID, guac.StatusSessionClosed, "logged out")
	a.clearCookie(c)
	a.audit(c, &u.ID, u.Username, "auth.logout", "user", idStr(u.ID), u.Username, "success", nil)
	return c.NoContent(http.StatusNoContent)
}

type mePayload struct {
	User        *store.User        `json:"user"`
	Permissions []string           `json:"permissions"`
	IsAdmin     bool               `json:"isAdmin"`
	CSRFToken   string             `json:"csrfToken"`
	Preferences *store.Preferences `json:"preferences"`
	ExpiresAt   int64              `json:"expiresAt"`
	Groups      []string           `json:"groups"`
	Build       BuildInfo          `json:"build"`
	Recording   bool               `json:"recording"`
	// the job dashboard (/asynqmon) is reachable: administrator and queue up
	JobsDashboard bool   `json:"jobsDashboard"`
	AppTitle      string `json:"appTitle"` // "" for the default name
}

func (a *App) mePayload(c *echo.Context, u *store.User, sess *store.Session, eff *access.Effective) mePayload {
	ctx := c.Request().Context()
	prefs, _ := a.Store.GetPreferences(ctx, u.ID)
	gs, _ := a.Store.GroupsOfUser(ctx, u.ID)
	names := []string{}
	for _, g := range gs {
		names = append(names, g.Name)
	}
	perms := eff.SystemList()
	if perms == nil {
		perms = []string{}
	}
	return mePayload{User: u, Permissions: perms, IsAdmin: eff.IsAdmin(), CSRFToken: sess.CSRFToken, Preferences: prefs,
		ExpiresAt: sess.ExpiresAt, Groups: names, Build: a.Build, Recording: a.Cfg.Recording.Enabled,
		JobsDashboard: eff.IsAdmin() && a.queueHealth().State == jobs.Up, AppTitle: a.appTitle(ctx)}
}

func (a *App) me(c *echo.Context) error {
	return c.JSON(http.StatusOK, a.mePayload(c, userOf(c), sessionOf(c), effOf(c)))
}

func (a *App) changeOwnPassword(c *echo.Context) error {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.Bind(&req); err != nil {
		return errBadRequest
	}
	u := userOf(c)
	if !auth.Verify(u, req.CurrentPassword).OK {
		return apiErr(http.StatusForbidden, "INVALID_CREDENTIALS", "current password is wrong")
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		if pe := passwordErr(err); pe != nil {
			pe.Field = "newPassword"
			return pe
		}
		return err
	}
	ctx := c.Request().Context()
	if err := a.Store.SetPassword(ctx, u.ID, hash, store.SchemeBcrypt, "", false); err != nil {
		return err
	}
	keep := sessionOf(c).ID
	if nu, err := a.Store.GetUser(ctx, u.ID); err == nil {
		_ = a.Store.SetSessionStamp(ctx, keep, passwordVersion(nu)) // the current session survives the change
	}
	ids, _ := a.Store.DeleteUserSessions(ctx, u.ID, keep)
	for _, id := range ids {
		a.Enforcer.RevokeSession(id, guac.StatusSessionClosed, "password changed")
	}
	a.audit(c, &u.ID, u.Username, "user.password.change", "user", idStr(u.ID), u.Username, "success", nil)
	return c.NoContent(http.StatusNoContent)
}

var themeID = regexp.MustCompile(`^[a-z0-9-]{0,32}$`)

func (a *App) getPreferences(c *echo.Context) error {
	p, err := a.Store.GetPreferences(c.Request().Context(), userOf(c).ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, p)
}

func (a *App) putPreferences(c *echo.Context) error {
	var p store.Preferences
	if err := c.Bind(&p); err != nil {
		return errBadRequest
	}
	if !themeID.MatchString(p.Theme) {
		return validation("theme", "unknown theme")
	}
	if p.Locale != "" && p.Locale != "en" && p.Locale != "pt-BR" {
		return validation("locale", "unsupported locale")
	}
	switch p.Effects {
	case "", "full", "reduced", "off":
	default:
		return validation("effects", "full, reduced or off")
	}
	if !validTerminalScheme(p.TerminalScheme) {
		return validation("terminalScheme", "unknown terminal color scheme")
	}
	if err := a.Store.PutPreferences(c.Request().Context(), userOf(c).ID, &p); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, p)
}
