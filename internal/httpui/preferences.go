package httpui

import (
	"encoding/base64"
	"net"
	"net/http"
	"strings"

	"github.com/valpere/slovnyk/internal/catalog"
	"golang.org/x/text/language"
)

const uiCookie = "slovnyk_ui"
const pairCookie = "slovnyk_pair"
const preferenceAge = 365 * 24 * 60 * 60

func cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
func preference(r *http.Request, name, value string) *http.Cookie {
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	secure := r.TLS != nil || strings.EqualFold(host, "slovnyk.org") || strings.EqualFold(host, "slovnyk.org.ua")
	return &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: preferenceAge, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure}
}
func encodePair(from, to string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(from + "\x00" + to))
}
func savedPair(r *http.Request) (string, string, bool) {
	raw := cookie(r, pairCookie)
	if len(raw) == 0 || len(raw) > 180 {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || len(parts[0]) > 64 || len(parts[1]) > 64 || !validText(parts[0]) || !validText(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// propose never rewrites endpoint parameters. An exact regional source wins;
// otherwise the primary language must identify one source with one target.
// Ambiguous inventory yields no suggestion instead of an arbitrary region.
func propose(s catalog.Snapshot, accept string) (string, string) {
	tags, _, err := language.ParseAcceptLanguage(accept)
	if err != nil {
		return "", ""
	}
	if len(tags) == 0 {
		if accept != "" {
			return "", ""
		}
		tags = []language.Tag{language.English}
	}
	for _, tag := range tags {
		base, _ := tag.Base()
		if base.String() == "und" {
			continue
		}
		target := "en"
		if base.String() == "en" {
			target = "uk"
		}
		exact := []catalog.Pair{}
		broad := []catalog.Pair{}
		for _, p := range s.Pairs {
			if primary(p.From) == primary(base.String()) && primary(p.To) == target {
				broad = append(broad, p)
				if strings.EqualFold(p.From, tag.String()) {
					exact = append(exact, p)
				}
			}
		}
		if len(exact) == 1 {
			return exact[0].From, exact[0].To
		}
		if len(exact) == 0 && len(broad) == 1 {
			return broad[0].From, broad[0].To
		}
		// First browser language with matching directions is authoritative. Do not
		// hide regional ambiguity by proposing a lower-priority browser language.
		if len(broad) > 0 {
			return "", ""
		}
	}
	return "", ""
}
func primary(code string) string {
	base := strings.ToLower(strings.Split(code, "-")[0])
	// Legacy dictionary no-* represents Norwegian; browsers commonly send nb.
	// This only proposes an existing raw pair, never rewrites endpoint codes.
	if base == "nb" {
		return "no"
	}
	return base
}
