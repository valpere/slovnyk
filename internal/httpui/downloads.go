package httpui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/valpere/slovnyk/internal/downloads"
	"github.com/valpere/slovnyk/internal/i18n"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/publication"
)

type DownloadsFunc func(context.Context) (downloads.Manifest, error)

// ExportFunc transfers ownership of the returned file to the handler. It must
// resolve identifiers using trusted metadata, never a path from HTTP input.
type ExportFunc func(context.Context, string, string) (downloads.Export, *os.File, error)

func WithDownloads(list DownloadsFunc, open ExportFunc) Option {
	return func(o *options) { o.downloads = list; o.export = open }
}

func downloadView(ctx context.Context, list DownloadsFunc, data *view) int {
	if list != nil {
		m, err := list(ctx)
		if err == nil {
			data.Downloads = m
			return 200
		}
	}
	data.Message = data.Text["downloads_unavailable"]
	return 503
}
func plainDownloads(w io.Writer, m downloads.Manifest) error {
	if _, err := fmt.Fprintf(w, "dataset\t%s\nfrom\tto\trows\tbytes\tsha256\turl\n", m.Dataset); err != nil {
		return err
	}
	for _, e := range m.Exports {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t/downloads/%s/%s\n", strconv.Quote(e.From), strconv.Quote(e.To), e.Rows, e.Artifact.Bytes, e.Artifact.SHA256, e.Dataset, e.ID); err != nil {
			return err
		}
	}
	return nil
}

// Slow transfers receive a fresh 30 s write deadline per chunk, rather than
// the search server's 15 s total write deadline. Admission bounds open files.
type transferWriter struct{ http.ResponseWriter }

func (w transferWriter) WriteHeader(status int) {
	if status >= 400 {
		w.Header().Del("Content-Disposition")
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w transferWriter) Write(p []byte) (int, error) {
	if err := http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	return w.ResponseWriter.Write(p)
}

func exportHandler(open ExportFunc) http.HandlerFunc {
	transfers := make(chan struct{}, 8)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, follow")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Queries never select a file or filename, but malformed encoding is rejected.
		values, err := url.ParseQuery(r.URL.RawQuery)
		valid := err == nil
		for k, vs := range values {
			valid = valid && validText(k)
			for _, v := range vs {
				valid = valid && validText(v)
			}
		}
		lang, _ := i18n.Select("", cookie(r, uiCookie), r.Header.Get("Accept-Language"))
		fail := func(status int, key string) {
			w.Header().Set("Content-Language", lang)
			w.Header().Set("Vary", "Accept-Language, Cookie")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(status)
			if r.Method != "HEAD" {
				_, _ = io.WriteString(w, i18n.Text(lang, key)+"\n")
			}
		}
		if !valid {
			fail(400, "invalid")
			return
		}
		dataset, id := r.PathValue("dataset"), r.PathValue("export")
		if !publication.ValidID(dataset) || !publication.ValidID(id) {
			fail(404, "file_not_found")
			return
		}
		if open == nil {
			fail(503, "downloads_unavailable")
			return
		}
		select {
		case transfers <- struct{}{}:
			defer func() { <-transfers }()
		default:
			fail(503, "downloads_unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), lookup.DBTimeout)
		e, f, err := open(ctx, dataset, id)
		cancel() // The metadata deadline must not cancel a large file transfer.
		if err != nil {
			if f != nil {
				_ = f.Close()
			}
			if errors.Is(err, downloads.ErrNotFound) {
				fail(404, "file_not_found")
			} else {
				fail(503, "downloads_unavailable")
			}
			return
		}
		if f == nil {
			fail(503, "downloads_unavailable")
			return
		}
		defer f.Close()

		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			fail(503, "downloads_unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="slovnyk-%s-%s.csv.gz"`, dataset, id))
		w.Header().Set("ETag", `"`+e.Artifact.SHA256+`"`)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		// The gzip is the attachment representation, not HTTP content encoding.
		http.ServeContent(transferWriter{w}, r, "", time.Time{}, f)
	}
}
