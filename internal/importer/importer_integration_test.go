//go:build integration

package importer

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func connect(t *testing.T, operator bool) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("SLOVNYK_TEST_DSN required; use make test-integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, e := postgres.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	if operator {
		if _, e = c.Exec(ctx, "SET ROLE slovnyk_operator"); e != nil {
			t.Fatal(e)
		}
	}
	return c
}
func scalar(t *testing.T, c *pgx.Conn, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if e := c.QueryRow(context.Background(), q, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func runFixture(t *testing.T, m importdata.Manifest, root string, opt Options) (Report, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return Run(ctx, connect(t, true), m, root, opt)
}
func variant(t *testing.T, name string) importdata.Manifest {
	t.Helper()
	m := fixture(t)
	m.Source.Authority = name
	return m
}

// snapshot compares raw identity, deterministic IDs, ownership and provenance,
// excluding attempt IDs/timestamps and the intentionally different dataset ID.
func snapshot(t *testing.T, c *pgx.Conn, id string) string {
	t.Helper()
	var s string
	e := c.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'translations',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM(SELECT id,source_locale,target_locale,source_text,target_text,source_key,encode(tuple_hash,'hex') tuple_hash FROM slovnyk.translations WHERE dataset_id=$1)x),
 'records',(SELECT jsonb_agg(to_jsonb(x) ORDER BY artifact,ordinal) FROM(SELECT encode(artifact_hash,'hex') artifact,ordinal,outcome,translation_id,issue_code FROM slovnyk.import_records WHERE dataset_id=$1)x),
 'origins',(SELECT jsonb_agg(to_jsonb(x) ORDER BY artifact,ordinal) FROM(SELECT encode(artifact_hash,'hex') artifact,ordinal,translation_id,source_locale,target_locale,source_text,target_text,legacy_count,legacy_status,source_locator,provenance FROM slovnyk.translation_origins WHERE dataset_id=$1)x),
 'pairs',(SELECT jsonb_agg(to_jsonb(x) ORDER BY source_locale,target_locale) FROM(SELECT source_locale,target_locale,unique_count FROM slovnyk.dataset_pairs WHERE dataset_id=$1)x))::text`, id).Scan(&s)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestRetryEveryBatchAndValidating(t *testing.T) {
	admin := connect(t, false)
	active := scalar(t, admin, "SELECT generation FROM slovnyk.active_dataset")
	clean, e := runFixture(t, variant(t, "clean"), "../../testdata/import", Options{BatchSize: 2})
	if e != nil {
		t.Fatalf("clean: %+v %v", clean, e)
	}
	want := snapshot(t, admin, clean.Dataset)
	for cut := 1; cut <= 6; cut++ {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			m := variant(t, fmt.Sprintf("retry-%d", cut))
			n := 0
			partial, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 2, afterBatch: func() error {
				n++
				if n == cut {
					return context.Canceled
				}
				return nil
			}})
			if !errors.Is(e, context.Canceled) || partial.State != "loading" {
				t.Fatalf("partial %+v %v", partial, e)
			}
			resumed, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 3})
			if e != nil || resumed.State != "validating" || resumed.Files[0].Unique != 10 || resumed.Files[0].Duplicate != 1 || !resumed.Files[0].Complete {
				t.Fatalf("resume %+v %v", resumed, e)
			}
			if got := snapshot(t, admin, resumed.Dataset); got != want {
				t.Fatalf("retry content mismatch\n%s\n%s", got, want)
			}
			again, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 1})
			if e != nil || !reflect.DeepEqual(again.Files, resumed.Files) || snapshot(t, admin, again.Dataset) != want {
				t.Fatalf("validating replay %+v %v", again, e)
			}
		})
	}
	if scalar(t, admin, "SELECT generation FROM slovnyk.active_dataset") != active {
		t.Fatal("import changed active pointer")
	}
}
func TestRollbackBeforeCheckpoint(t *testing.T) {
	m := variant(t, "rollback-before-commit")
	p, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 2, beforeCommit: func() error { return context.Canceled }})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	admin := connect(t, false)
	for _, table := range []string{"translations", "import_records", "translation_origins", "import_issues"} {
		if scalar(t, admin, "SELECT count(*) FROM slovnyk."+table+" WHERE dataset_id=$1", p.Dataset) != 0 {
			t.Fatal("partial transaction", table)
		}
	}
	if scalar(t, admin, "SELECT sum(committed_ordinal) FROM slovnyk.import_checkpoints WHERE dataset_id=$1", p.Dataset) != 0 {
		t.Fatal("checkpoint advanced")
	}
	r, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 2})
	if e != nil || r.Files[0].Parsed != 11 {
		t.Fatalf("%+v %v", r, e)
	}
}
func generated(t *testing.T, name string, raw []byte, corrupt bool) (importdata.Manifest, string) {
	t.Helper()
	m := variant(t, name)
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	_, _ = g.Write(raw)
	if e := g.Close(); e != nil {
		t.Fatal(e)
	}
	data := b.Bytes()
	if corrupt {
		data[len(data)-8] ^= 1
	}
	h := sha256.Sum256(data)
	m.Artifacts[0].SHA256 = hex.EncodeToString(h[:])
	m.Artifacts[0].Bytes = int64(len(data))
	m.Artifacts[0].Name = "input.csv.gz"
	if e := os.MkdirAll("../../tmp/m1-04", 0700); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp("../../tmp/m1-04", "fixture-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if e := os.WriteFile(filepath.Join(root, "input.csv.gz"), data, 0600); e != nil {
		t.Fatal(e)
	}
	return m, root
}
func TestRejectionsFatalStreamsAndIdentity(t *testing.T) {
	admin := connect(t, false)
	active := scalar(t, admin, "SELECT generation FROM slovnyk.active_dataset")
	raw := []byte("en-us,uk-ua,good,добре\n\nen-us,uk-ua,bad\x00,value\nen-us,uk-ua,\xff,value\n")
	m, root := generated(t, "rejections", raw, false)
	r, e := runFixture(t, m, root, Options{BatchSize: 1})
	if e != nil || r.Files[0].Parsed != 4 || r.Files[0].Unique != 1 || r.Files[0].Rejected != 3 || len(r.Issues) != 3 {
		t.Fatalf("%+v %v", r, e)
	}
	again, e := runFixture(t, m, root, Options{BatchSize: 2})
	if e != nil || !reflect.DeepEqual(again.Files, r.Files) {
		t.Fatalf("rejected replay %+v %v", again, e)
	}
	cases := []struct {
		name    string
		raw     []byte
		corrupt bool
	}{
		{"bad-trailer", []byte("en-us,uk-ua,one,один\nen-us,uk-ua,two,два\n"), true},
		{"bad-syntax", []byte("en-us,uk-ua,one,один\nen-us,uk-ua,\"unfinished"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, root := generated(t, tc.name, tc.raw, tc.corrupt)
			r, e := runFixture(t, m, root, Options{BatchSize: 1})
			if e != ErrStream || r.State != "failed" || r.Files[0].Complete || r.Files[0].ChecksumVerified {
				t.Fatalf("%+v %v", r, e)
			}
			if r.Files[0].Parsed != r.Files[0].Unique+r.Files[0].Duplicate+r.Files[0].Rejected {
				t.Fatal("prefix accounting")
			}
			if _, e = runFixture(t, m, root, Options{}); e != ErrTerminal {
				t.Fatal(e)
			}
		})
	}
	m = variant(t, "bad-identity")
	m.Artifacts[0].SHA256 = fmt.Sprintf("%064d", 0)
	r, e = runFixture(t, m, "../../testdata/import", Options{})
	if e != ErrArtifact || r.State != "failed" || r.Files[0].Parsed != 0 {
		t.Fatalf("%+v %v", r, e)
	}
	if scalar(t, admin, "SELECT generation FROM slovnyk.active_dataset") != active {
		t.Fatal("active pointer changed")
	}
}
func TestSessionLockAndAbandonedAttempt(t *testing.T) {
	m := variant(t, "lock-test")
	_, fp, _ := m.Canonical()
	holder := connect(t, true)
	if e := acquire(context.Background(), holder, lockKeyV1(fp), time.Second); e != nil {
		t.Fatal(e)
	}
	c := connect(t, true)
	start := time.Now()
	_, e := Run(context.Background(), c, m, "../../testdata/import", Options{LockWait: 120 * time.Millisecond})
	if e != ErrBusy || time.Since(start) > time.Second || !c.IsClosed() {
		t.Fatalf("bounded lock %v", e)
	}
	_ = holder.Close(context.Background())
	p, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 1, afterBatch: func() error { return context.Canceled }})
	if e != context.Canceled {
		t.Fatal(e)
	}
	admin := connect(t, false)
	// Model a process death: its session disappeared, but its run remained running.
	if _, e = admin.Exec(context.Background(), "UPDATE slovnyk.import_runs SET state='running' WHERE id=$1", p.Attempt); e != nil {
		t.Fatal(e)
	}
	r, e := runFixture(t, m, "../../testdata/import", Options{})
	if e != nil {
		t.Fatalf("%+v %v", r, e)
	}
	if scalar(t, admin, "SELECT count(*) FROM slovnyk.import_runs WHERE dataset_id=$1 AND state='running'", r.Dataset) != 0 {
		t.Fatal("orphan running attempt")
	}
}
func TestReplayDetectsDamagedLedger(t *testing.T) {
	m := variant(t, "damaged-ledger")
	p, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 2, afterBatch: func() error { return context.Canceled }})
	if e != context.Canceled {
		t.Fatal(e)
	}
	admin := connect(t, false)
	if _, e = admin.Exec(context.Background(), "UPDATE slovnyk.translation_origins SET source_locator='wrong' WHERE dataset_id=$1", p.Dataset); e != nil {
		t.Fatal(e)
	}
	r, e := runFixture(t, m, "../../testdata/import", Options{})
	if e != ErrReplay || r.State != "failed" {
		t.Fatalf("%+v %v", r, e)
	}
}

func TestCollisionGuardBeforeMerge(t *testing.T) {
	m := variant(t, "injected-hash-collision")
	// Commit one tuple, then substitute its hash in the private staging table for
	// a different raw tuple. No schema guards are disabled and no fake hash can
	// reach translations: the importer must reject it before insert/merge.
	p, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 1, afterBatch: func() error { return context.Canceled }})
	if e != context.Canceled {
		t.Fatal(e)
	}
	injected := false
	r, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 1, afterStage: func(ctx context.Context, tx pgx.Tx) error {
		var ordinal int64
		if e := tx.QueryRow(ctx, "SELECT min(ordinal) FROM pg_temp.slovnyk_import_batch").Scan(&ordinal); e != nil {
			return e
		}
		if ordinal > 1 && !injected {
			injected = true
			_, e := tx.Exec(ctx, `UPDATE pg_temp.slovnyk_import_batch SET source_text=source_text||'_injected_collision',tuple_hash=(SELECT tuple_hash FROM slovnyk.translations WHERE dataset_id=$1 ORDER BY id LIMIT 1)`, p.Dataset)
			return e
		}
		return nil
	}})
	if e != ErrCollision || r.State != "failed" || r.Files[0].Parsed != 1 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestLostSessionStopsWriterAndCanResume(t *testing.T) {
	admin := connect(t, false)
	c := connect(t, true)
	pid := c.PgConn().PID()
	m := variant(t, "lost-session")
	r, e := Run(context.Background(), c, m, "../../testdata/import", Options{BatchSize: 2, afterBatch: func() error {
		var terminated bool
		return admin.QueryRow(context.Background(), "SELECT pg_terminate_backend($1)", pid).Scan(&terminated)
	}})
	if e == nil || !c.IsClosed() {
		t.Fatalf("lost session continued %+v %v", r, e)
	}
	resumed, e := runFixture(t, m, "../../testdata/import", Options{BatchSize: 3})
	if e != nil || resumed.Files[0].Parsed != 11 || resumed.Files[0].Unique != 10 {
		t.Fatalf("%+v %v", resumed, e)
	}
}
func TestCrossFileOwnershipAndEmptyArtifact(t *testing.T) {
	m, root := generated(t, "multiple-artifacts", []byte("en-us,uk-ua,same,однаково\n"), false)
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	_, _ = g.Write([]byte("en-us,uk-ua,same,однаково\nen-us,uk-ua,new,новий\n"))
	_ = g.Close()
	h := sha256.Sum256(b.Bytes())
	a := m.Artifacts[0]
	a.Name = "second.csv.gz"
	a.Bytes = int64(b.Len())
	a.SHA256 = hex.EncodeToString(h[:])
	m.Artifacts = append(m.Artifacts, a)
	if e := os.WriteFile(filepath.Join(root, a.Name), b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	b.Reset()
	g = gzip.NewWriter(&b)
	_ = g.Close()
	h = sha256.Sum256(b.Bytes())
	a.Name = "empty.csv.gz"
	a.Bytes = int64(b.Len())
	a.SHA256 = hex.EncodeToString(h[:])
	m.Artifacts = append(m.Artifacts, a)
	if e := os.WriteFile(filepath.Join(root, a.Name), b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := runFixture(t, m, root, Options{BatchSize: 1})
	if e != nil {
		t.Fatalf("%+v %v", r, e)
	}
	var total, unique, duplicates int64
	for _, f := range r.Files {
		if !f.Complete {
			t.Fatal(f)
		}
		total += f.Parsed
		unique += f.Unique
		duplicates += f.Duplicate
	}
	if total != 3 || unique != 2 || duplicates != 1 {
		t.Fatal(r)
	}
	admin := connect(t, false)
	want := snapshot(t, admin, r.Dataset)
	m.Source.Authority = "multiple-artifacts-reordered"
	m.Artifacts[0], m.Artifacts[2] = m.Artifacts[2], m.Artifacts[0]
	other, e := runFixture(t, m, root, Options{BatchSize: 3})
	if e != nil || snapshot(t, admin, other.Dataset) != want {
		t.Fatalf("%+v %v", other, e)
	}
}
