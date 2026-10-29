package secrets

import "testing"

// guacd 1.6 credential parameters per supported protocol; every one must be
// flagged secret by the registry.
var guacdCredentials = map[string][]string{
	"ssh": {"password", "passphrase", "private-key", "sftp-password", "sftp-passphrase", "sftp-private-key"},
	"rdp": {"password", "gateway-password", "sftp-password", "sftp-passphrase", "sftp-private-key"},
	"vnc": {"password", "sftp-password", "sftp-passphrase", "sftp-private-key"},
}

func TestRegistryCoversCredentials(t *testing.T) {
	for proto, names := range guacdCredentials {
		for _, n := range names {
			if !IsSecret(proto, n) {
				t.Errorf("%s/%s not registered as secret", proto, n)
			}
		}
	}
	if IsSecret("ssh", "hostname") {
		t.Error("hostname must not be secret")
	}
}

func TestBoxRoundTrip(t *testing.T) {
	b, err := NewBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := b.Seal("Pa55")
	if err != nil {
		t.Fatal(err)
	}
	if ct == "Pa55" || !IsSealed(ct) {
		t.Fatalf("not encrypted: %s", ct)
	}
	pt, err := b.Open(ct)
	if err != nil || pt != "Pa55" {
		t.Fatalf("open: %q %v", pt, err)
	}
	other, _ := NewBox("ffffffffffffffffffffffffffffffff")
	if _, err := other.Open(ct); err != ErrKey {
		t.Fatalf("wrong key must fail, got %v", err)
	}
	empty, _ := NewBox("")
	if _, err := empty.Open(ct); err != ErrKey {
		t.Fatal("missing key must fail")
	}
}
