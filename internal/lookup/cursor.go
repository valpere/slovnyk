package lookup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

const MaxCursorBytes = 2048

var ErrCursor = errors.New("invalid_cursor")

// Cursor declaration order is the V1 canonical JSON object-key order.
// Strings use encoding/json escaping, no insignificant whitespace or trailing LF.
type Cursor struct {
	Anchor      int64  `json:"anchor_translation_id"`
	Dataset     string `json:"dataset_id"`
	Fingerprint string `json:"fingerprint"`
	KID         string `json:"kid"`
	Limit       int    `json:"limit"`
	Version     int    `json:"version"`
}
type Signer struct {
	active string
	keys   map[string][]byte
}

// NewSigner copies a bounded keyring. Keys must contain at least 256 bits of
// securely generated entropy. Retain old keys while their cursors are supported.
func NewSigner(active string, keys map[string][]byte) (*Signer, error) {
	if len(keys) == 0 || len(keys) > 16 {
		return nil, ErrCursor
	}
	copyKeys := make(map[string][]byte, len(keys))
	for kid, key := range keys {
		if len(kid) == 0 || len(kid) > 64 || len(key) < 32 || len(key) > 64 {
			return nil, ErrCursor
		}
		for _, r := range kid {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return nil, ErrCursor
			}
		}
		copyKeys[kid] = append([]byte{}, key...)
	}
	if copyKeys[active] == nil {
		return nil, ErrCursor
	}
	return &Signer{active: active, keys: copyKeys}, nil
}
func validID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || strings.ToLower(s) != s {
		return false
	}
	b, e := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return e == nil && len(b) == 16
}
func validCursor(c Cursor) bool {
	hash, e := hex.DecodeString(c.Fingerprint)
	return c.Version == 1 && validID(c.Dataset) && c.Anchor > 0 && c.Limit >= 1 && c.Limit <= 100 && e == nil && len(hash) == 32 && hex.EncodeToString(hash) == c.Fingerprint
}
func (s *Signer) Encode(c Cursor) (string, error) {
	if s == nil {
		return "", ErrCursor
	}
	c.Version = 1
	c.KID = s.active
	if !validCursor(c) {
		return "", ErrCursor
	}
	b, e := json.Marshal(c)
	if e != nil {
		return "", ErrCursor
	}
	h := hmac.New(sha256.New, s.keys[c.KID])
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
func (s *Signer) Decode(token string) (Cursor, error) {
	var c Cursor
	if s == nil || len(token) == 0 || len(token) > MaxCursorBytes {
		return c, ErrCursor
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return c, ErrCursor
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(payload)
	if e != nil || base64.RawURLEncoding.EncodeToString(b) != payload {
		return c, ErrCursor
	}
	sig, e := base64.RawURLEncoding.Strict().DecodeString(signature)
	if e != nil || len(sig) != sha256.Size || base64.RawURLEncoding.EncodeToString(sig) != signature {
		return c, ErrCursor
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return Cursor{}, ErrCursor
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return Cursor{}, ErrCursor
	}
	canonical, e := json.Marshal(c)
	if e != nil || !bytes.Equal(canonical, b) || !validCursor(c) {
		return Cursor{}, ErrCursor
	}
	key := s.keys[c.KID]
	if key == nil {
		return Cursor{}, ErrCursor
	}
	h := hmac.New(sha256.New, key)
	h.Write(b)
	if !hmac.Equal(h.Sum(nil), sig) {
		return Cursor{}, ErrCursor
	}
	return c, nil
}
func Fingerprint(version, from, to, mode, key string, limit int) [32]byte {
	h := sha256.New()
	for _, s := range []string{version, from, to, mode, key, strconv.Itoa(limit)} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(s)))
		h.Write(length[:])
		h.Write([]byte(s))
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
