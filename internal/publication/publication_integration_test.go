//go:build integration

package publication

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func conn(t *testing.T, role string) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("use make test-integration")
	}
	c, e := postgres.Connect(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { closeConn(c) })
	if role != "" {
		if _, e = c.Exec(context.Background(), "SET ROLE "+role); e != nil {
			t.Fatal(e)
		}
	}
	return c
}
func root(t *testing.T) string {
	t.Helper()
	if e := os.MkdirAll("../../tmp/m1-05", 0700); e != nil {
		t.Fatal(e)
	}
	p, e := os.MkdirTemp("../../tmp/m1-05", "publication-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}
func staged(t *testing.T, name string, change func(*importdata.Manifest)) string {
	t.Helper()
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "publication-" + name
	if change != nil {
		change(&m)
	}
	r, e := importer.Run(context.Background(), conn(t, "slovnyk_operator"), m, "../../testdata/import", importer.Options{BatchSize: 2})
	if e != nil {
		t.Fatal(e)
	}
	return r.Dataset
}
func ready(t *testing.T, name, dir string) Report {
	t.Helper()
	id := staged(t, name, nil)
	r, e := Verify(context.Background(), conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{})
	if e != nil {
		t.Fatalf("verify %s: %v", name, e)
	}
	return r
}
func generation(t *testing.T) int64 {
	t.Helper()
	var n int64
	if e := conn(t, "").QueryRow(context.Background(), "SELECT generation FROM slovnyk.active_dataset").Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestVerifyExportPublishRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := root(t)
	a := ready(t, "A", dir)
	if a.Parsed != 11 || a.Unique != 10 || a.Duplicates != 1 || len(a.Pairs) != 4 {
		t.Fatalf("counts: %+v", a)
	}
	repeated, e := Verify(ctx, conn(t, "slovnyk_operator"), a.Dataset, "missing-source-not-needed-for-ready", dir, Options{})
	if e != nil || !reflect.DeepEqual(a, repeated) {
		t.Fatalf("repeat verify: %+v %v", repeated, e)
	}
	exported, e := Export(ctx, conn(t, "slovnyk_operator"), a.Dataset, dir)
	if e != nil || !reflect.DeepEqual(a, exported) {
		t.Fatalf("repeat export: %+v %v", exported, e)
	}
	b := ready(t, "B", dir)
	n := generation(t)
	receipt, e := Publish(ctx, conn(t, "slovnyk_operator"), a.Dataset, dir, n, "synthetic operator", false)
	if e != nil || receipt.Generation != n+1 {
		t.Fatalf("publish: %+v %v", receipt, e)
	}
	checked, e := CheckActive(ctx, conn(t, "slovnyk_http"), dir)
	if e != nil || checked != receipt {
		t.Fatalf("HTTP role: %+v %v", checked, e)
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), b.Dataset, dir, n, "stale", false); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale: %v", e)
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), b.Dataset, dir, n+1, "next", false); e != nil {
		t.Fatal(e)
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), a.Dataset, dir, n+2, "rollback", true); e != nil {
		t.Fatal(e)
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), b.Dataset, dir, n+1, "ABA", true); !errors.Is(e, ErrConflict) {
		t.Fatalf("ABA: %v", e)
	}
	// Rollback requires a previously published retained version.
	c := ready(t, "never-published", dir)
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), c.Dataset, dir, n+3, "not-retained", true); !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	// Two writers with the same generation: exactly one commits.
	writers := []*pgx.Conn{conn(t, "slovnyk_operator"), conn(t, "slovnyk_operator")}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, c := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := Publish(ctx, c, b.Dataset, dir, n+3, "concurrent", false)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS: %d %d", success, conflict)
	}
	// Startup and publication refuse missing files without changing active state.
	file := filepath.Join(dir, b.Pairs[0].Artifact.Path)
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if _, e = CheckActive(ctx, conn(t, "slovnyk_http"), dir); e == nil {
		t.Fatal("missing active artifact admitted")
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), b.Dataset, dir, n+4, "missing", false); e == nil {
		t.Fatal("missing artifact published")
	}
	if generation(t) != n+4 {
		t.Fatal("failed publication changed pointer")
	}
}
func TestVerificationFaultIsolation(t *testing.T) {
	ctx := context.Background()
	dir := root(t)
	id := staged(t, "crash-before-ready", nil)
	before := generation(t)
	_, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{beforeReady: func() error { return context.Canceled }})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	// A real process exit after durable files but before the ready commit must
	// also leave the transaction rolled back, including inserted export rows.
	crash := exec.Command(os.Args[0], "-test.run=^TestVerificationCrashProcess$")
	crash.Env = append(os.Environ(), "SLOVNYK_TEST_VERIFY_CRASH_ID="+id, "SLOVNYK_TEST_VERIFY_CRASH_ROOT="+dir)
	err := crash.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatalf("verification child: %v", err)
	}
	var state string
	var exports int
	admin := conn(t, "")
	if e = admin.QueryRow(ctx, "SELECT state FROM slovnyk.datasets WHERE id=$1", id).Scan(&state); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, "SELECT count(*) FROM slovnyk.exports WHERE dataset_id=$1", id).Scan(&exports); e != nil {
		t.Fatal(e)
	}
	if state != "validating" || exports != 0 || generation(t) != before {
		t.Fatalf("partial commit: %s %d", state, exports)
	}
	if _, e = Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{}); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, query string
		change      func(*importdata.Manifest)
	}{
		{name: "pair-counter", query: "UPDATE slovnyk.dataset_pairs SET unique_count=0 WHERE dataset_id=$1"},
		{name: "origin-locator", query: "UPDATE slovnyk.translation_origins SET source_locator='wrong' WHERE dataset_id=$1"},
		{name: "metadata", query: "UPDATE slovnyk.translation_origins SET legacy_count=0 WHERE dataset_id=$1"},
		{name: "provenance", query: "UPDATE slovnyk.translation_origins SET provenance='{}' WHERE dataset_id=$1"},
		{name: "gap", change: func(m *importdata.Manifest) { m.KnownGaps = []string{"not owner approval"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := staged(t, tc.name, tc.change)
			if tc.query != "" {
				if _, e = admin.Exec(ctx, tc.query, id); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{}); e == nil {
				t.Fatal("invalid dataset verified")
			}
			if _, e = Publish(ctx, conn(t, "slovnyk_operator"), id, dir, before, "invalid", false); e == nil {
				t.Fatal("invalid dataset published")
			}
		})
	}
	if generation(t) != before {
		t.Fatal("negative fixture changed active pointer")
	}
	// Isolated failures do not compromise another clean candidate.
	clean := ready(t, "clean-after-faults", dir)
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), clean.Dataset, dir, before, "clean", false); e != nil {
		t.Fatal(e)
	}
}

