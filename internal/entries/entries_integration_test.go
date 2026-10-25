//go:build integration

package entries_test

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func TestPublishedDictionaryPages(t *testing.T) {
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
			if _, err = c.Exec(ctx, "SET ROLE "+role); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	owner, reader := connect(""), connect("slovnyk_http")
	if err := os.MkdirAll("../../tmp/seo", 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("../../tmp/seo", "integration-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	input, artifactRoot := filepath.Join(root, "input"), filepath.Join(root, "artifacts")
	if err = os.MkdirAll(input, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../testdata/import/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := importdata.ParseManifest(fixture)
	if err != nil {
		t.Fatal(err)
	}
	m.Locales = []string{"en-us", "uk-ua"}
	m.ExpectedPairs = []importdata.Pair{{From: "en-us", To: "uk-ua"}}
	var records [][]string
	for i := range 55 {
		records = append(records, []string{"en-us", "uk-ua", "word", fmt.Sprintf("translation %03d", i)})
	}
	variants := []string{"WORD", " word ", "*?%_\\", "É", "É", "a,\"b\r\n", strings.Repeat("x", 65536)}
	for _, source := range variants {
		records = append(records, []string{"en-us", "uk-ua", source, "raw translation"})
	}
	records = append(records, []string{"en-us", "uk-ua", "long-target", strings.Repeat("&", 65536)})
	stage := func(name string, data [][]string) publication.Report {
		t.Helper()
		path := filepath.Join(input, "corpus.csv.gz")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		z := gzip.NewWriter(f)
		w := csv.NewWriter(z)
		if err = w.WriteAll(data); err != nil {
			t.Fatal(err)
		}
		if err = z.Close(); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(b)
		m.Source.Authority = "entries-" + name
		m.Artifacts = []importdata.Artifact{{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(b)), Name: "corpus.csv.gz", Format: "csv4.gz", MultiPair: true}}
		staged, err := importer.Run(ctx, connect("slovnyk_operator"), m, input, importer.Options{})
		if err != nil {
			t.Fatal(err)
		}
		report, err := publication.Verify(ctx, connect("slovnyk_operator"), staged.Dataset, input, artifactRoot, publication.Options{})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	var generation int64
	if err = owner.QueryRow(ctx, `SELECT generation FROM slovnyk.active_dataset`).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	a := stage("A", records)
	if _, err = publication.Publish(ctx, connect("slovnyk_operator"), a.Dataset, artifactRoot, generation, "entries test", false); err != nil {
		t.Fatal(err)
	}
	signer, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	read := func(id, cursor string) (entries.Page, error) { return entries.Read(ctx, reader, signer, id, cursor) }
	id := entries.ID("en-us", "uk-ua", "word")
	// Force an index plan only to verify the expression is index-compatible;
	// this is not a production planner or capacity benchmark.
	if _, err = reader.Exec(ctx, `SET enable_seqscan=off`); err != nil {
		t.Fatal(err)
	}
	var plan string
	hash, _ := hex.DecodeString(id)
	err = reader.QueryRow(ctx, `EXPLAIN (FORMAT JSON) SELECT id FROM slovnyk.translations WHERE dataset_id=$1 AND slovnyk.entry_id(source_locale,target_locale,source_text)=$2 AND id>0 ORDER BY id LIMIT 51`, a.Dataset, hash).Scan(&plan)
	if err != nil || !strings.Contains(plan, "translations_entry") {
		t.Fatal("dictionary query cannot use its index", err, plan)
	}
	if _, err = reader.Exec(ctx, `RESET enable_seqscan`); err != nil {
		t.Fatal(err)
	}
	first, err := read(id, "")
	if err != nil || first.Dataset != a.Dataset || len(first.Translations) != 50 || first.Cursor == "" {
		t.Fatal("first page", first.Dataset, len(first.Translations), err)
	}
	second, err := read(id, first.Cursor)
	if err != nil || len(second.Translations) != 5 || second.Cursor != "" {
		t.Fatal("second page", len(second.Translations), err)
	}
	got := append(append([]string{}, first.Translations...), second.Translations...)
	for i, value := range got {
		if value != fmt.Sprintf("translation %03d", i) {
			t.Fatal("missing/duplicate/wrong raw spelling", i, value)
		}
	}
	for _, source := range append(variants, "long-target") {
		var sqlID string
		if err = reader.QueryRow(ctx, `SELECT encode(slovnyk.entry_id($1,$2,$3),'hex')`, "en-us", "uk-ua", source).Scan(&sqlID); err != nil || sqlID != entries.ID("en-us", "uk-ua", source) {
			t.Fatal("SQL/Go identity mismatch", err)
		}
		p, err := read(sqlID, "")
		if err != nil || p.Source != source || len(p.Translations) != 1 {
			t.Fatal("raw variant lost", len(source), err)
		}
	}
	if _, err = read(entries.ID("en-us", "uk-ua", "WORD"), first.Cursor); !errors.Is(err, lookup.ErrCursor) {
		t.Fatal("cursor moved to another spelling", err)
	}
	if _, err = read(entries.ID("uk-ua", "en-us", "word"), ""); !errors.Is(err, entries.ErrNotFound) {
		t.Fatal("pair changed by lookup", err)
	}
	handler := httpui.New(nil, signer, httpui.WithEntries(func(ctx context.Context, id, cursor string) (entries.Page, error) {
		return entries.Read(ctx, reader, signer, id, cursor)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", entries.Path(id), nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `rel="canonical"`) || response.Header().Get("X-Robots-Tag") != "" {
		t.Fatal("real anonymous HTTP page", response.Code)
	}
	// B changes row IDs/content; bare identity follows active, continuation pins A.
	bRecords := [][]string{{"en-us", "uk-ua", "word", "new translation"}}
	for _, r := range records {
		if r[2] != "WORD" {
			bRecords = append(bRecords, r)
		}
	}
	b := stage("B", bRecords)
	forged, _ := signer.Decode(first.Cursor)
	forged.Dataset = b.Dataset
	unpublished, _ := signer.Encode(forged)
	if _, err = read(id, unpublished); !errors.Is(err, lookup.ErrGone) {
		t.Fatal("unpublished page exposed", err)
	}
	if _, err = publication.Publish(ctx, connect("slovnyk_operator"), b.Dataset, artifactRoot, generation+1, "entries test B", false); err != nil {
		t.Fatal(err)
	}
	active, err := read(id, "")
	if err != nil || active.Dataset != b.Dataset || active.Translations[0] != "new translation" {
		t.Fatal("stable URL did not follow active", err)
	}
	if _, err = read(entries.ID("en-us", "uk-ua", "WORD"), ""); !errors.Is(err, entries.ErrNotFound) {
		t.Fatal("removed spelling silently replaced by normalized variant", err)
	}
	retained, err := read(id, first.Cursor)
	if err != nil || retained.Dataset != a.Dataset || !reflect.DeepEqual(retained.Translations, second.Translations) {
		t.Fatal("continuation lost retained pin", err)
	}
	lock, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err = lock.Exec(ctx, `LOCK TABLE slovnyk.active_dataset IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	if _, err = read(id, first.Cursor); err != nil {
		t.Fatal("retained page consulted active pointer", err)
	}
	blocked, stop := context.WithTimeout(ctx, 80*time.Millisecond)
	_, err = entries.Read(blocked, reader, signer, id, "")
	stop()
	if !errors.Is(err, lookup.ErrUnavailable) {
		t.Fatal("active pin bypassed deadline", err)
	}
}
