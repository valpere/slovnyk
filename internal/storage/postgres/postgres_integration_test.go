//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func database(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("integration requires disposable SLOVNYK_TEST_DSN; use make test-integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}
func execSQL(t *testing.T, c interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, sql string, args ...any) {
	t.Helper()
	if _, err := c.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("SQL failed: %v", err)
	}
}
func expectCode(t *testing.T, c interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, code, sql string, args ...any) {
	t.Helper()
	_, err := c.Exec(context.Background(), sql, args...)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code {
		t.Fatalf("expected SQLSTATE %s; got %v", code, err)
	}
}
func seed(t *testing.T, c *pgx.Conn) string {
	t.Helper()
	var id string
	if err := c.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	execSQL(t, c, `INSERT INTO slovnyk.datasets(id,fingerprint,manifest_bytes,schema_version,parser_version,mapping_version,normalization_version)
 VALUES($1::uuid,sha256(convert_to($1::text,'UTF8')),convert_to($1::text,'UTF8'),1,'lossless-csv4-v1','identity-v1','test-normalizer')`, id)
	execSQL(t, c, `UPDATE slovnyk.datasets SET state='loading' WHERE id=$1`, id)
	execSQL(t, c, `INSERT INTO slovnyk.dataset_locales(dataset_id,code) VALUES($1,'en'),($1,'uk')`, id)
	execSQL(t, c, `INSERT INTO slovnyk.dataset_pairs(dataset_id,source_locale,target_locale) VALUES($1,'en','uk'),($1,'en','en')`, id)
	return id
}

const translationSQL = `INSERT INTO slovnyk.translations(dataset_id,id,source_locale,target_locale,source_text,target_text,source_key,key_hash,tuple_hash)
 VALUES($1,$2,'en','uk',$3::text,$4::text,$5::text,sha256(convert_to($5::text,'UTF8')),
 sha256(int8send(2::bigint)||convert_to('en','UTF8')||int8send(2::bigint)||convert_to('uk','UTF8')||
 int8send(octet_length($3::text)::bigint)||convert_to($3::text,'UTF8')||int8send(octet_length($4::text)::bigint)||convert_to($4::text,'UTF8')))`

func ready(t *testing.T, c *pgx.Conn, id string) {
	t.Helper()
	execSQL(t, c, `UPDATE slovnyk.datasets SET state='validating' WHERE id=$1`, id)
	execSQL(t, c, `UPDATE slovnyk.datasets SET state='ready',report_path='synthetic/report.json',report_hash=sha256('synthetic'::bytea) WHERE id=$1`, id)
}

func TestMigrations(t *testing.T) {
	c := database(t)
	ctx := context.Background()
	if err := Migrate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, c); err != nil {
		t.Fatal("idempotent migration", err)
	}
	if err := CheckSchema(ctx, c); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := CheckSchema(cancelled, c); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled schema check: %v", err)
	}
	execSQL(t, c, `SET ROLE slovnyk_http`)
	if err := CheckSchema(ctx, c); err != nil {
		t.Fatal(err)
	}
	execSQL(t, c, `RESET ROLE`)
	// Corrupt inventory must be refused, never silently "repaired".
	execSQL(t, c, `UPDATE slovnyk.schema_migrations SET checksum=decode(repeat('00',32),'hex') WHERE version=1`)
	if err := Migrate(ctx, c); !errors.Is(err, ErrSchema) {
		t.Fatalf("tampered checksum: %v", err)
	}
	if err := CheckSchema(ctx, c); !errors.Is(err, ErrSchema) {
		t.Fatal(err)
	}
	// Restore the synthetic test ledger as owner so subsequent tests can proceed.
	execSQL(t, c, `DROP SCHEMA slovnyk CASCADE`)
	if err := Migrate(ctx, c); err != nil {
		t.Fatal("fresh rebuild", err)
	}
}

