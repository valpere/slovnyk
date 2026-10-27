package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/normalization"
)

const batchTable = `CREATE TEMP TABLE slovnyk_import_batch (
 ordinal bigint PRIMARY KEY, byte_offset bigint NOT NULL,
 from_code text COLLATE "C", to_code text COLLATE "C", source_text text COLLATE "C", target_text text COLLATE "C",
 source_key text COLLATE "C",key_hash bytea,tuple_hash bytea,reason text NOT NULL
) ON COMMIT DELETE ROWS`

func loadFile(ctx context.Context, c *pgx.Conn, id, run, root string, m importdata.Manifest, a importdata.Artifact, opt Options) (err error) {
	var checkpoint, parsed, ledger, origins, accepted int64
	e := c.QueryRow(ctx, `SELECT c.committed_ordinal,f.parsed,
 (SELECT count(*) FROM slovnyk.import_records r WHERE r.dataset_id=f.dataset_id AND r.artifact_hash=f.artifact_hash),
 (SELECT count(*) FROM slovnyk.translation_origins o WHERE o.dataset_id=f.dataset_id AND o.artifact_hash=f.artifact_hash),
 f.accepted_unique+f.duplicate_occurrences
 FROM slovnyk.source_files f JOIN slovnyk.import_checkpoints c USING(dataset_id,artifact_hash)
 WHERE f.dataset_id=$1 AND f.artifact_hash=$2`, id, digest(a)).Scan(&checkpoint, &parsed, &ledger, &origins, &accepted)
	if e != nil {
		return ErrStorage
	}
	var offset int64
	defer func() {
		if errors.Is(err, ErrArtifact) || errors.Is(err, ErrStream) || errors.Is(err, ErrReplay) || errors.Is(err, ErrCollision) {
			clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// The file is no longer a completed/verified input for this attempt.
			_, _ = c.Exec(clean, `UPDATE slovnyk.source_files SET complete=false,checksum_verified=false WHERE dataset_id=$1 AND artifact_hash=$2`, id, digest(a))
			// This diagnostic precedes the terminal dataset transition in Run.
			_, _ = c.Exec(clean, `INSERT INTO slovnyk.import_issues(dataset_id,artifact_hash,byte_offset,reason_code) VALUES($1,$2,$3,$4)`, id, digest(a), offset, ErrorCode(err))
		}
	}()
	if checkpoint != parsed || ledger != checkpoint || origins != accepted {
		return ErrReplay
	}
	s, e := reader(ctx, root, a, m)
	if e != nil {
		return e
	}
	defer s.close()
	batch := make([]importdata.Record, 0, opt.BatchSize)
	size := 0
	var last int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		replay := batch[len(batch)-1].Ordinal <= checkpoint
		e := commitBatch(ctx, c, id, run, a, batch, replay, opt)
		batch = batch[:0]
		size = 0
		if e == nil && !replay && opt.afterBatch != nil {
			e = opt.afterBatch()
		}
		return e
	}
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		rec, readErr := s.parser.Next()
		offset = rec.Offset
		if readErr == io.EOF {
			if last < checkpoint {
				return ErrReplay
			}
			if !s.verified() {
				return ErrArtifact
			}
			if e = flush(); e != nil {
				return e
			}
			break
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrStream
		}
		last = rec.Ordinal
		recSize := 0
		for _, f := range rec.Fields {
			recSize += len(f)
		}
		if size+recSize > 4<<20 {
			if e = flush(); e != nil {
				return e
			}
		}
		batch = append(batch, rec)
		size += recSize
		// Do not mix replay with new records in a transaction.
		if len(batch) >= opt.BatchSize || size >= 4<<20 || rec.Ordinal == checkpoint {
			if e = flush(); e != nil {
				return e
			}
		}
	}
	_, e = c.Exec(ctx, `UPDATE slovnyk.source_files SET complete=true,checksum_verified=true WHERE dataset_id=$1 AND artifact_hash=$2`, id, digest(a))
	if e != nil {
		return ErrStorage
	}
	return nil
}

