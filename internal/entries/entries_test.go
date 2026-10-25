package entries

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/lookup"
)

func TestStableRawIdentity(t *testing.T) {
	// Independent Python hashlib/struct.pack('>Q', len(utf8)) golden.
	if got := ID("uk-ua", "en-us", "Слово"); got != "072af9c78442d00d27913989886b452a54ddcb47e1f0b293caf60f8daabf00c4" {
		t.Fatal("identity encoding changed", got)
	}
	seen := map[string]bool{}
	for _, fields := range [][3]string{
		{"uk-ua", "en-us", "Слово"}, {"uk-ua", "en-us", "СЛОВО"},
		{"uk-ua", "en-us", " Слово "}, {"en-us", "uk-ua", "Слово"},
		{"sr-sp", "sr-rs", "reč"}, {"sr-rs", "sr-rs", "reč"},
		{"en", "uk", "É"}, {"en", "uk", "É"},
		{"ab", "c", "d"}, {"a", "bc", "d"},
		{"en", "uk", strings.Repeat("x", 65536)},
	} {
		id := ID(fields[0], fields[1], fields[2])
		if !ValidID(id) || seen[id] || len(Path(id)) != len("/word/")+64 {
			t.Fatal("raw identity merged or URL unbounded")
		}
		seen[id] = true
	}
}

func TestEntryValidationBeforeStorage(t *testing.T) {
	for _, id := range []string{"", "../x", strings.Repeat("A", 64), strings.Repeat("0", 63)} {
		if _, err := Read(context.Background(), nil, nil, id, ""); !errors.Is(err, ErrNotFound) {
			t.Fatal("invalid ID reached storage", err)
		}
	}
	if _, err := Read(context.Background(), nil, nil, strings.Repeat("0", 64), "broken"); !errors.Is(err, lookup.ErrCursor) {
		t.Fatal("invalid cursor reached storage", err)
	}
}
