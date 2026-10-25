//go:build integration

package sitemaps

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func TestReadyDatasetDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connect := func(role string) *pgx.Conn {
		t.Helper()
		c, err := postgres.Connect(ctx, os.Getenv("SLOVNYK_TEST_DSN"))
		if err != nil {
			t.Fatal("use make test-integration", err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		if role != "" {
			if _, err := c.Exec(ctx, "SET ROLE "+role); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	store, root := testStore(t)
	b, err := os.ReadFile("../../testdata/import/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := importdata.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	m.Source.Authority = "sitemap integration"
	m.Locales = []string{"en-us", "uk-ua"}
	m.ExpectedPairs = []importdata.Pair{{From: "en-us", To: "uk-ua"}, {From: "uk-ua", To: "en-us"}}
	records := [][]string{
		{"en-us", "uk-ua", "word", "one"},
		{"en-us", "uk-ua", "word", "two"},
		{"en-us", "uk-ua", "WORD", "case"},
		{"en-us", "uk-ua", " word ", "spaces"},
		{"en-us", "uk-ua", "É", "NFC"},
		{"en-us", "uk-ua", "É", "NFD"},
		{"uk-ua", "en-us", "word", "reverse pair"},
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if err := csv.NewWriter(z).WriteAll(records); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.Mkdir(input, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "corpus.csv.gz"), compressed.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(compressed.Bytes())
	m.Artifacts = []importdata.Artifact{{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(compressed.Len()), Name: "corpus.csv.gz", Format: "csv4.gz", MultiPair: true}}
	staged, err := importer.Run(ctx, connect("slovnyk_operator"), m, input, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Imported data is still validating, so it must never produce discovery output.
	if _, err := Generate(ctx, connect("slovnyk_operator"), staged.Dataset, root); !errors.Is(err, ErrState) {
		t.Fatalf("validating dataset: %v", err)
	}
	if _, err := Generate(ctx, connect("slovnyk_operator"), "00000000-0000-0000-0000-000000000000", root); !errors.Is(err, ErrState) {
		t.Fatalf("unknown dataset: %v", err)
	}
	if _, err := Generate(ctx, connect("slovnyk_operator"), "bad-id", root); !errors.Is(err, ErrArguments) {
		t.Fatalf("invalid dataset: %v", err)
	}
	if _, err := publication.Verify(ctx, connect("slovnyk_operator"), staged.Dataset, input, root, publication.Options{}); err != nil {
		t.Fatal(err)
	}
	owner := connect("")
	var generation int64
	var active string
	if err := owner.QueryRow(ctx, "SELECT generation,coalesce(dataset_id::text,'') FROM slovnyk.active_dataset").Scan(&generation, &active); err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, connect("slovnyk_operator"), staged.Dataset, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(ctx, connect("slovnyk_http"), staged.Dataset, result.Shards[0].Artifact.SHA256); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpublished sitemap exposed: %v", err)
	}
	// Independently construct expected identities from raw fields in the fixture;
	// SQL DISTINCT must collapse translations of the same raw source and pair.
	expected := map[string]bool{}
	for _, record := range records {
		expected[origin+entries.Path(entries.ID(record[0], record[1], record[2]))] = true
	}
	count := len(records)
	if result.Dataset != staged.Dataset || result.URLs != int64(len(expected)) || len(expected) >= count {
		t.Fatalf("expected deduplicated source identities: rows=%d result=%+v", count, result)
	}
	var actual []string
	for _, shard := range result.Shards {
		var doc struct {
			URLs []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal(artifactBytes(t, store, shard.Artifact), &doc); err != nil {
			t.Fatal(err)
		}
		for _, u := range doc.URLs {
			actual = append(actual, u.Loc)
		}
	}
	var want []string
	for url := range expected {
		want = append(want, url)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("wrong raw spelling/pair identities: %v, want %v", actual, want)
	}
	var durable Manifest
	if err := json.Unmarshal(artifactBytes(t, store, result.Report), &durable); err != nil || !reflect.DeepEqual(durable, result.Manifest) {
		t.Fatalf("durable report: %v", err)
	}
	var afterGeneration int64
	var afterActive string
	if err := owner.QueryRow(ctx, "SELECT generation,coalesce(dataset_id::text,'') FROM slovnyk.active_dataset").Scan(&afterGeneration, &afterActive); err != nil {
		t.Fatal(err)
	}
	if generation != afterGeneration || active != afterActive {
		t.Fatal("offline generation changed publication")
	}
	if _, err := publication.Publish(ctx, connect("slovnyk_operator"), staged.Dataset, root, generation, "sitemap test", false); err != nil {
		t.Fatal(err)
	}
	repeat, err := Generate(ctx, connect("slovnyk_operator"), staged.Dataset, root)
	if err != nil || !reflect.DeepEqual(repeat, result) {
		t.Fatalf("publication changed sitemap bytes: %v", err)
	}
	activeRef, err := Resolve(ctx, connect("slovnyk_http"), "", "")
	if err != nil || activeRef != result.Index {
		t.Fatalf("active index: %+v %v", activeRef, err)
	}
	shardRef, err := Resolve(ctx, connect("slovnyk_http"), staged.Dataset, result.Shards[0].Artifact.SHA256)
	if err != nil || shardRef != result.Shards[0].Artifact {
		t.Fatalf("registered shard: %+v %v", shardRef, err)
	}
	if err := Check(ctx, connect("slovnyk_http"), root, staged.Dataset); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, shardRef.Path)
	original := artifactBytes(t, store, shardRef)
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), original...)
	changed[len(changed)-1] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, connect("slovnyk_http"), root, staged.Dataset); err == nil {
		t.Fatal("same-size corrupt shard accepted")
	}
	if err := os.WriteFile(path, original, 0440); err != nil {
		t.Fatal(err)
	}
	expectCode := func(c *pgx.Conn, code, sql string, args ...any) {
		t.Helper()
		_, err := c.Exec(ctx, sql, args...)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != code {
			t.Fatalf("SQLSTATE want %s got %v", code, err)
		}
	}
	for _, role := range []string{"slovnyk_http", "slovnyk_operator"} {
		expectCode(connect(role), "42501", "UPDATE slovnyk.sitemap_inventory SET url_count=url_count WHERE dataset_id=$1", staged.Dataset)
		expectCode(connect(role), "42501", "DELETE FROM slovnyk.sitemap_shards WHERE dataset_id=$1", staged.Dataset)
	}
	expectCode(connect("slovnyk_http"), "42501", `SELECT slovnyk.register_sitemap($1,sha256('report'::bytea),100,sha256('index'::bytea),100,1,'[]'::jsonb)`, staged.Dataset)
	expectCode(connect("slovnyk_http"), "42501", `INSERT INTO slovnyk.sitemap_shards VALUES($1,sha256('late'::bytea),100,1)`, staged.Dataset)
	expectCode(connect("slovnyk_operator"), "42501", `INSERT INTO slovnyk.sitemap_shards VALUES($1,sha256('late'::bytea),100,1)`, staged.Dataset)
	// An interrupted registration must not leave an inventory or public index.
	m.Source.Authority = "incomplete sitemap inventory"
	second, err := importer.Run(ctx, connect("slovnyk_operator"), m, input, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publication.Verify(ctx, connect("slovnyk_operator"), second.Dataset, input, root, publication.Options{}); err != nil {
		t.Fatal(err)
	}
	expectCode(connect("slovnyk_operator"), "23514", `SELECT slovnyk.register_sitemap($1,sha256('report'::bytea),100,sha256('index'::bytea),100,2,
 '[{"urls":1,"artifact":{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":100}}]'::jsonb)`, second.Dataset)
	var partial int
	if err := owner.QueryRow(ctx, "SELECT count(*) FROM slovnyk.sitemap_inventory WHERE dataset_id=$1", second.Dataset).Scan(&partial); err != nil || partial != 0 {
		t.Fatal("partial inventory committed", err)
	}

}
