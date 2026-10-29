// Package auth implements password hashing, login, sessions and
// request-level protections.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// BcryptCost is the cost used for new hashes.
const BcryptCost = 12

// Password policy limits.
const (
	MinPasswordLen   = 8
	MaxPasswordBytes = 72
)

var (
	ErrPasswordTooShort = errors.New("PASSWORD_TOO_SHORT")
	ErrPasswordTooLong  = errors.New("PASSWORD_TOO_LONG")
)

// dummyHash is compared on every failed login to equalize timing.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("goguacamole-dummy-password"), BcryptCost)

// ValidatePassword enforces the password policy.
func ValidatePassword(p string) error {
	if len([]rune(p)) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	if len(p) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a bcrypt hash.
func HashPassword(p string) (string, error) {
	if err := ValidatePassword(p); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(p), BcryptCost)
	return string(b), err
}

// GuacHash computes the Apache Guacamole JDBC hash: SHA-256(password || UPPER_HEX(salt)).
// salt may be empty for unsalted hashes. Returns lowercase hex.
func GuacHash(password string, salt []byte) string {
	h := sha256.New()
	h.Write([]byte(password))
	if len(salt) > 0 {
		h.Write([]byte(strings.ToUpper(hex.EncodeToString(salt))))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyResult describes the outcome of a password check.
type VerifyResult struct {
	OK bool
	// NeedsUpgrade is set when a legacy hash matched and can be re-hashed.
	NeedsUpgrade bool
	// MustReplace is set when a legacy hash matched but the password cannot be
	// stored with bcrypt (longer than 72 bytes).
	MustReplace bool
}

// Verify checks a password against a stored user. A nil user (unknown
// username) still performs one bcrypt comparison.
func Verify(u *store.User, password string) VerifyResult {
	if u == nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return VerifyResult{}
	}
	switch u.PasswordScheme {
	case store.SchemeBcrypt:
		if len(password) > MaxPasswordBytes {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("x"))
			return VerifyResult{}
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil {
			return VerifyResult{OK: true}
		}
		return VerifyResult{}
	case store.SchemeGuacSHA256:
		salt, err := hex.DecodeString(u.PasswordSalt)
		want, err2 := hex.DecodeString(u.PasswordHash)
		ok := err == nil && err2 == nil
		got, _ := hex.DecodeString(GuacHash(password, salt))
		if ok && subtle.ConstantTimeCompare(got, want) == 1 {
			if len(password) > MaxPasswordBytes {
				return VerifyResult{OK: true, MustReplace: true}
			}
			return VerifyResult{OK: true, NeedsUpgrade: true}
		}
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return VerifyResult{}
	}
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
	return VerifyResult{}
}

// Burn performs one bcrypt comparison (used on failure paths that did not
// already do so, e.g. disabled accounts checked before verification).
func Burn() { _ = bcrypt.CompareHashAndPassword(dummyHash, []byte("x")) }