func commitBatch(ctx context.Context, c *pgx.Conn, id, run string, a importdata.Artifact, records []importdata.Record, replay bool, opt Options) error {
	tx, e := c.Begin(ctx)
	if e != nil {
		return ErrStorage
	}
	defer rollback(tx)
	if e = copyRecords(ctx, tx, records); e != nil {
		return e
	}
	if opt.afterStage != nil {
		if e = opt.afterStage(ctx, tx); e != nil {
			return e
		}
	}
	var bad bool
	if replay {
		if e = checkReplay(ctx, tx, id, a); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return ErrStorage
		}
		return nil
	}
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_temp.slovnyk_import_batch b
 JOIN slovnyk.translations t ON t.dataset_id=$1 AND t.tuple_hash=b.tuple_hash
 WHERE (t.source_locale,t.target_locale,t.source_text,t.target_text) IS DISTINCT FROM
 (b.from_code,b.to_code,b.source_text,b.target_text)) OR EXISTS(
 SELECT tuple_hash FROM pg_temp.slovnyk_import_batch WHERE reason='' GROUP BY tuple_hash
 HAVING count(DISTINCT (from_code,to_code,source_text,target_text))>1)`, id).Scan(&bad)
	if e != nil {
		return ErrStorage
	}
	if bad {
		return ErrCollision
	}
	var maxID int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(id),0) FROM slovnyk.translations WHERE dataset_id=$1`, id).Scan(&maxID); e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `INSERT INTO slovnyk.translations(dataset_id,id,source_locale,target_locale,source_text,target_text,source_key,key_hash,tuple_hash)
 SELECT $1,$2::bigint+row_number() OVER(ORDER BY ordinal),from_code,to_code,source_text,target_text,source_key,key_hash,tuple_hash
 FROM (SELECT DISTINCT ON(b.tuple_hash) b.* FROM pg_temp.slovnyk_import_batch b
 WHERE reason='' AND NOT EXISTS(SELECT 1 FROM slovnyk.translations t WHERE t.dataset_id=$1 AND t.tuple_hash=b.tuple_hash)
 ORDER BY b.tuple_hash,b.ordinal) firsts`, id, maxID)
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `INSERT INTO slovnyk.import_records(dataset_id,artifact_hash,ordinal,outcome,translation_id,issue_code)
 SELECT $1,$2,b.ordinal,CASE WHEN reason<>'' THEN 'rejected'
 WHEN t.id>$3 AND b.ordinal=min(b.ordinal) OVER(PARTITION BY b.tuple_hash) THEN 'unique' ELSE 'duplicate' END,
 t.id,nullif(reason,'') FROM pg_temp.slovnyk_import_batch b LEFT JOIN slovnyk.translations t ON t.dataset_id=$1 AND t.tuple_hash=b.tuple_hash`, id, digest(a), maxID)
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `INSERT INTO slovnyk.translation_origins(dataset_id,artifact_hash,ordinal,translation_id,source_locale,target_locale,source_text,target_text,source_locator,provenance)
 SELECT $1,$2,b.ordinal,r.translation_id,b.from_code,b.to_code,b.source_text,b.target_text,$3::text||':'||b.ordinal::text,
 jsonb_build_object('format','csv4.gz','mapping','identity-v1') FROM pg_temp.slovnyk_import_batch b
 JOIN slovnyk.import_records r ON r.dataset_id=$1 AND r.artifact_hash=$2 AND r.ordinal=b.ordinal WHERE b.reason=''`, id, digest(a), a.SHA256)
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `INSERT INTO slovnyk.import_issues(dataset_id,artifact_hash,ordinal,byte_offset,reason_code)
 SELECT $1,$2,ordinal,byte_offset,reason FROM pg_temp.slovnyk_import_batch WHERE reason<>''`, id, digest(a))
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `UPDATE slovnyk.source_files SET parsed=parsed+$3,accepted_unique=accepted_unique+x.u,
 duplicate_occurrences=duplicate_occurrences+x.d,rejected=rejected+x.r FROM
 (SELECT count(*) FILTER(WHERE outcome='unique') u,count(*) FILTER(WHERE outcome='duplicate') d,count(*) FILTER(WHERE outcome='rejected') r
 FROM slovnyk.import_records WHERE dataset_id=$1 AND artifact_hash=$2 AND ordinal BETWEEN $4 AND $5) x
 WHERE dataset_id=$1 AND artifact_hash=$2`, id, digest(a), len(records), records[0].Ordinal, records[len(records)-1].Ordinal)
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `UPDATE slovnyk.dataset_pairs p SET unique_count=unique_count+x.n FROM
 (SELECT source_locale,target_locale,count(*) n FROM slovnyk.translations WHERE dataset_id=$1 AND id>$2 GROUP BY source_locale,target_locale) x
 WHERE p.dataset_id=$1 AND p.source_locale=x.source_locale AND p.target_locale=x.target_locale`, id, maxID)
	if e != nil {
		return ErrStorage
	}
	_, e = tx.Exec(ctx, `UPDATE slovnyk.import_checkpoints SET run_id=$3,committed_ordinal=$4 WHERE dataset_id=$1 AND artifact_hash=$2`, id, digest(a), run, records[len(records)-1].Ordinal)
	if e != nil {
		return ErrStorage
	}
	if opt.beforeCommit != nil {
		if e = opt.beforeCommit(); e != nil {
			return fmt.Errorf("batch interrupted: %w", e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return ErrStorage
	}
	return nil
}

func copyRecords(ctx context.Context, tx pgx.Tx, records []importdata.Record) error {
	rows := make([][]any, 0, len(records))
	for _, r := range records {
		t := r.Fields
		var key string
		var kh, th []byte
		if r.Reason == "" {
			key = normalization.Key(t[2])
			k := normalization.KeyHash(key)
			h := importdata.TupleHash(t)
			kh = k[:]
			th = h[:]
		} else {
			t = importdata.Tuple{}
		}
		rows = append(rows, []any{r.Ordinal, r.Offset, t[0], t[1], t[2], t[3], key, kh, th, r.Reason})
	}
	_, e := tx.CopyFrom(ctx, pgx.Identifier{"pg_temp", "slovnyk_import_batch"}, []string{"ordinal", "byte_offset", "from_code", "to_code", "source_text", "target_text", "source_key", "key_hash", "tuple_hash", "reason"}, pgx.CopyFromRows(rows))
	if e != nil {
		return ErrStorage
	}

	return nil
}

func checkReplay(ctx context.Context, tx pgx.Tx, id string, a importdata.Artifact) error {
	var bad bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_temp.slovnyk_import_batch b
   LEFT JOIN slovnyk.import_records r ON r.dataset_id=$1 AND r.artifact_hash=$2 AND r.ordinal=b.ordinal
   LEFT JOIN slovnyk.translations t ON t.dataset_id=r.dataset_id AND t.id=r.translation_id
   LEFT JOIN slovnyk.translation_origins o ON o.dataset_id=r.dataset_id AND o.artifact_hash=r.artifact_hash AND o.ordinal=r.ordinal
   WHERE r.ordinal IS NULL OR
   (b.reason<>'' AND (r.outcome<>'rejected' OR r.issue_code IS DISTINCT FROM b.reason)) OR
   (b.reason='' AND (r.outcome='rejected' OR t.id IS NULL OR o.ordinal IS NULL OR
    (t.source_locale,t.target_locale,t.source_text,t.target_text,t.source_key,t.key_hash,t.tuple_hash) IS DISTINCT FROM
    (b.from_code,b.to_code,b.source_text,b.target_text,b.source_key,b.key_hash,b.tuple_hash) OR
    (o.source_locale,o.target_locale,o.source_text,o.target_text,o.source_locator) IS DISTINCT FROM
    (b.from_code,b.to_code,b.source_text,b.target_text,$3::text||':'||b.ordinal::text))))`, id, digest(a), a.SHA256).Scan(&bad)
	if e != nil {
		return ErrStorage
	}
	if bad {
		return ErrReplay
	}
	return nil
}
