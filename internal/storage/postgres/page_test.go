package postgres

import (
	"strings"
	"testing"
)

func TestPrefixPattern(t *testing.T) {
	for _, tc := range []struct {
		pattern, want string
		ok            bool
	}{
		{"", "", false}, {"%abc", "", false}, {"_abc", "", false},
		{"abc%def", "abc%", true}, {"abc_def", "abc%", true},
		{`a\%b\_c\\d%`, `a\%b\_c\\d%`, true},
		{`\%_`, `\%%`, true}, {`abc\`, "", false},
		{strings.Repeat("😀", 65) + "%", strings.Repeat("😀", 64) + "%", true},
		{strings.Repeat("é", 63) + `\%extra`, strings.Repeat("é", 63) + `\%%`, true},
		{strings.Repeat("x", 64) + "_", strings.Repeat("x", 64) + "%", true},
		{"i\u0307%", "i\u0307%", true}, {" edge %", " edge %", true},
		{"literal", "literal%", true},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			got, ok := prefixPattern(tc.pattern)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
