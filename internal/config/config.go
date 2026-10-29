// Package config defines the typed GoGuacamole configuration, its defaults
// and validation. Values are loaded by the cli package from flags, environment
// variables and a TOML file (in that order of precedence).
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Config is the complete application configuration.
// TOML section/key pairs map to environment variables as SECTION_KEY.
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	TLS       TLSConfig       `mapstructure:"tls"`
	DB        DBConfig        `mapstructure:"db"`
	Guacd     GuacdConfig     `mapstructure:"guacd"`
	Audit     AuditConfig     `mapstructure:"audit"`
	Recording RecordingConfig `mapstructure:"recording"`
}

// RecordingConfig controls guacd session recordings (screen) and SSH typescripts (text).
type RecordingConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Path          string `mapstructure:"path"`       // directory as seen by guacd
	LocalPath     string `mapstructure:"local_path"` // same directory as seen by goguacamole (defaults to path)
	Screen        bool   `mapstructure:"screen"`
	Typescript    bool   `mapstructure:"typescript"`
	IncludeKeys   bool   `mapstructure:"include_keys"`
	RetentionDays int    `mapstructure:"retention_days"`
}

// Dir returns the directory goguacamole reads recordings from.
func (r RecordingConfig) Dir() string {
	if r.LocalPath != "" {
		return r.LocalPath
	}
	return r.Path
}

type AppConfig struct {
	Env                string        `mapstructure:"env"`
	Addr               string        `mapstructure:"addr"`
	BaseURL            string        `mapstructure:"base_url"`
	LogLevel           string        `mapstructure:"log_level"`
	LogFormat          string        `mapstructure:"log_format"`
	SecretKey          string        `mapstructure:"secret_key"`
	SecretKeyFile      string        `mapstructure:"secret_key_file"`
	CookieSecure       bool          `mapstructure:"cookie_secure"`
	SessionIdleTimeout time.Duration `mapstructure:"session_idle_timeout"`
	SessionTTL         time.Duration `mapstructure:"session_ttl"`
	AllowedOrigins     []string      `mapstructure:"allowed_origins"`
	TrustedProxies     []string      `mapstructure:"trusted_proxies"`
	AdminUser          string        `mapstructure:"admin_user"`
	AdminPassword      string        `mapstructure:"admin_password"`
	ProbeInterval      time.Duration `mapstructure:"probe_interval"`
	ShutdownTimeout    time.Duration `mapstructure:"shutdown_timeout"`
}

type TLSConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CertFile     string `mapstructure:"cert_file"`
	KeyFile      string `mapstructure:"key_file"`
	MinVersion   string `mapstructure:"min_version"`
	SelfSigned   bool   `mapstructure:"self_signed"`
	RedirectAddr string `mapstructure:"redirect_addr"`
}

