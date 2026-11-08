package ops

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/normalization"
)

// A record belongs to one request. The HTTP adapter calls search/catalog
// synchronously; neither it nor the backends share this record across requests.
type searchTiming struct {
	searchMS, admissionMS, connectMS, lookupMS, cleanupMS, catalogMS float64
	searchResult, catalogResult, failureStage, queryKind             string
}
type searchTimingKey struct{}

func requestTiming(ctx context.Context) *searchTiming {
	t, _ := ctx.Value(searchTimingKey{}).(*searchTiming)
	return t
}
func milliseconds(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

// Result labels describe the observed operation/context, not a PostgreSQL
// SQLSTATE or proof that a particular SQL statement caused an HTTP failure.
func timingResult(err error, ctx context.Context) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "canceled"
	}
	switch lookup.Status(err) {
	case 400:
		return "invalid"
	case 404:
		return "pair_unavailable"
	case 410:
		return "dataset_unavailable"
	default:
		return "unavailable"
	}
}
func timingKind(r lookup.Request) string {
	if r.Mode == normalization.Mask || ((r.Mode == "" || r.Mode == normalization.Auto) && strings.ContainsAny(r.Q, "*?")) {
		return "mask"
	}
	return "exact"
}

type timingWriter struct {
	http.ResponseWriter
	status      int
	writeFailed bool
}

func (w *timingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *timingWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 200 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *timingWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	if err != nil || n != len(p) {
		w.writeFailed = true
	}
	return n, err
}

// Timing is opt-in and records only canonical form/search GET/HEAD handlers.
// It does not wrap file streams, log request-derived strings, change headers,
// or include header receipt, network transit or log-sink time in handler_ms.
func searchTimingHandler(next http.Handler, logger *slog.Logger, enabled bool) http.Handler {
	if !enabled || !logger.Enabled(context.Background(), slog.LevelInfo) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/" && r.URL.Path != "/search") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		t := &searchTiming{searchResult: "not_run", catalogResult: "not_run", failureStage: "none", queryKind: "not_run"}
		started := time.Now()
		rw := &timingWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), searchTimingKey{}, t)))
		elapsed := milliseconds(started)
		status := rw.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.LogAttrs(context.Background(), slog.LevelInfo, "Search timing",
			slog.String("event", "search_timing"), slog.Int("status", status),
			slog.String("query_kind", t.queryKind), slog.String("search_result", t.searchResult),
			slog.String("failure_stage", t.failureStage), slog.String("catalog_result", t.catalogResult),
			slog.Bool("write_failed", rw.writeFailed), slog.Float64("handler_ms", elapsed),
			slog.Float64("search_ms", t.searchMS), slog.Float64("admission_ms", t.admissionMS),
			slog.Float64("connect_ms", t.connectMS), slog.Float64("lookup_ms", t.lookupMS),
			slog.Float64("cleanup_ms", t.cleanupMS), slog.Float64("catalog_ms", t.catalogMS))
	})
}
