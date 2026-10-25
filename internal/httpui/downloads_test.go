package httpui

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/downloads"
	"github.com/valpere/slovnyk/internal/lookup"
)

func exportFixture() downloads.Export {
	return downloads.Export{Dataset: "00000000-0000-4000-8000-000000000001", ID: "00000000-0000-4000-8000-000000000002", From: "uk-ua", To: "en-us", Rows: 2, Artifact: artifacts.Ref{SHA256: strings.Repeat("a", 64), Bytes: 20}}
}
func fileURL(e downloads.Export) string { return "/downloads/" + e.Dataset + "/" + e.ID }

func TestDownloadManifest(t *testing.T) {
	e := exportFixture()
	list := func(ctx context.Context) (downloads.Manifest, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing metadata deadline")
		}
		return downloads.Manifest{Dataset: e.Dataset, Exports: []downloads.Export{e}}, nil
	}
	h := New(nil, signer(t), WithDownloads(list, nil))
	for _, format := range []string{"html", "plain"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/downloads?ui=uk&format="+format, nil))
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, e.Artifact.SHA256) || !strings.Contains(body, fileURL(e)) || !strings.Contains(body, e.Dataset) || w.Header().Get("Content-Language") != "uk" {
			t.Fatal("incomplete download manifest", w.Code, body)
		}
		if format == "html" && (!strings.Contains(body, "Завантаження словників") || strings.Contains(body, `action="/search"`)) {
			t.Fatal("wrong download page")
		}
	}
	w := httptest.NewRecorder()
	New(nil, signer(t)).ServeHTTP(w, httptest.NewRequest("HEAD", "/downloads", nil))
	if w.Code != 503 || w.Body.Len() != 0 {
		t.Fatal("unconfigured download index", w.Code)
	}
	// Bounded manifest rendering must not commit partial successful output.
	e.From = strings.Repeat("&", lookup.MaxResponseBytes)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/downloads?ui=uk", nil))
	if w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("manifest budget bypassed")
	}
}

