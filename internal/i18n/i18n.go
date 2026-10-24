// Package i18n loads complete embedded UI catalogs and negotiates browser tags.
package i18n

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/valpere/slovnyk/web"
	"golang.org/x/text/language"
)

type Locale struct {
	Code     string            `json:"code"`
	Name     string            `json:"native_name"`
	Messages map[string]string `json:"messages"`
}

var catalogs, locales, matcher = load()

func load() (map[string]Locale, []Locale, language.Matcher) {
	entries, err := web.Locales.ReadDir("locales")
	if err != nil {
		panic("missing interface catalogs")
	}
	all := map[string]Locale{}
	for _, entry := range entries {
		raw, err := web.Locales.ReadFile("locales/" + entry.Name())
		if err != nil {
			panic("unreadable interface catalog")
		}
		var c Locale
		if json.Unmarshal(raw, &c) != nil || c.Code == "" || c.Name == "" || all[c.Code].Code != "" {
			panic("invalid interface catalog")
		}
		all[c.Code] = c
	}
	en, ok := all["en"]
	if !ok || len(en.Messages) == 0 {
		panic("missing English fallback")
	}
	codes := []string{"en"}
	for code := range all {
		if code != "en" {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes[1:])
	ordered := make([]Locale, 0, len(codes))
	tags := make([]language.Tag, 0, len(codes))
	for _, code := range codes {
		c := all[code]
		if len(c.Messages) != len(en.Messages) {
			panic("incomplete interface catalog")
		}
		for key := range en.Messages {
			if strings.TrimSpace(c.Messages[key]) == "" {
				panic("empty interface translation")
			}
		}
		ordered = append(ordered, c)
		// The legacy UI identifier no contains Bokmål translations. Matching
		// the generic no tag can otherwise prefer Danish for nb-NO.
		matchCode := code
		if code == "no" {
			matchCode = "nb"
		}
		tags = append(tags, language.MustParse(matchCode))
	}
	return all, ordered, language.NewMatcher(tags)
}

// Supported accepts only published UI identifiers, not dictionary-locale aliases.
func Supported(code string) bool { _, ok := catalogs[code]; return ok }
func Choices() []Locale {
	out := make([]Locale, len(locales))
	for i, c := range locales {
		out[i] = Locale{Code: c.Code, Name: c.Name}
	}
	return out
}
func Text(code, key string) string {
	if c, ok := catalogs[code]; ok {
		if value := c.Messages[key]; value != "" {
			return value
		}
	}
	return catalogs["en"].Messages[key]
}
func Messages(code string) map[string]string {
	out := map[string]string{}
	for key := range catalogs["en"].Messages {
		out[key] = Text(code, key)
	}
	return out
}
func Native(code string) string {
	base := strings.ToLower(strings.Split(code, "-")[0])
	if c, ok := catalogs[base]; ok {
		return c.Name + " (" + code + ")"
	}
	return code
}

// Select applies explicit supported UI > supported saved choice > browser > en.
// The bool is true only for an explicit supported choice, eligible to persist.
func Select(explicit, saved, accept string) (string, bool) {
	if Supported(explicit) {
		return explicit, true
	}
	if Supported(saved) {
		return saved, false
	}
	tags, _, err := language.ParseAcceptLanguage(accept)
	if err == nil && len(tags) > 0 {
		_, index, confidence := matcher.Match(tags...)
		if confidence >= language.High {
			return locales[index].Code, false
		}
	}
	return "en", false
}
