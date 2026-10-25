package sitemaps

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/publication"
)

var ErrNotFound = errors.New("sitemap_not_found")

func register(ctx context.Context, c *pgx.Conn, result Result) error {
	shards, err := json.Marshal(result.Shards)
	if err != nil {
		return err
	}
	decode := func(s string) []byte { b, _ := hex.DecodeString(s); return b }
	// The database API serializes concurrent generators and atomically installs
	// all rows. Operator/HTTP roles cannot insert, update or delete table rows.
	_, err = c.Exec(ctx, `SELECT slovnyk.register_sitemap($1,$2,$3,$4,$5,$6,$7::jsonb)`, result.Dataset, decode(result.Report.SHA256), result.Report.Bytes, decode(result.Index.SHA256), result.Index.Bytes, result.URLs, string(shards))
	return err
}

func ref(dataset, hash string, size int64, suffix string) (artifacts.Ref, error) {
	if !publication.ValidID(dataset) || !entries.ValidID(hash) || size <= 0 {
		return artifacts.Ref{}, lookup.ErrUnavailable
	}
	return artifacts.Ref{Path: "datasets/" + dataset + "/sitemaps/" + hash + suffix, SHA256: hash, Bytes: size}, nil
}

// Resolve borrows c. Empty dataset/hash selects the active index in one SQL
// snapshot; explicit identifiers select a registered shard of a published version.
func Resolve(ctx context.Context, c *pgx.Conn, dataset, hash string) (artifacts.Ref, error) {
	var size int64
	var err error
	if dataset == "" && hash == "" {
		err = c.QueryRow(ctx, `SELECT i.dataset_id::text,encode(i.index_sha,'hex'),i.index_bytes
 FROM slovnyk.active_dataset a JOIN slovnyk.sitemap_inventory i ON i.dataset_id=a.dataset_id
 JOIN slovnyk.datasets d ON d.id=i.dataset_id WHERE a.singleton=1 AND d.state='ready'
 AND EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=d.id)`).Scan(&dataset, &hash, &size)
		if errors.Is(err, pgx.ErrNoRows) {
			return artifacts.Ref{}, lookup.ErrUnavailable
		}
	} else {
		if !publication.ValidID(dataset) || !entries.ValidID(hash) {
			return artifacts.Ref{}, ErrNotFound
		}
		decoded, _ := hex.DecodeString(hash)
		err = c.QueryRow(ctx, `SELECT s.byte_size FROM slovnyk.sitemap_shards s JOIN slovnyk.datasets d ON d.id=s.dataset_id
 WHERE s.dataset_id=$1 AND s.sha256=$2 AND d.state='ready'
 AND EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=d.id)`, dataset, decoded).Scan(&size)
		if errors.Is(err, pgx.ErrNoRows) {
			return artifacts.Ref{}, ErrNotFound
		}
	}
	if err != nil || ctx.Err() != nil {
		return artifacts.Ref{}, lookup.ErrUnavailable
	}
	return ref(dataset, hash, size, ".xml")
}

// Check optionally verifies an active dataset's registered files at startup,
// reconnection or generation change. Absence preserves existing publication behavior.
func Check(ctx context.Context, c *pgx.Conn, root, dataset string) error {
	var index, report string
	var indexSize, reportSize int64
	err := c.QueryRow(ctx, `SELECT encode(index_sha,'hex'),index_bytes,encode(report_sha,'hex'),report_bytes
 FROM slovnyk.sitemap_inventory WHERE dataset_id=$1`, dataset).Scan(&index, &indexSize, &report, &reportSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return lookup.ErrUnavailable
	}
	store, err := artifacts.Open(root)
	if err != nil {
		return lookup.ErrUnavailable
	}
	defer store.Close()
	check := func(hash string, size int64, suffix string) error {
		r, err := ref(dataset, hash, size, suffix)
		if err != nil {
			return err
		}
		return store.Check(ctx, r)
	}
	if err := check(index, indexSize, ".xml"); err != nil {
		return lookup.ErrUnavailable
	}
	if err := check(report, reportSize, ".json"); err != nil {
		return lookup.ErrUnavailable
	}
	rows, err := c.Query(ctx, `SELECT encode(sha256,'hex'),byte_size FROM slovnyk.sitemap_shards WHERE dataset_id=$1`, dataset)
	if err != nil {
		return lookup.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		var size int64
		if err := rows.Scan(&hash, &size); err != nil {
			return lookup.ErrUnavailable
		}
		if err := check(hash, size, ".xml"); err != nil {
			return lookup.ErrUnavailable
		}
	}
	if rows.Err() != nil || ctx.Err() != nil {
		return lookup.ErrUnavailable
	}
	return nil
}
