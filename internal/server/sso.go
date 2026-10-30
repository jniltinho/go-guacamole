package server

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/crewjam/saml/samlsp"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// ---- OpenID Connect (authorization code + PKCE) ----

type oidcClient struct {
	key      string // settings it was built from
	conf     oauth2.Config
	verifier *oidc.IDTokenVerifier
}

const oidcCookie = "gg_oidc"

// oidcFor returns the cached client for the current settings (discovery runs once per change).
func (a *App) oidcFor(ctx context.Context, c *echo.Context, s auth.OIDCSettings) (*oidcClient, error) {
	redirect := a.publicURL(c) + "/api/v1/auth/oidc/callback"
	key := strings.Join([]string{s.Issuer, s.ClientID, s.ClientSecret, s.Scopes, redirect}, "\x00")
	a.authMu.Lock()
	cached := a.oidc
	a.authMu.Unlock()
	if cached != nil && cached.key == key {
		return cached, nil
	}
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(dctx, s.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	scopes := strings.Fields(s.Scopes)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID}
	}
	oc := &oidcClient{key: key, verifier: p.Verifier(&oidc.Config{ClientID: s.ClientID}),
		conf: oauth2.Config{ClientID: s.ClientID, ClientSecret: s.ClientSecret, Endpoint: p.Endpoint(), RedirectURL: redirect, Scopes: scopes}}
	a.authMu.Lock()
	a.oidc = oc
	a.authMu.Unlock()
	return oc, nil
}

func (a *App) oidcLogin(c *echo.Context) error {
	ctx := c.Request().Context()
	s, err := a.authSettings(ctx)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	if !s.OIDC.Enabled {
		return errNotFound
	}
	oc, err := a.oidcFor(ctx, c, s.OIDC)
	if err != nil {
		a.Log.Error("oidc", "err", err)
		return a.ssoFail(c, "SSO_FAILED")
	}
	state, nonce, verifier := store.RandomHex(16), store.RandomHex(16), oauth2.GenerateVerifier()
	c.SetCookie(&http.Cookie{Name: oidcCookie, Value: state + "." + nonce + "." + verifier, Path: "/api/v1/auth/oidc",
		MaxAge: 600, HttpOnly: true, Secure: a.Cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	return c.Redirect(http.StatusFound, oc.conf.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)))
}

func (a *App) oidcCallback(c *echo.Context) error {
	ctx := c.Request().Context()
	s, err := a.authSettings(ctx)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	if !s.OIDC.Enabled {
		return errNotFound
	}
	ck, err := c.Cookie(oidcCookie)
	c.SetCookie(&http.Cookie{Name: oidcCookie, Value: "", Path: "/api/v1/auth/oidc", MaxAge: -1, HttpOnly: true,
		Secure: a.Cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	parts := []string{}
	if err == nil {
		parts = strings.Split(ck.Value, ".")
	}
	if len(parts) != 3 || c.QueryParam("state") != parts[0] || c.QueryParam("code") == "" {
		return a.ssoFail(c, "SSO_FAILED")
	}
	oc, err := a.oidcFor(ctx, c, s.OIDC)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	id, err := oidcIdentity(ctx, oc, s.OIDC, c.QueryParam("code"), parts[1], parts[2])
	if err != nil {
		a.Log.Warn("oidc callback", "err", err)
		return a.ssoFail(c, "SSO_FAILED")
	}
	return a.ssoFinish(c, &s, store.SourceOIDC, id)
}

func oidcIdentity(ctx context.Context, oc *oidcClient, s auth.OIDCSettings, code, nonce, verifier string) (*auth.Identity, error) {
	tok, err := oc.conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := oc.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("id token: %w", err)
	}
	if idt.Nonce != nonce {
		return nil, errors.New("id token nonce mismatch")
	}
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		return nil, err
	}
	id := &auth.Identity{ExternalID: idt.Issuer + "|" + idt.Subject}
	id.Username, _ = claims[s.UsernameClaim].(string)
	id.FullName, _ = claims["name"].(string)
	id.Email, _ = claims["email"].(string)
	if s.GroupsClaim != "" {
		id.Groups = []string{}
		if list, ok := claims[s.GroupsClaim].([]any); ok {
			for _, g := range list {
				if v, ok := g.(string); ok {
					id.Groups = append(id.Groups, v)
				}
			}
		}
	}
	if id.Username == "" {
		return nil, fmt.Errorf("claim %q missing in id token", s.UsernameClaim)
	}
	return id, nil
}

