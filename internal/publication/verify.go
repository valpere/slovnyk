package publication

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/normalization"
)

func inventory(ctx context.Context, tx pgx.Tx, id string, m importdata.Manifest) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM slovnyk.source_files WHERE dataset_id=$1)=$2 AND
 (SELECT count(*) FROM slovnyk.dataset_locales WHERE dataset_id=$1)=$3 AND
 (SELECT count(*) FROM slovnyk.dataset_pairs WHERE dataset_id=$1)=$4 AND
 NOT EXISTS(SELECT 1 FROM slovnyk.import_issues WHERE dataset_id=$1) AND
 NOT EXISTS(SELECT 1 FROM slovnyk.legacy_query_snapshots WHERE dataset_id=$1) AND
 NOT EXISTS(SELECT 1 FROM slovnyk.import_runs WHERE dataset_id=$1 AND state='running') AND
 EXISTS(SELECT 1 FROM slovnyk.import_runs WHERE dataset_id=$1 AND state='completed') AND
 NOT EXISTS(SELECT 1 FROM slovnyk.import_records WHERE dataset_id=$1 AND outcome='rejected') AND
 NOT EXISTS(SELECT 1 FROM slovnyk.exports WHERE dataset_id=$1) AND
 NOT EXISTS(SELECT 1 FROM slovnyk.translation_origins WHERE dataset_id=$1 AND
 (legacy_count IS NOT NULL OR legacy_status IS NOT NULL OR provenance <> '{"format":"csv4.gz","mapping":"identity-v1"}'::jsonb)) AND
 NOT EXISTS(SELECT 1 FROM slovnyk.translations t WHERE dataset_id=$1 AND NOT EXISTS
 (SELECT 1 FROM slovnyk.import_records r WHERE r.dataset_id=t.dataset_id AND r.translation_id=t.id AND outcome='unique')) AND
 NOT EXISTS(SELECT 1 FROM (SELECT outcome,row_number() OVER(PARTITION BY translation_id ORDER BY artifact_hash,ordinal) n
 FROM slovnyk.import_records WHERE dataset_id=$1 AND outcome<>'rejected') x WHERE (n=1)<>(outcome='unique'))`, id, len(m.Artifacts), len(m.Locales), len(m.ExpectedPairs)).Scan(&ok)
	if e != nil {
		return ErrStorage
	}
	if !ok {
		return ErrVerification
	}
	for _, l := range m.Locales {
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM slovnyk.dataset_locales WHERE dataset_id=$1 AND code=$2 AND display_name IS NULL)", id, l).Scan(&ok); e != nil {
			return ErrStorage
		}
		if !ok {
			return ErrVerification
		}
	}
	for _, p := range m.ExpectedPairs {
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM slovnyk.dataset_pairs p WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3 AND unique_count=
  (SELECT count(*) FROM slovnyk.translations t WHERE t.dataset_id=p.dataset_id AND t.source_locale=p.source_locale AND t.target_locale=p.target_locale))`, id, p.From, p.To).Scan(&ok); e != nil {
			return ErrStorage
		}
		if !ok {
			return ErrVerification
		}
	}
	return nil
}

