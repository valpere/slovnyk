// Package normalization implements the versioned dictionary key and query rules.
package normalization

import (
	"crypto/sha256"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Version changes when the algorithm, tables, or trim set changes. Runtime table
// versions are included because x/text can select tables by Go build version.
const Version = "nfc-fold-nfc-v1;xtext=0.43.0;nfc=" + norm.Version + ";fold=" + cases.UnicodeVersion + ";trim=whitespace-15.0"

var fold = cases.Fold() // Fold is stateless and safe for concurrent use.

// Key preserves edge spaces; callers must validate imported fields separately.
func Key(raw string) string { return nfc(fold.String(nfc(raw))) }

// x/text's stream-safe NFC inserts CGJ after 30 non-starters. Dictionary keys
// require Unicode NFC without inserting characters. Keep its fast path and use
// canonical decomposition/order/composition for those unusually long segments.
func nfc(s string) string {
	n := norm.NFC.String(s)
	if strings.Count(n, "\u034f") <= strings.Count(s, "\u034f") {
		return n
	}
	var decomposed []rune
	for _, r := range s {
		decomposed = append(decomposed, []rune(norm.NFD.String(string(r)))...)
	}
	ccc := func(r rune) uint8 { return norm.NFD.PropertiesString(string(r)).CCC() }
	for start := 0; start < len(decomposed); {
		if ccc(decomposed[start]) == 0 {
			start++
			continue
		}
		end := start + 1
		for end < len(decomposed) && ccc(decomposed[end]) != 0 {
			end++
		}
		sort.SliceStable(decomposed[start:end], func(i, j int) bool { return ccc(decomposed[start+i]) < ccc(decomposed[start+j]) })
		start = end
	}
	out := make([]rune, 0, len(decomposed))
	starter, lastCCC := -1, uint8(0)
	for _, r := range decomposed {
		class := ccc(r)
		if starter >= 0 && (lastCCC == 0 || lastCCC < class) {
			pair := norm.NFC.String(string(out[starter]) + string(r))
			if utf8.RuneCountInString(pair) == 1 {
				out[starter], _ = utf8.DecodeRuneInString(pair)
				continue
			}
		}
		if class == 0 {
			starter = len(out)
		}
		out = append(out, r)
		lastCCC = class
	}
	return string(out)
}

func KeyHash(key string) [32]byte { return sha256.Sum256([]byte(key)) }

type Mode string

const (
	Auto  Mode = "auto"
	Exact Mode = "exact"
	Mask  Mode = "mask"
)

type Query struct {
	Key  string
	Mode Mode
	// Pattern is a LIKE parameter for mask queries, using ESCAPE E'\\\\'.
	Pattern string
}

// Compile validates decoded query text, trims only its edges, and resolves auto.
// HTTP parameter presence/duplication and dataset selection belong to the caller.
func Compile(raw string, mode Mode) (Query, error) {
	if mode != Auto && mode != Exact && mode != Mask {
		return Query{}, errors.New("invalid_mode")
	}
	var err error
	raw, err = Validate(raw)
	if err != nil {
		return Query{}, err
	}
	if mode == Auto {
		mode = Exact
		if strings.ContainsAny(raw, "*?") {
			mode = Mask
		}
	}
	q := Query{Key: Key(raw), Mode: mode}
	if mode == Mask {
		q.Pattern = Like(q.Key)
	}
	return q, nil
}

// Like compiles an already-normalized mask without interpolating it into SQL.
func Like(mask string) string {
	var b strings.Builder
	for _, r := range mask {
		switch r {
		case '*':
			b.WriteByte('%')
		case '?':
			b.WriteByte('_')
		case '%', '_', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Explicit Unicode 15 White_Space, independent of the toolchain's unicode table.
func whiteSpace(r rune) bool {
	return (r >= 0x9 && r <= 0xd) || r == 0x20 || r == 0x85 || r == 0xa0 || r == 0x1680 ||
		(r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}

// Validate checks decoded query size/encoding/controls and trims the versioned
// edge whitespace set without normalizing. Dataset selection precedes Compile.
func Validate(raw string) (string, error) {
	if len(raw) > 8192 || !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 1024 {
		return "", errors.New("invalid_query_size_or_encoding")
	}
	for _, r := range raw {
		if r == 0 || r == 127 || (r < 32 && r != '\t' && r != '\n' && r != '\r') {
			return "", errors.New("invalid_query_control")
		}
	}
	return strings.TrimFunc(raw, whiteSpace), nil
}