func TestConstraintsAndPrivileges(t *testing.T) {
	c := database(t)
	a, b := seed(t, c), seed(t, c)
	execSQL(t, c, translationSQL, a, 1, "Straße", "вулиця", "strasse")
	execSQL(t, c, translationSQL, a, 7, "STRASSE", "вулиця", "strasse")
	long := strings.Repeat("é", 32768)
	execSQL(t, c, translationSQL, a, 2, long, long, long)
	expectCode(t, c, "23514", translationSQL, a, 3, long+"x", "x", "key")
	expectCode(t, c, "23505", translationSQL, a, 4, "Straße", "вулиця", "strasse")
	expectCode(t, c, "23514", `INSERT INTO slovnyk.translations SELECT dataset_id,9,source_locale,target_locale,source_text,target_text,source_key,decode('00','hex'),tuple_hash FROM slovnyk.translations WHERE dataset_id=$1 AND id=1`, a)
	expectCode(t, c, "23503", `INSERT INTO slovnyk.dataset_pairs VALUES($1,'xx','uk',0)`, a)
	execSQL(t, c, `INSERT INTO slovnyk.source_files(dataset_id,artifact_hash,relative_name,format,byte_size) VALUES($1,sha256('artifact'::bytea),'a.csv.gz','csv4.gz',100)`, a)
	execSQL(t, c, `INSERT INTO slovnyk.source_files(dataset_id,artifact_hash,relative_name,format,byte_size) VALUES($1,sha256('artifact'::bytea),'a.csv.gz','csv4.gz',100)`, b)
	execSQL(t, c, `INSERT INTO slovnyk.import_runs(id,dataset_id,attempt,state) VALUES('11111111-1111-1111-1111-111111111111',$1,1,'running')`, a)
	expectCode(t, c, "23505", `INSERT INTO slovnyk.import_runs(id,dataset_id,attempt,state) VALUES('22222222-2222-2222-2222-222222222222',$1,2,'running')`, a)
	expectCode(t, c, "23503", `INSERT INTO slovnyk.import_checkpoints VALUES($1,sha256('artifact'::bytea),'11111111-1111-1111-1111-111111111111',0)`, b)
	execSQL(t, c, `INSERT INTO slovnyk.import_checkpoints VALUES($1,sha256('artifact'::bytea),'11111111-1111-1111-1111-111111111111',0)`, a)
	expectCode(t, c, "23503", `INSERT INTO slovnyk.import_records VALUES($1,sha256('artifact'::bytea),1,'unique',1,NULL)`, b)
	execSQL(t, c, `INSERT INTO slovnyk.import_records VALUES($1,sha256('artifact'::bytea),1,'unique',1,NULL),($1,sha256('artifact'::bytea),2,'duplicate',1,NULL),($1,sha256('artifact'::bytea),3,'rejected',NULL,'nul')`, a)
	expectCode(t, c, "23514", `INSERT INTO slovnyk.import_records VALUES($1,sha256('artifact'::bytea),4,'rejected',1,'nul')`, a)
	execSQL(t, c, `INSERT INTO slovnyk.translation_origins(dataset_id,artifact_hash,ordinal,translation_id,source_locale,target_locale,source_text,target_text,legacy_count,legacy_status,source_locator)
 VALUES($1,sha256('artifact'::bytea),1,1,'en','uk','Straße','вулиця',NULL,0,'ordinal:1'),($1,sha256('artifact'::bytea),2,1,'en','uk','Straße','вулиця',0,-1,'ordinal:2')`, a)
	expectCode(t, c, "23514", `UPDATE slovnyk.translation_origins SET source_text='wrong' WHERE dataset_id=$1`, a)
	expectCode(t, c, "23503", `INSERT INTO slovnyk.translation_origins(dataset_id,artifact_hash,ordinal,translation_id,source_locale,target_locale,source_text,target_text,source_locator) VALUES($1,sha256('artifact'::bytea),3,1,'en','uk','Straße','вулиця','ordinal:3')`, a)
	expectCode(t, c, "23514", `UPDATE slovnyk.source_files SET complete=true,checksum_verified=true,parsed=100 WHERE dataset_id=$1`, a)
	execSQL(t, c, `SET ROLE slovnyk_http`)
	for _, sql := range []string{
		`UPDATE slovnyk.active_dataset SET generation=generation+1`,
		`INSERT INTO slovnyk.dataset_locales(dataset_id,code) VALUES('00000000-0000-0000-0000-000000000001','x')`,
		`DELETE FROM slovnyk.translations`, `TRUNCATE slovnyk.translations`,
		`SELECT * FROM slovnyk.translation_origins`, `CREATE TABLE slovnyk.forbidden(id int)`,
	} {
		expectCode(t, c, "42501", sql)
	}
	execSQL(t, c, `SELECT source_text FROM slovnyk.translations`)
	entries, err := Exact(context.Background(), c, a, "en", "uk", "strasse", 100)
	if err != nil || len(entries) != 2 || entries[0].SourceText != "STRASSE" || entries[1].SourceText != "Straße" {
		t.Fatalf("C-sorted raw variants: %+v %v", entries, err)
	}
	entries, err = Exact(context.Background(), c, a, "en", "uk", long, 1)
	if err != nil || len(entries) != 1 || entries[0].SourceText != long {
		t.Fatal("long exact key", err)
	}
	entries, err = Exact(context.Background(), c, b, "en", "uk", "strasse", 1)
	if err != nil || len(entries) != 0 {
		t.Fatal("dataset isolation", err)
	}
	execSQL(t, c, `RESET ROLE`)
	execSQL(t, c, `SET ROLE slovnyk_operator`)
	execSQL(t, c, translationSQL, a, 5, "word", "слово", "word")
	expectCode(t, c, "42501", `TRUNCATE slovnyk.translations`)
	expectCode(t, c, "42501", `ALTER TABLE slovnyk.translations DISABLE TRIGGER ALL`)
	expectCode(t, c, "42501", `UPDATE slovnyk.schema_migrations SET version=2`)
	execSQL(t, c, `RESET ROLE`)
	ready(t, c, a)
	execSQL(t, c, `SET ROLE slovnyk_operator`)
	expectCode(t, c, "55000", translationSQL, a, 6, "other", "інше", "other")
	expectCode(t, c, "55000", `UPDATE slovnyk.dataset_pairs SET unique_count=3 WHERE dataset_id=$1`, a)
	expectCode(t, c, "55000", `DELETE FROM slovnyk.translation_origins WHERE dataset_id=$1`, a)
	expectCode(t, c, "55000", `UPDATE slovnyk.datasets SET state='loading' WHERE id=$1`, a)
	execSQL(t, c, `RESET ROLE`)
}

