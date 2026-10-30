package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/duosecurity/duo_universal_golang/duouniversal"
	"github.com/labstack/echo/v5"
	"rsc.io/qr"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/store"
)

const settingAuth = "auth"

// authSettings returns the sign-in settings saved in the admin UI (defaults when
// unset). Callers fail closed on error: unreadable settings must not silently
// turn second factors off.
func (a *App) authSettings(ctx context.Context) (auth.Settings, error) {
	s := auth.DefaultSettings()
	if err := a.Store.GetSetting(ctx, settingAuth, &s); err != nil {
		a.Log.Error("load auth settings", "err", err)
		return s, err
	}
	return s, nil
}

// publicURL is the external base URL (APP_BASE_URL, else derived from the request).
func (a *App) publicURL(c *echo.Context) string {
	if a.Cfg.App.BaseURL != "" {
		return strings.TrimRight(a.Cfg.App.BaseURL, "/")
	}
	return c.Scheme() + "://" + c.Request().Host
}

// startSession creates the session and its cookie after every check passed.
func (a *App) startSession(c *echo.Context, u *store.User, method string) (*store.Session, error) {
	token := store.RandomHex(32)
	now := time.Now()
	sess := &store.Session{
		ID: hashToken(token), UserID: u.ID, CSRFToken: store.RandomHex(32), SecurityStamp: passwordVersion(u),
		CreatedAt: now.UnixMilli(), LastSeen: now.UnixMilli(), ExpiresAt: now.Add(a.Cfg.App.SessionTTL).UnixMilli(),
		ClientIP: c.RealIP(), UserAgent: c.Request().UserAgent(),
	}
	if err := a.Store.CreateSession(c.Request().Context(), sess); err != nil {
		return nil, err
	}
	c.SetCookie(&http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.Cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode, Expires: now.Add(a.Cfg.App.SessionTTL)})
	a.audit(c, &u.ID, u.Username, "auth.login.success", "user", idStr(u.ID), u.Username, "success", map[string]any{"method": method})
	return sess, nil
}

var errExternalConflict = errors.New("a user with this name exists with another sign-in method")

