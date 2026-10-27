//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func TestImportRunCLI(t *testing.T) {
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("SLOVNYK_TEST_DSN required")
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	admin, e := postgres.Connect(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(context.Background())
	if _, e = admin.Exec(context.Background(), "CREATE ROLE slovnyk_cli_test LOGIN PASSWORD 'synthetic-cli-only' IN ROLE slovnyk_operator"); e != nil {
		t.Fatal(e)
	}
	u.User = url.UserPassword("slovnyk_cli_test", "synthetic-cli-only")
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "CLI integration"
	b, _ = json.Marshal(m)
	if e = os.MkdirAll("../../tmp/m1-04", 0700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp("../../tmp/m1-04", "cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "manifest.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		var out bytes.Buffer
		code := run(ctx, []string{"import", "run", "--manifest", path, "--source", "../../testdata/import", "--batch-size", "2"}, func(key string) string {
			if key == "SLOVNYK_OPERATOR_DSN" {
				return u.String()
			}
			return ""
		}, &out)
		var r importer.Report
		if e = json.Unmarshal(out.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		if code != 0 || r.State != "validating" || r.Files[0].Unique != 10 || r.Files[0].Duplicate != 1 || !r.Files[0].Complete || strings.Contains(out.String(), "synthetic-test-only") || strings.Contains(out.String(), "synthetic-cli-only") {
			t.Fatalf("exit=%d %+v", code, r)
		}
	}
}

func TestDatasetCLI(t *testing.T) {
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("use make test-integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, e := postgres.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "dataset CLI"
	staged, e := importer.Run(ctx, c, m, "../../testdata/import", importer.Options{})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll("../../tmp/m1-05", 0700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp("../../tmp/m1-05", "cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("role", "slovnyk_operator")
	u.RawQuery = q.Encode()
	env := func(key string) string {
		if key == "SLOVNYK_OPERATOR_DSN" {
			return u.String()
		}
		return ""
	}
	for _, command := range []string{"reconcile", "verify", "export", "sitemap"} {
		args := []string{"dataset", command, staged.Dataset, "--artifacts", dir}
		if command == "reconcile" {
			args = []string{"dataset", command, staged.Dataset, "--source", "../../testdata/import"}
		}
		if command == "verify" {
			args = append(args, "--source", "../../testdata/import")
		}
		var out bytes.Buffer
		if code := run(ctx, args, env, &out); code != 0 {
			t.Fatalf("%s: %d %s", command, code, out.String())
		}
		var report map[string]any
		if json.Unmarshal(out.Bytes(), &report) != nil || report["dataset"] != staged.Dataset {
			t.Fatalf("invalid report %s", out.String())
		}
	}
	c, e = postgres.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	var n int64
	if e = c.QueryRow(ctx, "SELECT generation FROM slovnyk.active_dataset").Scan(&n); e != nil {
		t.Fatal(e)
	}
	args := []string{"dataset", "publish", staged.Dataset, "--artifacts", dir, "--operator-ref", "CLI test", "--expected-generation", fmt.Sprint(n)}
	var out bytes.Buffer
	if code := run(ctx, args, env, &out); code != 0 {
		t.Fatalf("publish: %d %s", code, out.String())
	}
	out.Reset()
	if code := run(ctx, args, env, &out); code != 1 || !strings.Contains(out.String(), "publication_generation_conflict") {
		t.Fatalf("stale CLI: %d %s", code, out.String())
	}
}
