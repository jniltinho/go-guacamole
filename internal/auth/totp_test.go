package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTOTPRFC6238(t *testing.T) {
	// RFC 6238 appendix B, SHA1 secret "12345678901234567890" (last 6 of the 8-digit values)
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		unix int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		step, ok := VerifyTOTP(secret, c.code, time.Unix(c.unix, 0))
		if !ok || step != c.unix/30 {
			t.Errorf("t=%d code %s: ok=%v step=%d", c.unix, c.code, ok, step)
		}
	}
	if _, ok := VerifyTOTP(secret, "000000", time.Unix(59, 0)); ok {
		t.Error("wrong code accepted")
	}
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(59+120, 0)); ok {
		t.Error("code outside the drift window accepted")
	}
}

func TestTOTPSecretAndURI(t *testing.T) {
	s := NewTOTPSecret()
	if len(s) != 32 || s == NewTOTPSecret() {
		t.Fatalf("bad secret %q", s)
	}
	u := TOTPURI("GoGuacamole", "maria", s)
	if !strings.HasPrefix(u, "otpauth://totp/GoGuacamole:maria?") || !strings.Contains(u, "secret="+s) {
		t.Fatalf("bad uri %s", u)
	}
}
