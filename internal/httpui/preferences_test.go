package httpui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/valpere/slovnyk/internal/catalog"
	"github.com/valpere/slovnyk/internal/lookup"
)

func inventory() catalog.Snapshot {
	return catalog.Snapshot{Dataset: "00000000-0000-0000-0000-000000000001", Locales: []string{"en-us", "uk-ua", "sr-rs"}, Pairs: []catalog.Pair{{From: "uk-ua", To: "en-us", Count: 2}, {From: "en-us", To: "uk-ua", Count: 3}, {From: "sr-rs", To: "sr-rs", Count: 1}}}
}
func catalogOption(s catalog.Snapshot) Option {
	return WithCatalog(func(context.Context, string) (catalog.Snapshot, error) { return s, nil })
}
func hasSelected(body, code string) bool { return strings.Contains(body, `value="`+code+`" selected`) }
func TestPreferencesAndExplicitPairs(t *testing.T) {
	h := New(nil, signer(t), catalogOption(inventory()))
	run := func(path, accept, saved string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept-Language", accept)
		if saved != "" {
			r.AddCookie(&http.Cookie{Name: pairCookie, Value: saved})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	first := run("/", "uk-UA", "")
	if first.Code != 200 || first.Header().Get("Content-Language") != "uk" || !hasSelected(first.Body.String(), "uk-ua") || !hasSelected(first.Body.String(), "en-us") || len(first.Result().Cookies()) != 0 {
		t.Fatal("browser proposal failed or persisted automatically", first.Body.String())
	}
	saved := run("/", "en-US", encodePair("sr-rs", "sr-rs"))
	if !hasSelected(saved.Body.String(), "sr-rs") || hasSelected(saved.Body.String(), "en-us") {
		t.Fatal("saved pair ignored")
	}
	unavailable := run("/", "en-US", encodePair("gone", "en-us"))
	if hasSelected(unavailable.Body.String(), "en-us") || !strings.Contains(unavailable.Body.String(), "saved language pair is unavailable") {
		t.Fatal("unavailable preference silently replaced")
	}
	explicit := run("/?from=en-us&to=uk-ua", "uk", encodePair("sr-rs", "sr-rs"))
	if !hasSelected(explicit.Body.String(), "en-us") || hasSelected(explicit.Body.String(), "sr-rs") {
		t.Fatal("explicit pair ignored")
	}
	cookies := explicit.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != pairCookie || cookies[0].MaxAge != preferenceAge || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("pair persistence missing", cookies)
	}
	partial := run("/?from=uk-ua", "en-US", encodePair("sr-rs", "sr-rs"))
	if hasSelected(partial.Body.String(), "sr-rs") || len(partial.Result().Cookies()) != 0 {
		t.Fatal("partial explicit pair was guessed")
	}
	// Saved preferences are for prefill only, never missing endpoint parameters.
	missing := run("/search?q=test", "en-US", encodePair("en-us", "uk-ua"))
	if missing.Code != 400 {
		t.Fatal("endpoint silently filled pair", missing.Code)
	}
}
func TestUIChoiceAndCookies(t *testing.T) {
	h := New(nil, signer(t), catalogOption(inventory()))
	for _, tc := range []struct {
		explicit, saved, browser, want string
		persist                        bool
	}{{"fr", "uk", "de", "fr", true}, {"removed", "uk", "de", "uk", false}, {"", "removed", "uk-UA", "uk", false}, {"", "", "fr-CA", "fr", false}} {
		r := httptest.NewRequest("GET", "http://slovnyk.org/?ui="+tc.explicit, nil)
		r.Header.Set("Accept-Language", tc.browser)
		r.AddCookie(&http.Cookie{Name: uiCookie, Value: tc.saved})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("Content-Language") != tc.want || !strings.Contains(w.Body.String(), `<html lang="`+tc.want+`"`) {
			t.Fatal("UI priority failed", tc, w.Body.String())
		}
		cookies := w.Result().Cookies()
		if tc.persist {
			if len(cookies) != 1 || cookies[0].Name != uiCookie || cookies[0].Value != tc.want || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].MaxAge != preferenceAge {
				t.Fatal("unsafe manual cookie", cookies)
			}
		} else if len(cookies) != 0 {
			t.Fatal("automatic or invalid choice persisted", cookies)
		}
	}
}
func TestCatalogRouteAndPinnedSearch(t *testing.T) {
	s := inventory()
	calls := 0
	read := WithCatalog(func(_ context.Context, dataset string) (catalog.Snapshot, error) {
		calls++
		if dataset != "" && dataset != s.Dataset {
			t.Fatal("wrong pin", dataset)
		}
		return s, nil
	})
	var deadline time.Time
	search := func(ctx context.Context, r lookup.Request) (lookup.Page, error) {
		deadline, _ = ctx.Deadline()
		return lookup.Page{Dataset: s.Dataset, Groups: []lookup.Group{}}, nil
	}
	h := New(search, signer(t), read)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/languages?ui=uk", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Мовні пари") || !strings.Contains(w.Body.String(), "from=uk-ua&amp;to=en-us") || strings.Contains(w.Body.String(), `action="/search"`) {
		t.Fatal("missing pair inventory", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/languages?format=plain", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "uk-ua") || !strings.Contains(w.Body.String(), "\t2") {
		t.Fatal("plain catalog failed")
	}
	h = New(search, signer(t), WithCatalog(func(ctx context.Context, pin string) (catalog.Snapshot, error) {
		d, _ := ctx.Deadline()
		if !d.Equal(deadline) || pin != s.Dataset {
			t.Fatal("search/catalog deadline or pin differs")
		}
		return s, nil
	}))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/search?from=uk-ua&to=en-us&q=x&ui=uk", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Збігів немає") {
		t.Fatal("localized no-match missing")
	}
	invalidCalls := calls
	h = New(nil, signer(t), read)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/?mode=invalid", nil))
	if w.Code != 400 || calls != invalidCalls {
		t.Fatal("catalog ran before structure validation")
	}
	h = New(nil, signer(t), WithCatalog(func(context.Context, string) (catalog.Snapshot, error) {
		return catalog.Snapshot{}, lookup.ErrUnavailable
	}))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/languages", nil))
	if w.Code != 503 {
		t.Fatal("catalog error shown as empty success")
	}
}
func TestProposalAmbiguityAndMalformedPreferences(t *testing.T) {
	s := inventory()
	for _, tc := range []struct{ accept, from, to string }{{"uk-UA", "uk-ua", "en-us"}, {"en-US", "en-us", "uk-ua"}, {"", "en-us", "uk-ua"}, {"uk;q=0,en-US;q=1", "en-us", "uk-ua"}, {"zh-CN", "", ""}, {"*", "", ""}, {"not valid", "", ""}} {
		from, to := propose(s, tc.accept)
		if from != tc.from || to != tc.to {
			t.Fatal("proposal mismatch", tc, from, to)
		}
	}
	s.Pairs = append(s.Pairs, catalog.Pair{From: "en-gb", To: "uk-ua"})
	if from, _ := propose(s, "en-AU"); from != "" {
		t.Fatal("ambiguous region selected")
	}
	if from, _ := propose(s, "en-US"); from != "en-us" {
		t.Fatal("exact browser region lost")
	}
	for _, raw := range []string{"broken!", encodePair("", "uk-ua"), encodePair("en-us", strings.Repeat("x", 65))} {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: pairCookie, Value: raw})
		if _, _, valid := savedPair(r); valid {
			t.Fatal("malformed pair accepted", raw)
		}
	}
	raw := "%2F& <x>"
	r := request{Search: lookup.Request{From: "uk-ua", To: "en-us", Q: raw, Mode: "auto", Limit: 50}, UI: "uk"}
	u, _ := url.Parse(nextURL(r, "token"))
	if u.Query().Get("ui") != "uk" || u.Query().Get("q") != raw {
		t.Fatal("continuation changed UI or raw query")
	}
}

