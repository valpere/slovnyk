package lookup

import (
	"context"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/normalization"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func TestValidationBeforeStorage(t *testing.T) {
	service, _ := New(signer(t))
	for _, r := range []Request{{Q: "q", Mode: "wrong"}, {Q: "q", Limit: -1}, {Q: "\x00"}, {Q: strings.Repeat("x", 1025)}, {Q: "", Cursor: "invalid"}, {Q: "q", Cursor: strings.Repeat("x", 2049)}} {
		_, e := service.Search(context.Background(), nil, r)
		if Status(e) != 400 {
			t.Fatalf("%+v: %v", r, e)
		}
	}
	if page, e := service.Search(context.Background(), nil, Request{Q: " \t\r\n"}); e != nil || len(page.Entries) != 0 {
		t.Fatalf("empty form: %+v %v", page, e)
	}
	if _, e := service.Search(context.Background(), nil, Request{From: "a", To: "b", Q: "q"}); Status(e) != 503 {
		t.Fatal("nil storage not unavailable", e)
	}
}
func TestFingerprintEffectiveMode(t *testing.T) {
	a, _ := normalization.Compile(" STRAẞE ", normalization.Auto)
	b, _ := normalization.Compile("strasse", normalization.Exact)
	if Fingerprint(normalization.Version, "a", "b", string(a.Mode), a.Key, 50) != Fingerprint(normalization.Version, "a", "b", string(b.Mode), b.Key, 50) {
		t.Fatal("equivalent requests differ")
	}
	// Length prefixes avoid concatenation ambiguity.
	if Fingerprint("ab", "c", "d", "exact", "q", 1) == Fingerprint("a", "bc", "d", "exact", "q", 1) {
		t.Fatal("ambiguous fingerprint")
	}
}
func TestLargestEscapedEntryFits(t *testing.T) {
	t1 := postgres.Translation{SourceText: strings.Repeat(`"`, 65536), TargetText: strings.Repeat(`'`, 65536)}
	if EntryBytes(t1) > EntryBudget {
		t.Fatal("admitted entry cannot fit")
	}
}

func TestErrorStatuses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{nil, 200}, {ErrInput, 400}, {ErrCursor, 400}, {ErrLocale, 400}, {ErrPair, 404}, {ErrGone, 410}, {ErrUnavailable, 503}, {ErrBudget, 503}} {
		if Status(tc.err) != tc.status {
			t.Fatalf("%v: %d", tc.err, Status(tc.err))
		}
	}
}
