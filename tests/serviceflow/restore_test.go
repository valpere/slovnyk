//go:build integration

package serviceflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type digestCount struct {
	SHA256 string `json:"sha256"`
	Count  int64  `json:"count"` // Rows for tables, bytes for files.
}

// tableInventory compares all persisted rows, including manifests, provenance,
// reports, publication history and schema checksums. The caller stops writers.
func tableInventory(t *testing.T, ctx context.Context, c *pgx.Conn) map[string]digestCount {
	t.Helper()
	if _, err := c.Exec(ctx, `SET timezone='UTC'; SET datestyle='ISO,YMD'`); err != nil {
		t.Fatal("inventory session setup failed")
	}
	rows, err := c.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='slovnyk' ORDER BY tablename COLLATE "C"`)
	if err != nil {
		t.Fatal("table inventory unavailable")
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal("table inventory scan failed")
		}
		tables = append(tables, name)
	}
	rows.Close()
	if rows.Err() != nil || len(tables) == 0 {
		t.Fatal("empty or incomplete table inventory")
	}
	out := map[string]digestCount{}
	for _, table := range tables {
		rows, err := c.Query(ctx, `SELECT row_to_json(t)::text FROM `+pgx.Identifier{"slovnyk", table}.Sanitize()+` t ORDER BY row_to_json(t)::text COLLATE "C"`)
		if err != nil {
			t.Fatal("table digest query failed")
		}
		hash := sha256.New()
		var count int64
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				t.Fatal("table digest scan failed")
			}
			var size [8]byte
			binary.BigEndian.PutUint64(size[:], uint64(len(text)))
			_, _ = hash.Write(size[:])
			_, _ = io.WriteString(hash, text)
			count++
		}
		rows.Close()
		if rows.Err() != nil {
			t.Fatal("table digest incomplete")
		}
		out[table] = digestCount{hex.EncodeToString(hash.Sum(nil)), count}
	}
	return out
}

// fixtureTree copies regular test files only, and records every byte. These are
// synthetic test fixtures, not an operator backup utility or untrusted restore API.
func fixtureTree(t *testing.T, source, destination string) map[string]digestCount {
	t.Helper()
	out := map[string]digestCount{}
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(destination, rel), 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fs.ErrInvalid
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		file, err := os.OpenFile(filepath.Join(destination, rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer file.Close()
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(file, hash), in)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = digestCount{hex.EncodeToString(hash.Sum(nil)), n}
		return nil
	})
	if err != nil || len(out) == 0 {
		t.Fatal("fixture copy failed or empty")
	}
	return out
}

// restoreFixture uses pg_dump/pg_restore from the runner's pinned PostgreSQL
// containers. The target is a separate empty cluster with a different owner.
// No credentials enter command arguments or test output.
func restoreFixture(t *testing.T, ctx context.Context, admin *pgx.Conn, sourceDSN, sourceArtifacts, dir string) (string, string, string) {
	t.Helper()
	started := time.Now()
	checkContainer := func(variable string) string {
		t.Helper()
		container := os.Getenv(variable)
		decoded, err := hex.DecodeString(container)
		if err != nil || len(decoded) != 32 {
			t.Fatal("use disposable runner containers for restore rehearsal")
		}
		inspect := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }}`, container)
		labels, err := inspect.Output()
		if err != nil || !strings.HasPrefix(string(labels), "slovnyk-test-") || !strings.HasSuffix(string(labels), "|db\n") {
			t.Fatal("restore container is not the disposable runner")
		}
		return container
	}
	container := checkContainer("SLOVNYK_TEST_DB_CONTAINER")
	targetContainer := checkContainer("SLOVNYK_RESTORE_TEST_CONTAINER")
	if container == targetContainer {
		t.Fatal("restore requires a separate cluster")
	}
	targetURL, err := url.Parse(os.Getenv("SLOVNYK_RESTORE_TEST_DSN"))
	if err != nil || targetURL.Scheme != "postgresql" || targetURL.Hostname() != "127.0.0.1" || targetURL.Path != "/slovnyk_restore_test" {
		t.Fatal("restore target is not the isolated runner cluster")
	}
	target, err := pgx.Connect(ctx, targetURL.String())
	if err != nil {
		t.Fatal("restore cluster unavailable")
	}
	t.Cleanup(func() { _ = target.Close(context.Background()) })
	var sourceID, targetID string
	if admin.QueryRow(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&sourceID) != nil ||
		target.QueryRow(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&targetID) != nil || sourceID == targetID {
		t.Fatal("restore must use a different PostgreSQL cluster")
	}
	var existing int
	if target.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname IN ('slovnyk_http','slovnyk_operator','slovnyk_test')`).Scan(&existing) != nil || existing != 0 {
		t.Fatal("restore cluster already has application or source owner roles")
	}
	u, err := url.Parse(sourceDSN)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(u.Path, "/slovnyk_flow_") {
		t.Fatal("restore source is not the isolated fixture database")
	}
	source, err := pgx.Connect(ctx, sourceDSN)
	if err != nil {
		t.Fatal("source inventory connection failed")
	}
	defer source.Close(ctx)
	expected := tableInventory(t, ctx, source)
	bundle := filepath.Join(dir, "backup")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	dump, err := os.OpenFile(filepath.Join(bundle, "database.dump"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer dump.Close()
	command := func(container, database, role, script string, input io.Reader, output io.Writer) bool {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "PGDATABASE="+database, "-e", "PGROLE="+role, container, "sh", "-ec", script)
		cmd.Env, cmd.Stdin, cmd.Stdout = cleanEnv(), input, output
		var diagnostic bytes.Buffer
		cmd.Stderr = &diagnostic
		return cmd.Run() == nil // PostgreSQL diagnostics can contain private data.
	}
	if !command(container, strings.TrimPrefix(u.Path, "/"), "", `export PGPASSWORD="$POSTGRES_PASSWORD"; exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" --format=custom --no-owner`, nil, dump) {
		t.Fatal("PostgreSQL archive creation failed (diagnostics withheld)")
	}
	if err := dump.Sync(); err != nil {
		t.Fatal("dump sync failed")
	}
	if _, err := dump.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	dumpBytes, err := io.Copy(hash, dump)
	if err != nil || dumpBytes == 0 {
		t.Fatal("empty or unreadable dump")
	}
	files := fixtureTree(t, sourceArtifacts, filepath.Join(bundle, "artifacts"))
	// Preserve original compressed source bytes as well as embedded DB manifests.
	sources := map[string]digestCount{}
	for _, name := range []string{"A", "B"} {
		for path, digest := range fixtureTree(t, filepath.Join(dir, name), filepath.Join(bundle, "sources", name)) {
			sources[name+"/"+path] = digest
		}
	}
	manifest, err := json.Marshal(struct {
		Database digestCount            `json:"database"`
		Tables   map[string]digestCount `json:"tables"`
		Files    map[string]digestCount `json:"artifacts"`
		Sources  map[string]digestCount `json:"sources"`
	}{digestCount{hex.EncodeToString(hash.Sum(nil)), dumpBytes}, expected, files, sources})
	if err != nil || os.WriteFile(filepath.Join(bundle, "inventory.json"), manifest, 0600) != nil {
		t.Fatal("backup inventory write failed")
	}
	suffix := strings.TrimPrefix(u.Path, "/slovnyk_flow_")
	restoredDB, owner := "slovnyk_restore_"+suffix, "restore_owner_"+suffix
	reader, operator := "restore_http_"+suffix, "restore_operator_"+suffix
	// Register cleanup before provisioning, including partially created roles.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := target.Exec(cleanup, "DROP DATABASE IF EXISTS "+pgx.Identifier{restoredDB}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error("restore target cleanup failed")
		}
		for _, role := range []string{reader, operator, owner, "slovnyk_http", "slovnyk_operator"} {
			if _, err := target.Exec(cleanup, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Error("restore role cleanup failed")
			}
		}
	})
	if _, err := target.Exec(ctx, "CREATE ROLE "+pgx.Identifier{owner}.Sanitize()+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal("restore owner creation failed")
	}
	if _, err := target.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{restoredDB}.Sanitize()+" OWNER "+pgx.Identifier{owner}.Sanitize()+" TEMPLATE template0 ENCODING 'UTF8'"); err != nil {
		t.Fatal("restore target creation failed")
	}
	restore := func() bool {
		t.Helper()
		if _, err := dump.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		return command(targetContainer, restoredDB, owner, `export PGPASSWORD="$POSTGRES_PASSWORD"; exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" --role="$PGROLE" --no-owner --exit-on-error --single-transaction --dbname="$PGDATABASE"`, dump, io.Discard)
	}
	// Without the named ACL grantees, fail closed and roll back the entire restore.
	if restore() {
		t.Fatal("restore unexpectedly accepted missing group roles")
	}
	targetURL.Path = "/" + restoredDB
	restored, err := pgx.Connect(ctx, targetURL.String())
	if err != nil {
		t.Fatal("restore inventory connection failed")
	}
	defer restored.Close(ctx)
	if restored.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname='slovnyk'`).Scan(&existing) != nil || existing != 0 {
		t.Fatal("failed restore left a partial application schema")
	}
	for _, group := range []string{"slovnyk_http", "slovnyk_operator"} {
		if _, err := target.Exec(ctx, "CREATE ROLE "+pgx.Identifier{group}.Sanitize()+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
			t.Fatal("restore group provisioning failed")
		}
	}
	for login, group := range map[string]string{reader: "slovnyk_http", operator: "slovnyk_operator"} {
		if _, err := target.Exec(ctx, "CREATE ROLE "+pgx.Identifier{login}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'synthetic-restore-only' IN ROLE "+pgx.Identifier{group}.Sanitize()); err != nil {
			t.Fatal("restore LOGIN provisioning failed")
		}
	}
	if !restore() {
		t.Fatal("PostgreSQL restore failed (diagnostics withheld)")
	}
	var unsafe int
	if restored.QueryRow(ctx, `SELECT count(*) FROM (
 SELECT nspowner AS owner FROM pg_namespace WHERE nspname='slovnyk'
 UNION ALL SELECT c.relowner FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='slovnyk'
 UNION ALL SELECT p.proowner FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='slovnyk'
 UNION ALL SELECT t.typowner FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname='slovnyk'
) objects WHERE owner <> $1::regrole`, owner).Scan(&unsafe) != nil || unsafe != 0 {
		t.Fatal("restored objects do not belong to the new schema owner")
	}
	if restored.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname=ANY($1) AND
 (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)`, []string{owner, reader, operator, "slovnyk_http", "slovnyk_operator"}).Scan(&unsafe) != nil || unsafe != 0 {
		t.Fatal("restore provisioned unsafe service roles")
	}
	if !maps.Equal(expected, tableInventory(t, ctx, restored)) {
		t.Fatal("restored table counts/digests differ")
	}
	restoredArtifacts := filepath.Join(dir, "restored-artifacts")
	if !maps.Equal(files, fixtureTree(t, filepath.Join(bundle, "artifacts"), restoredArtifacts)) {
		t.Fatal("restored artifact paths/bytes/digests differ")
	}
	if !maps.Equal(sources, fixtureTree(t, filepath.Join(bundle, "sources"), filepath.Join(dir, "restored-sources"))) {
		t.Fatal("restored source paths/bytes/digests differ")
	}
	t.Logf("restore rehearsal: %d tables, %d artifacts and %d source files match; dump %d bytes; local preparation/restore %s", len(expected), len(files), len(sources), dumpBytes, time.Since(started).Round(time.Millisecond))
	t.Log("fresh cluster: source owner absent; missing groups fail atomically; new non-superuser owner owns schema/tables/functions")
	roleDSN := func(login string) string {
		v := *targetURL
		v.User = url.UserPassword(login, "synthetic-restore-only")
		return v.String()
	}
	return roleDSN(reader), roleDSN(operator), restoredArtifacts
}