func TestUnconfiguredCatalogNotice(t *testing.T) {
	for _, path := range []string{"/", "/languages"} {
		w := httptest.NewRecorder()
		New(nil, signer(t)).ServeHTTP(w, httptest.NewRequest("GET", path+"?ui=uk", nil))
		want := 200
		if path == "/languages" {
			want = 503
		}
		if w.Code != want || !strings.Contains(w.Body.String(), "Каталог мов тимчасово недоступний") {
			t.Fatal("unconfigured catalog hidden", path, w.Code)
		}
	}
}

func TestBokmalProposalUsesExistingRawCodes(t *testing.T) {
	s := catalog.Snapshot{Pairs: []catalog.Pair{{From: "no-no", To: "en-us"}}}
	from, to := propose(s, "nb-NO")
	if from != "no-no" || to != "en-us" {
		t.Fatal("Norwegian browser preference lost", from, to)
	}
}

func TestCatalogFailurePreservesSearch(t *testing.T) {
	h := New(func(context.Context, lookup.Request) (lookup.Page, error) {
		return lookup.Page{Dataset: inventory().Dataset, Groups: []lookup.Group{{Source: "слово", Translations: []string{"word"}}}}, nil
	}, signer(t), WithCatalog(func(context.Context, string) (catalog.Snapshot, error) {
		return catalog.Snapshot{}, lookup.ErrUnavailable
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/search?from=uk-ua&to=en-us&q=x&ui=uk", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Каталог мов тимчасово недоступний") || !strings.Contains(w.Body.String(), "word") {
		t.Fatal("catalog failure discarded valid results", w.Code)
	}
}
