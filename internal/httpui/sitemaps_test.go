package httpui

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/sitemaps"
)

func TestSitemapHTTP(t *testing.T) {
	if err := os.MkdirAll("../../tmp/sitemap-http", 0700); err != nil {
		t.Fatal(err)
	}
	path, err := os.CreateTemp("../../tmp/sitemap-http", "xml-")
	if err != nil {
		t.Fatal(err)
	}
	name := path.Name()
	defer os.Remove(name)
	body := "<?xml version=\"1.0\"?><urlset/>\n"
	if _, err := path.WriteString(body); err != nil {
		t.Fatal(err)
	}
	_ = path.Close()
	dataset := "00000000-0000-0000-0000-000000000001"
	hash := strings.Repeat("a", 64)
	shardURL := "/sitemap-" + dataset + "-" + hash + ".xml"
	calls := 0
	open := func(ctx context.Context, d, h string) (artifacts.Ref, *os.File, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("metadata deadline missing")
		}
		if (d != "" || h != "") && (d != dataset || h != hash) {
			t.Fatal("wrong identities")
		}
		f, err := os.Open(name)
		return artifacts.Ref{SHA256: hash, Bytes: int64(len(body))}, f, err
	}
	handler := New(nil, signer(t), WithSitemaps(open))
	for _, url := range []string{"/sitemap.xml", shardURL} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, url, nil)
			r.Header.Set("Cookie", "slovnyk_ui=uk")
			r.Header.Set("Accept-Language", "uk")
			r.Header.Set("X-Forwarded-Host", "evil.example")
			handler.ServeHTTP(w, r)
			if w.Code != 200 || w.Header().Get("Content-Type") != "application/xml; charset=utf-8" || w.Header().Get("ETag") != `"`+hash+`"` || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Vary") != "" {
				t.Fatalf("invalid sitemap: %d %v", w.Code, w.Header())
			}
			if method == "HEAD" && w.Body.Len() != 0 || method == "GET" && w.Body.String() != body {
				t.Fatal("wrong XML body")
			}
			if url == "/sitemap.xml" && w.Header().Get("Cache-Control") != "no-store" || url == shardURL && !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("wrong cache policy")
			}
		}
	}
	for _, tc := range []struct {
		url    string
		status int
	}{
		{"/sitemap.xml?ui=uk", 400}, {shardURL + "?", 400}, {"/sitemap-bad.xml", 404}, {"/sitemap-" + dataset + "-" + strings.Repeat("A", 64) + ".xml", 404}, {"/unrelated", 404},
	} {
		before := calls
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
		if w.Code != tc.status || calls != before {
			t.Fatalf("invalid route reached backend: %s %d", tc.url, w.Code)
		}
	}
	for _, tc := range []struct {
		method, header, value string
		status                int
	}{
		{"GET", "If-None-Match", `"` + hash + `"`, 304},
		{"GET", "Range", "bytes=0-4", 206},
		{"GET", "Range", "bytes=99999-", 416},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, shardURL, nil)
		r.Header.Set(tc.header, tc.value)
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("conditional/range got %d want %d", w.Code, tc.status)
		}
		if tc.status >= 400 && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex, follow") {
			t.Fatal("cached range error")
		}
	}
	for _, err := range []error{sitemaps.ErrNotFound, errors.New("private database failure"), nil} {
		h := New(nil, signer(t), WithSitemaps(func(context.Context, string, string) (artifacts.Ref, *os.File, error) {
			return artifacts.Ref{}, nil, err
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", shardURL, nil))
		want := 503
		if errors.Is(err, sitemaps.ErrNotFound) {
			want = 404
		}
		if w.Code != want || strings.Contains(w.Body.String(), "private") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe failure", w.Code)
		}
	}
	w := httptest.NewRecorder()
	New(nil, signer(t)).ServeHTTP(w, httptest.NewRequest("GET", "/sitemap.xml", nil))
	if w.Code != 503 {
		t.Fatal("missing backend accepted")
	}
}
