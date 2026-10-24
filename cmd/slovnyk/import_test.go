package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/valpere/slovnyk/internal/importer"
)

func TestImportInspectCLI(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"import", "inspect", "--manifest", "../../testdata/import/manifest.json", "--source", "../../testdata/import"}, func(string) string { return "" }, &out)
	var r importer.Report
	if e := json.Unmarshal(out.Bytes(), &r); e != nil {
		t.Fatal(e)
	}
	if code != 0 || r.State != "inspected" || r.Files[0].Parsed != 11 || r.DistinctCountsAvailable {
		t.Fatalf("exit=%d %+v", code, r)
	}
}
func TestImportCLIErrorsHideArgumentsAndDSN(t *testing.T) {
	for _, args := range [][]string{
		{"import", "synthetic-secret"},
		{"import", "run", "--synthetic-secret"},
		{"import", "run", "--manifest", "../../testdata/import/manifest.json", "--source", "../../testdata/import"},
	} {
		var out bytes.Buffer
		code := run(context.Background(), args, func(string) string { return "postgres://u:synthetic-secret@host:invalid/db" }, &out)
		if code == 0 || strings.Contains(out.String(), "synthetic-secret") {
			t.Fatalf("unsafe error: %d %s", code, out.String())
		}
	}
}
