//go:build integration

package publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/importdata"
)

func approvalFor(t *testing.T, id string) LossApproval {
	t.Helper()
	r, e := Reconcile(context.Background(), conn(t, "slovnyk_operator"), id, "../../testdata/import")
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(append(b, '\n'))
	a := sampleApproval()
	a.Dataset, a.Fingerprint, a.ReconciliationSHA256 = id, r.Fingerprint, hex.EncodeToString(h[:])
	return a
}

func TestReviewedMetadataGaps(t *testing.T) {
	ctx := context.Background()
	dir := root(t)
	id := staged(t, "reviewed-metadata", func(m *importdata.Manifest) {
		m.KnownGaps = []string{"CSV lacks historical count/status/query metadata"}
	})
	before := generation(t)
	a := approvalFor(t, id)
	for _, change := range []func(*LossApproval){
		func(a *LossApproval) { a.Dataset = "00000000-0000-0000-0000-000000000001" },
		func(a *LossApproval) { a.Fingerprint = strings.Repeat("0", 64) },
		func(a *LossApproval) { a.ReconciliationSHA256 = strings.Repeat("0", 64) },
		func(a *LossApproval) { a.OwnerRef = "" },
	} {
		bad := a
		change(&bad)
		if _, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{LossApproval: &bad}); !errors.Is(e, ErrVerification) {
			t.Fatalf("invalid decision admitted: %v", e)
		}
		assertStillValidating(t, id, before)
	}
	if _, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{LossApproval: &a, beforeReady: func() error { return context.Canceled }}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	assertStillValidating(t, id, before)
	r, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{LossApproval: &a})
	if e != nil || r.LossApproval == nil || *r.LossApproval != a || len(r.KnownGaps) != 1 {
		t.Fatalf("reviewed verify: %+v %v", r, e)
	}
	// Ready retries retain the original decision; they cannot silently replace it.
	again, e := Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{LossApproval: &a})
	if e != nil || !reflect.DeepEqual(r, again) {
		t.Fatalf("ready retry: %v", e)
	}
	bad := a
	bad.DecisionRef = "a different decision"
	if _, e = Verify(ctx, conn(t, "slovnyk_operator"), id, "../../testdata/import", dir, Options{LossApproval: &bad}); !errors.Is(e, ErrVerification) {
		t.Fatal("ready retry replaced approval", e)
	}
	if _, e = Publish(ctx, conn(t, "slovnyk_operator"), id, dir, before, "synthetic approved publication", false); e != nil {
		t.Fatal(e)
	}
	if _, e = CheckActive(ctx, conn(t, "slovnyk_http"), dir); e != nil {
		t.Fatal("HTTP cannot read retained approval", e)
	}
}

func TestApprovalDoesNotBypassGuards(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"metadata", "incomplete", "source", "exception"} {
		t.Run(name, func(t *testing.T) {
			id := staged(t, "approval-guard-"+name, func(m *importdata.Manifest) {
				m.KnownGaps = []string{"synthetic historical gap"}
				if name == "exception" {
					m.Exceptions = []importdata.Exception{{ArtifactSHA256: strings.Repeat("f", 64), Decision: "exclude_artifact", Reason: "synthetic excluded source"}}
				}
			})
			before := generation(t)
			a := approvalFor(t, id)
			admin := conn(t, "")
			queries := map[string]string{
				"metadata":   "UPDATE slovnyk.translation_origins SET legacy_count=0 WHERE dataset_id=$1",
				"incomplete": "UPDATE slovnyk.source_files SET complete=false,checksum_verified=false WHERE dataset_id=$1",
			}
			if q := queries[name]; q != "" {
				if _, e := admin.Exec(ctx, q, id); e != nil {
					t.Fatal(e)
				}
			}
			source := "../../testdata/import"
			if name == "source" {
				source = root(t)
			}
			dir := root(t)
			if _, e := Verify(ctx, conn(t, "slovnyk_operator"), id, source, dir, Options{LossApproval: &a}); e == nil {
				t.Fatal("approval bypassed data verification")
			}
			assertStillValidating(t, id, before)
			// Invalid candidates must not have generated any retained artifacts.
			entries, e := os.ReadDir(dir)
			if e != nil {
				t.Fatal(e)
			}
			if len(entries) != 0 {
				t.Fatal("invalid candidate created retained artifacts")
			}
		})
	}
}
