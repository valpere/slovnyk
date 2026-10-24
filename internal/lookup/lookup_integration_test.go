//go:build integration

package lookup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/normalization"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func db(t *testing.T, role string) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("use make test-integration")
	}
	c, e := postgres.Connect(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	if role != "" {
		if _, e = c.Exec(context.Background(), "SET ROLE "+role); e != nil {
			t.Fatal(e)
		}
	}
	return c
}
func scratch(t *testing.T) string {
	t.Helper()
	base := "../../tmp/m1-06"
	if e := os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp(base, "lookup-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}
func candidate(t *testing.T, root, name string, tuples []importdata.Tuple) string {
	t.Helper()
	ctx := context.Background()
	source := scratch(t)
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "lookup-" + name
	m.ExpectedPairs = nil
	pairs := map[importdata.Pair]bool{}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	for _, tuple := range tuples {
		p := importdata.Pair{From: tuple[0], To: tuple[1]}
		if !pairs[p] {
			m.ExpectedPairs = append(m.ExpectedPairs, p)
			pairs[p] = true
		}
		if e = importdata.WriteCSV4(gz, tuple); e != nil {
			t.Fatal(e)
		}
	}
	if e = gz.Close(); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(compressed.Bytes())
	m.Artifacts[0].SHA256 = hex.EncodeToString(hash[:])
	m.Artifacts[0].Bytes = int64(compressed.Len())
	if e = os.WriteFile(filepath.Join(source, m.Artifacts[0].Name), compressed.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	staged, e := importer.Run(ctx, db(t, "slovnyk_operator"), m, source, importer.Options{BatchSize: 3})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = publication.Verify(ctx, db(t, "slovnyk_operator"), staged.Dataset, source, root, publication.Options{}); e != nil {
		t.Fatal(e)
	}
	return staged.Dataset
}
func publish(t *testing.T, id, root string) {
	t.Helper()
	var n int64
	c := db(t, "slovnyk_operator")
	if e := c.QueryRow(context.Background(), "SELECT generation FROM slovnyk.active_dataset").Scan(&n); e != nil {
		t.Fatal(e)
	}
	if _, e := publication.Publish(context.Background(), c, id, root, n, "lookup fixture", false); e != nil {
		t.Fatal(e)
	}
}
func fixtures() []importdata.Tuple {
	raw := [][2]string{{"Straße", "A"}, {"STRASSE", "B"}, {"strasse", "C"}, {"Straße", "D"}, {"café", "coffee"}, {"cafe\u0301", "other"}, {"ß", "ss"}, {"é", "acute"}, {"e\u0301", "decomposed"}, {"👩‍💻", "dev"}, {"literal*?", "stars"}, {"a%b", "percent"}, {"axb", "not percent"}, {"a_b", "underscore"}, {"a\\b", "backslash"}, {" edge ", "spaced"}, {"İ", "dot"}, {"tabs\tand\nlines", "multiline"}, {"<script>'&", "<target>\"'"}}
	out := []importdata.Tuple{}
	for _, t := range raw {
		out = append(out, importdata.Tuple{"en-us", "uk-ua", t[0], t[1]})
	}
	return append(out, importdata.Tuple{"sr-rs", "sr-rs", "исто", "same"})
}
func search(t *testing.T, s *Service, c *pgx.Conn, r Request) Page {
	t.Helper()
	p, e := s.Search(context.Background(), c, r)
	if e != nil {
		t.Fatalf("search %+v: %v", r, e)
	}
	return p
}
func collect(t *testing.T, s *Service, c *pgx.Conn, r Request) []postgres.Translation {
	t.Helper()
	out := []postgres.Translation{}
	seen := map[int64]bool{}
	for n := 0; n < 100; n++ {
		p := search(t, s, c, r)
		for _, entry := range p.Entries {
			if seen[entry.ID] {
				t.Fatal("duplicate page ID", entry.ID)
			}
			seen[entry.ID] = true
			out = append(out, entry)
		}
		if !p.HasMore {
			if p.Cursor != "" {
				t.Fatal("terminal cursor")
			}
			return out
		}
		if p.Cursor == "" || len(p.Cursor) > 2048 {
			t.Fatal("invalid continuation")
		}
		r.Cursor = p.Cursor
	}
	t.Fatal("pagination did not terminate")
	return nil
}
func TestSearchModesAndStablePages(t *testing.T) {
	root := scratch(t)
	id := candidate(t, root, "search", fixtures())
	publish(t, id, root)
	service, _ := New(signer(t))
	c := db(t, "slovnyk_http")
	base := Request{From: "en-us", To: "uk-ua", Q: " STRAẞE ", Limit: 2}
	first := search(t, service, c, base)
	if len(first.Entries) != 2 || !first.HasMore || first.Dataset != id {
		t.Fatalf("first: %+v", first)
	}
	base.Cursor = first.Cursor
	base.Q = "strasse"
	base.Mode = normalization.Exact
	second := search(t, service, c, base)
	if len(second.Entries) != 2 || second.HasMore || first.Groups[len(first.Groups)-1].Source != "Straße" || second.Groups[0].Source != "Straße" {
		t.Fatalf("split group: %+v %+v", first, second)
	}
	all := collect(t, service, c, Request{From: "en-us", To: "uk-ua", Q: "*", Limit: 3})
	want := fixtures()[:len(fixtures())-1]
	sort.Slice(want, func(i, j int) bool {
		a, b := want[i], want[j]
		ka, kb := normalization.Key(a[2]), normalization.Key(b[2])
		if ka != kb {
			return ka < kb
		}
		if a[2] != b[2] {
			return a[2] < b[2]
		}
		return a[3] < b[3]
	})
	if len(all) != len(want) {
		t.Fatalf("entries %d want %d", len(all), len(want))
	}
	for i, v := range all {
		if v.SourceText != want[i][2] || v.TargetText != want[i][3] {
			t.Fatalf("C order %d: %+v want %v", i, v, want[i])
		}
	}
	for _, tc := range []struct {
		q    string
		mode normalization.Mode
		want []string
	}{
		{"a%b", normalization.Mask, []string{"a%b"}}, {"a_b", normalization.Mask, []string{"a_b"}}, {"a\\b", normalization.Mask, []string{"a\\b"}},
		{"literal*?", normalization.Exact, []string{"literal*?"}}, {"cafe\u0301", normalization.Auto, []string{"cafe\u0301", "café"}},
		{"?", normalization.Auto, []string{"e\u0301", "é"}}, {"??", normalization.Auto, []string{"İ", "ß"}}, {"???", normalization.Auto, []string{"a%b", "a\\b", "a_b", "axb", "👩‍💻"}},
		{" edge ", normalization.Auto, []string{}}, {"*edge*", normalization.Auto, []string{" edge "}},
		{"<script>'&", normalization.Exact, []string{"<script>'&"}},
	} {
		t.Run(tc.q+string(tc.mode), func(t *testing.T) {
			p := search(t, service, c, Request{From: "en-us", To: "uk-ua", Q: tc.q, Mode: tc.mode})
			got := []string{}
			for _, v := range p.Entries {
				got = append(got, v.SourceText)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%q: %q want %q", tc.q, got, tc.want)
			}
		})
	}
	same := search(t, service, c, Request{From: "sr-rs", To: "sr-rs", Q: "исто"})
	if len(same.Entries) != 1 {
		t.Fatal("same locale missing")
	}
	for _, tc := range []struct {
		from, to string
		status   int
	}{{"EN-US", "uk-ua", 400}, {"en-us", "sr-rs", 404}, {"sr-sp", "sr-rs", 404}} {
		_, e := service.Search(context.Background(), c, Request{From: tc.from, To: tc.to, Q: "q"})
		if Status(e) != tc.status {
			t.Fatalf("%+v: %v", tc, e)
		}
	}
}

func TestCursorPinAuthorizationAndChanges(t *testing.T) {
	ctx := context.Background()
	root := scratch(t)
	id := candidate(t, root, "pin-old", fixtures())
	publish(t, id, root)
	s, _ := New(signer(t))
	c := db(t, "slovnyk_http")
	r := Request{From: "en-us", To: "uk-ua", Q: "strasse", Limit: 1}
	first := search(t, s, c, r)
	r.Cursor = first.Cursor
	newID := candidate(t, root, "pin-new", []importdata.Tuple{{"sr-rs", "sr-rs", "new", "new"}})
	publish(t, newID, root)
	continued := search(t, s, c, r)
	if continued.Dataset != id || len(continued.Entries) != 1 {
		t.Fatal("cursor switched datasets")
	}
	for _, change := range []func(*Request){func(r *Request) { r.Q = "café" }, func(r *Request) { r.Limit = 2 }, func(r *Request) { r.To = "unknown" }, func(r *Request) { r.Mode = normalization.Mask }} {
		altered := r
		change(&altered)
		if _, e := s.Search(ctx, c, altered); Status(e) != 400 {
			t.Fatalf("changed params: %v", e)
		}
	}
	decoded, e := s.signer.Decode(first.Cursor)
	if e != nil {
		t.Fatal(e)
	}
	for _, dataset := range []string{"00000000-0000-0000-0000-000000000099", candidate(t, root, "not-published", fixtures())} {
		token := decoded
		token.Dataset = dataset
		encoded, e := s.signer.Encode(token)
		if e != nil {
			t.Fatal(e)
		}
		altered := r
		altered.Cursor = encoded
		if _, e = s.Search(ctx, c, altered); Status(e) != 410 {
			t.Fatalf("unpublished/missing cursor: %v", e)
		}
	}
	// A correctly signed token still needs a real anchor matching pair and filter.
	token := decoded
	token.Anchor = 999999
	r.Cursor, _ = s.signer.Encode(token)
	if _, e = s.Search(ctx, c, r); Status(e) != 400 {
		t.Fatal("missing anchor admitted", e)
	}
	for _, source := range []string{"исто", "café"} {
		var anchor int64
		if e = c.QueryRow(ctx, "SELECT id FROM slovnyk.translations WHERE dataset_id=$1 AND source_text=$2", id, source).Scan(&anchor); e != nil {
			t.Fatal(e)
		}
		token = decoded
		token.Anchor = anchor
		r.Cursor, _ = s.signer.Encode(token)
		if _, e = s.Search(ctx, c, r); Status(e) != 400 {
			t.Fatalf("anchor outside pair/filter admitted: %v", e)
		}
	}
	// Simulate a future publisher with a normalizer this binary does not contain.
	future := futurePublished(t, db(t, "slovnyk_operator"))
	r.Cursor = first.Cursor
	if p, e := s.Search(ctx, c, r); e != nil || p.Dataset != id {
		t.Fatalf("old normalizer lost after active change: %v", e)
	}
	if _, e = s.Search(ctx, c, Request{From: "en-us", To: "uk-ua", Q: "strasse"}); Status(e) != 503 {
		t.Fatal("unknown active normalizer admitted", e)
	}
	token = decoded
	token.Dataset = future
	r.Cursor, _ = s.signer.Encode(token)
	if _, e = s.Search(ctx, c, r); Status(e) != 410 {
		t.Fatal("unknown pinned normalizer admitted", e)
	}
}

// This isolated SQL fixture models a future application's published version.
// It uses the real state/history guards; it does not weaken verification in code.
func futurePublished(t *testing.T, c *pgx.Conn) string {
	t.Helper()
	ctx := context.Background()
	b, e := os.ReadFile("../../testdata/import/manifest.canonical.json")
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	m["normalization_version"] = "future-normalizer"
	b, e = json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	fp := sha256.Sum256(b)
	id := "00000000-0000-0000-0000-000000001006"
	tx, e := c.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO slovnyk.datasets(id,fingerprint,manifest_bytes,schema_version,parser_version,mapping_version,normalization_version) VALUES($1,$2,$3,1,$4,$5,'future-normalizer')`, id, fp[:], b, importdata.ParserVersion, importdata.MappingVersion)
	if e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"loading", "validating", "ready"} {
		if _, e = tx.Exec(ctx, "UPDATE slovnyk.datasets SET state=$2,report_path='future-fixture-report',report_hash=sha256('future'::bytea) WHERE id=$1", id, state); e != nil {
			t.Fatal(e)
		}
	}
	var old *string
	var generation int64
	if e = tx.QueryRow(ctx, "SELECT dataset_id::text,generation FROM slovnyk.active_dataset FOR UPDATE").Scan(&old, &generation); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "INSERT INTO slovnyk.publication_events(generation,previous_dataset_id,new_dataset_id,operator_ref) VALUES($1,$2,$3,'future fixture')", generation+1, old, id); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "UPDATE slovnyk.active_dataset SET dataset_id=$1,generation=$2", id, generation+1); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return id
}
func TestByteBudgetAndLongAnchor(t *testing.T) {
	root := scratch(t)
	tuples := []importdata.Tuple{}
	for i := 0; i < 3; i++ {
		tuples = append(tuples, importdata.Tuple{"en-us", "uk-ua", strings.Repeat(`"`, 65535) + fmt.Sprint(i), strings.Repeat(`'`, 65536)})
	}
	tuples = append(tuples, importdata.Tuple{"en-us", "uk-ua", strings.Repeat("İ", 32768), "expanded-key"})
	id := candidate(t, root, "long", tuples)
	publish(t, id, root)
	s, _ := New(signer(t))
	c := db(t, "slovnyk_http")
	r := Request{From: "en-us", To: "uk-ua", Q: "*", Limit: 100}
	first := search(t, s, c, r)
	if len(first.Entries) != 1 || !first.HasMore || first.EscapedBytes > EntryBudget || len(first.Cursor) > 2048 {
		t.Fatalf("budget: count=%d bytes=%d cursor=%d", len(first.Entries), first.EscapedBytes, len(first.Cursor))
	}
	all := collect(t, s, c, r)
	if len(all) != 4 {
		t.Fatalf("long pagination lost rows: %d", len(all))
	}
	if len(all[3].SourceKey) <= 65536 {
		t.Fatal("fixture did not expand normalized key")
	}
}

type pinDelay struct{}

func (pinDelay) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "active_dataset") {
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
		}
	}
	return ctx
}
func (pinDelay) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestWholeLookupDeadlineAndRecovery(t *testing.T) {
	root := scratch(t)
	id := candidate(t, root, "timeout", fixtures())
	publish(t, id, root)
	blocker := db(t, "slovnyk_operator")
	tx, e := blocker.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(context.Background(), "LOCK TABLE slovnyk.translations IN ACCESS EXCLUSIVE MODE"); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgx.ParseConfig(os.Getenv("SLOVNYK_TEST_DSN"))
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	cfg.RuntimeParams["role"] = "slovnyk_http"
	cfg.Tracer = pinDelay{}
	c, e := pgx.ConnectConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal("test connection failed")
	}
	defer c.Close(context.Background())
	s, _ := New(signer(t))
	start := time.Now()
	_, e = s.Search(context.Background(), c, Request{From: "en-us", To: "uk-ua", Q: "strasse"})
	elapsed := time.Since(start)
	if Status(e) != 503 || elapsed < 1800*time.Millisecond || elapsed > 2800*time.Millisecond {
		t.Fatalf("whole deadline: %v %s", e, elapsed)
	}
	if e = tx.Rollback(context.Background()); e != nil {
		t.Fatal(e)
	}
	if p := search(t, s, db(t, "slovnyk_http"), Request{From: "en-us", To: "uk-ua", Q: "strasse"}); len(p.Entries) != 4 {
		t.Fatal("query after timeout failed")
	}
}
