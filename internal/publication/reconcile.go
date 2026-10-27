package publication

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
)

// ReconciliationReport is diagnostic evidence, not a retained verification
// report or owner approval. Known gaps and exceptions remain unresolved.
type ReconciliationReport struct {
	Version     int                    `json:"version"`
	Kind        string                 `json:"kind"`
	Dataset     string                 `json:"dataset"`
	Fingerprint string                 `json:"fingerprint"`
	State       string                 `json:"state"`
	Parsed      int64                  `json:"parsed"`
	Unique      int64                  `json:"unique"`
	Duplicates  int64                  `json:"duplicates"`
	Rejected    int64                  `json:"rejected"`
	Files       []importer.FileReport  `json:"files"`
	KnownGaps   []string               `json:"known_gaps"`
	Exceptions  []importdata.Exception `json:"exceptions"`
}

// Reconcile owns and closes c. It replays every source record and checks the
// staged inventory without installing artifacts or changing persistent data.
// It takes writer/row locks and uses a bounded temporary table, then rolls back.
// Only validating datasets with no rejected records/issues are supported.
func Reconcile(ctx context.Context, c *pgx.Conn, id, source string) (ReconciliationReport, error) {
	defer closeConn(c)
	var out ReconciliationReport
	if source == "" {
		return out, ErrArguments
	}
	tx, m, fp, state, e := beginValidation(ctx, c, id)
	if e != nil {
		return out, e
	}
	defer rollback(tx)
	if state != "validating" {
		return out, ErrState
	}
	if e = importer.Reconcile(ctx, tx, id, m, source); e != nil {
		return out, e
	}
	if e = inventory(ctx, tx, id, m); e != nil {
		return out, e
	}
	r, e := collectReport(ctx, tx, id, fp)
	if e != nil {
		return out, e
	}
	return ReconciliationReport{
		Version: 1, Kind: "reconciliation", Dataset: id, Fingerprint: fp, State: state,
		Parsed: r.Parsed, Unique: r.Unique, Duplicates: r.Duplicates, Rejected: r.Rejected,
		Files: r.Files, KnownGaps: m.KnownGaps, Exceptions: m.Exceptions,
	}, nil
}
