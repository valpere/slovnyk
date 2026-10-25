package httpui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/sitemaps"
)

// SitemapFunc transfers the open file to the handler. Empty IDs select the
// active index; explicit IDs must resolve registered published shards.
type SitemapFunc func(context.Context, string, string) (artifacts.Ref, *os.File, error)

func WithSitemaps(open SitemapFunc) Option { return func(o *options) { o.sitemap = open } }

func sitemapHandler(open SitemapFunc) http.HandlerFunc {
	transfers := make(chan struct{}, 8)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fail := func(code int) {
			w.Header().Set("X-Robots-Tag", "noindex, follow")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(code)
			if r.Method != "HEAD" {
				_, _ = io.WriteString(w, http.StatusText(code)+"\n")
			}
		}
		var dataset, hash string
		active := r.URL.Path == "/sitemap.xml"
		if !active {
			name := strings.TrimPrefix(r.URL.Path, "/sitemap-")
			if !strings.HasPrefix(r.URL.Path, "/sitemap-") || len(name) != 36+1+64+4 || name[36] != '-' || !strings.HasSuffix(name, ".xml") {
				fail(404)
				return
			}
			dataset, hash = name[:36], name[37:101]
			if !publication.ValidID(dataset) || !entries.ValidID(hash) {
				fail(404)
				return
			}
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			fail(400)
			return
		}
		if open == nil {
			fail(503)
			return
		}
		select {
		case transfers <- struct{}{}:
			defer func() { <-transfers }()
		default:
			fail(503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), lookup.DBTimeout)
		ref, f, err := open(ctx, dataset, hash)
		cancel()
		if err != nil || f == nil {
			if f != nil {
				_ = f.Close()
			}
			if errors.Is(err, sitemaps.ErrNotFound) {
				fail(404)
			} else {
				fail(503)
			}
			return
		}
		defer f.Close()
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			fail(503)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("ETag", `"`+ref.SHA256+`"`)
		// The active index must be re-resolved after publish/rollback. Shards are pinned.
		if !active {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(transferWriter{w}, r, "", time.Time{}, f)
	}
}
