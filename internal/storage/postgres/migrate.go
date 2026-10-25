// Package postgres supplies PostgreSQL schema and storage primitives.
package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/valpere/slovnyk/migrations"
)

const SchemaVersion = 2

var (
	ErrUnavailable = errors.New("database_unavailable")
	ErrConfig      = errors.New("invalid_database_configuration")
	ErrSchema      = errors.New("incompatible_schema")
	ErrMigration   = errors.New("migration_failed")
)

// Connect requires an explicit DSN and never returns parser/driver errors that
// can contain credentials. The caller owns and must close the connection.
func Connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrConfig
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, ErrConfig
	}
	cfg.ConnectTimeout = 5 * time.Second
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, ErrUnavailable
	}
	return conn, nil
}

// Migrate is an explicit operator operation requiring the schema owner and
// CREATEROLE for the initial group roles. HTTP startup never runs migrations.
// Every pending migration and its ledger entry commit in one transaction.
func Migrate(ctx context.Context, conn *pgx.Conn) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var encoding string
	if err := tx.QueryRow(ctx, `SHOW server_encoding`).Scan(&encoding); err != nil {
		return ErrUnavailable
	}
	if encoding != "UTF8" {
		return ErrSchema
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='30s';
 SELECT pg_advisory_xact_lock(1397509974,1);
 CREATE SCHEMA IF NOT EXISTS slovnyk;
 CREATE TABLE IF NOT EXISTS slovnyk.schema_migrations (
 version integer PRIMARY KEY CHECK(version>0), checksum bytea NOT NULL CHECK(octet_length(checksum)=32),
 applied_at timestamptz NOT NULL DEFAULT clock_timestamp());`); err != nil {
		return ErrMigration
	}
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return ErrMigration
	}
	sort.Strings(entries)
	applied := map[int][]byte{}
	rows, err := tx.Query(ctx, `SELECT version,checksum FROM slovnyk.schema_migrations ORDER BY version`)
	if err != nil {
		return ErrSchema
	}
	for rows.Next() {
		var version int
		var sum []byte
		if err := rows.Scan(&version, &sum); err != nil {
			rows.Close()
			return ErrSchema
		}
		applied[version] = sum
	}
	rows.Close()
	if rows.Err() != nil {
		return ErrSchema
	}
	if len(applied) > len(entries) {
		return ErrSchema
	}
	for i, name := range entries {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil || version != i+1 {
			return ErrSchema
		}
		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			return ErrMigration
		}
		sum := sha256.Sum256(sql)
		if old, ok := applied[version]; ok {
			if string(old) != string(sum[:]) {
				return ErrSchema
			}
			continue
		}
		// Reject histories with a gap, even if a newer known version was injected.
		for known := range applied {
			if known > version {
				return ErrSchema
			}
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%w: version %d", ErrMigration, version)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO slovnyk.schema_migrations(version,checksum) VALUES($1,$2)`, version, sum[:]); err != nil {
			return ErrMigration
		}
	}
	if len(entries) != SchemaVersion {
		return ErrSchema
	}
	if err := tx.Commit(ctx); err != nil {
		return ErrMigration
	}
	return nil
}

// CheckSchema verifies the full migration inventory, not just a version number.
// It is usable by the HTTP group and performs no DDL or writes.
func CheckSchema(ctx context.Context, conn *pgx.Conn) error {
	rows, err := conn.Query(ctx, `SELECT version,checksum FROM slovnyk.schema_migrations ORDER BY version`)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			return ErrSchema
		}
		return ErrUnavailable
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var sum []byte
		if err := rows.Scan(&version, &sum); err != nil {
			return ErrSchema
		}
		count++
		if version != count || version > SchemaVersion {
			return ErrSchema
		}
		files, _ := fs.Glob(migrations.Files, fmt.Sprintf("%06d_*.sql", version))
		if len(files) != 1 {
			return ErrSchema
		}
		data, _ := migrations.Files.ReadFile(files[0])
		want := sha256.Sum256(data)
		if string(want[:]) != string(sum) {
			return ErrSchema
		}
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	if count != SchemaVersion {
		return ErrSchema
	}
	return nil
}
