// Package secrets holds the registry of secret connection parameters and the
// AES-256-GCM box used to encrypt them at rest.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// RegistryVersion is bumped whenever the secret parameter list changes.
const RegistryVersion = 1

// registry lists guacd 1.6 parameters that carry credentials, per protocol.
var registry = map[string][]string{
	"ssh":        {"password", "passphrase", "private-key", "sftp-password", "sftp-passphrase", "sftp-private-key"},
	"rdp":        {"password", "gateway-password", "sftp-password", "sftp-passphrase", "sftp-private-key"},
	"vnc":        {"password", "sftp-password", "sftp-passphrase", "sftp-private-key"},
	"telnet":     {"password"},
	"kubernetes": {"client-key", "client-cert", "ca-cert"},
}

// IsSecret reports whether a parameter is secret for the protocol. Unknown
// protocols fall back to the union of all known secret names.
func IsSecret(protocol, name string) bool {
	if list, ok := registry[protocol]; ok {
		for _, n := range list {
			if n == name {
				return true
			}
		}
		return false
	}
	for _, list := range registry {
		for _, n := range list {
			if n == name {
				return true
			}
		}
	}
	return false
}

// Names returns the secret parameter names for a protocol.
func Names(protocol string) []string {
	return append([]string(nil), registry[protocol]...)
}

const prefix = "v1:"

// ErrKey is returned when no key is configured or the key cannot decrypt.
var ErrKey = errors.New("secrets: missing or wrong APP_SECRET_KEY")

// Box encrypts and decrypts parameter values.
type Box struct {
	aead cipher.AEAD
}

// NewBox derives an AES-256 key from the secret via HKDF-SHA256. An empty
// secret yields a Box that refuses to seal or open ciphertext.
func NewBox(secret string) (*Box, error) {
	if secret == "" {
		return &Box{}, nil
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), []byte("goguacamole"), []byte("connection-parameters")), key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Enabled reports whether a key is configured.
func (b *Box) Enabled() bool { return b.aead != nil }

// Seal encrypts a plaintext value.
func (b *Box) Seal(plain string) (string, error) {
	if b.aead == nil {
		return "", ErrKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := b.aead.Seal(nil, nonce, []byte(plain), []byte(prefix))
	return prefix + base64.RawStdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// IsSealed reports whether a stored value is ciphertext.
func IsSealed(v string) bool { return strings.HasPrefix(v, prefix) }

// Open decrypts a stored value; plaintext (legacy) values are returned as is.
func (b *Box) Open(stored string) (string, error) {
	if !IsSealed(stored) {
		return stored, nil
	}
	if b.aead == nil {
		return "", ErrKey
	}
	raw, err := base64.RawStdEncoding.DecodeString(stored[len(prefix):])
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", fmt.Errorf("secrets: corrupt value")
	}
	ns := b.aead.NonceSize()
	pt, err := b.aead.Open(nil, raw[:ns], raw[ns:], []byte(prefix))
	if err != nil {
		return "", ErrKey
	}
	return string(pt), nil
}
