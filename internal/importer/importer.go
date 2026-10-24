// Package importer stages immutable CSV snapshots. It never publishes a dataset.
package importer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

var (
	ErrInput     = errors.New("invalid_import_input")
	ErrArtifact  = errors.New("artifact_identity_failure")
	ErrStream    = errors.New("artifact_stream_failure")
	ErrCollision = errors.New("tuple_hash_collision")
	ErrReplay    = errors.New("committed_prefix_mismatch")
	ErrTerminal  = errors.New("terminal_dataset")
	ErrBusy      = errors.New("import_writer_busy")
	ErrStorage   = errors.New("import_storage_failure")
)

// ErrorCode returns bounded diagnostics without paths, raw data or driver errors.
func ErrorCode(err error) string {
	for _, known := range []error{ErrInput, ErrArtifact, ErrStream, ErrCollision, ErrReplay, ErrTerminal, ErrBusy, ErrStorage, postgres.ErrConfig, postgres.ErrSchema, postgres.ErrUnavailable} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "import_interrupted"
	}
	return "import_interrupted"
}

type FileReport struct {
	Artifact         string `json:"artifact_sha256"`
	Parsed           int64  `json:"parsed"`
	Unique           int64  `json:"accepted_unique"`
	Duplicate        int64  `json:"duplicate_occurrences"`
	Rejected         int64  `json:"rejected"`
	Complete         bool   `json:"complete"`
	ChecksumVerified bool   `json:"checksum_verified"`
}
type IssueReport struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}
type Report struct {
	DistinctCountsAvailable bool          `json:"distinct_counts_available"`
	Fingerprint             string        `json:"fingerprint"`
	Dataset                 string        `json:"dataset_id,omitempty"`
	Attempt                 string        `json:"attempt_id,omitempty"`
	State                   string        `json:"state"`
	Error                   string        `json:"error,omitempty"`
	Files                   []FileReport  `json:"files"`
	Issues                  []IssueReport `json:"issues"`
}

type Options struct {
	// BatchSize is 1..1000 records, additionally bounded to 4 MiB of decoded fields.
	BatchSize int
	LockWait  time.Duration
	// Fault injection is package-private and never exposed through the CLI.
	afterBatch   func() error
	beforeCommit func() error
	afterStage   func(context.Context, pgx.Tx) error
}

func artifacts(m importdata.Manifest) []importdata.Artifact {
	a := append([]importdata.Artifact(nil), m.Artifacts...)
	sort.Slice(a, func(i, j int) bool { return a[i].SHA256 < a[j].SHA256 })
	return a
}
func digest(a importdata.Artifact) []byte { b, _ := hex.DecodeString(a.SHA256); return b }
func uuid() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// lockKeyV1 maps SHA-256("slovnyk-import-lock-v1\x00" || fingerprint) to
// its first signed big-endian int64. A collision only serializes unrelated work.
func lockKeyV1(fp [32]byte) int64 {
	h := sha256.New()
	h.Write([]byte("slovnyk-import-lock-v1\x00"))
	h.Write(fp[:])
	return int64(binary.BigEndian.Uint64(h.Sum(nil)[:8]))
}
func acquire(ctx context.Context, c *pgx.Conn, key int64, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		var locked bool
		if e := c.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); e != nil {
			if ctx.Err() != nil {
				return ErrBusy
			}
			return ErrStorage
		}
		if locked {
			return nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ErrBusy
		case <-timer.C:
		}
	}
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}

type stream struct {
	parser   *importdata.Reader
	close    func()
	verified func() bool
}

func reader(ctx context.Context, root string, a importdata.Artifact, m importdata.Manifest) (*stream, error) {
	f, e := importdata.OpenArtifact(root, a)
	if e != nil {
		return nil, ErrArtifact
	}
	closeFile := func() { _ = f.Close() }
	h := sha256.New()
	n, e := io.Copy(h, contextReader{ctx, f})
	if e != nil {
		closeFile()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrArtifact
	}
	if n != a.Bytes || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		closeFile()
		return nil, ErrArtifact
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		closeFile()
		return nil, ErrArtifact
	}
	// Hash the same open descriptor again while parsing; path replacement cannot
	// redirect this read and in-place mutation cannot silently complete the file.
	actual := sha256.New()
	counter := &countReader{r: io.TeeReader(contextReader{ctx, f}, actual)}
	p, e := importdata.NewGzipReader(counter)
	if e != nil {
		closeFile()
		return nil, ErrStream
	}
	p.Locales = map[string]bool{}
	for _, l := range m.Locales {
		p.Locales[l] = true
	}
	p.Pairs = map[importdata.Pair]bool{}
	if a.Pair != nil {
		p.Pairs[*a.Pair] = true
	} else {
		for _, pair := range m.ExpectedPairs {
			p.Pairs[pair] = true
		}
	}
	cleanup := func() { _ = p.Close(); closeFile() }
	// checksum validation is performed by walk after parser EOF.
	return &stream{p, cleanup, func() bool { return counter.n == a.Bytes && hex.EncodeToString(actual.Sum(nil)) == a.SHA256 }}, nil
}

