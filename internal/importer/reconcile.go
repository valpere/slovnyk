package importer

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
)

// Reconcile replays every immutable source byte against ledger and raw origins,
// inside the verifier's transaction. The caller holds the dataset writer lock
// and row lock. No dictionary data is mutated; only a bounded temp table is used.
func Reconcile(ctx context.Context, tx pgx.Tx, id string, m importdata.Manifest, root string) error {
	if _, e := tx.Exec(ctx, batchTable); e != nil {
		return ErrStorage
	}
	for _, a := range artifacts(m) {
		if e := reconcileFile(ctx, tx, id, m, root, a); e != nil {
			return e
		}
	}
	return nil
}
func reconcileFile(ctx context.Context, tx pgx.Tx, id string, m importdata.Manifest, root string, a importdata.Artifact) error {
	var complete, verified bool
	var parsed, checkpoint, ledger, origins, unique, duplicate, rejected int64
	var name, format string
	var size int64
	e := tx.QueryRow(ctx, `SELECT f.complete,f.checksum_verified,f.parsed,c.committed_ordinal,f.relative_name,f.format,f.byte_size,
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash),
 (SELECT count(*) FROM slovnyk.translation_origins o WHERE o.dataset_id=f.dataset_id AND o.artifact_hash=f.artifact_hash),
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='unique'),
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='duplicate'),
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='rejected')
 FROM slovnyk.source_files f JOIN slovnyk.import_checkpoints c USING(dataset_id,artifact_hash)
 WHERE f.dataset_id=$1 AND f.artifact_hash=$2 AND (f.accepted_unique,f.duplicate_occurrences,f.rejected)=
 ((SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='unique'),
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='duplicate'),
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash AND outcome='rejected'))`, id, digest(a)).Scan(&complete, &verified, &parsed, &checkpoint, &name, &format, &size, &ledger, &origins, &unique, &duplicate, &rejected)
	if e == pgx.ErrNoRows {
		return ErrReplay
	}
	if e != nil {
		return ErrStorage
	}
	if !complete || !verified || parsed != checkpoint || parsed != ledger || origins != unique+duplicate || parsed != unique+duplicate+rejected || name != a.Name || format != a.Format || size != a.Bytes {
		return ErrReplay
	}
	s, e := reader(ctx, root, a, m)
	if e != nil {
		return e
	}
	defer s.close()
	records := make([]importdata.Record, 0, 64)
	var seen int64
	flush := func() error {
		if len(records) == 0 {
			return nil
		}
		if _, e := tx.Exec(ctx, "TRUNCATE pg_temp.slovnyk_import_batch"); e != nil {
			return ErrStorage
		}
		if e := copyRecords(ctx, tx, records); e != nil {
			return e
		}
		if e := checkReplay(ctx, tx, id, a); e != nil {
			return e
		}
		records = records[:0]
		return nil
	}
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		rec, readErr := s.parser.Next()
		if readErr == io.EOF {
			if !s.verified() {
				return ErrArtifact
			}
			if seen != parsed {
				return ErrReplay
			}
			return flush()
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrStream
		}
		seen++
		records = append(records, rec)
		if len(records) == 64 {
			if e = flush(); e != nil {
				return e
			}
		}
	}
}
