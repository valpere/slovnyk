// Package publication reconciles staged data, retains immutable artifacts, and
// switches the active dataset using an explicit publication generation.
package publication

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

var (
	ErrVerification = errors.New("dataset_verification_failed")
	ErrStorage      = errors.New("publication_storage_unavailable")
	ErrState        = errors.New("dataset_not_ready_or_validating")
	ErrConflict     = errors.New("publication_generation_conflict")
	ErrArguments    = errors.New("invalid_publication_arguments")
)

type PairReport struct {
	From        string        `json:"from"`
	To          string        `json:"to"`
	Rows        int64         `json:"rows"`
	TupleDigest string        `json:"tuple_digest"`
	ExportID    string        `json:"export_id"`
	Artifact    artifacts.Ref `json:"artifact"`
}
type Report struct {
	Files           []importer.FileReport `json:"files"`
	ControlsChecked bool                  `json:"controls_checked"`
	Version         int                   `json:"version"`
	Dataset         string                `json:"dataset"`
	Fingerprint     string                `json:"fingerprint"`
	Parsed          int64                 `json:"parsed"`
	Unique          int64                 `json:"unique"`
	Duplicates      int64                 `json:"duplicates"`
	Rejected        int64                 `json:"rejected"`
	Pairs           []PairReport          `json:"pairs"`
}
type Receipt struct {
	Dataset    string `json:"dataset"`
	Generation int64  `json:"generation"`
}
type Options struct{ beforeReady func() error }

