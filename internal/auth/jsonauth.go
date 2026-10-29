package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// JSONPayload is the guacamole-auth-json document: a user name, an expiry and
// connections that exist only for this user.
type JSONPayload struct {
	Username    string                    `json:"username"`
	Expires     int64                     `json:"expires"` // Unix ms
	Connections map[string]JSONConnection `json:"connections"`
}

// JSONConnection is one connection of a JSONPayload.
type JSONConnection struct {
	Protocol   string            `json:"protocol"`
	Parameters map[string]string `json:"parameters"`
}

// ErrJSONInvalid covers a bad key, signature, encoding or expiry.
var ErrJSONInvalid = errors.New("invalid or expired JSON authentication data")

// DecodeJSONAuth verifies data produced like guacamole-auth-json: base64 of
// AES-128-CBC (zero IV, PKCS#5) over HMAC-SHA256(json) || json, with the same
// 128-bit key. It returns the payload and its signature (for replay checks).
func DecodeJSONAuth(hexKey, data string, now time.Time) (*JSONPayload, string, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 16 {
		return nil, "", errors.New("JSON secret key must be 32 hexadecimal digits")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
	if err != nil || len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, "", ErrJSONInvalid
	}
	block, _ := aes.NewCipher(key)
	plain := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, raw)
	pad := int(plain[len(plain)-1])
	if pad < 1 || pad > aes.BlockSize || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, "", ErrJSONInvalid
	}
	plain = plain[:len(plain)-pad]
	if len(plain) <= sha256.Size {
		return nil, "", ErrJSONInvalid
	}
	sig, doc := plain[:sha256.Size], plain[sha256.Size:]
	m := hmac.New(sha256.New, key)
	m.Write(doc)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return nil, "", ErrJSONInvalid
	}
	var p JSONPayload
	if err := json.Unmarshal(doc, &p); err != nil || p.Username == "" {
		return nil, "", ErrJSONInvalid
	}
	// unlike the Java extension, an expiry is mandatory: the data is a bearer token
	if p.Expires == 0 || now.UnixMilli() > p.Expires {
		return nil, "", ErrJSONInvalid
	}
	return &p, hex.EncodeToString(sig), nil
}

// EncodeJSONAuth is the inverse of DecodeJSONAuth (tests and the admin "Test" button).
func EncodeJSONAuth(hexKey string, p *JSONPayload) (string, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 16 {
		return "", errors.New("JSON secret key must be 32 hexadecimal digits")
	}
	doc, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, key)
	m.Write(doc)
	plain := append(m.Sum(nil), doc...)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := aes.NewCipher(key)
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(out), nil
}
