package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/lookup"
)

func timingRecord(t *testing.T, raw string) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	allowed := strings.Fields("time level msg event status query_kind search_result failure_stage catalog_result write_failed handler_ms search_ms admission_ms connect_ms lookup_ms cleanup_ms catalog_ms")
	if len(record) != len(allowed) {
		t.Fatalf("unexpected timing fields: %v", record)
	}
	for _, key := range allowed {
		v, ok := record[key]
		if !ok {
			t.Fatalf("missing field %s", key)
		}
		if strings.HasSuffix(key, "_ms") && v.(float64) < 0 {
			t.Fatalf("negative %s", key)
		}
	}
	if record["event"] != "search_timing" {
		t.Fatal("incorrect event")
	}
	return record
}

func TestTimingPreservesHTTPAndExcludesPrivateData(t *testing.T) {
	signer, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		ui := httpui.New(func(context.Context, lookup.Request) (lookup.Page, error) {
			if fail {
				return lookup.Page{}, errors.New("private-backend-error")
			}
			return lookup.Page{Groups: []lookup.Group{{Source: "private-term", Translations: []string{"private-result"}}}}, nil
		}, signer)
		for _, method := range []string{"GET", "HEAD"} {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			plain, timed := httptest.NewRecorder(), httptest.NewRecorder()
			req := httptest.NewRequest(method, "/search?from=uk-ua&to=en-us&q=private-term&format=plain", nil)
			req.Header.Set("Cookie", "private-cookie=private-value")
			req.Header.Set("User-Agent", "private-agent")
			ui.ServeHTTP(plain, req)
			searchTimingHandler(ui, logger, true).ServeHTTP(timed, req)
			if plain.Code != timed.Code || plain.Body.String() != timed.Body.String() || !reflect.DeepEqual(plain.Header(), timed.Header()) {
				t.Fatal("timing changed HTTP response")
			}
			record := timingRecord(t, logs.String())
			if int(record["status"].(float64)) != timed.Code {
				t.Fatal("wrong status")
			}
			for _, secret := range []string{"private-", "uk-ua", "en-us", "/search", "q="} {
				if strings.Contains(logs.String(), secret) {
					t.Fatal("request/backend data leaked")
				}
			}
		}
	}
}

func TestTimingScopeAndLogLevel(t *testing.T) {
	for _, tc := range []struct {
		path, method string
		enabled      bool
		level        slog.Level
		want         bool
	}{
		{"/", "GET", true, slog.LevelInfo, true}, {"/search", "HEAD", true, slog.LevelInfo, true},
		{"/search", "POST", true, slog.LevelInfo, false}, {"/word/id", "GET", true, slog.LevelInfo, false},
		{"/healthz", "GET", true, slog.LevelInfo, false}, {"/exports/id", "GET", true, slog.LevelInfo, false},
		{"/search", "GET", false, slog.LevelInfo, false}, {"/search", "GET", true, slog.LevelWarn, false},
	} {
		var logs bytes.Buffer
		h := searchTimingHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }), slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: tc.level})), tc.enabled)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, tc.path, nil))
		if (logs.Len() > 0) != tc.want {
			t.Fatalf("incorrect scope: %+v", tc)
		}
		if tc.want {
			record := timingRecord(t, logs.String())
			if record["status"] != float64(503) || record["search_result"] != "not_run" {
				t.Fatal("incorrect unexecuted operation")
			}
		}
	}
}

type brokenTimingWriter struct{ header http.Header }

func (w *brokenTimingWriter) Header() http.Header     { return w.header }
func (*brokenTimingWriter) WriteHeader(int)           {}
func (*brokenTimingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestTimingObservesWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	h := searchTimingHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n, err := w.Write([]byte("private-body"))
		if n != 0 || !errors.Is(err, io.ErrClosedPipe) {
			t.Error("write result changed")
		}
	}), slog.New(slog.NewJSONHandler(&logs, nil)), true)
	h.ServeHTTP(&brokenTimingWriter{header: make(http.Header)}, httptest.NewRequest("GET", "/search", nil))
	record := timingRecord(t, logs.String())
	if record["write_failed"] != true || strings.Contains(logs.String(), "private-body") {
		t.Fatal("write failure not safely recorded")
	}
}
