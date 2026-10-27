//go:build integration

package publication

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
)

func assertStillValidating(t *testing.T, id string, before int64) {
	t.Helper()
	var unchanged bool
	e := conn(t, "").QueryRow(context.Background(), `SELECT state='validating' AND report_path IS NULL AND report_hash IS NULL
 AND NOT EXISTS(SELECT 1 FROM slovnyk.exports WHERE dataset_id=$1)
 FROM slovnyk.datasets WHERE id=$1`, id).Scan(&unchanged)
	if e != nil || !unchanged || generation(t) != before {
		t.Fatalf("reconciliation changed publication state: %t %v", unchanged, e)
	}
}

func TestReconcileWithoutPublication(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"clean", "known-gap", "exception"} {
		t.Run(name, func(t *testing.T) {
			id := staged(t, "reconcile-"+name, func(m *importdata.Manifest) {
				if name == "known-gap" {
					m.KnownGaps = []string{"CSV lacks historical metadata"}
				}
				if name == "exception" {
					m.Exceptions = []importdata.Exception{{ArtifactSHA256: strings.Repeat("f", 64), Decision: "exclude_artifact", Reason: "synthetic unavailable artifact"}}
				}
			})
			before := generation(t)
			r, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import")
			if e != nil {
				t.Fatal(e)
			}
			if r.Version != 1 || r.Kind != "reconciliation" || r.Dataset != id || r.State != "validating" || r.Parsed != 11 || r.Unique != 10 || r.Duplicates != 1 || r.Rejected != 0 || len(r.Files) != 1 || !r.Files[0].Complete || !r.Files[0].ChecksumVerified {
				t.Fatalf("unexpected accounting: %+v", r)
			}
			var fp string
			if e = conn(t, "").QueryRow(ctx, "SELECT encode(fingerprint,'hex') FROM slovnyk.datasets WHERE id=$1", id).Scan(&fp); e != nil || r.Fingerprint != fp {
				t.Fatalf("fingerprint mismatch: %v", e)
			}
			if (name == "known-gap") != (len(r.KnownGaps) == 1) || (name == "exception") != (len(r.Exceptions) == 1) {
				t.Fatal("manifest limitations missing from report")
			}
			repeated, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import")
			if e != nil || !reflect.DeepEqual(r, repeated) {
				t.Fatalf("repeat reconciliation: %v", e)
			}
			assertStillValidating(t, id, before)
			dir := root(t)
			if _, e = Publish(ctx, conn(t, "slovnyk_operator"), id, dir, before, "not approval", false); !errors.Is(e, ErrState) {
				t.Fatalf("reconciliation permitted publication: %v", e)
			}
			if name != "clean" {
				if _, e = Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{}); !errors.Is(e, ErrVerification) {
					t.Fatalf("reconciliation bypassed approval: %v", e)
				}
				assertStillValidating(t, id, before)
			}
		})
	}
}

func TestReconcileDetectsCorruption(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, query string }{
		{"origin", "UPDATE slovnyk.translation_origins SET source_locator='corrupt' WHERE dataset_id=$1"},
		{"metadata", "UPDATE slovnyk.translation_origins SET legacy_count=0 WHERE dataset_id=$1"},
		{"counter", "UPDATE slovnyk.dataset_pairs SET unique_count=0 WHERE dataset_id=$1"},
		{"incomplete", "UPDATE slovnyk.source_files SET complete=false,checksum_verified=false WHERE dataset_id=$1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := staged(t, "reconcile-corrupt-"+tc.name, nil)
			before := generation(t)
			if _, e := conn(t, "").Exec(ctx, tc.query, id); e != nil {
				t.Fatal(e)
			}
			r, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import")
			if e == nil || r.Dataset != "" {
				t.Fatalf("corruption accepted: %+v %v", r, e)
			}
			assertStillValidating(t, id, before)
		})
	}
}

func TestReconcileRejectsOtherStates(t *testing.T) {
	ctx := context.Background()
	failed := staged(t, "reconcile-failed", nil)
	if _, e := conn(t, "").Exec(ctx, "UPDATE slovnyk.datasets SET state='failed' WHERE id=$1", failed); e != nil {
		t.Fatal(e)
	}
	r := ready(t, "reconcile-ready", root(t))
	before := generation(t)
	for _, id := range []string{failed, r.Dataset} {
		if _, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import"); !errors.Is(e, ErrState) {
			t.Fatalf("non-validating dataset admitted: %v", e)
		}
	}
	if generation(t) != before {
		t.Fatal("state failure changed active pointer")
	}
}

func TestReconcileSourceAndLockFailures(t *testing.T) {
	ctx := context.Background()
	id := staged(t, "reconcile-failures", nil)
	before := generation(t)
	for _, source := range []string{"", root(t)} {
		if _, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, source); e == nil {
			t.Fatal("absent source accepted")
		}
	}
	// Same-size changed compressed bytes must not pass checksum verification.
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile(filepath.Join("../../testdata/import", m.Artifacts[0].Name))
	if e != nil {
		t.Fatal(e)
	}
	b[4] ^= 1 // gzip mtime; decompressed bytes remain unchanged
	dir := root(t)
	if e = os.WriteFile(filepath.Join(dir, m.Artifacts[0].Name), b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Reconcile(ctx, conn(t, "slovnyk_operator"), id, dir); !errors.Is(e, importer.ErrArtifact) {
		t.Fatalf("changed source admitted: %v", e)
	}
	c := conn(t, "slovnyk_operator")
	var raw []byte
	if e = c.QueryRow(ctx, "SELECT manifest_bytes FROM slovnyk.datasets WHERE id=$1", id).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	m, e = importdata.ParseManifest(raw)
	if e != nil {
		t.Fatal(e)
	}
	_, fp, e := m.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	if e = importer.AcquireWriter(ctx, c, fp); e != nil {
		t.Fatal(e)
	}
	if _, e = Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import"); !errors.Is(e, importer.ErrBusy) {
		t.Fatalf("concurrent writer admitted: %v", e)
	}
	closeConn(c)
	// A failed operation releases its locks and does not poison the next run.
	if _, e = Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import"); e != nil {
		t.Fatal(e)
	}
	assertStillValidating(t, id, before)
	c = conn(t, "slovnyk_operator")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = Reconcile(canceled, c, id, "../../testdata/import"); e == nil {
		t.Fatal("canceled reconciliation succeeded")
	}
	if !c.IsClosed() {
		t.Fatal("owned connection left open")
	}
	assertStillValidating(t, id, before)
	// Diagnostic output excludes readiness and export attestations.
	r, e := Reconcile(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import")
	if e != nil {
		t.Fatal(e)
	}
	b, e = json.Marshal(r)
	if e != nil || strings.Contains(string(b), "controls_checked") || strings.Contains(string(b), "export_id") {
		t.Fatalf("unexpected report shape: %v", e)
	}
}
