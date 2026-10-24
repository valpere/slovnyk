package importer

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/valpere/slovnyk/internal/importdata"
)

func fixture(t *testing.T) importdata.Manifest {
	t.Helper()
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestInspect(t *testing.T) {
	r, e := Inspect(context.Background(), fixture(t), "../../testdata/import")
	if e != nil || r.State != "inspected" || r.DistinctCountsAvailable || len(r.Files) != 1 || r.Files[0].Parsed != 11 || !r.Files[0].Complete {
		t.Fatalf("%+v %v", r, e)
	}
	b, _ := json.Marshal(r)
	if len(b) == 0 {
		t.Fatal("missing JSON")
	}
	m := fixture(t)
	m.Artifacts[0].SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, e = Inspect(context.Background(), m, "../../testdata/import"); e != ErrArtifact {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Inspect(ctx, fixture(t), "../../testdata/import"); e != context.Canceled {
		t.Fatal(e)
	}
}
