package postgres

import (
	"context"
	"crypto/sha256"

	"github.com/jackc/pgx/v5"
)

type Translation struct {
	ID         int64
	SourceKey  string
	SourceText string
	TargetText string
}

// Exact reads an initial bounded page using both the indexed hash and residual
// key equality. The application must pin/authorize dataset and set its overall
// lookup deadline; cursor seek and HTTP handling are supplied by M1-06.
func Exact(ctx context.Context, conn *pgx.Conn, dataset, from, to, key string, limit int) ([]Translation, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrConfig
	}
	sum := sha256.Sum256([]byte(key))
	rows, err := conn.Query(ctx, `SELECT id,source_key,source_text,target_text FROM slovnyk.translations
 WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3 AND key_hash=$4 AND source_key=$5
 ORDER BY source_key COLLATE "C",source_text COLLATE "C",target_text COLLATE "C",id LIMIT $6`, dataset, from, to, sum[:], key, limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	entries := make([]Translation, 0)
	for rows.Next() {
		var e Translation
		if err := rows.Scan(&e.ID, &e.SourceKey, &e.SourceText, &e.TargetText); err != nil {
			return nil, ErrUnavailable
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return entries, nil
}
