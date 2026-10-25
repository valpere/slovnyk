// Package sitemaps builds immutable offline discovery artifacts for ready datasets.
package sitemaps

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

const (
	origin    = "https://slovnyk.org"
	namespace = "http://www.sitemaps.org/schemas/sitemap/0.9"
	maxURLs   = 50000
	maxShards = 50000
	maxBytes  = 52428800
)

var (
	ErrArguments = errors.New("invalid_sitemap_arguments")
	ErrState     = errors.New("sitemap_dataset_not_ready")
	ErrStorage   = errors.New("sitemap_storage_unavailable")
	ErrEmpty     = errors.New("sitemap_empty_dataset")
	ErrLimit     = errors.New("sitemap_protocol_limit")
)

type Shard struct {
	URLs     int           `json:"urls"`
	Artifact artifacts.Ref `json:"artifact"`
}
type Manifest struct {
	Version int           `json:"version"`
	Dataset string        `json:"dataset"`
	URLs    int64         `json:"urls"`
	Shards  []Shard       `json:"shards"`
	Index   artifacts.Ref `json:"index"`
}
type Result struct {
	Manifest
	Report artifacts.Ref `json:"report"`
}

// Generate owns and closes c. It does not publish, consult the active pointer or
// change dataset state. The caller must supply an existing durable artifact root.
func Generate(ctx context.Context, c *pgx.Conn, id, root string) (Result, error) {
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(closeCtx)
	}()
	if !publication.ValidID(id) {
		return Result{}, ErrArguments
	}
	if err := postgres.CheckSchema(ctx, c); err != nil {
		return Result{}, ErrStorage
	}
	var state string
	if err := c.QueryRow(ctx, "SELECT state FROM slovnyk.datasets WHERE id=$1", id).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrState
	} else if err != nil {
		return Result{}, ErrStorage
	}
	if state != "ready" {
		return Result{}, ErrState
	}
	store, err := artifacts.Open(root)
	if err != nil {
		return Result{}, ErrStorage
	}
	defer store.Close()
	rows, err := c.Query(ctx, `SELECT DISTINCT slovnyk.entry_id(source_locale,target_locale,source_text)
 FROM slovnyk.translations WHERE dataset_id=$1 ORDER BY 1`, id)
	if err != nil {
		return Result{}, ErrStorage
	}
	defer rows.Close()
	result, err := generate(ctx, store, id, func() (string, bool, error) {
		if !rows.Next() {
			return "", false, rows.Err()
		}
		var hash []byte
		if err := rows.Scan(&hash); err != nil {
			return "", false, err
		}
		return hex.EncodeToString(hash), true, nil
	}, maxURLs, maxShards)
	rows.Close()
	if err != nil {
		return Result{}, err
	}
	if err := register(ctx, c, result); err != nil {
		return Result{}, ErrStorage
	}
	return result, nil
}

type nextID func() (string, bool, error)

// Limits are private parameters so boundary tests need no enormous fixtures.
func generate(ctx context.Context, store *artifacts.Store, id string, next nextID, shardSize, shardLimit int) (Result, error) {
	out := Result{Manifest: Manifest{Version: 1, Dataset: id}}
	dir := "datasets/" + id + "/sitemaps"
	previous := ""
	advance := func() (string, bool, error) {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		value, ok, err := next()
		if err != nil {
			return "", false, err
		}
		if ok && (!entries.ValidID(value) || value <= previous) {
			return "", false, ErrStorage
		}
		if ok {
			previous = value
		}
		return value, ok, nil
	}
	pending, more, err := advance()
	if err != nil {
		return Result{}, ErrStorage
	}
	if !more {
		return Result{}, ErrEmpty
	}
	for more {
		if len(out.Shards) >= shardLimit {
			return Result{}, ErrLimit
		}
		count := 0
		ref, err := store.Install(ctx, dir, ".xml", func(w io.Writer) error {
			buffer := bufio.NewWriterSize(w, 64<<10)
			w = buffer
			if _, err := fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"%s\">\n", namespace); err != nil {
				return err
			}
			for more && count < shardSize {
				// Only a fixed origin and validated lowercase hex enter the XML.
				if _, err := fmt.Fprintf(w, "<url><loc>%s%s</loc></url>\n", origin, entries.Path(pending)); err != nil {
					return err
				}
				count++
				pending, more, err = advance()
				if err != nil {
					return err
				}
			}
			if _, err := io.WriteString(w, "</urlset>\n"); err != nil {
				return err
			}
			return buffer.Flush()
		})
		if err != nil {
			return Result{}, ErrStorage
		}
		// Fixed-length URLs and the 50,000-entry bound keep each shard below 6 MiB.
		if ref.Bytes > maxBytes {
			return Result{}, ErrLimit
		}
		out.Shards = append(out.Shards, Shard{URLs: count, Artifact: ref})
		out.URLs += int64(count)
	}
	out.Index, err = store.Install(ctx, dir, ".xml", func(w io.Writer) error {
		buffer := bufio.NewWriterSize(w, 64<<10)
		w = buffer
		if _, err := fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<sitemapindex xmlns=\"%s\">\n", namespace); err != nil {
			return err
		}
		for _, shard := range out.Shards {
			if _, err := fmt.Fprintf(w, "<sitemap><loc>%s/sitemap-%s-%s.xml</loc></sitemap>\n", origin, id, shard.Artifact.SHA256); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "</sitemapindex>\n"); err != nil {
			return err
		}
		return buffer.Flush()
	})
	if err != nil {
		return Result{}, ErrStorage
	}
	if out.Index.Bytes > maxBytes {
		return Result{}, ErrLimit
	}
	// The complete report is installed last. Failed attempts may leave immutable
	// unreferenced fragments; they never return a successful report.
	out.Report, err = store.Install(ctx, dir, ".json", func(w io.Writer) error {
		return json.NewEncoder(w).Encode(out.Manifest)
	})
	if err != nil {
		return Result{}, ErrStorage
	}
	return out, nil
}
