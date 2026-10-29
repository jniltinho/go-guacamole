package server

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/auth"
)

// authSettingsView is what the admin page reads: secrets are write-only and
// reported in secretsSet.
type authSettingsView struct {
	auth.Settings
	SecretsSet     map[string]bool `json:"secretsSet"`
	PublicURL      string          `json:"publicUrl"`
	TrustedProxies []string        `json:"trustedProxies"`
	SecureCookies  bool            `json:"secureCookies"`
	LoadError      string          `json:"loadError,omitempty"`
}

func (a *App) getAuthSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	// unreadable settings (e.g. APP_SECRET_KEY changed) show defaults so they can be saved again
	s, err := a.authSettings(c.Request().Context())
	v := authSettingsView{SecretsSet: map[string]bool{}, PublicURL: a.publicURL(c),
		TrustedProxies: a.Cfg.App.TrustedProxies, SecureCookies: a.Cfg.SecureCookies()}
	if err != nil {
		v.LoadError = "stored settings cannot be read; saving replaces them"
	}
	for name, p := range s.SecretFields() {
		v.SecretsSet[name] = *p != ""
		*p = ""
	}
	v.Settings = s
	return c.JSON(http.StatusOK, v)
}

// mergeAuthSettings validates in and fills omitted secrets from the stored settings.
// clear lists secrets the administrator removed explicitly.
func (a *App) mergeAuthSettings(ctx context.Context, in *auth.Settings, clear []string) error {
	cur, _ := a.authSettings(ctx) // unreadable → defaults: the admin re-enters secrets
	old := cur.SecretFields()
	for name, p := range in.SecretFields() {
		if *p == "" && !containsStr(clear, name) {
			*p = *old[name]
		}
	}
	if in.TOTP.Issuer = strings.TrimSpace(in.TOTP.Issuer); in.TOTP.Issuer == "" {
		in.TOTP.Issuer = "GoGuacamole"
	}
	if l := in.LDAP; l.Enabled {
		if !strings.HasPrefix(l.URL, "ldap://") && !strings.HasPrefix(l.URL, "ldaps://") {
			return validation("ldap.url", "use ldap:// or ldaps://")
		}
		if l.UserBaseDN == "" || l.UsernameAttribute == "" {
			return validation("ldap.userBaseDn", "user base DN and username attribute are required")
		}
	}
	if o := in.OIDC; o.Enabled && (o.Issuer == "" || o.ClientID == "" || o.UsernameClaim == "") {
		return validation("oidc.issuer", "issuer, client ID and username claim are required")
	}
	if s := in.SAML; s.Enabled && (s.IDPMetadataURL == "" || s.Certificate == "" || s.PrivateKey == "") {
		return validation("saml.idpMetadataUrl", "IdP metadata URL, certificate and private key are required")
	}
	if h := in.Header; h.Enabled {
		if h.Name == "" {
			return validation("header.name", "header name is required")
		}
		if len(a.Cfg.App.TrustedProxies) == 0 {
			return validation("header.enabled", "set APP_TRUSTED_PROXIES first: only the proxy may set the user header")
		}
	}
	if j := in.JSON; j.Enabled {
		if k, err := hex.DecodeString(j.SecretKey); err != nil || len(k) != 16 {
			return validation("json.secretKey", "secret key must be 32 hexadecimal digits")
		}
	}
	if d := in.Duo; d.Enabled && (d.ClientID == "" || d.ClientSecret == "" || d.APIHostname == "") {
		return validation("duo.clientId", "client ID, client secret and API hostname are required")
	}
	return nil
}

type authSettingsInput struct {
	auth.Settings
	Clear []string `json:"clear"`
}

func (a *App) putAuthSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	in := authSettingsInput{Settings: auth.DefaultSettings()}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	if err := a.mergeAuthSettings(ctx, &in.Settings, in.Clear); err != nil {
		return err
	}
	if err := a.Store.PutSetting(ctx, settingAuth, &in.Settings); err != nil {
		return err
	}
	a.auditBy(c, "settings.auth.update", "settings", settingAuth, "authentication", map[string]any{
		"totp": in.TOTP.Enabled, "ldap": in.LDAP.Enabled, "oidc": in.OIDC.Enabled, "saml": in.SAML.Enabled,
		"header": in.Header.Enabled, "json": in.JSON.Enabled, "duo": in.Duo.Enabled})
	return a.getAuthSettings(c)
}

// testAuthSettings checks one method with the settings being edited (not yet saved).
func (a *App) testAuthSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var req struct {
		authSettingsInput
		Username string `json:"username"`
	}
	req.Settings = auth.DefaultSettings()
	if err := c.Bind(&req); err != nil {
		return errBadRequest
	}
	s := req.Settings
	ctx := c.Request().Context()
	// fill stored secrets; validation of "enabled" is skipped so disabled methods can be tested
	cur, _ := a.authSettings(ctx) // unreadable → defaults: the admin re-enters secrets
	old := cur.SecretFields()
	for name, p := range s.SecretFields() {
		if *p == "" && !containsStr(req.Clear, name) {
			*p = *old[name]
		}
	}
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	msg, err := a.testMethod(tctx, c, c.Param("method"), &s, req.Username)
	out := map[string]any{"ok": err == nil, "message": msg}
	if err != nil {
		out["message"] = err.Error()
	}
	a.auditBy(c, "settings.auth.test", "settings", settingAuth, c.Param("method"), map[string]any{"ok": err == nil})
	return c.JSON(http.StatusOK, out)
}

func (a *App) testMethod(ctx context.Context, c *echo.Context, method string, s *auth.Settings, username string) (string, error) {
	switch method {
	case "ldap":
		return auth.LDAPTest(s.LDAP, username)
	case "oidc":
		a.authMu.Lock()
		a.oidc = nil // force discovery with the edited settings
		a.authMu.Unlock()
		oc, err := a.oidcFor(ctx, c, s.OIDC)
		if err != nil {
			return "", err
		}
		return "discovery OK; authorization endpoint " + oc.conf.Endpoint.AuthURL + "; redirect URI " + oc.conf.RedirectURL, nil
	case "saml":
		a.authMu.Lock()
		a.saml = nil
		a.authMu.Unlock()
		sp, err := a.samlFor(ctx, c, s.SAML)
		if err != nil {
			return "", err
		}
		return "IdP metadata OK (" + sp.ServiceProvider.IDPMetadata.EntityID + "); ACS " + sp.ServiceProvider.AcsURL.String() +
			"; SP metadata " + sp.ServiceProvider.MetadataURL.String(), nil
	case "header":
		r := c.Request()
		if !a.fromTrustedProxy(r) {
			return "", errors.New("this request did not come from APP_TRUSTED_PROXIES (" + r.RemoteAddr + ")")
		}
		if v := r.Header.Get(s.Header.Name); v != "" {
			return "proxy sent " + s.Header.Name + ": " + v, nil
		}
		return "", errors.New("trusted proxy, but header " + s.Header.Name + " is missing on this request")
	case "json":
		if username == "" {
			username = "json-test"
		}
		data, err := auth.EncodeJSONAuth(s.JSON.SecretKey, &auth.JSONPayload{Username: username, Expires: time.Now().Add(5 * time.Minute).UnixMilli()})
		if err != nil {
			return "", err
		}
		return a.publicURL(c) + "/api/v1/auth/json?data=" + url.QueryEscape(data), nil
	case "duo":
		client, err := a.duoClient(c, s.Duo)
		if err != nil {
			return "", err
		}
		if _, err := client.HealthCheck(); err != nil {
			return "", err
		}
		return "Duo health check OK", nil
	}
	return "", errNotFound
}
