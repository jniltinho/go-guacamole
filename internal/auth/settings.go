package auth

// Settings configures sign-in methods. It is edited in the admin UI
// (Settings → Authentication) and stored sealed in the database.
type Settings struct {
	// LinkExternalUsers lets an external identity (LDAP, OIDC, SAML, header, JSON)
	// sign in as an existing local user with the same name (Java client behaviour).
	LinkExternalUsers bool           `json:"linkExternalUsers"`
	TOTP              TOTPSettings   `json:"totp"`
	LDAP              LDAPSettings   `json:"ldap"`
	OIDC              OIDCSettings   `json:"oidc"`
	SAML              SAMLSettings   `json:"saml"`
	Header            HeaderSettings `json:"header"`
	JSON              JSONSettings   `json:"json"`
	Duo               DuoSettings    `json:"duo"`
}

// TOTPSettings requires a TOTP code after password sign-in (local and LDAP users).
type TOTPSettings struct {
	Enabled bool   `json:"enabled"`
	Issuer  string `json:"issuer"`
}

// LDAPSettings authenticates users against a directory (OpenLDAP, Active Directory).
// Directory groups grant the permissions of local user groups with the same name.
type LDAPSettings struct {
	Enabled            bool   `json:"enabled"`
	URL                string `json:"url"` // ldap://host:389 or ldaps://host:636
	StartTLS           bool   `json:"startTls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	BindDN             string `json:"bindDn"`
	BindPassword       string `json:"bindPassword"`
	UserBaseDN         string `json:"userBaseDn"`
	UsernameAttribute  string `json:"usernameAttribute"`
	UserFilter         string `json:"userFilter"`
	GroupBaseDN        string `json:"groupBaseDn"`
	GroupFilter        string `json:"groupFilter"`
	MemberAttribute    string `json:"memberAttribute"`
	GroupNameAttribute string `json:"groupNameAttribute"`
}

// OIDCSettings delegates sign-in to an OpenID Connect provider (Keycloak, Authentik, Entra ID…).
type OIDCSettings struct {
	Enabled       bool   `json:"enabled"`
	Label         string `json:"label"`
	Issuer        string `json:"issuer"`
	ClientID      string `json:"clientId"`
	ClientSecret  string `json:"clientSecret"`
	Scopes        string `json:"scopes"` // space separated
	UsernameClaim string `json:"usernameClaim"`
	GroupsClaim   string `json:"groupsClaim"`
}

// SAMLSettings delegates sign-in to a SAML 2.0 identity provider (ADFS, Entra ID, Keycloak…).
type SAMLSettings struct {
	Enabled           bool   `json:"enabled"`
	Label             string `json:"label"`
	IDPMetadataURL    string `json:"idpMetadataUrl"`
	EntityID          string `json:"entityId"`          // empty = metadata URL
	Certificate       string `json:"certificate"`       // SP certificate (PEM)
	PrivateKey        string `json:"privateKey"`        // SP RSA key (PEM)
	UsernameAttribute string `json:"usernameAttribute"` // empty = NameID
	GroupsAttribute   string `json:"groupsAttribute"`
}

// HeaderSettings trusts a user name set by an authenticating reverse proxy
// (only for requests whose peer address is in APP_TRUSTED_PROXIES).
type HeaderSettings struct {
	Enabled      bool   `json:"enabled"`
	Name         string `json:"name"`
	GroupsHeader string `json:"groupsHeader"`
}

// JSONSettings accepts signed and encrypted JSON (guacamole-auth-json format).
type JSONSettings struct {
	Enabled   bool   `json:"enabled"`
	SecretKey string `json:"secretKey"` // 32 hex digits (128-bit key)
}

// DuoSettings requires Duo Universal Prompt after password sign-in.
type DuoSettings struct {
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	APIHostname  string `json:"apiHostname"`
}

// DefaultSettings are used until an administrator saves the page.
func DefaultSettings() Settings {
	return Settings{
		TOTP: TOTPSettings{Issuer: "GoGuacamole"},
		LDAP: LDAPSettings{UsernameAttribute: "uid", UserFilter: "(objectClass=*)",
			GroupFilter: "(|(objectClass=groupOfNames)(objectClass=group))", MemberAttribute: "member", GroupNameAttribute: "cn"},
		OIDC:   OIDCSettings{Label: "Single sign-on", Scopes: "openid profile email", UsernameClaim: "preferred_username", GroupsClaim: "groups"},
		SAML:   SAMLSettings{Label: "SAML"},
		Header: HeaderSettings{Name: "REMOTE_USER"},
	}
}

// SecretFields lists the write-only fields: the API never returns them and an
// empty value on save keeps the stored one.
func (s *Settings) SecretFields() map[string]*string {
	return map[string]*string{
		"ldap.bindPassword": &s.LDAP.BindPassword, "oidc.clientSecret": &s.OIDC.ClientSecret,
		"saml.privateKey": &s.SAML.PrivateKey, "json.secretKey": &s.JSON.SecretKey, "duo.clientSecret": &s.Duo.ClientSecret,
	}
}

// Identity is a user asserted by a sign-in method. Groups nil means the method
// does not report groups (memberships are left alone).
type Identity struct {
	Username string
	FullName string
	Email    string
	Groups   []string
	// ExternalID is the stable identity at the provider (OIDC iss|sub, SAML NameID);
	// once recorded, another identity with the same user name is refused.
	ExternalID string
}
