// Package catalog reads bounded, immutable dataset language inventories.
package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/normalization"
)

const MaxLocales = 256
const MaxPairs = 4096

type Pair struct {
	From, To string
	Count    int64
}
type Snapshot struct {
	Dataset string
	Locales []string
	Pairs   []Pair
}

func (s Snapshot) Has(from, to string) bool {
	for _, p := range s.Pairs {
		if p.From == from && p.To == to {
			return true
		}
	}
	return false
}

// Read pins active once, or uses the published retained dataset from lookup.
// Limits fail closed rather than returning a misleading partial inventory.
func Read(ctx context.Context, c *pgx.Conn, dataset string) (Snapshot, error) {
	s := Snapshot{Dataset: dataset}
	if dataset == "" {
		if err := c.QueryRow(ctx, "SELECT coalesce(dataset_id::text,'') FROM slovnyk.active_dataset WHERE singleton=1").Scan(&s.Dataset); err != nil || s.Dataset == "" {
			return Snapshot{}, lookup.ErrUnavailable
		}
	}
	var valid bool
	err := c.QueryRow(ctx, `SELECT state='ready' AND normalization_version=$2 AND EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=d.id) FROM slovnyk.datasets d WHERE id=$1`, s.Dataset, normalization.Version).Scan(&valid)
	if err != nil || !valid {
		return Snapshot{}, lookup.ErrUnavailable
	}
	rows, err := c.Query(ctx, `SELECT code FROM slovnyk.dataset_locales WHERE dataset_id=$1 ORDER BY code COLLATE "C" LIMIT $2`, s.Dataset, MaxLocales+1)
	if err != nil {
		return Snapshot{}, lookup.ErrUnavailable
	}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			rows.Close()
			return Snapshot{}, lookup.ErrUnavailable
		}
		s.Locales = append(s.Locales, code)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(s.Locales) > MaxLocales {
		return Snapshot{}, lookup.ErrUnavailable
	}
	rows, err = c.Query(ctx, `SELECT source_locale,target_locale,unique_count FROM slovnyk.dataset_pairs WHERE dataset_id=$1 ORDER BY source_locale COLLATE "C",target_locale COLLATE "C" LIMIT $2`, s.Dataset, MaxPairs+1)
	if err != nil {
		return Snapshot{}, lookup.ErrUnavailable
	}
	for rows.Next() {
		var p Pair
		if err = rows.Scan(&p.From, &p.To, &p.Count); err != nil {
			rows.Close()
			return Snapshot{}, lookup.ErrUnavailable
		}
		s.Pairs = append(s.Pairs, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(s.Pairs) > MaxPairs || ctx.Err() != nil {
		return Snapshot{}, lookup.ErrUnavailable
	}
	return s, nil
}
