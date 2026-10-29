package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// Stock Guacamole 1.6 guacadmin hash/salt from the official initdb.sql.
const (
	guacadminHash = "CA458A7D494E3BE824F5E1E175A1556C0F8EEF2C2D7DF3633BEC4A29C4411960"
	guacadminSalt = "FE24ADC5E11E2B25288D1704ABE67A79E342ECC26064CE69C5B3177795A82264"
)

func TestGuacadminLegacyHash(t *testing.T) {
	u := &store.User{PasswordScheme: store.SchemeGuacSHA256, PasswordHash: strings.ToLower(guacadminHash), PasswordSalt: strings.ToLower(guacadminSalt)}
	r := Verify(u, "guacadmin")
	if !r.OK || !r.NeedsUpgrade {
		t.Fatalf("stock guacadmin must verify: %+v", r)
	}
	if Verify(u, "wrong").OK {
		t.Fatal("wrong password accepted")
	}
}

func TestUnsaltedAndUnicodeLegacy(t *testing.T) {
	u := &store.User{PasswordScheme: store.SchemeGuacSHA256, PasswordHash: GuacHash("senha-ção", nil)}
	if !Verify(u, "senha-ção").OK {
		t.Fatal("unsalted unicode must verify")
	}
	salt, _ := hex.DecodeString("00ff")
	long := strings.Repeat("é", 40) // 80 bytes
	u2 := &store.User{PasswordScheme: store.SchemeGuacSHA256, PasswordHash: GuacHash(long, salt), PasswordSalt: "00ff"}
	if r := Verify(u2, long); !r.OK || !r.MustReplace || r.NeedsUpgrade {
		t.Fatalf("long legacy password: %+v", r)
	}
}

func TestBcryptAndPolicy(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordTooShort {
		t.Fatal("short password accepted")
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); err != ErrPasswordTooLong {
		t.Fatal("73-byte password accepted")
	}
	h, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{PasswordScheme: store.SchemeBcrypt, PasswordHash: h}
	if !Verify(u, "correct-horse").OK || Verify(u, "nope-nope").OK {
		t.Fatal("bcrypt verify wrong")
	}
	if Verify(nil, "anything").OK {
		t.Fatal("nil user verified")
	}
}

func sp(s string) *string { return &s }

func TestRestrictions(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	at := time.Date(2026, 10, 29, 19, 0, 0, 0, loc)
	u := &store.User{Timezone: sp("America/Sao_Paulo"), AccessWindowStart: sp("08:00"), AccessWindowEnd: sp("18:00")}
	if err := CheckRestrictions(u, at); err != ErrAccessWindow {
		t.Fatalf("expected ACCESS_WINDOW, got %v", err)
	}
	if err := CheckRestrictions(u, at.Add(-3*time.Hour)); err != nil {
		t.Fatalf("inside window: %v", err)
	}
	night := &store.User{Timezone: sp("America/Sao_Paulo"), AccessWindowStart: sp("22:00"), AccessWindowEnd: sp("06:00")}
	if err := CheckRestrictions(night, at.Add(4*time.Hour)); err != nil {
		t.Fatalf("cross-midnight window: %v", err)
	}
	exp := &store.User{ValidUntil: sp("2026-10-28"), Timezone: sp("America/Sao_Paulo")}
	if err := CheckRestrictions(exp, at); err != ErrAccountExpired {
		t.Fatalf("expected ACCOUNT_EXPIRED, got %v", err)
	}
	if err := CheckRestrictions(&store.User{Disabled: true}, at); err != ErrAccountDisabled {
		t.Fatal("disabled accepted")
	}
	dl := NextRestrictionDeadline(u, at.Add(-3*time.Hour))
	if want := time.Date(2026, 10, 29, 18, 0, 0, 0, loc); !dl.Equal(want) {
		t.Fatalf("deadline %v want %v", dl, want)
	}
	vu := &store.User{ValidUntil: sp("2026-10-29"), Timezone: sp("America/Sao_Paulo")}
	if want := time.Date(2026, 10, 30, 0, 0, 0, 0, loc); !NextRestrictionDeadline(vu, at).Equal(want) {
		t.Fatal("validity deadline wrong")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	l.Hit("ip")
	l.Hit("ip")
	if !l.Blocked("ip") || l.Hit("ip") {
		t.Fatal("expected blocked")
	}
	now = now.Add(time.Minute)
	if l.Blocked("ip") {
		t.Fatal("window should have expired")
	}
}

func TestRestrictionsFailClosed(t *testing.T) {
	now := time.Now()
	for _, u := range []*store.User{
		{AccessWindowStart: sp("08:00")},                               // incomplete
		{AccessWindowStart: sp("99:00"), AccessWindowEnd: sp("18:00")}, // malformed
		{ValidUntil: sp("2026-13-40")},                                 // malformed date
	} {
		if err := CheckRestrictions(u, now); err == nil {
			t.Errorf("malformed restriction allowed: %+v", u)
		}
	}
	if ValidClock("24:00") || !ValidClock("23:59") || !ValidClock("08:00:00") {
		t.Error("ValidClock wrong")
	}
}
