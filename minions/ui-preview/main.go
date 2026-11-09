// ui-preview is an agent-only loopback preview of the real HTTP templates.
// All callbacks use fixed synthetic cases; this is not a dictionary backend.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/catalog"
	"github.com/valpere/slovnyk/internal/downloads"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/lookup"
)

const dataset = "00000000-0000-0000-0000-000000000001"

func fixtures() []entries.Page {
	return []entries.Page{
		{Dataset: dataset, From: "en-us", To: "uk-ua", Source: "hello", Translations: []string{"привіт", "вітаю"}},
		{Dataset: dataset, From: "uk-ua", To: "en-us", Source: "привіт", Translations: []string{"hello", "hi"}},
		{Dataset: dataset, From: "ar-sa", To: "en-us", Source: "مرحبا", Translations: []string{"hello", "A synthetic long translation: " + strings.Repeat("dictionary", 35), "Line one\nLine two <safe> & raw"}},
	}
}

// newPreview writes only synthetic CSVs under a caller-owned scratch directory.
func newPreview(dir string) (http.Handler, error) {
	pages := fixtures()
	inventory := catalog.Snapshot{Dataset: dataset, Locales: []string{"ar-sa", "en-us", "uk-ua"}}
	manifest := downloads.Manifest{Dataset: dataset}
	files := map[string]string{}
	for i, p := range pages {
		inventory.Pairs = append(inventory.Pairs, catalog.Pair{From: p.From, To: p.To, Count: int64(len(p.Translations))})
		var body bytes.Buffer
		gz := gzip.NewWriter(&body)
		writer := csv.NewWriter(gz)
		for _, translation := range p.Translations {
			if err := writer.Write([]string{p.From, p.To, p.Source, translation}); err != nil {
				return nil, err
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body.Bytes())
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+2)
		file := filepath.Join(dir, id+".csv.gz")
		if err := os.WriteFile(file, body.Bytes(), 0600); err != nil {
			return nil, err
		}
		files[id] = file
		manifest.Exports = append(manifest.Exports, downloads.Export{Dataset: dataset, ID: id, From: p.From, To: p.To, Rows: int64(len(p.Translations)), Artifact: artifacts.Ref{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(body.Len())}})
	}
	search := func(_ context.Context, r lookup.Request) (lookup.Page, error) {
		if !inventory.Has(r.From, r.To) {
			return lookup.Page{}, lookup.ErrPair
		}
		if r.Q == "failure" {
			return lookup.Page{}, lookup.ErrUnavailable
		}
		out := lookup.Page{Dataset: dataset}
		for _, p := range pages {
			// Fixed visual scenarios only; no production mask/cursor implementation.
			if p.From == r.From && p.To == r.To && (r.Q == p.Source || r.Q == "*") {
				out.Groups = append(out.Groups, lookup.Group{Source: p.Source, Translations: p.Translations})
			}
		}
		return out, nil
	}
	entry := func(_ context.Context, id, cursor string) (entries.Page, error) {
		if cursor != "" {
			return entries.Page{}, lookup.ErrCursor
		}
		for _, p := range pages {
			if entries.ID(p.From, p.To, p.Source) == id {
				return p, nil
			}
		}
		return entries.Page{}, entries.ErrNotFound
	}
	open := func(_ context.Context, ds, id string) (downloads.Export, *os.File, error) {
		if ds != dataset {
			return downloads.Export{}, nil, downloads.ErrNotFound
		}
		for _, e := range manifest.Exports {
			if e.ID == id {
				f, err := os.Open(files[id])
				return e, f, err
			}
		}
		return downloads.Export{}, nil, downloads.ErrNotFound
	}
	return httpui.New(search, nil,
		httpui.WithCatalog(func(context.Context, string) (catalog.Snapshot, error) { return inventory, nil }),
		httpui.WithEntries(entry),
		httpui.WithDownloads(func(context.Context) (downloads.Manifest, error) { return manifest, nil }, open)), nil
}

func run() error {
	port := flag.Int("port", 0, "loopback port; 0 selects an available port")
	flag.Parse()
	if flag.NArg() != 0 || *port < 0 || *port > 65535 {
		return errors.New("usage: ui-preview [-port 0..65535]")
	}
	if err := os.MkdirAll("tmp", 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("tmp", "ui-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) // Only this invocation's generated synthetic CSVs.
	handler, err := newPreview(dir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"address": listener.Addr().String(), "data": "synthetic only; no DB; fixed visual cases", "entry": entries.Path(entries.ID("ar-sa", "en-us", "مرحبا"))}); err != nil {
		_ = server.Close()
		return err
	}
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		err = <-done
	case err = <-done:
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
