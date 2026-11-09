package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/i18n"
)

func TestPreviewDownloadsMatchVisibleFixtures(t *testing.T) {
	handler, err := newPreview(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range fixtures() {
		t.Run(p.From+"-"+p.To, func(t *testing.T) {
			path := fmt.Sprintf("/downloads/%s/00000000-0000-0000-0000-%012d", dataset, i+2)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 200 || w.Header().Get("Content-Type") != "application/gzip" {
				t.Fatalf("download: %d %s", w.Code, w.Body.String())
			}
			sum := sha256.Sum256(w.Body.Bytes())
			if w.Header().Get("ETag") != `"`+hex.EncodeToString(sum[:])+`"` {
				t.Fatal("incorrect fixture hash")
			}
			gz, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			rows, err := csv.NewReader(gz).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(p.Translations) {
				t.Fatal("incorrect count")
			}
			for n, row := range rows {
				if strings.Join(row, "\x00") != strings.Join([]string{p.From, p.To, p.Source, p.Translations[n]}, "\x00") {
					t.Fatal("CSV differs from visible fixture")
				}
			}
			article := httptest.NewRecorder()
			handler.ServeHTTP(article, httptest.NewRequest("GET", entries.Path(entries.ID(p.From, p.To, p.Source)), nil))
			if article.Code != http.StatusOK {
				t.Fatal("missing fixture article")
			}
		})
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/downloads/00000000-0000-0000-0000-000000000099/00000000-0000-0000-0000-000000000002", nil))
	if w.Code != 404 {
		t.Fatal("unknown dataset exposes file")
	}
}

func TestPreviewStatesAndAllUICatalogs(t *testing.T) {
	handler, err := newPreview(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range i18n.Choices() {
		for _, tc := range []struct {
			q      string
			status int
			key    string
		}{{"hello", 200, ""}, {"missing", 200, "no_matches"}, {"failure", 503, "unavailable"}} {
			r := httptest.NewRequest("GET", "/search?from=en-us&to=uk-ua&q="+tc.q+"&ui="+c.Code, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Content-Language") != c.Code {
				t.Fatalf("%s/%s: %d", c.Code, tc.q, w.Code)
			}
			if tc.key != "" && !strings.Contains(w.Body.String(), i18n.Text(c.Code, tc.key)) {
				t.Fatalf("missing %s/%s", c.Code, tc.key)
			}
		}
	}
}
