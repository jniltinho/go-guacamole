package auth

import (
	"testing"
	"time"
)

func TestJSONAuthRoundTrip(t *testing.T) {
	key := "4c0b569e4c96df157eee1b65dd0e4d41"
	now := time.UnixMilli(1_700_000_000_000)
	p := &JSONPayload{Username: "maria", Expires: now.UnixMilli() + 60_000,
		Connections: map[string]JSONConnection{"web": {Protocol: "ssh", Parameters: map[string]string{"hostname": "10.0.0.1"}}}}
	data, err := EncodeJSONAuth(key, p)
	if err != nil {
		t.Fatal(err)
	}
	got, sig, err := DecodeJSONAuth(key, data, now)
	if err != nil || got.Username != "maria" || got.Connections["web"].Parameters["hostname"] != "10.0.0.1" || len(sig) != 64 {
		t.Fatalf("decode: %+v %v", got, err)
	}
	if _, _, err := DecodeJSONAuth(key, data, now.Add(2*time.Minute)); err == nil {
		t.Error("expired data accepted")
	}
	if _, _, err := DecodeJSONAuth("00000000000000000000000000000000", data, now); err == nil {
		t.Error("wrong key accepted")
	}
	p.Expires = 0
	noExp, _ := EncodeJSONAuth(key, p)
	if _, _, err := DecodeJSONAuth(key, noExp, now); err == nil {
		t.Error("data without expiry accepted")
	}
}
