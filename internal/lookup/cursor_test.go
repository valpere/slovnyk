package lookup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/normalization"
)

func signer(t *testing.T) *Signer {
	t.Helper()
	s, e := NewSigner("test-v1", map[string][]byte{"test-v1": []byte("0123456789abcdef0123456789abcdef")})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestCursorGolden(t *testing.T) {
	b, e := os.ReadFile("../../testdata/lookup/cursor-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var golden struct {
		KeyHex      string `json:"key_hex"`
		Fingerprint string
		Payload     string
		Token       string
	}
	if e = json.Unmarshal(b, &golden); e != nil {
		t.Fatal(e)
	}
	fp := Fingerprint(normalization.Version, "en-us", "uk-ua", "exact", "strasse", 2)
	if hex.EncodeToString(fp[:]) != golden.Fingerprint {
		t.Fatal("fingerprint drift")
	}
	key, e := hex.DecodeString(golden.KeyHex)
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewSigner("test-v1", map[string][]byte{"test-v1": key})
	if e != nil {
		t.Fatal(e)
	}
	token, e := s.Encode(Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Anchor: 7, Limit: 2, Fingerprint: golden.Fingerprint})
	if e != nil || token != golden.Token {
		t.Fatalf("token drift: %s %v", token, e)
	}
	c, e := s.Decode(token)
	if e != nil || c.Anchor != 7 || c.Limit != 2 {
		t.Fatalf("decode: %+v %v", c, e)
	}
	payload, _, _ := strings.Cut(token, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	if string(raw) != golden.Payload {
		t.Fatal("payload drift")
	}
}
func signRaw(payload string) string {
	key := []byte("0123456789abcdef0123456789abcdef")
	h := hmac.New(sha256.New, key)
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func TestCursorRejectsAlterations(t *testing.T) {
	s := signer(t)
	valid, e := s.Encode(Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Anchor: 7, Limit: 2, Fingerprint: strings.Repeat("a", 64)})
	if e != nil {
		t.Fatal(e)
	}
	payload, sig, _ := strings.Cut(valid, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	cases := []string{"", strings.Repeat("a", 2049), valid + "=", payload + ".A" + sig[1:], payload + "\n." + sig, valid + ".extra",
		signRaw(strings.Replace(string(raw), `"anchor_translation_id":7`, `"anchor_translation_id":0`, 1)),
		signRaw(strings.Replace(string(raw), `"version":1`, `"version":2`, 1)),
		signRaw(strings.Replace(string(raw), `"kid":"test-v1"`, `"kid":"unknown"`, 1)),
		signRaw(strings.Replace(string(raw), `"limit":2`, `"limit":2,"limit":2`, 1)),
		signRaw(" " + string(raw)), signRaw(strings.Replace(string(raw), `"limit":2`, `"other":2`, 1)),
	}
	for _, token := range cases {
		if _, e = s.Decode(token); e == nil {
			t.Fatalf("invalid token admitted: %q", token)
		}
	}
}
func TestKeyRotationAndCopies(t *testing.T) {
	keys := map[string][]byte{"old": []byte(strings.Repeat("o", 32)), "new": []byte(strings.Repeat("n", 32))}
	old, e := NewSigner("old", keys)
	if e != nil {
		t.Fatal(e)
	}
	token, e := old.Encode(Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Anchor: 1, Limit: 100, Fingerprint: strings.Repeat("0", 64)})
	if e != nil {
		t.Fatal(e)
	}
	rotated, e := NewSigner("new", keys)
	if e != nil {
		t.Fatal(e)
	}
	keys["old"][0] = 'x'
	delete(keys, "old")
	if _, e = rotated.Decode(token); e != nil {
		t.Fatal("old key not retained/copied", e)
	}
	removed, e := NewSigner("new", keys)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = removed.Decode(token); e == nil {
		t.Fatal("unknown signing key admitted")
	}
	for _, keyring := range []map[string][]byte{nil, {"x": []byte("short")}, {"bad.kid": make([]byte, 32)}} {
		if _, e = NewSigner("x", keyring); e == nil {
			t.Fatal("invalid keyring admitted")
		}
	}
}
func FuzzCursor(f *testing.F) {
	f.Add("")
	f.Add("a.b")
	f.Add(strings.Repeat("x", 2049))
	s, _ := NewSigner("x", map[string][]byte{"x": make([]byte, 32)})
	valid, _ := s.Encode(Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Anchor: 1, Limit: 50, Fingerprint: strings.Repeat("0", 64)})
	f.Add(valid)
	f.Fuzz(func(t *testing.T, token string) {
		c, e := s.Decode(token)
		if e == nil {
			again, e := s.Encode(c)
			if e != nil || again != token {
				t.Fatal("noncanonical accepted cursor")
			}
		}
	})
}
