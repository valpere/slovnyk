package postgres

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/normalization"
)

var ErrAnchor = errors.New("invalid_lookup_anchor")

type Filter struct {
	Dataset string
	From    string
	To      string
	Query   normalization.Query
}

// ReadPage streams at most limit entries plus one lookahead. visit returns false
// when a whole entry exceeds the caller's byte budget. The anchor is recovered
// from immutable data and must match both pair and filter before seek execution.
func ReadPage(ctx context.Context, c *pgx.Conn, f Filter, anchor int64, limit int, visit func(Translation) bool) (bool, error) {
	if limit < 1 || limit > 100 || anchor < 0 || visit == nil {
		return false, ErrConfig
	}
	where := `dataset_id=$1 AND source_locale=$2 AND target_locale=$3`
	args := []any{f.Dataset, f.From, f.To}
	switch f.Query.Mode {
	case normalization.Exact:
		h := normalization.KeyHash(f.Query.Key)
		where += ` AND key_hash=$4 AND source_key=$5`
		args = append(args, h[:], f.Query.Key)
	case normalization.Mask:
		where += ` AND source_key LIKE $4 ESCAPE '\'`
		args = append(args, f.Query.Pattern)
	default:
		return false, ErrConfig
	}
	var boundary Translation
	if anchor > 0 {
		q := `SELECT id,source_key,source_text,target_text FROM slovnyk.translations WHERE ` + where + ` AND id=$` + strconv.Itoa(len(args)+1)
		e := c.QueryRow(ctx, q, append(args, anchor)...).Scan(&boundary.ID, &boundary.SourceKey, &boundary.SourceText, &boundary.TargetText)
		if errors.Is(e, pgx.ErrNoRows) {
			return false, ErrAnchor
		}
		if e != nil {
			return false, ErrUnavailable
		}
		n := len(args)
		where += ` AND (source_key COLLATE "C",source_text COLLATE "C",target_text COLLATE "C",id) > ($` + strconv.Itoa(n+1) + `::text COLLATE "C",$` + strconv.Itoa(n+2) + `::text COLLATE "C",$` + strconv.Itoa(n+3) + `::text COLLATE "C",$` + strconv.Itoa(n+4) + `::bigint)`
		args = append(args, boundary.SourceKey, boundary.SourceText, boundary.TargetText, boundary.ID)
	}
	q := `SELECT id,source_key,source_text,target_text FROM slovnyk.translations WHERE ` + where + `
 ORDER BY source_key COLLATE "C",source_text COLLATE "C",target_text COLLATE "C",id LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)
	rows, e := c.Query(ctx, q, args...)
	if e != nil {
		return false, ErrUnavailable
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		if n == limit {
			return true, nil
		}
		var t Translation
		if rows.Scan(&t.ID, &t.SourceKey, &t.SourceText, &t.TargetText) != nil {
			return false, ErrUnavailable
		}
		if !visit(t) {
			return true, nil
		}
		n++
	}
	if rows.Err() != nil {
		return false, ErrUnavailable
	}
	return false, nil
}
