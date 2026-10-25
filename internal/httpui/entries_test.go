package httpui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/lookup"
)

func TestEntryAnonymousRepresentation(t *testing.T) {
	p := entries.Page{From: "uk-ua", To: "en-us", Source: "<Слово>&\n", Translations: []string{"<word> & term"}, Cursor: "signed-next"}
	id := entries.ID(p.From, p.To, p.Source)
	h := New(nil, signer(t), WithEntries(func(ctx context.Context, got, cursor string) (entries.Page, error) {
		if got != id {
			t.Fatal("entry ID changed")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("metadata budget missing")
		}
		return p, nil
	}))
	var first string
	for _, personalized := range []bool{false, true} {
		r := httptest.NewRequest("GET", "https://untrusted.example"+entries.Path(id), nil)
		if personalized {
			r.Header.Set("Accept-Language", "de")
			r.AddCookie(&http.Cookie{Name: uiCookie, Value: "de"})
			r.AddCookie(&http.Cookie{Name: pairCookie, Value: encodePair("de", "fr")})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("X-Robots-Tag") != "" || w.Header().Get("Content-Language") != "uk" || len(w.Result().Cookies()) != 0 || w.Header().Get("Vary") != "" {
			t.Fatal("indexable representation changed", w.Code, w.Header())
		}
		body := w.Body.String()
		for _, want := range []string{`rel="canonical" href="https://slovnyk.org` + entries.Path(id) + `"`, `&lt;Слово&gt;&amp;`, `&lt;word&gt; &amp; term`, `lang="en-US"`, `?cursor=signed-next`, `name="description"`} {
			if !strings.Contains(body, want) {
				t.Fatal("missing entry content", want)
			}
		}
		if strings.Contains(body, "<word>") || strings.Contains(body, "untrusted.example") || strings.Contains(body, `action="/search"`) {
			t.Fatal("unsafe or incorrect entry content")
		}
		if personalized && first != body {
			t.Fatal("cookies/browser altered dictionary representation")
		}
		first = body
	}
	for _, tc := range []struct{ method, query string }{{"HEAD", ""}, {"GET", "?cursor=signed"}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, entries.Path(id)+tc.query, nil))
		if w.Code != 200 || tc.method == "HEAD" && w.Body.Len() != 0 || tc.query != "" && w.Header().Get("X-Robots-Tag") != "noindex, follow" {
			t.Fatal("HEAD/continuation indexing", w.Code, w.Header())
		}
	}
	// Final response guard applies before headers even to admitted raw text.
	p.Source = strings.Repeat("&", lookup.MaxResponseBytes)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", entries.Path(id), nil))
	if w.Code != 503 || w.Header().Get("X-Robots-Tag") != "noindex, follow" {
		t.Fatal("unbounded entry render")
	}
}

func TestEntryFailuresBeforeBackend(t *testing.T) {
	calls := 0
	h := New(nil, signer(t), WithEntries(func(context.Context, string, string) (entries.Page, error) {
		calls++
		return entries.Page{}, entries.ErrNotFound
	}))
	path := entries.Path(strings.Repeat("0", 64))
	for _, tc := range []struct {
		path string
		code int
	}{{"/word/bad", 404}, {path + "?ui=de", 400}, {path + "?cursor=", 400}, {path + "?cursor=x&cursor=y", 400}, {path + "?cursor=%FF", 400}, {path, 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.code || w.Header().Get("X-Robots-Tag") != "noindex, follow" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc.path, w.Code, w.Header())
		}
	}
	if calls != 1 {
		t.Fatal("invalid query reached backend", calls)
	}
	w := httptest.NewRecorder()
	New(nil, signer(t)).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 503 {
		t.Fatal("unconfigured entry", w.Code)
	}
}

func TestSearchLinksToRawDictionaryPages(t *testing.T) {
	h := New(func(context.Context, lookup.Request) (lookup.Page, error) {
		return lookup.Page{Groups: []lookup.Group{{Source: "Word", Translations: []string{"one"}}, {Source: "WORD", Translations: []string{"two"}}}}, nil
	}, signer(t))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/search?from=en-us&to=uk-ua&q=word", nil))
	for _, source := range []string{"Word", "WORD"} {
		if w.Code != 200 || !strings.Contains(w.Body.String(), `href="`+entries.Path(entries.ID("en-us", "uk-ua", source))+`"`) {
			t.Fatal("missing spelling-specific discovery link", source)
		}
	}
	if w.Header().Get("X-Robots-Tag") != "noindex, follow" {
		t.Fatal("search became indexable")
	}
}

func TestRobotsAndPrimaryDomain(t *testing.T) {
	h := New(nil, signer(t))
	for _, method := range []string{"GET", "HEAD"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "https://slovnyk.org/robots.txt", nil))
		if w.Code != 200 || method == "GET" && w.Body.String() != "User-agent: *\nAllow: /\n" || method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("robots blocked dictionary crawling")
		}
	}
	for _, host := range []string{"slovnyk.org.ua", "SLOVNYK.ORG.UA:443", "slovnyk.org.ua."} {
		r := httptest.NewRequest("GET", "https://"+host+"/search?q=a%2Fb&from=en&to=uk", nil)
		r.Header.Set("X-Forwarded-Host", "attacker.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 308 || w.Header().Get("Location") != "https://slovnyk.org/search?q=a%2Fb&from=en&to=uk" {
			t.Fatal("unsafe/lossy domain redirect", w.Code, w.Header())
		}
	}
	r := httptest.NewRequest("GET", "http://localhost/robots.txt", nil)
	r.Header.Set("X-Forwarded-Host", "slovnyk.org.ua")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("forwarded host controlled redirect")
	}
}