// ---- SAML 2.0 (service provider) ----

type samlClient struct {
	key string
	sp  *samlsp.Middleware
}

func (a *App) samlFor(ctx context.Context, c *echo.Context, s auth.SAMLSettings) (*samlsp.Middleware, error) {
	base := a.publicURL(c)
	raw, _ := json.Marshal(s)
	key := base + "\x00" + string(raw)
	a.authMu.Lock()
	cached := a.saml
	a.authMu.Unlock()
	if cached != nil && cached.key == key {
		return cached.sp, nil
	}
	pair, err := tls.X509KeyPair([]byte(s.Certificate), []byte(s.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("SP certificate/key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	rsaKey, ok := pair.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("SP private key must be RSA")
	}
	mdURL, err := url.Parse(s.IDPMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("IdP metadata URL: %w", err)
	}
	fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	md, err := samlsp.FetchMetadata(fctx, http.DefaultClient, *mdURL)
	if err != nil {
		return nil, fmt.Errorf("IdP metadata: %w", err)
	}
	root, _ := url.Parse(base + "/api/v1/auth/") // trailing slash: samlsp resolves saml/{metadata,acs} against it
	sameSite := http.SameSiteLaxMode
	if a.Cfg.SecureCookies() {
		sameSite = http.SameSiteNoneMode // the IdP POSTs the response cross-site to the ACS
	}
	sp, err := samlsp.New(samlsp.Options{EntityID: s.EntityID, URL: *root, Key: rsaKey, Certificate: leaf,
		IDPMetadata: md, CookieSameSite: sameSite, CookieName: "gg_saml", DefaultRedirectURI: "/api/v1/auth/saml/login"})
	if err != nil {
		return nil, err
	}
	a.authMu.Lock()
	a.saml = &samlClient{key: key, sp: sp}
	a.authMu.Unlock()
	return sp, nil
}

func (a *App) samlEnabled(c *echo.Context) (*auth.Settings, *samlsp.Middleware, error) {
	ctx := c.Request().Context()
	s, err := a.authSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !s.SAML.Enabled {
		return nil, nil, errNotFound
	}
	sp, err := a.samlFor(ctx, c, s.SAML)
	if err != nil {
		a.Log.Error("saml", "err", err)
		return nil, nil, err
	}
	return &s, sp, nil
}

func (a *App) samlMetadata(c *echo.Context) error {
	_, sp, err := a.samlEnabled(c)
	if err != nil {
		return err
	}
	sp.ServeMetadata(c.Response(), c.Request())
	return nil
}

func (a *App) samlACS(c *echo.Context) error {
	_, sp, err := a.samlEnabled(c)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	sp.ServeACS(c.Response(), c.Request())
	return nil
}

// samlLogin starts the SAML flow, and after the ACS redirect back turns the
// SAML session into a GoGuacamole session.
func (a *App) samlLogin(c *echo.Context) error {
	s, sp, err := a.samlEnabled(c)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	w, r := c.Response(), c.Request()
	sess, err := sp.Session.GetSession(r)
	if errors.Is(err, samlsp.ErrNoSession) {
		sp.HandleStartAuthFlow(w, r)
		return nil
	}
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	_ = sp.Session.DeleteSession(w, r) // single use: our own session takes over
	claims, ok := sess.(samlsp.JWTSessionClaims)
	if !ok {
		return a.ssoFail(c, "SSO_FAILED")
	}
	id := &auth.Identity{Username: claims.Subject, ExternalID: "saml|" + claims.Subject}
	attrs := claims.GetAttributes()
	if s.SAML.UsernameAttribute != "" {
		id.Username = attrs.Get(s.SAML.UsernameAttribute)
	}
	if s.SAML.GroupsAttribute != "" {
		id.Groups = append([]string{}, attrs[s.SAML.GroupsAttribute]...)
	}
	return a.ssoFinish(c, s, store.SourceSAML, id)
}

// ---- encrypted JSON (guacamole-auth-json) ----

// jsonLogin accepts ?data= (or a form field) signed and encrypted with the
// shared key; its connections are kept in the folder JSON/<user>, granted
// only to that user and replaced on every sign-in.
func (a *App) jsonLogin(c *echo.Context) error {
	ctx := c.Request().Context()
	s, err := a.authSettings(ctx)
	if err != nil {
		return a.ssoFail(c, "SSO_FAILED")
	}
	if !s.JSON.Enabled {
		return errNotFound
	}
	data := c.QueryParam("data")
	if data == "" {
		data = c.FormValue("data")
	}
	p, sig, err := auth.DecodeJSONAuth(s.JSON.SecretKey, data, time.Now())
	if err != nil {
		a.audit(c, nil, "", "auth.login.failure", "user", "", "", "failure", map[string]any{"method": "json"})
		return a.ssoFail(c, "INVALID_CREDENTIALS")
	}
	// single use, persisted: survives restarts and works across replicas
	if fresh, err := a.Store.UseToken(ctx, "json:"+sig[:64], p.Expires); err != nil || !fresh {
		return a.ssoFail(c, "INVALID_CREDENTIALS")
	}
	u, err := a.provision(ctx, &s, store.SourceJSON, &auth.Identity{Username: p.Username})
	if err != nil {
		return a.ssoFail(c, "USER_CONFLICT")
	}
	if err := a.syncJSONConnections(ctx, u, p.Connections); err != nil {
		a.Log.Warn("json auth connections", "err", err, "user", u.Username)
		return a.ssoFail(c, "SSO_FAILED")
	}
	return a.ssoComplete(c, u, store.SourceJSON)
}

func (a *App) syncJSONConnections(ctx context.Context, u *store.User, conns map[string]auth.JSONConnection) error {
	folder, err := a.folder(ctx, nil, "JSON")
	if err != nil {
		return err
	}
	if folder, err = a.folder(ctx, &folder, u.Username); err != nil {
		return err
	}
	all, err := a.Store.ListConnections(ctx)
	if err != nil {
		return err
	}
	// only connections this sign-in created are reused: anything else found in the
	// managed folder (e.g. planted by another user) is replaced
	existing := map[string]*store.Connection{}
	for _, c := range all {
		if c.ParentID == nil || *c.ParentID != folder {
			continue
		}
		if c.CreatedBy == nil || *c.CreatedBy != u.ID {
			if err := a.deleteConnectionsKeepingRetention(ctx, []*store.Connection{c}, func(tx *store.Store) error {
				return tx.DeleteConnection(ctx, c.ID)
			}); err != nil {
				return err
			}
			continue
		}
		existing[c.Name] = c
	}
	parent := idStr(folder)
	for name, jc := range conns {
		params := map[string]*string{}
		for k, v := range jc.Parameters {
			params[k] = &v
		}
		in := connectionInput{Name: name, Protocol: jc.Protocol, ParentID: &parent, Parameters: params}
		conn := existing[name]
		if conn == nil {
			conn = &store.Connection{CreatedBy: &u.ID}
		}
		delete(existing, name)
		if err := in.apply(conn, nil); err != nil {
			return fmt.Errorf("connection %q: %w", name, err)
		}
		if conn.ID == 0 {
			err = a.Store.CreateConnection(ctx, conn)
		} else {
			err = a.Store.UpdateConnection(ctx, conn)
		}
		if err != nil {
			return err
		}
		if err := a.Store.GrantConnection(ctx, store.SubjectUser, u.ID, conn.ID); err != nil {
			return err
		}
	}
	for _, stale := range existing {
		if err := a.deleteConnectionsKeepingRetention(ctx, []*store.Connection{stale}, func(tx *store.Store) error {
			return tx.DeleteConnection(ctx, stale.ID)
		}); err != nil {
			return err
		}
	}
	return a.Store.BumpAccessEpoch(ctx)
}

// folder returns the id of the managed connection folder name under parent,
// creating it. Folders created by a user (created_by set) are never adopted.
func (a *App) folder(ctx context.Context, parent *int64, name string) (int64, error) {
	groups, err := a.Store.ListConnectionGroups(ctx)
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		if g.Name == name && ((g.ParentID == nil && parent == nil) || (g.ParentID != nil && parent != nil && *g.ParentID == *parent)) {
			if g.CreatedBy != nil {
				return 0, fmt.Errorf("connection folder %q was created by a user, not by JSON sign-in", name)
			}
			return g.ID, nil
		}
	}
	g := &store.ConnectionGroup{ParentID: parent, Name: name}
	if err := a.Store.CreateConnectionGroup(ctx, g); err != nil {
		return 0, err
	}
	return g.ID, nil
}