type DBConfig struct {
	Driver       string `mapstructure:"driver"`
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

type GuacdConfig struct {
	Addr          string `mapstructure:"addr"`
	TLS           bool   `mapstructure:"tls"`
	TLSCA         string `mapstructure:"tls_ca"`
	TLSServerName string `mapstructure:"tls_server_name"`
	TLSInsecure   bool   `mapstructure:"tls_insecure"`
}

type AuditConfig struct {
	RetentionDays int `mapstructure:"retention_days"`
}

// Defaults returns the built-in default values as a flat key map
// (section.key → value) suitable for viper.SetDefault.
func Defaults() map[string]any {
	return map[string]any{
		"app.env":                  "development",
		"app.addr":                 ":8080",
		"app.base_url":             "",
		"app.log_level":            "info",
		"app.log_format":           "text",
		"app.secret_key":           "",
		"app.secret_key_file":      "./data/secret.key",
		"app.cookie_secure":        false,
		"app.session_idle_timeout": "30m",
		"app.session_ttl":          "8h",
		"app.allowed_origins":      []string{},
		"app.trusted_proxies":      []string{},
		"app.admin_user":           "",
		"app.admin_password":       "",
		"app.probe_interval":       "60s",
		"app.shutdown_timeout":     "15s",
		"tls.enabled":              false,
		"tls.cert_file":            "",
		"tls.key_file":             "",
		"tls.min_version":          "1.2",
		"tls.self_signed":          false,
		"tls.redirect_addr":        "",
		"db.driver":                "sqlite",
		"db.dsn":                   "./data/goguacamole.db",
		"db.max_open_conns":        10,
		"guacd.addr":               "127.0.0.1:4822",
		"guacd.tls":                false,
		"guacd.tls_ca":             "",
		"guacd.tls_server_name":    "",
		"guacd.tls_insecure":       false,
		"audit.retention_days":     90,
		"recording.enabled":        false,
		"recording.path":           "/record",
		"recording.local_path":     "",
		"recording.screen":         true,
		"recording.typescript":     true,
		"recording.include_keys":   false,
		"recording.retention_days": 90,
	}
}

// Drivers lists the supported database drivers.
var Drivers = []string{"sqlite", "mysql", "postgres"}

// IsProduction reports whether APP_ENV is "production".
func (c *Config) IsProduction() bool { return c.App.Env == "production" }

// SecureCookies reports whether cookies must carry the Secure flag.
func (c *Config) SecureCookies() bool { return c.TLS.Enabled || c.App.CookieSecure }

// Validate checks the configuration and returns an error naming the first
// offending key.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	switch c.App.Env {
	case "development", "production", "test":
	default:
		add("invalid APP_ENV %q (expected development, production or test)", c.App.Env)
	}
	if _, _, err := net.SplitHostPort(c.App.Addr); err != nil {
		add("invalid APP_ADDR %q: %v", c.App.Addr, err)
	}
	if c.App.BaseURL != "" {
		if u, err := url.Parse(c.App.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			add("invalid APP_BASE_URL %q", c.App.BaseURL)
		}
	}
	switch c.App.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		add("invalid APP_LOG_LEVEL %q", c.App.LogLevel)
	}
	switch c.App.LogFormat {
	case "text", "json":
	default:
		add("invalid APP_LOG_FORMAT %q", c.App.LogFormat)
	}
	if c.App.SessionIdleTimeout < time.Minute {
		add("invalid APP_SESSION_IDLE_TIMEOUT %s (minimum 1m)", c.App.SessionIdleTimeout)
	}
	if c.App.SessionTTL < c.App.SessionIdleTimeout {
		add("invalid APP_SESSION_TTL %s (must be >= APP_SESSION_IDLE_TIMEOUT)", c.App.SessionTTL)
	}
	if c.App.ProbeInterval != 0 && c.App.ProbeInterval < 5*time.Second {
		add("invalid APP_PROBE_INTERVAL %s (0 disables, minimum 5s)", c.App.ProbeInterval)
	}
	for _, p := range c.App.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			add("invalid APP_TRUSTED_PROXIES entry %q", p)
		}
	}
	if c.App.SecretKey != "" && len(c.App.SecretKey) < 32 {
		add("invalid APP_SECRET_KEY (minimum 32 characters)")
	}

	if c.TLS.Enabled && !c.TLS.SelfSigned && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		add("TLS_CERT_FILE and TLS_KEY_FILE are required when TLS_ENABLED=true")
	}
	if _, err := ParseTLSVersion(c.TLS.MinVersion); err != nil {
		add("invalid TLS_MIN_VERSION %q (expected 1.2 or 1.3)", c.TLS.MinVersion)
	}
	if c.TLS.RedirectAddr != "" && !c.TLS.Enabled {
		add("TLS_REDIRECT_ADDR requires TLS_ENABLED=true")
	}

	if !contains(Drivers, c.DB.Driver) {
		add("unsupported DB_DRIVER %q (supported: %s)", c.DB.Driver, strings.Join(Drivers, ", "))
	}
	if c.DB.DSN == "" {
		add("DB_DSN is required")
	}
	if c.DB.MaxOpenConns < 1 {
		add("invalid DB_MAX_OPEN_CONNS %d", c.DB.MaxOpenConns)
	}
	if _, _, err := net.SplitHostPort(c.Guacd.Addr); err != nil {
		add("invalid GUACD_ADDR %q: %v", c.Guacd.Addr, err)
	}
	if c.Audit.RetentionDays < 1 {
		add("invalid AUDIT_RETENTION_DAYS %d", c.Audit.RetentionDays)
	}
	if c.Recording.Enabled {
		if c.Recording.Path == "" || c.Recording.Path[0] != '/' {
			add("invalid RECORDING_PATH %q (absolute path required)", c.Recording.Path)
		}
		if c.Recording.RetentionDays < 1 {
			add("invalid RECORDING_RETENTION_DAYS %d", c.Recording.RetentionDays)
		}
	}

	if c.IsProduction() {
		if c.App.SecretKey == "" && c.App.SecretKeyFile == "" {
			add("APP_SECRET_KEY or APP_SECRET_KEY_FILE is required when APP_ENV=production")
		}
		if !c.SecureCookies() {
			add("APP_ENV=production requires TLS_ENABLED=true or APP_COOKIE_SECURE=true (TLS terminated by a proxy)")
		}
		if c.TLS.SelfSigned {
			add("TLS_SELF_SIGNED is not allowed when APP_ENV=production")
		}
		if c.Guacd.TLSInsecure {
			add("GUACD_TLS_INSECURE is not allowed when APP_ENV=production")
		}
	}
	return errors.Join(errs...)
}

// ParseTLSVersion converts "1.2"/"1.3" to the crypto/tls constant value.
func ParseTLSVersion(v string) (uint16, error) {
	switch v {
	case "1.2":
		return 0x0303, nil
	case "1.3":
		return 0x0304, nil
	}
	return 0, fmt.Errorf("unsupported TLS version %q", v)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
