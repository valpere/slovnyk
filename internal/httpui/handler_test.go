package httpui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/normalization"
)

func signer(t *testing.T) *lookup.Signer {
	t.Helper()
	s, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestValidationBeforeLookup(t *testing.T) {
	s := signer(t)
	called := 0
	h := New(func(context.Context, lookup.Request) (lookup.Page, error) { called++; return lookup.Page{}, nil }, s)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", 200}, {"q=%20", 200}, {"q=&mode=bad", 400}, {"mode=", 400}, {"limit=", 400}, {"limit=0", 400}, {"limit=101", 400}, {"limit=%2B1", 400}, {"limit=1.0", 400}, {"format=", 400}, {"q=a", 400}, {"from=&q=", 400}, {"to=&q=", 400}, {"cursor=", 400}, {"cursor=forged&q=a&from=x&to=y", 400}, {"q=a&q=b", 400}, {"ui=en&ui=en", 400}, {"ignored=%ff", 400}, {"%ff=x", 400}, {"ignored=%00", 400}, {"ignored=%ZZ", 400}, {"q=%01", 400}, {"q=%7f", 400}, {"q=" + strings.Repeat("a", 1025), 400}, {"ignored=" + strings.Repeat("x", MaxTargetBytes), 414},
	} {
		t.Run(tc.query[:min(60, len(tc.query))], func(t *testing.T) {
			r := httptest.NewRequest("GET", "/search", nil)
			r.URL.RawQuery = tc.query
			r.RequestURI = "/search?" + tc.query
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d want %d", w.Code, tc.status)
			}
		})
	}
	if called != 0 {
		t.Fatalf("invalid/empty requests reached lookup %d times", called)
	}
}

func TestDecodedOnceAndContinuation(t *testing.T) {
	s := signer(t)
	token, e := s.Encode(lookup.Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Fingerprint: strings.Repeat("a", 64), Anchor: 1, Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	raw := "<script>&%2F\t\n"
	var got lookup.Request
	h := New(func(_ context.Context, r lookup.Request) (lookup.Page, error) {
		got = r
		return lookup.Page{Groups: []lookup.Group{{Source: raw, Translations: []string{"<img src=x>"}}}, HasMore: true, Cursor: token}, nil
	}, s)
	query := url.Values{"q": {raw}, "from": {"uk-ua"}, "to": {"en-gb"}, "mode": {"exact"}, "limit": {"2"}, "ignored": {"ok", "also ok"}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/search?"+query.Encode(), nil))
	if w.Code != 200 || got.Q != raw || got.Mode != normalization.Exact || got.Limit != 2 {
		t.Fatalf("request changed: %d %+v", w.Code, got)
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "<img src=x>") || !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "rel=\"next\"") {
		t.Fatal("escaping or continuation missing", body)
	}
	if w.Header().Get("X-Robots-Tag") != "noindex, follow" {
		t.Fatal("missing search crawl guard")
	}
	parsed := request{Search: got}
	next := nextURL(parsed, token)
	u, e := url.Parse(next)
	if e != nil {
		t.Fatal(e)
	}
	if u.Query().Get("q") != raw || u.Query().Get("cursor") != token || u.Query().Has("ignored") {
		t.Fatal("continuation changed query", next)
	}
	query.Set("format", "plain")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/search?"+query.Encode(), nil))
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || !strings.Contains(w.Body.String(), raw+"\n\t<img src=x>\n") || !strings.Contains(w.Body.String(), "format=plain") {
		t.Fatal("plain response changed data", w.Body.String())
	}
}

func TestErrorsAndMethods(t *testing.T) {
	for _, err := range []error{lookup.ErrLocale, lookup.ErrPair, lookup.ErrCursor, lookup.ErrGone, lookup.ErrUnavailable, errors.New("synthetic-private-driver-message")} {
		h := New(func(context.Context, lookup.Request) (lookup.Page, error) { return lookup.Page{}, err }, signer(t))
		for _, format := range []string{"html", "plain"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/search?from=x&to=y&q=a&format="+format, nil))
			if w.Code != lookup.Status(err) || strings.Contains(w.Body.String(), "synthetic-private") {
				t.Fatal("unsafe error", w.Code, w.Body.String())
			}
		}
	}
	h := New(nil, nil)
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"HEAD", "/", 200}, {"POST", "/search", 405}, {"GET", "/unknown", 404}, {"GET", "/search?from=x&to=y&q=a", 503}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
func TestFinalBodyBudget(t *testing.T) {
	// A faulty upstream page must never cause partially committed 200 headers.
	h := New(func(context.Context, lookup.Request) (lookup.Page, error) {
		return lookup.Page{Groups: []lookup.Group{{Source: strings.Repeat("&", lookup.MaxResponseBytes), Translations: []string{"x"}}}}, nil
	}, signer(t))
	for _, format := range []string{"html", "plain"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/search?from=x&to=y&q=a&format="+format, nil))
		if w.Code != 503 || w.Body.Len() > lookup.MaxResponseBytes || strings.Contains(w.Body.String(), "&&&&") {
			t.Fatal("response exceeded budget or committed partial output", format, w.Code, w.Body.Len())
		}
	}
	b := &boundedBuffer{}
	if _, e := b.Write([]byte(strings.Repeat("x", lookup.MaxResponseBytes))); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Write([]byte("x")); e == nil || b.Len() != lookup.MaxResponseBytes {
		t.Fatal("boundary not enforced")
	}
}

func TestValidCursorWithEmptyQueryRejected(t *testing.T) {
	s := signer(t)
	token, e := s.Encode(lookup.Cursor{Dataset: "00000000-0000-0000-0000-000000000001", Fingerprint: strings.Repeat("a", 64), Anchor: 1, Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodGet, "/search?cursor="+url.QueryEscape(token), nil)
	_, status := parse(r, s)
	if status != 400 {
		t.Fatal("empty cursor query accepted", status)
	}
}

func TestTargetLimitBeforeRouting(t *testing.T) {
	handler := New(nil, nil)
	for _, path := range []string{"/search", "//search", "/a/../search"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path+"?ignored="+strings.Repeat("x", MaxTargetBytes), nil))
		if response.Code != 414 || response.Header().Get("Location") != "" {
			t.Fatal("oversized target routed or redirected", path, response.Code)
		}
	}
}
