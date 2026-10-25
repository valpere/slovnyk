package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
)

func scratch(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll("../../tmp/utilities-tests", 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("../../tmp/utilities-tests", "csv-manifest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func fixture(t *testing.T, dir, name, csv string) []byte {
	t.Helper()
	var data bytes.Buffer
	z := gzip.NewWriter(&data)
	if _, err := io.WriteString(z, csv); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

var metadata = importdata.Source{Kind: "csv_snapshot", Authority: "synthetic", SnapshotDate: "unknown", RightsStatus: "unknown"}

func TestManifestRoundTripInspectAndDeterminism(t *testing.T) {
	base := scratch(t)
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	inputs := map[string]string{
		"slovnyk_en-gb_en-us.csv.gz": "en-gb,en-us,colour,color\n",
		"slovnyk_eo-xx_la-va.csv.gz": "eo-xx,la-va,\"saluton\r\nmondo\",\"salve, munde\"\n",
		"slovnyk_sr-rs_sr-rs.csv.gz": "sr-rs,sr-rs,реч,реч\n",
	}
	original := map[string][]byte{}
	for _, name := range []string{"slovnyk_en-gb_en-us.csv.gz", "slovnyk_eo-xx_la-va.csv.gz", "slovnyk_sr-rs_sr-rs.csv.gz"} {
		original[name] = fixture(t, source, name, inputs[name])
	}
	data, fingerprint, m, err := inventory(source, metadata, []string{"legacy counts unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(data) != fingerprint {
		t.Fatal("fingerprint is not canonical file hash")
	}
	parsed, err := importdata.ParseManifest(data)
	if err != nil || len(parsed.Artifacts) != 3 || len(parsed.Locales) != 5 || len(parsed.ExpectedPairs) != 3 || len(parsed.KnownGaps) != 1 || len(parsed.Exceptions) != 0 {
		t.Fatal("invalid inventory", err)
	}
	for _, artifact := range parsed.Artifacts {
		want := sha256.Sum256(original[artifact.Name])
		if artifact.SHA256 != hex.EncodeToString(want[:]) || artifact.Bytes != int64(len(original[artifact.Name])) || artifact.Pair == nil || artifact.MultiPair {
			t.Fatal("artifact differs")
		}
	}
	report, err := importer.Inspect(context.Background(), m, source)
	if err != nil || report.Error != "" || len(report.Files) != 3 {
		t.Fatal("generated manifest failed inspect", err)
	}
	for _, file := range report.Files {
		if !file.Complete || !file.ChecksumVerified || file.Parsed != 1 || file.Rejected != 0 {
			t.Fatal("inspect did not validate fixture", file)
		}
	}
	// Re-create in a different order/location; neither path nor directory order belongs to identity.
	copyDir := filepath.Join(base, "copy")
	if err := os.Mkdir(copyDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"slovnyk_sr-rs_sr-rs.csv.gz", "slovnyk_eo-xx_la-va.csv.gz", "slovnyk_en-gb_en-us.csv.gz"} {
		if err := os.WriteFile(filepath.Join(copyDir, name), original[name], 0600); err != nil {
			t.Fatal(err)
		}
	}
	repeated, fp, _, err := inventory(copyDir, metadata, []string{"legacy counts unavailable"})
	if err != nil || !bytes.Equal(data, repeated) || fp != fingerprint {
		t.Fatal("manifest depends on host/order", err)
	}
	for name, want := range original {
		got, err := os.ReadFile(filepath.Join(source, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("source changed")
		}
	}
}

func TestInventoryRejectsUnsafeAndAmbiguousInputs(t *testing.T) {
	for _, kind := range []string{"empty", "unexpected", "directory", "symlink", "fifo", "empty-file", "bad-name", "duplicate-hash"} {
		t.Run(kind, func(t *testing.T) {
			dir := scratch(t)
			name := filepath.Join(dir, "slovnyk_en-us_uk-ua.csv.gz")
			switch kind {
			case "unexpected":
				if err := os.WriteFile(filepath.Join(dir, "README"), []byte("not silently ignored"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(name, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("/etc/passwd", name); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(name, 0600); err != nil {
					t.Fatal(err)
				}
			case "empty-file":
				if err := os.WriteFile(name, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "bad-name":
				fixture(t, dir, "slovnyk_en-us_uk-ua_extra.csv.gz", "en-us,uk-ua,a,b\n")
			case "duplicate-hash":
				b := fixture(t, dir, filepath.Base(name), "en-us,uk-ua,a,b\n")
				if err := os.WriteFile(filepath.Join(dir, "slovnyk_uk-ua_en-us.csv.gz"), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if data, _, _, err := inventory(dir, metadata, nil); err == nil || data != nil {
				t.Fatal("accepted unsafe inventory")
			}
		})
	}
}

func TestInventoryDoesNotClaimContentValidation(t *testing.T) {
	for _, input := range []string{"not gzip", "mismatched-pair", "corrupt-trailer"} {
		t.Run(input, func(t *testing.T) {
			dir := scratch(t)
			name := "slovnyk_en-us_uk-ua.csv.gz"
			b := fixture(t, dir, name, "uk-ua,en-us,слово,word\n")
			if input == "not gzip" {
				b = []byte("not a gzip stream")
			}
			if input == "corrupt-trailer" {
				b[len(b)-8] ^= 1
			}
			if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
				t.Fatal(err)
			}
			_, _, m, err := inventory(dir, metadata, nil)
			if err != nil {
				t.Fatal("byte inventory unexpectedly parsed content", err)
			}
			r, e := importer.Inspect(context.Background(), m, dir)
			if e == nil && len(r.Issues) == 0 {
				t.Fatal("inspect accepted invalid content")
			}
		})
	}
}

func TestCLIOutputIsExclusiveAndOutsideSource(t *testing.T) {
	base := scratch(t)
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	fixture(t, source, "slovnyk_en-us_uk-ua.csv.gz", "en-us,uk-ua,word,слово\n")
	output := filepath.Join(base, "manifest.json")
	args := []string{"--source", source, "--output", output, "--authority", "synthetic"}
	var out, diagnostic bytes.Buffer
	if code := run(args, &out, &diagnostic); code != 0 {
		t.Fatal(code, diagnostic.String())
	}
	var summary struct {
		ContentVerified bool   `json:"content_verified"`
		Fingerprint     string `json:"fingerprint"`
		Artifacts       int    `json:"artifacts"`
	}
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil || summary.ContentVerified || summary.Fingerprint == "" || summary.Artifacts != 1 {
		t.Fatal("misleading summary", err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("manifest is not private")
	}
	out.Reset()
	diagnostic.Reset()
	if run(args, &out, &diagnostic) == 0 || out.Len() != 0 {
		t.Fatal("overwrote manifest")
	}
	after, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing manifest changed")
	}
	// A symlink alias of the source directory must not allow output inside it.
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(alias, "manifest.json")
	if save(source, inside, before) == nil {
		t.Fatal("wrote inside source via alias")
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("unexpected source write")
	}
	for _, extra := range [][]string{{"--snapshot-date", "yesterday"}, {"--rights-status", "unverified"}, {"--known-gap", ""}} {
		out.Reset()
		diagnostic.Reset()
		candidate := filepath.Join(base, "invalid.json")
		bad := append([]string{"--source", source, "--output", candidate, "--authority", "private-authority"}, extra...)
		if run(bad, &out, &diagnostic) == 0 || out.Len() != 0 || strings.Contains(diagnostic.String(), "private-authority") {
			t.Fatal("invalid metadata accepted or exposed")
		}
		if _, err := os.Stat(candidate); !os.IsNotExist(err) {
			t.Fatal("invalid manifest created")
		}
	}
	if run([]string{"--source", "private-path"}, io.Discard, &diagnostic) != 2 || strings.Contains(diagnostic.String(), "private-path") {
		t.Fatal("usage exposed arguments")
	}
}