func exportPair(ctx context.Context, tx pgx.Tx, store *artifacts.Store, id string, p importdata.Pair) (PairReport, error) {
	out := PairReport{From: p.From, To: p.To, ExportID: uuid()}
	rows, e := tx.Query(ctx, `SELECT source_locale,target_locale,source_text,target_text FROM slovnyk.translations
 WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3 ORDER BY source_locale,target_locale,source_text,target_text`, id, p.From, p.To)
	if e != nil {
		return out, ErrStorage
	}
	defer rows.Close()
	digest := sha256.New()
	var sample importdata.Tuple
	ref, e := store.Install(ctx, "datasets/"+id+"/exports/"+out.ExportID, ".csv.gz", func(w io.Writer) error {
		gz := gzip.NewWriter(w)
		for rows.Next() {
			var tuple importdata.Tuple
			if err := rows.Scan(&tuple[0], &tuple[1], &tuple[2], &tuple[3]); err != nil {
				_ = gz.Close()
				return ErrStorage
			}
			if err := importdata.WriteCSV4(gz, tuple); err != nil {
				_ = gz.Close()
				return err
			}
			if out.Rows == 0 {
				sample = tuple
			}
			importdata.WriteTuple(digest, tuple)
			out.Rows++
		}
		if rows.Err() != nil {
			_ = gz.Close()
			return ErrStorage
		}
		return gz.Close()
	})
	rows.Close()
	if e != nil {
		return out, e
	}
	if out.Rows > 0 {
		key := normalization.Key(sample[2])
		h := normalization.KeyHash(key)
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(key)
		var found bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM slovnyk.translations WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3 AND key_hash=$4 AND source_key=$5 AND source_key LIKE $6 ESCAPE '\')`, id, p.From, p.To, h[:], key, literal).Scan(&found); e != nil {
			return out, ErrStorage
		}
		if !found {
			return out, ErrVerification
		}
	}
	out.Artifact = ref
	out.TupleDigest = hex.EncodeToString(digest.Sum(nil))
	if e = roundTrip(ctx, store, out); e != nil {
		return out, e
	}
	return out, nil
}
func roundTrip(ctx context.Context, store *artifacts.Store, p PairReport) error {
	if e := store.Check(ctx, p.Artifact); e != nil {
		return e
	}
	f, e := store.Reader(p.Artifact)
	if e != nil {
		return e
	}
	defer f.Close()
	r, e := importdata.NewGzipReader(f)
	if e != nil {
		return ErrVerification
	}
	defer r.Close()
	digest := sha256.New()
	var n int64
	var previous importdata.Tuple
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		record, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil || record.Reason != "" {
			return ErrVerification
		}
		t := record.Fields
		if t[0] != p.From || t[1] != p.To {
			return ErrVerification
		}
		if n > 0 && compareTuple(previous, t) >= 0 {
			return ErrVerification
		}
		previous = t
		importdata.WriteTuple(digest, t)
		n++
	}
	if n != p.Rows || hex.EncodeToString(digest.Sum(nil)) != p.TupleDigest {
		return ErrVerification
	}
	return nil
}
func compareTuple(a, b importdata.Tuple) int {
	for i := range a {
		if c := strings.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return 0
}

// Report hash binds the full inventory. DB immutability guards protect ready
// metadata. Files are rehashed before export, publication, startup and recovery.
func readReport(ctx context.Context, tx pgx.Tx, store *artifacts.Store, id string) (Report, error) {
	var out Report
	var state, p, fp string
	var hash []byte
	e := tx.QueryRow(ctx, "SELECT state,coalesce(report_path,''),report_hash,encode(fingerprint,'hex') FROM slovnyk.datasets WHERE id=$1", id).Scan(&state, &p, &hash, &fp)
	if e != nil {
		return out, ErrStorage
	}
	if state != "ready" {
		return out, ErrState
	}
	prefix := "datasets/" + id + "/reports/"
	parts := strings.Split(p, "/")
	if !strings.HasPrefix(p, prefix) || len(hash) != 32 || len(parts) != 5 || !ValidID(parts[3]) {
		return out, ErrVerification
	}
	// The report is deliberately small; CSV data is streamed separately.
	// Size is not stored in datasets, so read at most the report limit then hash.
	f, e := store.ReaderSize(p, 4<<20)
	if e != nil {
		return out, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if e != nil || len(b) > 4<<20 {
		return out, ErrVerification
	}
	actual := sha256.Sum256(b)
	if !bytes.Equal(hash, actual[:]) {
		return out, ErrVerification
	}
	if path.Base(p) != hex.EncodeToString(hash)+".json" {
		return out, ErrVerification
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil {
		return out, ErrVerification
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return out, ErrVerification
	}
	if !out.ControlsChecked || out.Version != 1 || out.Dataset != id || out.Fingerprint != fp || out.Rejected != 0 || out.Parsed != out.Unique+out.Duplicates || out.Unique < 0 || out.Duplicates < 0 || len(out.Pairs) == 0 {
		return out, ErrVerification
	}
	// File counters are bound to the canonical manifest without granting HTTP
	// access to the private import ledger.
	var raw []byte
	if e = tx.QueryRow(ctx, "SELECT manifest_bytes FROM slovnyk.datasets WHERE id=$1", id).Scan(&raw); e != nil {
		return out, ErrStorage
	}
	m, err := importdata.ParseManifest(raw)
	if err != nil || len(m.Artifacts) != len(out.Files) {
		return out, ErrVerification
	}
	files := map[string]bool{}
	for _, a := range m.Artifacts {
		files[a.SHA256] = true
	}
	var parsed, unique, duplicates int64
	for _, f := range out.Files {
		if !files[f.Artifact] || !f.Complete || !f.ChecksumVerified || f.Rejected != 0 || f.Unique < 0 || f.Duplicate < 0 || f.Parsed != f.Unique+f.Duplicate {
			return out, ErrVerification
		}
		delete(files, f.Artifact)
		parsed += f.Parsed
		unique += f.Unique
		duplicates += f.Duplicate
	}
	if parsed != out.Parsed || unique != out.Unique || duplicates != out.Duplicates {
		return out, ErrVerification
	}
	rows, e := tx.Query(ctx, `SELECT export_id::text,source_locale,target_locale,artifact_path,encode(sha256,'hex'),byte_size,logical_rows,verified
 FROM slovnyk.exports WHERE dataset_id=$1`, id)
	if e != nil {
		return out, ErrStorage
	}
	inventory := map[string]PairReport{}
	for rows.Next() {
		var v PairReport
		var verified bool
		if rows.Scan(&v.ExportID, &v.From, &v.To, &v.Artifact.Path, &v.Artifact.SHA256, &v.Artifact.Bytes, &v.Rows, &verified) != nil || !verified {
			rows.Close()
			return out, ErrVerification
		}
		inventory[v.ExportID] = v
	}
	rows.Close()
	if rows.Err() != nil {
		return out, ErrStorage
	}
	if len(inventory) != len(out.Pairs) {
		return out, ErrVerification
	}
	seen := map[importdata.Pair]bool{}
	var total int64
	for _, v := range out.Pairs {
		expected, ok := inventory[v.ExportID]
		pair := importdata.Pair{From: v.From, To: v.To}
		if !ok || seen[pair] || v.From != expected.From || v.To != expected.To || v.Rows != expected.Rows || v.Rows < 0 || v.Artifact != expected.Artifact || !ValidID(v.ExportID) {
			return out, ErrVerification
		}
		if v.Artifact.Path != "datasets/"+id+"/exports/"+v.ExportID+"/"+v.Artifact.SHA256+".csv.gz" {
			return out, ErrVerification
		}
		// Compare the immutable report to the immutable pair counters as well.
		var count int64
		if e = tx.QueryRow(ctx, "SELECT unique_count FROM slovnyk.dataset_pairs WHERE dataset_id=$1 AND source_locale=$2 AND target_locale=$3", id, v.From, v.To).Scan(&count); e != nil {
			return out, ErrStorage
		}
		if count != v.Rows {
			return out, ErrVerification
		}
		if e = roundTrip(ctx, store, v); e != nil {
			return out, e
		}
		seen[pair] = true
		delete(inventory, v.ExportID)
		total += v.Rows
	}
	var pairs int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM slovnyk.dataset_pairs WHERE dataset_id=$1", id).Scan(&pairs); e != nil {
		return out, ErrStorage
	}
	if total != out.Unique || pairs != len(out.Pairs) {
		return out, ErrVerification
	}
	return out, nil
}
