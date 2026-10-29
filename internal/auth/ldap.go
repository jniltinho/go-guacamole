package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ErrLDAPInvalid means the user does not exist in the directory or the password is wrong.
var ErrLDAPInvalid = errors.New("invalid LDAP credentials")

func ldapDial(s LDAPSettings) (*ldap.Conn, error) {
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") {
		return nil, fmt.Errorf("invalid LDAP URL %q", s.URL)
	}
	tlsCfg := &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: s.InsecureSkipVerify, MinVersion: tls.VersionTLS12} //nolint:gosec // admin opt-in
	c, err := ldap.DialURL(s.URL, ldap.DialWithTLSConfig(tlsCfg), ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	c.SetTimeout(10 * time.Second)
	if s.StartTLS && u.Scheme == "ldap" {
		if err := c.StartTLS(tlsCfg); err != nil {
			c.Close()
			return nil, fmt.Errorf("StartTLS: %w", err)
		}
	}
	return c, nil
}

func ldapServiceBind(c *ldap.Conn, s LDAPSettings) error {
	if s.BindDN == "" {
		// anonymous search; also drops a previous user bind on this connection
		if err := c.UnauthenticatedBind(""); err != nil {
			return fmt.Errorf("anonymous bind: %w", err)
		}
		return nil
	}
	if err := c.Bind(s.BindDN, s.BindPassword); err != nil {
		return fmt.Errorf("service bind: %w", err)
	}
	return nil
}

func ldapFindUser(c *ldap.Conn, s LDAPSettings, username string) (*ldap.Entry, error) {
	filter := fmt.Sprintf("(&%s(%s=%s))", s.UserFilter, s.UsernameAttribute, ldap.EscapeFilter(username))
	res, err := c.Search(ldap.NewSearchRequest(s.UserBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false,
		filter, []string{s.UsernameAttribute, "cn", "displayName", "mail"}, nil))
	if err != nil {
		return nil, fmt.Errorf("user search: %w", err)
	}
	if len(res.Entries) != 1 {
		return nil, ErrLDAPInvalid
	}
	return res.Entries[0], nil
}

func ldapGroups(c *ldap.Conn, s LDAPSettings, userDN string) ([]string, error) {
	if s.GroupBaseDN == "" {
		return nil, nil
	}
	filter := fmt.Sprintf("(&%s(%s=%s))", s.GroupFilter, s.MemberAttribute, ldap.EscapeFilter(userDN))
	res, err := c.Search(ldap.NewSearchRequest(s.GroupBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 10, false,
		filter, []string{s.GroupNameAttribute}, nil))
	if err != nil {
		return nil, fmt.Errorf("group search: %w", err)
	}
	out := []string{}
	for _, e := range res.Entries {
		if v := e.GetAttributeValue(s.GroupNameAttribute); v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// LDAPAuthenticate finds the user with the service account, binds as the user
// to check the password and returns the identity with its directory groups.
func LDAPAuthenticate(_ context.Context, s LDAPSettings, username, password string) (*Identity, error) {
	if username == "" || password == "" {
		return nil, ErrLDAPInvalid // an empty password would be an unauthenticated bind
	}
	c, err := ldapDial(s)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := ldapServiceBind(c, s); err != nil {
		return nil, err
	}
	e, err := ldapFindUser(c, s, username)
	if err != nil {
		return nil, err
	}
	if err := c.Bind(e.DN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return nil, ErrLDAPInvalid
		}
		return nil, fmt.Errorf("user bind: %w", err)
	}
	// group membership is read with the service account (users often cannot search)
	if err := ldapServiceBind(c, s); err != nil {
		return nil, err
	}
	groups, err := ldapGroups(c, s, e.DN)
	if err != nil {
		return nil, err
	}
	id := &Identity{Username: e.GetAttributeValue(s.UsernameAttribute), Email: e.GetAttributeValue("mail"), Groups: groups}
	if id.Username == "" {
		id.Username = username
	}
	if id.FullName = e.GetAttributeValue("displayName"); id.FullName == "" {
		id.FullName = e.GetAttributeValue("cn")
	}
	return id, nil
}

// LDAPTest connects with the service account and, when username is given,
// looks the user up and lists its groups (admin "Test" button; no password check).
func LDAPTest(s LDAPSettings, username string) (string, error) {
	c, err := ldapDial(s)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if err := ldapServiceBind(c, s); err != nil {
		return "", err
	}
	if username == "" {
		return "connected and bound as " + orAnonymous(s.BindDN), nil
	}
	e, err := ldapFindUser(c, s, username)
	if err != nil {
		return "", fmt.Errorf("user %q: %w", username, err)
	}
	groups, err := ldapGroups(c, s, e.DN)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("found %s; groups: %v", e.DN, groups), nil
}

func orAnonymous(dn string) string {
	if dn == "" {
		return "anonymous"
	}
	return dn
}
