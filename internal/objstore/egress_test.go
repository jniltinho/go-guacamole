//go:build !noobjstore

package objstore

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointPolicy(t *testing.T) {
	var p Policy
	for _, ip := range []string{"169.254.169.254", "fd00:ec2::254", "10.1.2.3", "172.16.5.5", "192.168.1.1", "127.0.0.1", "::1", "100.64.0.1"} {
		if err := p.checkIP("h", net.ParseIP(ip)); !errors.Is(err, ErrEndpointNotAllowed) {
			t.Fatalf("%s allowed: %v", ip, err)
		}
	}
	if err := p.checkIP("s3.amazonaws.com", net.ParseIP("52.216.1.1")); err != nil {
		t.Fatalf("public address refused: %v", err)
	}
	allow := Policy{Allowed: []string{"minio.lab.internal", "10.9.0.0/16"}}
	if allow.checkIP("minio.lab.internal", net.ParseIP("10.1.1.1")) != nil || allow.checkIP("x", net.ParseIP("10.9.3.3")) != nil {
		t.Fatal("operator allowlist ignored")
	}
	if !errors.Is(allow.checkIP("x", net.ParseIP("169.254.169.254")), ErrEndpointNotAllowed) {
		t.Fatal("metadata allowed by an unrelated allowlist entry")
	}
	// names are resolved at dial time: localhost is a loopback address
	if _, err := p.dial(context.Background(), "tcp", "localhost:443"); !errors.Is(err, ErrEndpointNotAllowed) {
		t.Fatalf("localhost dialed: %v", err)
	}
}

// The egress proxy tunnels HTTPS only to allowed destinations, whatever HTTPS_PROXY says.
func TestEgressTunnel(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1") // a dead proxy: using it would fail
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	get := func(p Policy) (int, error) {
		e, err := startEgress(p)
		if err != nil {
			t.Fatal(err)
		}
		defer e.close()
		pu, _ := url.Parse(e.url())
		c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		res, err := c.Get(srv.URL)
		if err != nil {
			return 0, err
		}
		res.Body.Close()
		return res.StatusCode, nil
	}
	if _, err := get(Policy{}); err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("loopback destination tunnelled: %v", err)
	}
	if code, err := get(Policy{Allowed: []string{"127.0.0.1"}}); err != nil || code != 200 {
		t.Fatalf("allowed destination: %d %v", code, err)
	}
}

// rclone never reads nor writes a configuration file.
func TestNoRcloneConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	Setup(nil)
	// a loopback endpoint is refused by the egress proxy at once: no network needed for this test
	_, _, err := Open(context.Background(), Target{Provider: S3, Bucket: "b", Prefix: "p", Endpoint: "https://127.0.0.1:9",
		AccessKey: "AKIAEXAMPLE", SecretKey: "verysecretvalue"}, Policy{})
	if err != nil && strings.Contains(err.Error(), "verysecretvalue") {
		t.Fatalf("secret in error: %v", err)
	}
	_ = filepath.Walk(home, func(p string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			t.Fatalf("file written: %s", p)
		}
		return nil
	})
	if c := Clean("failed with secret verysecretvalue"); strings.Contains(c, "verysecretvalue") {
		t.Fatalf("secret in sanitized text: %s", c)
	}
}
