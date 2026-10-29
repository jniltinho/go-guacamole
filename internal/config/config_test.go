package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrecedence(t *testing.T) {
	p := writeTOML(t, "[app]\naddr = \":9000\"\n")
	cfg, _, err := Load(p, nil)
	if err != nil || cfg.App.Addr != ":9000" {
		t.Fatalf("toml: %v %v", cfg, err)
	}
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_ALLOWED_ORIGINS", "https://a.example, https://b.example")
	cfg, _, err = Load(p, nil)
	if err != nil || cfg.App.Addr != ":9090" || len(cfg.App.AllowedOrigins) != 2 || cfg.App.AllowedOrigins[1] != "https://b.example" {
		t.Fatalf("env: %+v %v", cfg.App, err)
	}
	fs := pflag.NewFlagSet("x", pflag.ContinueOnError)
	fs.String("addr", "", "")
	_ = fs.Parse([]string{"--addr", ":7070"})
	cfg, _, err = Load(p, fs)
	if err != nil || cfg.App.Addr != ":7070" {
		t.Fatalf("flag: %v %v", cfg.App.Addr, err)
	}
}

func TestUnknownKeyAndInvalid(t *testing.T) {
	if _, _, err := Load(writeTOML(t, "[app]\nadress = \":80\"\n"), nil); err == nil || !strings.Contains(err.Error(), "unknown config key app.adress") {
		t.Fatalf("unknown key: %v", err)
	}
	t.Setenv("APP_SESSION_TTL", "abc")
	if _, _, err := Load("", nil); err == nil {
		t.Fatal("invalid duration accepted")
	}
}

func TestProductionRules(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	_, _, err := Load(writeTOML(t, "[app]\nsecret_key_file = \"\"\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "APP_SECRET_KEY or APP_SECRET_KEY_FILE is required") || !strings.Contains(err.Error(), "TLS_ENABLED=true or APP_COOKIE_SECURE=true") {
		t.Fatalf("production rules: %v", err)
	}
}

func TestRedacted(t *testing.T) {
	t.Setenv("APP_SECRET_KEY", strings.Repeat("k", 40))
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DSN", "gg:supersecret@tcp(db:3306)/gg")
	cfg, _, err := Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cfg.Redacted()
	if err != nil || strings.Contains(out, "supersecret") || strings.Contains(out, strings.Repeat("k", 40)) || !strings.Contains(out, "[db]") {
		t.Fatalf("redacted:\n%s %v", out, err)
	}
}
