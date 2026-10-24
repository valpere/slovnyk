package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func runImport(ctx context.Context, args []string, getenv func(string) string, out io.Writer) int {
	fail := func(code string, status int) int {
		_ = json.NewEncoder(out).Encode(map[string]string{"error": code})
		return status
	}
	if len(args) < 1 || (args[0] != "inspect" && args[0] != "run") {
		return fail("usage: slovnyk import inspect|run --manifest FILE --source DIRECTORY", 2)
	}
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifest := flags.String("manifest", "", "manifest JSON")
	root := flags.String("source", "", "immutable snapshot directory")
	batch := flags.Int("batch-size", 100, "records per transaction (run only)")
	if e := flags.Parse(args[1:]); e != nil || flags.NArg() != 0 || *manifest == "" || *root == "" || *batch < 1 || *batch > 1000 {
		return fail("invalid_import_arguments", 2)
	}
	f, e := os.Open(*manifest)
	if e != nil {
		return fail("manifest_unavailable", 2)
	}
	b, e := io.ReadAll(io.LimitReader(f, importdata.MaxManifestBytes+1))
	_ = f.Close()
	if e != nil {
		return fail("manifest_unavailable", 2)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		return fail("invalid_manifest", 2)
	}
	var r importer.Report
	if args[0] == "inspect" {
		r, e = importer.Inspect(ctx, m, *root)
	} else {
		if getenv("SLOVNYK_OPERATOR_DSN") == "" {
			return fail("SLOVNYK_OPERATOR_DSN is required", 2)
		}
		c, err := postgres.Connect(ctx, getenv("SLOVNYK_OPERATOR_DSN"))
		if err != nil {
			status := 1
			if errors.Is(err, postgres.ErrConfig) {
				status = 2
			}
			return fail(importer.ErrorCode(err), status)
		}
		r, e = importer.Run(ctx, c, m, *root, importer.Options{BatchSize: *batch})
	}
	if e != nil {
		r.Error = importer.ErrorCode(e)
	}
	if err := json.NewEncoder(out).Encode(r); err != nil {
		return 1
	}
	if e != nil {
		return 1
	}
	return 0
}
