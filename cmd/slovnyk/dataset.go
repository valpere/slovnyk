package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"

	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/sitemaps"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func runDataset(ctx context.Context, args []string, getenv func(string) string, out io.Writer) int {
	fail := func(code string, status int) int {
		_ = json.NewEncoder(out).Encode(map[string]string{"error": code})
		return status
	}
	if len(args) < 2 || !publication.ValidID(args[1]) {
		return fail("usage: slovnyk dataset reconcile ID --source DIRECTORY | verify|export|sitemap|publish|rollback ID --artifacts DIRECTORY", 2)
	}
	command, id := args[0], args[1]
	if command != "reconcile" && command != "verify" && command != "export" && command != "sitemap" && command != "publish" && command != "rollback" {
		return fail("invalid_dataset_command", 2)
	}
	flags := flag.NewFlagSet("dataset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("artifacts", "", "existing durable artifact root")
	source := flags.String("source", "", "immutable source snapshot (reconcile or verify)")
	generation := flags.Int64("expected-generation", -1, "required CAS generation")
	operator := flags.String("operator-ref", "", "operator or audit reference")
	if e := flags.Parse(args[2:]); e != nil || flags.NArg() != 0 || (command != "reconcile" && *root == "") {
		return fail("invalid_dataset_arguments", 2)
	}
	mutation := command == "publish" || command == "rollback"
	readsSource := command == "reconcile" || command == "verify"
	if (readsSource && *source == "") || (mutation && (*generation < 0 || *operator == "")) {
		return fail("invalid_dataset_arguments", 2)
	}
	valid := true
	flags.Visit(func(f *flag.Flag) {
		if (f.Name == "source" && !readsSource) || (f.Name == "artifacts" && command == "reconcile") || ((f.Name == "expected-generation" || f.Name == "operator-ref") && !mutation) {
			valid = false
		}
	})
	if !valid {
		return fail("invalid_dataset_arguments", 2)
	}
	dsn := getenv("SLOVNYK_OPERATOR_DSN")
	if dsn == "" {
		return fail("SLOVNYK_OPERATOR_DSN is required", 2)
	}
	c, e := postgres.Connect(ctx, dsn)
	if e != nil {
		return fail("publication_storage_unavailable", 1)
	}
	var result any
	switch command {
	case "reconcile":
		result, e = publication.Reconcile(ctx, c, id, *source)
	case "verify":
		result, e = publication.Verify(ctx, c, id, *source, *root, publication.Options{})
	case "sitemap":
		result, e = sitemaps.Generate(ctx, c, id, *root)
	case "export":
		result, e = publication.Export(ctx, c, id, *root)
	default:
		result, e = publication.Publish(ctx, c, id, *root, *generation, *operator, command == "rollback")
	}
	if e != nil {
		code := "dataset_operation_failed"
		switch e {
		case importer.ErrArtifact, importer.ErrStream, importer.ErrReplay, importer.ErrBusy, importer.ErrStorage, sitemaps.ErrArguments, sitemaps.ErrState, sitemaps.ErrStorage, sitemaps.ErrEmpty, sitemaps.ErrLimit, publication.ErrArguments, publication.ErrConflict, publication.ErrState, publication.ErrVerification, publication.ErrStorage:
			code = e.Error()
		}
		return fail(code, 1)
	}
	if json.NewEncoder(out).Encode(result) != nil {
		return 1
	}
	return 0
}
