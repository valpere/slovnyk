package httpui

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/i18n"
	"github.com/valpere/slovnyk/internal/lookup"
	"golang.org/x/text/language"
)

type EntryFunc func(context.Context, string, string) (entries.Page, error)

func WithEntries(read EntryFunc) Option { return func(o *options) { o.entry = read } }

// Metadata may summarize long text; the visible dictionary content stays raw.
func snippet(s string, max int) string {
	r := []rune(strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " "))
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return string(r)
}

func contentLang(raw string) string {
	tag, err := language.Parse(raw)
	if err != nil || tag == language.Und {
		return ""
	}
	return tag.String()
}

func entryHandler(read EntryFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, follow")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fail := func(status int) {
			w.WriteHeader(status)
			if r.Method != "HEAD" {
				_, _ = io.WriteString(w, http.StatusText(status)+"\n")
			}
		}
		id := r.PathValue("id")
		if !entries.ValidID(id) {
			fail(404)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			fail(400)
			return
		}
		for key, values := range q {
			if key != "cursor" || len(values) != 1 || values[0] == "" || len(values[0]) > lookup.MaxCursorBytes || !validText(values[0]) {
				fail(400)
				return
			}
		}
		if read == nil {
			fail(503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), lookup.DBTimeout)
		defer cancel()
		p, err := read(ctx, id, q.Get("cursor"))
		if err != nil {
			fail(lookup.Status(err))
			return
		}
		lang := strings.ToLower(strings.Split(p.From, "-")[0])
		if !i18n.Supported(lang) {
			lang = "en"
		}
		data := view{IsEntry: true, Entry: p, Lang: lang, Text: i18n.Messages(lang), UIAction: "/", SourceLang: contentLang(p.From), TargetLang: contentLang(p.To)}
		data.Title = snippet(p.Source, 100) + " · " + i18n.Native(p.From) + " → " + i18n.Native(p.To) + " · Slovnyk"
		data.Description = snippet(p.Source, 120) + " → " + i18n.Native(p.To)
		if len(p.Translations) > 0 {
			data.Description += ": " + snippet(p.Translations[0], 120)
		}
		data.Canonical = "https://slovnyk.org" + entries.Path(id)
		if p.Cursor != "" {
			data.Next = entries.Path(id) + "?" + url.Values{"cursor": {p.Cursor}}.Encode()
		}
		for _, c := range i18n.Choices() {
			data.UILocales = append(data.UILocales, choice{Code: c.Code, Label: c.Name, Selected: c.Code == lang})
		}
		body := &boundedBuffer{}
		if err = pageTemplate.Execute(body, data); err != nil {
			fail(503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
		w.Header().Set("Content-Language", lang)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		if q.Get("cursor") == "" {
			w.Header().Del("X-Robots-Tag")
		}
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			_, _ = w.Write(body.Bytes())
		}
	}
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	const body = "User-agent: *\nAllow: /\nSitemap: https://slovnyk.org/sitemap.xml\n"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if r.Method != "HEAD" {
		_, _ = io.WriteString(w, body)
	}
}

// The only public alias redirects to a fixed origin. Forwarded headers never
// select the origin. Local hosts remain usable for development and probes.
func primaryDomain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if strings.EqualFold(strings.TrimSuffix(host, "."), "slovnyk.org.ua") {
			target := &url.URL{Scheme: "https", Host: "slovnyk.org", Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: r.URL.RawQuery}
			http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
