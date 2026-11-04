// corpus-progress summarizes the assistant's local reconciliation observations.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type sample struct {
	Elapsed float64 `json:"elapsed_seconds"`
	Free    int64   `json:"free_bytes"`
	DB      int64   `json:"database_bytes"`
	Source  string  `json:"source_file"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: corpus-progress RUN_DIRECTORY MANIFEST_JSON")
	}
	f, err := os.Open(filepath.Join(os.Args[1], "progress.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var last, observed sample
	var n int
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			break // The writer may still be appending the final line.
		}
		if err != nil {
			return err
		}
		last = sample{}
		if err = json.Unmarshal(line, &last); err != nil {
			return err
		}
		n++
		if last.Source != "" {
			observed = last
		}
	}
	if n == 0 {
		return fmt.Errorf("no complete progress observations")
	}
	b, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	var manifest struct {
		Artifacts []struct {
			Name string `json:"name"`
			Hash string `json:"sha256"`
		} `json:"artifacts"`
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	sort.Slice(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].Hash < manifest.Artifacts[j].Hash })
	index := 0
	for i, a := range manifest.Artifacts {
		if a.Name == observed.Source {
			index = i + 1
			break
		}
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Elapsed       float64 `json:"elapsed_seconds"`
		FreeGiB       float64 `json:"free_gib"`
		DatabaseGiB   float64 `json:"database_gib"`
		ObservedIndex int     `json:"last_observed_file_index"`
		Files         int     `json:"manifest_files"`
		ObservationAt float64 `json:"source_observation_elapsed_seconds"`
	}{last.Elapsed, float64(last.Free) / (1 << 30), float64(last.DB) / (1 << 30), index, len(manifest.Artifacts), observed.Elapsed})
}
