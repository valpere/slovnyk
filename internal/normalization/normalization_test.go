package normalization

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

func TestIndependentUnicodeGolden(t *testing.T) {
	b, err := os.ReadFile("../../testdata/normalization.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Raw, Key string }
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		if got := Key(c.Raw); got != c.Key {
			t.Fatalf("golden %d: got %q want %q", i, got, c.Key)
		}
	}
}

func TestKeys(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"Straße", "strasse"}, {"E\u0301", "é"}, {"Σςσ", "σσσ"},
		{"İ", "i\u0307"}, {" слово ", " слово "}, {"a’b", "a’b"},
		{"👩‍💻", "👩‍💻"}, {"\ufeffA", "\ufeffa"},
	} {
		if got := Key(tc.raw); got != tc.want {
			t.Errorf("Key(%q)=%q; want %q", tc.raw, got, tc.want)
		}
		if Key(Key(tc.raw)) != Key(tc.raw) {
			t.Errorf("not idempotent: %q", tc.raw)
		}
	}
	if norm.Version != "15.0.0" || cases.UnicodeVersion != "15.0.0" {
		t.Fatal("review new Unicode tables and versioned golden fixtures before upgrading Go")
	}
}

func TestCompile(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		mode      Mode
		key       string
		effective Mode
		like      string
	}{
		{"\u00a0 Straße \u3000", Auto, "strasse", Exact, ""},
		{"  E\u0301?*%_\\  ", Auto, "é?*%_\\", Mask, "é_%\\%\\_\\\\"},
		{"*?", Exact, "*?", Exact, ""}, {"word", Mask, "word", Mask, "word"},
		{"\u200bA\u200b", Auto, "\u200ba\u200b", Exact, ""},
		{"a\r\n\tb", Auto, "a\r\n\tb", Exact, ""},
		{" \n", Auto, "", Exact, ""},
	} {
		q, err := Compile(tc.raw, tc.mode)
		if err != nil || q.Key != tc.key || q.Mode != tc.effective || q.Pattern != tc.like {
			t.Errorf("Compile(%q,%q)=%+v,%v", tc.raw, tc.mode, q, err)
		}
	}
	for _, raw := range []string{"\x00", "\x01", "\x7f", "\xff", strings.Repeat("a", 1025), strings.Repeat("a", 8193)} {
		if _, err := Compile(raw, Auto); err == nil {
			t.Errorf("accepted invalid query len=%d", len(raw))
		}
	}
	if _, err := Compile("", ""); err == nil {
		t.Fatal("explicit empty mode accepted")
	}
}

func TestLongCombiningSequenceDoesNotInsertCGJ(t *testing.T) {
	raw := "A" + strings.Repeat("\u0301", 40)
	want := "á" + strings.Repeat("\u0301", 39)
	if got := Key(raw); got != want {
		t.Fatalf("long NFC: %q", got)
	}
	if got := Key(strings.Repeat("\u0301", 40) + "A"); got != strings.Repeat("\u0301", 40)+"a" {
		t.Fatalf("leading marks: %q", got)
	}
	if got := Key(raw + "\u0327"); got != "á\u0327"+strings.Repeat("\u0301", 39) {
		t.Fatalf("canonical reordering/composition: %q", got)
	}
	if got := Key("\u1100\u1161\u11a8" + strings.Repeat("\u0301", 40)); got != "각"+strings.Repeat("\u0301", 40) {
		t.Fatalf("Hangul composition: %q", got)
	}
}
