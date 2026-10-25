package i18n

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/web"
)

func TestSelection(t *testing.T) {
	for _, tc := range []struct {
		explicit, saved, browser, want string
		manual                         bool
	}{
		{"uk", "fr", "de-DE", "uk", true}, {"xx", "uk", "de-DE", "uk", false}, {"", "removed", "fr-CA,uk;q=0.5", "fr", false}, {"", "", "uk-UA,en;q=0.5", "uk", false}, {"", "", "zh-CN", "en", false}, {"", "", "uk;q=0,en;q=1", "en", false}, {"", "", "not valid", "en", false}, {"", "", "nb-NO", "no", false},
		{"", "", "nn-NO", "no", false}, {"", "", "sr-Latn-RS", "en", false},
		{"", "", "sr-Cyrl-RS", "sr", false}, {"", "", "", "en", false},
	} {
		got, manual := Select(tc.explicit, tc.saved, tc.browser)
		if got != tc.want || manual != tc.manual {
			t.Fatalf("%+v got %s/%v", tc, got, manual)
		}
	}
}
func TestCatalogCompletenessAndProvenance(t *testing.T) {
	if len(Choices()) != 31 {
		t.Fatal("legacy language family inventory changed")
	}
	reused := 0
	for _, c := range Choices() {
		raw, err := web.Locales.ReadFile("locales/" + c.Code + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var meta struct {
			Status, LegacyFile, LegacySHA256 string
			Messages, Origins                map[string]string
		}
		// Metadata field names are explicit in the public resource format.
		var doc map[string]json.RawMessage
		if json.Unmarshal(raw, &doc) != nil {
			t.Fatal("invalid catalog")
		}
		json.Unmarshal(doc["status"], &meta.Status)
		json.Unmarshal(doc["legacy_file"], &meta.LegacyFile)
		json.Unmarshal(doc["legacy_sha256"], &meta.LegacySHA256)
		json.Unmarshal(doc["messages"], &meta.Messages)
		json.Unmarshal(doc["origins"], &meta.Origins)
		hash, err := hex.DecodeString(meta.LegacySHA256)
		if err != nil || len(hash) != 32 || meta.Status != "ai-draft" || !strings.HasPrefix(meta.LegacyFile, "slovnyk.") {
			t.Fatal("missing provenance", c.Code)
		}
		if len(meta.Messages) != 29 || len(meta.Origins) != 29 {
			t.Fatal("incomplete translation inventory", c.Code)
		}
		for key, value := range meta.Messages {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "<>\x00") {
				t.Fatal("empty or markup-bearing text", c.Code, key)
			}
			if origin := meta.Origins[key]; origin == "legacy-po" {
				reused++
			} else if origin != "ai" {
				t.Fatal("unknown translation origin", c.Code, key)
			}
			if Text(c.Code, key) != value {
				t.Fatal("translation not used", c.Code, key)
			}
		}
		text := Text(c.Code, "no_matches")
		if !strings.Contains(text, "*") || !strings.Contains(text, "?") {
			t.Fatal("missing mask hint", c.Code)
		}
	}
	if reused == 0 {
		t.Fatal("PO reuse lost")
	}
	en := Text("en", "invalid")
	if Text("removed", "invalid") != en {
		t.Fatal("English fallback lost")
	}
	values := Messages("uk")
	values["invalid"] = "tampered"
	if Text("uk", "invalid") == "tampered" {
		t.Fatal("caller mutated shared translations")
	}
	t.Logf("31 draft catalogs; %d messages reused from PO", reused)
}