// provision returns the local user for an external identity, creating it on
// first sign-in. Groups reported by the identity provider grant the local user
// groups with the same name (like the Java LDAP extension); they replace the
// memberships of provisioned users and are only added to linked local users.
func (a *App) provision(ctx context.Context, s *auth.Settings, source string, id *auth.Identity) (*store.User, error) {
	if id.Username == "" || len(id.Username) > 128 {
		return nil, errors.New("invalid user name from identity provider")
	}
	u, err := a.Store.GetUserByName(ctx, id.Username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		u = &store.User{Username: id.Username, PasswordHash: "!", AuthSource: source}
		if id.FullName != "" {
			u.FullName = &id.FullName
		}
		if id.Email != "" {
			u.Email = &id.Email
		}
		if err := a.Store.CreateUser(ctx, u); err != nil {
			return nil, err
		}
		a.audit(nil, nil, "system", "user.provision", "user", idStr(u.ID), u.Username, "success", map[string]any{"source": source})
	case err != nil:
		return nil, err
	case u.AuthSource != source && !(s.LinkExternalUsers && u.AuthSource == store.SourceLocal):
		return nil, errExternalConflict
	}
	if id.ExternalID != "" {
		ok, err := a.Store.BindExternalID(ctx, u.ID, id.ExternalID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errExternalConflict
		}
	}
	if id.Groups == nil {
		return u, nil
	}
	var gids []int64
	for _, name := range id.Groups {
		if g, err := a.Store.GetUserGroupByName(ctx, name); err == nil {
			gids = append(gids, g.ID)
		}
	}
	if u.AuthSource == source {
		err = a.Store.SetUserGroups(ctx, u.ID, gids)
	} else {
		for _, g := range gids {
			if err = a.Store.AddGroupMember(ctx, g, u.ID); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return a.Store.GetUser(ctx, u.ID)
}

// passwordAuth checks a user name and password against the local database and,
// for users that are not local, the LDAP directory.
func (a *App) passwordAuth(ctx context.Context, s *auth.Settings, username, password string) (*store.User, auth.VerifyResult, error) {
	var u *store.User
	if username != "" {
		u, _ = a.Store.GetUserByName(ctx, username)
	}
	if u == nil || u.AuthSource == store.SourceLocal {
		if res := auth.Verify(u, password); res.OK || !s.LDAP.Enabled {
			return u, res, nil
		}
	}
	if !s.LDAP.Enabled || (u != nil && u.AuthSource != store.SourceLDAP && !s.LinkExternalUsers) {
		return u, auth.VerifyResult{}, nil
	}
	id, err := auth.LDAPAuthenticate(ctx, s.LDAP, username, password)
	if err != nil {
		if !errors.Is(err, auth.ErrLDAPInvalid) {
			a.Log.Warn("ldap authentication failed", "err", err)
		}
		return u, auth.VerifyResult{}, nil
	}
	pu, err := a.provision(ctx, s, store.SourceLDAP, id)
	if err != nil {
		return nil, auth.VerifyResult{}, err
	}
	return pu, auth.VerifyResult{OK: true}, nil
}

// totpEnroll is the error that carries the secret to enroll (first sign-in).
func (a *App) totpEnroll(s *auth.Settings, u *store.User, secret string, status int, code, msg string) *APIError {
	uri := auth.TOTPURI(s.TOTP.Issuer, u.Username, secret)
	data := map[string]any{"secret": secret, "uri": uri}
	if q, err := qr.Encode(uri, qr.M); err == nil {
		data["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(q.PNG())
	}
	return &APIError{Status: status, Code: code, Message: msg, Data: data}
}

// checkTOTP enforces the second factor; an unconfirmed user gets a secret to enroll.
func (a *App) checkTOTP(c *echo.Context, s *auth.Settings, u *store.User, code string) error {
	ctx := c.Request().Context()
	secret, confirmed, _, err := a.Store.TOTP(ctx, u.ID)
	if err != nil {
		return err
	}
	if !confirmed && secret == "" {
		secret = auth.NewTOTPSecret()
		if err := a.Store.SetTOTPSecret(ctx, u.ID, secret); err != nil {
			return err
		}
	}
	if code == "" {
		if !confirmed {
			return a.totpEnroll(s, u, secret, http.StatusForbidden, "TOTP_ENROLL", "scan the code and enter the 6-digit code")
		}
		return apiErr(http.StatusForbidden, "TOTP_REQUIRED", "enter the code from your authenticator app")
	}
	step, ok := auth.VerifyTOTP(secret, code, time.Now())
	if ok {
		if ok, err = a.Store.UseTOTPStep(ctx, u.ID, step); err != nil {
			return err
		}
	}
	if !ok {
		a.audit(c, &u.ID, u.Username, "auth.totp.failure", "user", idStr(u.ID), u.Username, "failure", nil)
		if !confirmed {
			return a.totpEnroll(s, u, secret, http.StatusUnauthorized, "TOTP_INVALID", "invalid code")
		}
		return apiErr(http.StatusUnauthorized, "TOTP_INVALID", "invalid code")
	}
	if !confirmed {
		a.audit(c, &u.ID, u.Username, "auth.totp.enroll", "user", idStr(u.ID), u.Username, "success", nil)
	}
	return nil
}

// resetTOTP removes a user's TOTP secret: the next sign-in enrolls again.
func (a *App) resetTOTP(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	u, err := a.Store.GetUser(c.Request().Context(), id)
	if err != nil {
		return err
	}
	if err := a.Store.SetTOTPSecret(c.Request().Context(), id, ""); err != nil {
		return err
	}
	a.auditBy(c, "user.totp.reset", "user", idStr(id), u.Username, nil)
	return c.NoContent(http.StatusNoContent)
}

// ---- Duo Universal Prompt (second factor after password sign-in) ----

type duoPending struct {
	userID   int64
	username string
	version  string // password version when Duo started
	newHash  string // forced password change, applied only after Duo approves
	expires  time.Time
}

const duoCookie = "gg_duo"

func (a *App) duoClient(c *echo.Context, s auth.DuoSettings) (*duouniversal.Client, error) {
	return duouniversal.NewClient(s.ClientID, s.ClientSecret, s.APIHostname, a.publicURL(c)+"/api/v1/auth/duo/callback")
}

// duoStart answers the password step with the Duo URL the browser must open.
// The state is also kept in a cookie so the callback only completes in the
// browser that started it (no login CSRF with someone else's callback URL).
func (a *App) duoStart(c *echo.Context, s *auth.Settings, u *store.User, newHash string) error {
	client, err := a.duoClient(c, s.Duo)
	if err != nil {
		return err
	}
	state, err := client.GenerateState()
	if err != nil {
		return err
	}
	redirect, err := client.CreateAuthURL(u.Username, state)
	if err != nil {
		return err
	}
	a.authMu.Lock()
	now := time.Now()
	for k, p := range a.duoPending {
		if now.After(p.expires) {
			delete(a.duoPending, k)
		}
	}
	a.duoPending[state] = duoPending{userID: u.ID, username: u.Username, version: passwordVersion(u), newHash: newHash, expires: now.Add(5 * time.Minute)}
	a.authMu.Unlock()
	c.SetCookie(&http.Cookie{Name: duoCookie, Value: state, Path: "/api/v1/auth/duo", MaxAge: 300, HttpOnly: true,
		Secure: a.Cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	return &APIError{Status: http.StatusForbidden, Code: "DUO_REQUIRED", Message: "continue with Duo", Data: map[string]any{"redirect": redirect}}
}

func (a *App) duoCallback(c *echo.Context) error {
	state, code := c.QueryParam("state"), c.QueryParam("duo_code")
	ck, ckErr := c.Cookie(duoCookie)
	c.SetCookie(&http.Cookie{Name: duoCookie, Value: "", Path: "/api/v1/auth/duo", MaxAge: -1, HttpOnly: true,
		Secure: a.Cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	if ckErr != nil || ck.Value != state {
		return a.ssoFail(c, "DUO_EXPIRED")
	}
	a.authMu.Lock()
	p, ok := a.duoPending[state]
	delete(a.duoPending, state)
	a.authMu.Unlock()
	if !ok || state == "" || time.Now().After(p.expires) {
		return a.ssoFail(c, "DUO_EXPIRED")
	}
	s, err := a.authSettings(c.Request().Context())
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	client, err := a.duoClient(c, s.Duo)
	if err != nil {
		return err
	}
	tok, err := client.ExchangeAuthorizationCodeFor2faResult(code, p.username)
	if err != nil || tok.AuthResult.Result != "allow" {
		a.audit(c, &p.userID, p.username, "auth.duo.failure", "user", idStr(p.userID), p.username, "failure", nil)
		return a.ssoFail(c, "DUO_DENIED")
	}
	ctx := c.Request().Context()
	u, err := a.Store.GetUser(ctx, p.userID)
	if err != nil || passwordVersion(u) != p.version {
		return a.ssoFail(c, "INVALID_CREDENTIALS")
	}
	if p.newHash != "" {
		if err := a.Store.SetPassword(ctx, u.ID, p.newHash, store.SchemeBcrypt, "", false); err != nil {
			return err
		}
		a.audit(c, &u.ID, u.Username, "user.password.change", "user", idStr(u.ID), u.Username, "success", nil)
		if u, err = a.Store.GetUser(ctx, u.ID); err != nil {
			return err
		}
	}
	return a.ssoComplete(c, u, "password+duo")
}

// ---- redirect-based sign-in helpers (OIDC, SAML, header, JSON, Duo) ----

func (a *App) ssoFail(c *echo.Context, code string) error {
	return c.Redirect(http.StatusFound, "/login?error="+url.QueryEscape(code))
}

// ssoFinish provisions an external identity and signs it in.
func (a *App) ssoFinish(c *echo.Context, s *auth.Settings, source string, id *auth.Identity) error {
	u, err := a.provision(c.Request().Context(), s, source, id)
	if err != nil {
		a.audit(c, nil, id.Username, "auth.login.failure", "user", "", id.Username, "failure", map[string]any{"method": source, "reason": err.Error()})
		if errors.Is(err, errExternalConflict) {
			return a.ssoFail(c, "USER_CONFLICT")
		}
		return a.ssoFail(c, "SSO_FAILED")
	}
	return a.ssoComplete(c, u, source)
}

func (a *App) ssoComplete(c *echo.Context, u *store.User, method string) error {
	if err := auth.CheckRestrictions(u, time.Now()); err != nil {
		a.audit(c, &u.ID, u.Username, "auth.login.denied", "user", idStr(u.ID), u.Username, "denied", map[string]any{"reason": err.Error()})
		return a.ssoFail(c, err.Error())
	}
	if _, err := a.startSession(c, u, method); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/")
}

// fromTrustedProxy reports whether the TCP peer is one of APP_TRUSTED_PROXIES.
func (a *App) fromTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil {
		return false
	}
	for _, p := range a.Cfg.App.TrustedProxies {
		if _, n, err := net.ParseCIDR(p); err == nil && n.Contains(ip) {
			return true
		}
		if pip := net.ParseIP(p); pip != nil && pip.Equal(ip) {
			return true
		}
	}
	return false
}

// headerLogin signs in the user named by the reverse proxy header.
func (a *App) headerLogin(c *echo.Context) error {
	s, err := a.authSettings(c.Request().Context())
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	if !s.Header.Enabled {
		return errNotFound
	}
	r := c.Request()
	name := strings.TrimSpace(r.Header.Get(s.Header.Name))
	if !a.fromTrustedProxy(r) || name == "" {
		return a.ssoFail(c, "HEADER_MISSING")
	}
	id := &auth.Identity{Username: name}
	if s.Header.GroupsHeader != "" {
		id.Groups = splitList(r.Header.Get(s.Header.GroupsHeader))
	}
	return a.ssoFinish(c, &s, store.SourceHeader, id)
}

func splitList(v string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// authMethods tells the login page which buttons to show (public).
func (a *App) authMethods(c *echo.Context) error {
	s, err := a.authSettings(c.Request().Context())
	if err != nil {
		return err
	}
	type btn struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	// the release number only (no commit or build date): shown discreetly on the sign-in page
	out := map[string]any{"header": s.Header.Enabled, "sso": []btn{}, "version": a.Build.Version,
		"appTitle": a.appTitle(c.Request().Context())}
	var sso []btn
	if s.OIDC.Enabled {
		sso = append(sso, btn{s.OIDC.Label, "/api/v1/auth/oidc/login"})
	}
	if s.SAML.Enabled {
		sso = append(sso, btn{s.SAML.Label, "/api/v1/auth/saml/login"})
	}
	if sso != nil {
		out["sso"] = sso
	}
	return c.JSON(http.StatusOK, out)
}
