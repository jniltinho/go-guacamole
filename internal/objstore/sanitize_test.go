//go:build !noobjstore

package objstore

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"

	"github.com/rclone/rclone/fs"
)

func TestSanitizer(t *testing.T) {
	s := NewSanitizer("wJalrXUtnFEMI/K7MDENG", "")
	cases := map[string]string{
		"key wJalrXUtnFEMI/K7MDENG leaked":                                                                           "wJalrXUtnFEMI",
		"b64 " + base64.StdEncoding.EncodeToString([]byte("wJalrXUtnFEMI/K7MDENG")):                                  base64.StdEncoding.EncodeToString([]byte("wJalrXUtnFEMI/K7MDENG")),
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIA/2026, Signature=abc":                                        "AWS4-HMAC-SHA256",
		"GET /b/k?X-Amz-Signature=deadbeef&X-Amz-Credential=AKIA%2F":                                                 "deadbeef",
		`{"private_key_id":"123abc","private_key":"-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n"}`: "MIIE",
		"https://storage.googleapis.com/b/o?X-Goog-Signature=f00d":                                                   "f00d",
	}
	for in, secret := range cases {
		if out := s.Clean(in); strings.Contains(out, secret) {
			t.Fatalf("%q → %q still holds %q", in, out, secret)
		}
	}
	if s.Clean("plain message") != "plain message" {
		t.Fatal("clean text changed")
	}
}

// rclone's own log lines go through the sanitizer before reaching the application log.
func TestRcloneLogsAreSanitized(t *testing.T) {
	var buf bytes.Buffer
	installLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	defer installLogger(slog.Default())
	remember("super-secret-key-123")
	fs.Errorf(nil, "request failed: secret_access_key=super-secret-key-123 Authorization: AWS4 x")
	out := buf.String()
	if !strings.Contains(out, "request failed") || strings.Contains(out, "super-secret-key-123") || strings.Contains(out, "AWS4 x") {
		t.Fatalf("rclone log: %q", out)
	}
}