func TestBatchRollbackAndPublicationConstraints(t *testing.T) {
	c := database(t)
	id := seed(t, c)
	ctx := context.Background()
	tx, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, tx, translationSQL, id, 1, "word", "слово", "word")
	execSQL(t, tx, `INSERT INTO slovnyk.source_files(dataset_id,artifact_hash,relative_name,format,byte_size) VALUES($1,sha256('batch'::bytea),'batch.csv.gz','csv4.gz',1)`, id)
	execSQL(t, tx, `INSERT INTO slovnyk.import_records VALUES($1,sha256('batch'::bytea),1,'unique',1,NULL)`, id)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := c.QueryRow(ctx, `SELECT count(*) FROM slovnyk.translations WHERE dataset_id=$1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	expectCode(t, c, "55000", `UPDATE slovnyk.active_dataset SET dataset_id=$1,generation=1`, id)
	ready(t, c, id)
	expectCode(t, c, "23514", `UPDATE slovnyk.active_dataset SET dataset_id=$1,generation=1`, id)
	tx, err = c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, tx, `UPDATE slovnyk.active_dataset SET dataset_id=$1,generation=1`, id)
	execSQL(t, tx, `INSERT INTO slovnyk.publication_events(generation,new_dataset_id,operator_ref) VALUES(1,$1,'synthetic')`, id)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	expectCode(t, c, "55000", `UPDATE slovnyk.active_dataset SET generation=1`)
	expectCode(t, c, "55000", `UPDATE slovnyk.publication_events SET operator_ref='changed'`)
	expectCode(t, c, "23514", `INSERT INTO slovnyk.publication_events(generation,new_dataset_id,operator_ref) VALUES(2,$1,'fake')`, id)
}

func TestReadyTransitionSerializesWithWrites(t *testing.T) {
	c := database(t)
	other := database(t)
	id := seed(t, c)
	ctx := context.Background()
	execSQL(t, c, `UPDATE slovnyk.datasets SET state='validating' WHERE id=$1`, id)
	tx, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	execSQL(t, tx, translationSQL, id, 1, "word", "слово", "word")
	blocked, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	_, err = other.Exec(blocked, `UPDATE slovnyk.datasets SET state='ready',report_path='synthetic',report_hash=sha256('r'::bytea) WHERE id=$1`, id)
	cancel()
	if err == nil {
		t.Fatal("ready did not wait for content writer")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Cancellation may discard the other connection; a new one proves recovery.
	next := database(t)
	execSQL(t, next, `UPDATE slovnyk.datasets SET state='ready',report_path='synthetic',report_hash=sha256('r'::bytea) WHERE id=$1`, id)
	expectCode(t, next, "55000", translationSQL, id, 2, "later", "пізніше", "later")
}
