// Command prepare-csv-manifest inventories a frozen flat legacy CSV snapshot.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/normalization"
)

type gaps []string

func (g *gaps) String() string     { return "" }
func (g *gaps) Set(s string) error { *g = append(*g, s); return nil }

// inventory hashes compressed bytes only. Filename pairs are expectations for
// subsequent import inspect, not evidence of valid CSV, gzip or record contents.
func inventory(source string, metadata importdata.Source, knownGaps []string) ([]byte, [32]byte, importdata.Manifest, error) {
	var zero [32]byte
	m := importdata.Manifest{
		SchemaVersion: 1, Source: metadata,
		ParserVersion: importdata.ParserVersion, MappingVersion: importdata.MappingVersion,
		NormalizationVersion: normalization.Version,
		AdmissionLimits:      importdata.Limits{FieldBytes: importdata.MaxFieldBytes, RecordBytes: importdata.MaxRecordBytes, ArtifactBytes: importdata.MaxArtifactBytes},
		KnownGaps:            append([]string{}, knownGaps...), Exceptions: []importdata.Exception{},
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, zero, m, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, zero, m, err
	}
	defer dir.Close()
	locales, pairs := map[string]bool{}, map[importdata.Pair]bool{}
	// Stream the directory too; reject oversized metadata before keeping it all.
	metadataBytes := 0
	for {
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && readErr != io.EOF {
			return nil, zero, m, readErr
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "slovnyk_") || !strings.HasSuffix(name, ".csv.gz") {
				return nil, zero, m, errors.New("unexpected_snapshot_entry")
			}
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, "slovnyk_"), ".csv.gz"), "_")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, zero, m, errors.New("invalid_pair_filename")
			}
			info, err := root.Lstat(name)
			if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
				return nil, zero, m, errors.New("invalid_snapshot_file")
			}
			metadataBytes += len(name) + len(parts[0]) + len(parts[1]) + 256
			if metadataBytes > importdata.MaxManifestBytes {
				return nil, zero, m, errors.New("snapshot_inventory_limit")
			}
			pair := importdata.Pair{From: parts[0], To: parts[1]}
			a := importdata.Artifact{Name: name, Bytes: info.Size(), Format: "csv4.gz", Pair: &pair}
			f, err := importdata.OpenArtifact(source, a)
			if err != nil {
				return nil, zero, m, err
			}
			hash := sha256.New()
			n, copyErr := io.Copy(hash, f)
			after, statErr := f.Stat()
			pathInfo, pathErr := root.Lstat(name)
			closeErr := f.Close()
			if copyErr != nil || statErr != nil || pathErr != nil || closeErr != nil || n != info.Size() ||
				!os.SameFile(info, after) || !os.SameFile(after, pathInfo) || !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
				return nil, zero, m, errors.New("snapshot_changed_or_unreadable")
			}
			a.SHA256 = hex.EncodeToString(hash.Sum(nil))
			m.Artifacts = append(m.Artifacts, a)
			locales[pair.From], locales[pair.To], pairs[pair] = true, true, true
		}
		if readErr == io.EOF {
			break
		}
	}
	for locale := range locales {
		m.Locales = append(m.Locales, locale)
	}
	for pair := range pairs {
		m.ExpectedPairs = append(m.ExpectedPairs, pair)
	}
	canonical, fingerprint, err := m.Canonical()
	if err != nil {
		return nil, zero, m, err
	}
	if _, err := importdata.ParseManifest(canonical); err != nil {
		return nil, zero, m, err
	}
	return canonical, fingerprint, m, nil
}

// save creates a new manifest only. Keep output outside the frozen flat source.
func save(source, output string, data []byte) error {
	dir, err := os.Open(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer dir.Close()
	parent, err := dir.Stat()
	if err != nil {
		return err
	}
	input, err := os.Stat(source)
	if err != nil {
		return err
	}
	if os.SameFile(parent, input) {
		return errors.New("output_inside_snapshot")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		_ = f.Close()
		if !complete {
			_ = os.Remove(output)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		return err
	}
	complete = true
	return nil
}

func run(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("prepare-csv-manifest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	source := flags.String("source", "", "frozen flat CSV snapshot")
	output := flags.String("output", "", "new manifest outside snapshot")
	authority := flags.String("authority", "", "source authority")
	date := flags.String("snapshot-date", "unknown", "YYYY-MM-DD or unknown")
	rights := flags.String("rights-status", "unknown", "owner_confirmed or unknown")
	var knownGaps gaps
	flags.Var(&knownGaps, "known-gap", "known omission; repeatable")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *source == "" || *output == "" || *authority == "" {
		fmt.Fprintln(diagnostic, "usage: prepare-csv-manifest --source DIR --output FILE --authority NAME [--snapshot-date YYYY-MM-DD|unknown] [--rights-status owner_confirmed|unknown] [--known-gap TEXT]")
		return 2
	}
	data, fingerprint, m, err := inventory(*source, importdata.Source{Kind: "csv_snapshot", Authority: *authority, SnapshotDate: *date, RightsStatus: *rights}, knownGaps)
	if err != nil {
		fmt.Fprintln(diagnostic, "Cannot inventory snapshot; no manifest written")
		return 1
	}
	if err := save(*source, *output, data); err != nil {
		fmt.Fprintln(diagnostic, "Cannot create new manifest outside snapshot")
		return 1
	}
	if err := json.NewEncoder(out).Encode(struct {
		Fingerprint     string `json:"fingerprint"`
		Artifacts       int    `json:"artifacts"`
		Locales         int    `json:"locales"`
		Pairs           int    `json:"expected_pairs"`
		ContentVerified bool   `json:"content_verified"`
	}{hex.EncodeToString(fingerprint[:]), len(m.Artifacts), len(m.Locales), len(m.ExpectedPairs), false}); err != nil {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