func TestReportAndSourceFailures(t *testing.T) {
	ctx := context.Background()
	dir := root(t)
	n := generation(t)
	id := staged(t, "missing-source", nil)
	if _, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../tmp/m1-05/absent-source", dir, Options{}); e == nil {
		t.Fatal("missing source accepted")
	}
	if _, e := conn(t, "").Exec(ctx, "UPDATE slovnyk.datasets SET state='failed' WHERE id=$1", id); e != nil {
		t.Fatal(e)
	}
	if _, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{}); !errors.Is(e, ErrState) {
		t.Fatal("failed dataset accepted", e)
	}
	r := ready(t, "report-corruption", dir)
	var p string
	if e := conn(t, "").QueryRow(ctx, "SELECT report_path FROM slovnyk.datasets WHERE id=$1", r.Dataset).Scan(&p); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, p)
	if e := os.Chmod(file, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte("{}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Export(ctx, conn(t, "slovnyk_operator"), r.Dataset, dir); e == nil {
		t.Fatal("corrupt report exported")
	}
	if _, e := Publish(ctx, conn(t, "slovnyk_operator"), r.Dataset, dir, n, "corrupt report", false); e == nil {
		t.Fatal("corrupt report published")
	}
	if generation(t) != n {
		t.Fatal("failure changed active pointer")
	}
}

func TestRejectedRecordsCannotBecomeReady(t *testing.T) {
	ctx := context.Background()
	source := root(t)
	dest := root(t)
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "rejected-publication"
	var content bytes.Buffer
	gz := gzip.NewWriter(&content)
	if _, e = gz.Write([]byte("\"en-us\",\"uk-ua\",\"ok\",\"так\"\n\"en-us\",\"uk-ua\",\"\",\"\"\n")); e != nil {
		t.Fatal(e)
	}
	if e = gz.Close(); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(content.Bytes())
	m.Artifacts[0].SHA256 = hex.EncodeToString(hash[:])
	m.Artifacts[0].Bytes = int64(content.Len())
	if e = os.WriteFile(filepath.Join(source, m.Artifacts[0].Name), content.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	staged, e := importer.Run(ctx, conn(t, "slovnyk_operator"), m, source, importer.Options{})
	if e != nil {
		t.Fatal(e)
	}
	if staged.Files[0].Rejected != 1 {
		t.Fatal("fixture has no rejected record")
	}
	if _, e = Verify(ctx, conn(t, "slovnyk_operator"), staged.Dataset, source, dest, Options{}); !errors.Is(e, ErrVerification) {
		t.Fatalf("rejected ready: %v", e)
	}
}
func TestEmptyPairRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := root(t)
	id := staged(t, "empty-pair", func(m *importdata.Manifest) {
		m.ExpectedPairs = append(m.ExpectedPairs, importdata.Pair{From: "uk-ua", To: "uk-ua"})
	})
	r, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{})
	if e != nil {
		t.Fatal(e)
	}
	empty := 0
	for _, p := range r.Pairs {
		if p.Rows == 0 {
			empty++
		}
	}
	if empty != 1 {
		t.Fatal("empty pair not retained")
	}
	if _, e = Export(ctx, conn(t, "slovnyk_operator"), id, dir); e != nil {
		t.Fatal(e)
	}
}

func TestVerificationCrashProcess(t *testing.T) {
	id := os.Getenv("SLOVNYK_TEST_VERIFY_CRASH_ID")
	if id == "" {
		return
	}
	_, _ = Verify(context.Background(), conn(t, "slovnyk_operator"), id, "../../testdata/import", os.Getenv("SLOVNYK_TEST_VERIFY_CRASH_ROOT"), Options{beforeReady: func() error { os.Exit(42); return nil }})
	os.Exit(92)
}