type countReader struct {
	r io.Reader
	n int64
}

func (r *countReader) Read(b []byte) (int, error) { n, e := r.r.Read(b); r.n += int64(n); return n, e }

// Inspect validates compressed identity and streams records without a database.
// It reports occurrences/rejections, not distinct tuple counts.
func Inspect(ctx context.Context, m importdata.Manifest, root string) (Report, error) {
	_, fp, e := m.Canonical()
	r := Report{Fingerprint: hex.EncodeToString(fp[:]), State: "inspected", Files: []FileReport{}, Issues: []IssueReport{}}
	if e != nil {
		return r, ErrInput
	}
	issues := map[string]int64{}
	for _, a := range artifacts(m) {
		fr := FileReport{Artifact: a.SHA256}
		s, e := reader(ctx, root, a, m)
		if e == nil {
			for {
				rec, readErr := s.parser.Next()
				if readErr == io.EOF {
					if !s.verified() {
						e = ErrArtifact
					} else {
						fr.Complete = true
						fr.ChecksumVerified = true
					}
					break
				}
				if readErr != nil {
					e = ErrStream
					if ctx.Err() != nil {
						e = ctx.Err()
					}
					break
				}
				fr.Parsed++
				if rec.Reason != "" {
					fr.Rejected++
					issues[rec.Reason]++
				}
			}
			s.close()
		}
		r.Files = append(r.Files, fr)
		if e != nil {
			r.State = "failed"
			r.Error = ErrorCode(e)
			r.Issues = issueReports(issues)
			return r, e
		}
	}
	r.Issues = issueReports(issues)
	return r, nil
}