func TestPlainManifestPreservesRawLocales(t *testing.T) {
	e := exportFixture()
	e.From, e.To = "raw\tlocale\r\n", "quote\"\\locale"
	var body bytes.Buffer
	if err := plainDownloads(&body, downloads.Manifest{Dataset: e.Dataset, Exports: []downloads.Export{e}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(body.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatal("locale injected manifest records", body.String())
	}
	fields := strings.Split(lines[2], "\t")
	if len(fields) != 6 {
		t.Fatal("locale injected manifest fields", fields)
	}
	for i, want := range []string{e.From, e.To} {
		got, err := strconv.Unquote(fields[i])
		if err != nil || got != want {
			t.Fatal("raw locale changed", got, err)
		}
	}
}

func TestExportHTTPRepresentation(t *testing.T) {
	e := exportFixture()
	payload := "synthetic-gzip-bytes"
	path := filepath.Join(t.TempDir(), "export")
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	var opened *os.File
	open := func(ctx context.Context, dataset, id string) (downloads.Export, *os.File, error) {
		if dataset != e.Dataset || id != e.ID {
			t.Fatal("request identity changed")
		}
		d, ok := ctx.Deadline()
		if !ok || time.Until(d) > lookup.DBTimeout {
			t.Fatal("metadata deadline missing")
		}
		f, err := os.Open(path)
		opened = f
		return e, f, err
	}
	h := New(nil, signer(t), WithDownloads(nil, open))
	for _, tc := range []struct {
		method, header, value string
		status                int
		body                  string
	}{
		{"GET", "", "", 200, payload}, {"HEAD", "", "", 200, ""}, {"GET", "Range", "bytes=2-5", 206, payload[2:6]},
		{"GET", "If-None-Match", `"` + e.Artifact.SHA256 + `"`, 304, ""},
		{"GET", "Range", "bytes=999-", 416, "invalid range: failed to overlap\n"},
	} {
		r := httptest.NewRequest(tc.method, fileURL(e)+"?path=../../ignored&filename=ignored", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || w.Body.String() != tc.body {
			t.Fatalf("%s %s: %d %q", tc.method, tc.header, w.Code, w.Body.String())
		}
		if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatal("file leaked")
		}
		if tc.status >= 400 && (w.Header().Get("Content-Disposition") != "" || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("cached or attachment error", w.Header())
		}
		if tc.status == 200 || tc.status == 206 {
			if w.Header().Get("Content-Type") != "application/gzip" || w.Header().Get("Content-Encoding") != "" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || w.Header().Get("ETag") != `"`+e.Artifact.SHA256+`"` || len(w.Result().Cookies()) != 0 {
				t.Fatal("wrong export headers", w.Header())
			}
		}
	}
}

func TestExportFailuresAndAdmission(t *testing.T) {
	e := exportFixture()
	calls := 0
	for _, tc := range []struct {
		path   string
		err    error
		status int
	}{
		{"/downloads/../outside", nil, 307}, {"/downloads/not-a-uuid/not-a-uuid", nil, 404},
		{fileURL(e) + "?bad=%FF", nil, 400}, {fileURL(e), downloads.ErrNotFound, 404}, {fileURL(e), lookup.ErrUnavailable, 503},
	} {
		h := New(nil, signer(t), WithDownloads(nil, func(context.Context, string, string) (downloads.Export, *os.File, error) {
			calls++
			return downloads.Export{}, nil, tc.err
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatal(tc.path, w.Code)
		}
		if tc.status != 307 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cached failure")
		}
	}
	if calls != 2 {
		t.Fatal("bad input reached export backend", calls)
	}
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var wg sync.WaitGroup
	h := New(nil, signer(t), WithDownloads(nil, func(ctx context.Context, _, _ string) (downloads.Export, *os.File, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return downloads.Export{}, nil, lookup.ErrUnavailable
	}))
	defer close(release)
	for i := 0; i < 8; i++ {
		wg.Go(func() { w := httptest.NewRecorder(); h.ServeHTTP(w, httptest.NewRequest("GET", fileURL(e), nil)) })
	}
	for i := 0; i < 8; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("admission did not reach eight")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", fileURL(e), nil))
	if w.Code != 503 {
		t.Fatal("transfer bound missing", w.Code)
	}
	// Return promptly; blocked handlers also release on their bounded contexts.
	// release is closed by defer, and completion is collected in cleanup.
	t.Cleanup(wg.Wait)
}

// Delay the first response chunk past the server's ordinary total deadline.
// Export streaming must use its renewable deadline instead.
type delayedWriter struct {
	http.ResponseWriter
	delayed bool
}

func (w *delayedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *delayedWriter) Write(p []byte) (int, error) {
	if !w.delayed {
		w.delayed = true
		time.Sleep(40 * time.Millisecond)
	}
	return w.ResponseWriter.Write(p)
}
func TestLargeExportOutlivesSearchWriteTimeout(t *testing.T) {
	var compressed bytes.Buffer
	z, err := gzip.NewWriterLevel(&compressed, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = z.Write(bytes.Repeat([]byte("\"uk-ua\",\"a\",\"en-us\",\"b\"\n"), 50000)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if compressed.Len() <= lookup.MaxResponseBytes {
		t.Fatal("fixture must exceed page budget")
	}
	path := filepath.Join(t.TempDir(), "large.csv.gz")
	if err = os.WriteFile(path, compressed.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	e := exportFixture()
	sum := sha256.Sum256(compressed.Bytes())
	e.Artifact.SHA256 = hex.EncodeToString(sum[:])
	e.Artifact.Bytes = int64(compressed.Len())
	h := New(nil, signer(t), WithDownloads(nil, func(context.Context, string, string) (downloads.Export, *os.File, error) {
		f, err := os.Open(path)
		return e, f, err
	}))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(&delayedWriter{ResponseWriter: w}, r) }))
	server.Config.WriteTimeout = 10 * time.Millisecond
	server.Start()
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(server.URL + fileURL(e))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !bytes.Equal(body, compressed.Bytes()) {
		t.Fatal("large export truncated", response.StatusCode, len(body))
	}
}