func uuid() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func ValidID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	b, e := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return e == nil && len(b) == 16 && strings.ToLower(s) == s
}
func closeConn(c *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Close(ctx)
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// Verify owns and closes c. The artifact root must already exist on durable local
// storage; the source snapshot must remain immutable throughout verification.
func Verify(ctx context.Context, c *pgx.Conn, id, source, root string, opt Options) (Report, error) {
	defer closeConn(c)
	var out Report
	if !ValidID(id) {
		return out, ErrArguments
	}
	if e := postgres.CheckSchema(ctx, c); e != nil {
		return out, ErrStorage
	}
	var raw, fp []byte
	if e := c.QueryRow(ctx, "SELECT manifest_bytes,fingerprint FROM slovnyk.datasets WHERE id=$1", id).Scan(&raw, &fp); e != nil {
		return out, ErrStorage
	}
	m, e := importdata.ParseManifest(raw)
	if e != nil {
		return out, ErrVerification
	}
	canonical, expected, e := m.Canonical()
	if e != nil || !bytes.Equal(raw, canonical) || !bytes.Equal(fp, expected[:]) {
		return out, ErrVerification
	}
	if e = importer.AcquireWriter(ctx, c, expected); e != nil {
		return out, e
	}
	tx, e := c.Begin(ctx)
	if e != nil {
		return out, ErrStorage
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); e != nil {
		return out, ErrStorage
	}
	var state string
	if e = tx.QueryRow(ctx, "SELECT state FROM slovnyk.datasets WHERE id=$1 FOR UPDATE", id).Scan(&state); e != nil {
		return out, ErrStorage
	}
	store, e := artifacts.Open(root)
	if e != nil {
		return out, e
	}
	defer store.Close()
	if state == "ready" {
		return readReport(ctx, tx, store, id)
	}
	if state != "validating" {
		return out, ErrState
	}
	// The owner-approval adapter is not implemented. A manifest declaration is
	// never treated as authorization to lose data.
	if len(m.KnownGaps) > 0 || len(m.Exceptions) > 0 {
		return out, ErrVerification
	}
	if e = importer.Reconcile(ctx, tx, id, m, source); e != nil {
		return out, e
	}
	if e = inventory(ctx, tx, id, m); e != nil {
		return out, e
	}
	out = Report{Version: 1, Dataset: id, Fingerprint: hex.EncodeToString(fp), Pairs: []PairReport{}}
	if e = tx.QueryRow(ctx, `SELECT coalesce(sum(parsed),0)::bigint,coalesce(sum(accepted_unique),0)::bigint,
 coalesce(sum(duplicate_occurrences),0)::bigint,coalesce(sum(rejected),0)::bigint FROM slovnyk.source_files WHERE dataset_id=$1`, id).Scan(&out.Parsed, &out.Unique, &out.Duplicates, &out.Rejected); e != nil {
		return out, ErrStorage
	}
	// Pair metadata has been reconciled. Collect the small catalog before opening
	// a streaming translation query on this same dedicated connection.
	pairs := append([]importdata.Pair{}, m.ExpectedPairs...)
	fileRows, e := tx.Query(ctx, `SELECT encode(artifact_hash,'hex'),parsed,accepted_unique,duplicate_occurrences,rejected,complete,checksum_verified FROM slovnyk.source_files WHERE dataset_id=$1 ORDER BY artifact_hash`, id)
	if e != nil {
		return out, ErrStorage
	}
	for fileRows.Next() {
		var f importer.FileReport
		if e = fileRows.Scan(&f.Artifact, &f.Parsed, &f.Unique, &f.Duplicate, &f.Rejected, &f.Complete, &f.ChecksumVerified); e != nil {
			fileRows.Close()
			return out, ErrStorage
		}
		out.Files = append(out.Files, f)
	}
	fileRows.Close()
	if fileRows.Err() != nil {
		return out, ErrStorage
	}
	for _, p := range pairs {
		pr, err := exportPair(ctx, tx, store, id, p)
		if err != nil {
			return out, err
		}
		out.Pairs = append(out.Pairs, pr)
		hash, _ := hex.DecodeString(pr.Artifact.SHA256)
		if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.exports(dataset_id,export_id,source_locale,target_locale,artifact_path,sha256,byte_size,logical_rows,verified) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true)`, id, pr.ExportID, pr.From, pr.To, pr.Artifact.Path, hash, pr.Artifact.Bytes, pr.Rows); e != nil {
			return out, ErrStorage
		}
	}
	out.ControlsChecked = true
	reportBytes, e := json.Marshal(out)
	if e != nil || len(reportBytes)+1 > 4<<20 {
		return out, ErrVerification
	}
	ref, e := store.Install(ctx, "datasets/"+id+"/reports/"+uuid(), ".json", func(w io.Writer) error { _, err := w.Write(append(reportBytes, '\n')); return err })
	if e != nil {
		return out, e
	}
	if opt.beforeReady != nil {
		if e = opt.beforeReady(); e != nil {
			return out, e
		}
	}
	hash, _ := hex.DecodeString(ref.SHA256)
	if _, e = tx.Exec(ctx, "UPDATE slovnyk.datasets SET state='ready',report_path=$2,report_hash=$3 WHERE id=$1", id, ref.Path, hash); e != nil {
		return out, ErrStorage
	}
	if e = tx.Commit(ctx); e != nil {
		return out, ErrStorage
	}
	return out, nil
}

// Export returns the retained verified inventory; it never regenerates files.
// It owns and closes c, as do Verify and Publish.
func Export(ctx context.Context, c *pgx.Conn, id, root string) (Report, error) {
	defer closeConn(c)
	if !ValidID(id) {
		return Report{}, ErrArguments
	}
	if e := postgres.CheckSchema(ctx, c); e != nil {
		return Report{}, ErrStorage
	}
	store, e := artifacts.Open(root)
	if e != nil {
		return Report{}, e
	}
	defer store.Close()
	tx, e := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Report{}, ErrStorage
	}
	defer rollback(tx)
	return readReport(ctx, tx, store, id)
}

// Publish serializes the generation comparison, complete artifact validation,
// pointer change and history insertion under the active singleton row lock.
func Publish(ctx context.Context, c *pgx.Conn, id, root string, expected int64, operator string, isRollback bool) (Receipt, error) {
	defer closeConn(c)
	var out Receipt
	if !ValidID(id) || expected < 0 || len(operator) == 0 || len(operator) > 256 || strings.ContainsRune(operator, 0) {
		return out, ErrArguments
	}
	if e := postgres.CheckSchema(ctx, c); e != nil {
		return out, ErrStorage
	}
	store, e := artifacts.Open(root)
	if e != nil {
		return out, e
	}
	defer store.Close()
	tx, e := c.Begin(ctx)
	if e != nil {
		return out, ErrStorage
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); e != nil {
		return out, ErrStorage
	}
	var previous *string
	var generation int64
	if e = tx.QueryRow(ctx, "SELECT dataset_id::text,generation FROM slovnyk.active_dataset WHERE singleton=1 FOR UPDATE").Scan(&previous, &generation); e != nil {
		return out, ErrStorage
	}
	if generation != expected {
		return out, ErrConflict
	}
	if isRollback {
		var retained bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM slovnyk.publication_events WHERE new_dataset_id=$1)", id).Scan(&retained); e != nil {
			return out, ErrStorage
		}
		if !retained {
			return out, ErrState
		}
	}
	if _, e = readReport(ctx, tx, store, id); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO slovnyk.publication_events(generation,previous_dataset_id,new_dataset_id,operator_ref) VALUES($1,$2,$3,$4)", generation+1, previous, id, operator); e != nil {
		return out, ErrStorage
	}
	if _, e = tx.Exec(ctx, "UPDATE slovnyk.active_dataset SET dataset_id=$1,generation=$2 WHERE singleton=1", id, generation+1); e != nil {
		return out, ErrStorage
	}
	if e = tx.Commit(ctx); e != nil {
		return out, ErrStorage
	}
	return Receipt{Dataset: id, Generation: generation + 1}, nil
}

// CheckActive checks schema, the active pointer and every retained reference.
// Read-only HTTP credentials are sufficient; no import/provenance access needed.
func CheckActive(ctx context.Context, c *pgx.Conn, root string) (Receipt, error) {
	if e := postgres.CheckSchema(ctx, c); e != nil {
		return Receipt{}, ErrStorage
	}
	store, e := artifacts.Open(root)
	if e != nil {
		return Receipt{}, e
	}
	defer store.Close()
	tx, e := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Receipt{}, ErrStorage
	}
	defer rollback(tx)
	var out Receipt
	if e = tx.QueryRow(ctx, "SELECT coalesce(dataset_id::text,''),generation FROM slovnyk.active_dataset WHERE singleton=1").Scan(&out.Dataset, &out.Generation); e != nil {
		return out, ErrStorage
	}
	if out.Dataset == "" {
		return out, ErrState
	}
	if _, e = readReport(ctx, tx, store, out.Dataset); e != nil {
		return out, e
	}
	return out, nil
}