// Run takes ownership of a dedicated connection and ALWAYS closes it, releasing
// its session lock even on cancellation, transaction failure or lost connection.
// The caller must not share it, put it in a pool or reuse it after this call.
func Run(ctx context.Context, c *pgx.Conn, m importdata.Manifest, root string, opt Options) (report Report, err error) {
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(clean)
	}()
	canonical, fp, e := m.Canonical()
	report = Report{Fingerprint: hex.EncodeToString(fp[:]), Files: []FileReport{}, Issues: []IssueReport{}}
	if e != nil {
		return report, ErrInput
	}
	if opt.BatchSize == 0 {
		opt.BatchSize = 100
	}
	if opt.LockWait == 0 {
		opt.LockWait = 5 * time.Second
	}
	if opt.BatchSize < 1 || opt.BatchSize > 1000 || opt.LockWait < 0 || opt.LockWait > time.Minute {
		return report, ErrInput
	}
	if e = postgres.CheckSchema(ctx, c); e != nil {
		return report, e
	}
	if e = acquire(ctx, c, lockKeyV1(fp), opt.LockWait); e != nil {
		return report, e
	}
	id, run, state, e := begin(ctx, c, m, canonical, fp)
	report.Dataset = id
	report.Attempt = run
	report.State = state
	if e != nil {
		return report, e
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			code := ErrorCode(err)
			report.Error = code
			fatal := errors.Is(err, ErrArtifact) || errors.Is(err, ErrStream) || errors.Is(err, ErrCollision) || errors.Is(err, ErrReplay)
			runState := "interrupted"
			if fatal {
				runState = "failed"
			}
			tx, e := c.Begin(clean)
			if e == nil {
				_, e = tx.Exec(clean, `UPDATE slovnyk.import_runs SET state=$2,error_class=$3 WHERE id=$1`, run, runState, code)
				if e == nil && fatal {
					_, e = tx.Exec(clean, `UPDATE slovnyk.datasets SET state='failed' WHERE id=$1`, id)
				}
				if e == nil {
					e = tx.Commit(clean)
				} else {
					_ = tx.Rollback(clean)
				}
				if e == nil && fatal {
					report.State = "failed"
				}
			}
		}
		if e := loadReport(clean, c, &report); e != nil {
			report.Files = nil
			report.Issues = nil
			report.DistinctCountsAvailable = false
			if err == nil {
				err = ErrStorage
			}
			report.Error = ErrorCode(err)
		} else {
			report.DistinctCountsAvailable = true
		}
	}()
	if _, e = c.Exec(ctx, batchTable); e != nil {
		return report, ErrStorage
	}
	for _, a := range artifacts(m) {
		if e = loadFile(ctx, c, id, run, root, m, a, opt); e != nil {
			return report, e
		}
	}
	tx, e := c.Begin(ctx)
	if e != nil {
		return report, ErrStorage
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, `UPDATE slovnyk.datasets SET state='validating' WHERE id=$1`, id); e != nil {
		return report, ErrStorage
	}
	if _, e = tx.Exec(ctx, `UPDATE slovnyk.import_runs SET state='completed' WHERE id=$1`, run); e != nil {
		return report, ErrStorage
	}
	if e = tx.Commit(ctx); e != nil {
		return report, ErrStorage
	}
	report.State = "validating"
	return report, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func begin(ctx context.Context, c *pgx.Conn, m importdata.Manifest, canonical []byte, fp [32]byte) (id, run, state string, err error) {
	tx, e := c.Begin(ctx)
	if e != nil {
		return "", "", "", ErrStorage
	}
	defer rollback(tx)
	var stored []byte
	e = tx.QueryRow(ctx, `SELECT id::text,state,manifest_bytes FROM slovnyk.datasets WHERE fingerprint=$1`, fp[:]).Scan(&id, &state, &stored)
	if e == pgx.ErrNoRows {
		id = uuid()
		state = "created"
		_, e = tx.Exec(ctx, `INSERT INTO slovnyk.datasets(id,fingerprint,manifest_bytes,schema_version,parser_version,mapping_version,normalization_version) VALUES($1,$2,$3,1,$4,$5,$6)`, id, fp[:], canonical, m.ParserVersion, m.MappingVersion, m.NormalizationVersion)
		if e != nil {
			return id, "", state, ErrStorage
		}
		for _, l := range m.Locales {
			if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.dataset_locales(dataset_id,code) VALUES($1,$2)`, id, l); e != nil {
				return id, "", state, ErrStorage
			}
		}
		for _, p := range m.ExpectedPairs {
			if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.dataset_pairs(dataset_id,source_locale,target_locale) VALUES($1,$2,$3)`, id, p.From, p.To); e != nil {
				return id, "", state, ErrStorage
			}
		}
		for _, a := range artifacts(m) {
			if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.source_files(dataset_id,artifact_hash,relative_name,format,byte_size) VALUES($1,$2,$3,$4,$5)`, id, digest(a), a.Name, a.Format, a.Bytes); e != nil {
				return id, "", state, ErrStorage
			}
		}
	} else if e != nil {
		return id, "", state, ErrStorage
	}
	if stored != nil && !bytes.Equal(stored, canonical) {
		return id, "", state, ErrCollision
	}
	if state == "failed" || state == "ready" {
		return id, "", state, ErrTerminal
	}
	// Holding the session lock proves any earlier running attempt lost its writer.
	if _, e = tx.Exec(ctx, `UPDATE slovnyk.import_runs SET state='interrupted',error_class='writer_disconnected' WHERE dataset_id=$1 AND state='running'`, id); e != nil {
		return id, "", state, ErrStorage
	}
	run = uuid()
	if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.import_runs(id,dataset_id,attempt,state) SELECT $1,$2,coalesce(max(attempt),0)+1,'running' FROM slovnyk.import_runs WHERE dataset_id=$2`, run, id); e != nil {
		return id, run, state, ErrStorage
	}
	if state == "created" {
		if _, e = tx.Exec(ctx, `UPDATE slovnyk.datasets SET state='loading' WHERE id=$1`, id); e != nil {
			return id, run, state, ErrStorage
		}
		state = "loading"
	}
	for _, a := range artifacts(m) {
		if _, e = tx.Exec(ctx, `INSERT INTO slovnyk.import_checkpoints(dataset_id,artifact_hash,run_id,committed_ordinal) VALUES($1,$2,$3,0) ON CONFLICT DO NOTHING`, id, digest(a), run); e != nil {
			return id, run, state, ErrStorage
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return id, run, state, ErrStorage
	}
	return id, run, state, nil
}

func loadReport(ctx context.Context, c *pgx.Conn, r *Report) error {
	if e := c.QueryRow(ctx, `SELECT state FROM slovnyk.datasets WHERE id=$1`, r.Dataset).Scan(&r.State); e != nil {
		return e
	}
	rows, e := c.Query(ctx, `SELECT encode(artifact_hash,'hex'),parsed,accepted_unique,duplicate_occurrences,rejected,complete,checksum_verified FROM slovnyk.source_files WHERE dataset_id=$1 ORDER BY artifact_hash`, r.Dataset)
	if e != nil {
		return e
	}
	defer rows.Close()
	r.Files = []FileReport{}
	for rows.Next() {
		var f FileReport
		if e = rows.Scan(&f.Artifact, &f.Parsed, &f.Unique, &f.Duplicate, &f.Rejected, &f.Complete, &f.ChecksumVerified); e != nil {
			return e
		}
		r.Files = append(r.Files, f)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	rows.Close()
	rows, e = c.Query(ctx, `SELECT reason_code,count(*) FROM slovnyk.import_issues WHERE dataset_id=$1 GROUP BY reason_code ORDER BY reason_code`, r.Dataset)
	if e != nil {
		return e
	}
	defer rows.Close()
	r.Issues = []IssueReport{}
	for rows.Next() {
		var i IssueReport
		if e = rows.Scan(&i.Reason, &i.Count); e != nil {
			return e
		}
		r.Issues = append(r.Issues, i)
	}
	return rows.Err()
}

func issueReports(issues map[string]int64) []IssueReport {
	keys := make([]string, 0, len(issues))
	for k := range issues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]IssueReport, 0, len(keys))
	for _, k := range keys {
		result = append(result, IssueReport{k, issues[k]})
	}
	return result
}

// AcquireWriter shares the versioned import/verify session lock. The caller
// owns a dedicated connection and must close it on EVERY exit path.
func AcquireWriter(ctx context.Context, c *pgx.Conn, fp [32]byte) error {
	return acquire(ctx, c, lockKeyV1(fp), 5*time.Second)
}
