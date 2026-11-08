package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(base string) config {
	return config{Base: base, Phase: "test", Workers: 2, TimeoutMS: 500, MaximumSeconds: 5, BodyLimit: 1024, Cases: []probeCase{{ID: 0, Kind: "exact", Path: "/"}}, Schedule: []int{0}}
}

func TestPrivateResponseAndAllArrivals(t *testing.T) {
	const secret = "private-query-and-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(8 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(secret))
	}))
	defer server.Close()
	c := fixture(server.URL)
	c.Cases[0].Path = "/?q=" + secret
	c.Schedule = []int{0, 0, 0, 0, 0, 0}
	c.IntervalMS = 0
	h := sha256.Sum256([]byte(secret))
	c.Cases[0].Expected = &oracle{200, len(secret), hex.EncodeToString(h[:]), "text/plain", "no-store"}
	seen := map[int]bool{}
	lagged := false
	err := execute(context.Background(), c, func(r observation) error {
		if seen[r.Sequence] {
			t.Fatal("duplicate arrival")
		}
		seen[r.Sequence] = true
		if !r.OracleChecked || !r.OracleMatch || r.ErrorClass != "" {
			t.Fatalf("unexpected outcome: %+v", r)
		}
		if r.DispatchLagMS > 5 {
			lagged = true
		}
		if r.ScheduledToCompleteMS < r.ElapsedMS || r.Bytes != len(secret) {
			t.Fatal("missing queue/body accounting")
		}
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), secret) || strings.Contains(string(b), "?q=") {
			t.Fatal("private data leaked")
		}
		return nil
	})
	if err != nil || len(seen) != len(c.Schedule) || !lagged {
		t.Fatalf("accounting error: count=%d lag=%v err=%v", len(seen), lagged, err)
	}
}

func TestDiagnosticFailuresRemainObservations(t *testing.T) {
	var followed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/followed", 302)
		case "/followed":
			followed.Store(true)
		case "/large":
			w.Write([]byte(strings.Repeat("x", 2048)))
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			w.Write([]byte("late"))
		default:
			http.Error(w, "unavailable", 503)
		}
	}))
	defer server.Close()
	c := fixture(server.URL)
	c.Workers = 1
	c.TimeoutMS = 100
	c.Cases = []probeCase{{ID: 0, Kind: "redirect", Path: "/redirect"}, {ID: 1, Kind: "large", Path: "/large"}, {ID: 2, Kind: "slow", Path: "/slow"}, {ID: 3, Kind: "unavailable", Path: "/"}}
	c.Schedule = []int{0, 1, 2, 3}
	got := map[int]observation{}
	if err := execute(context.Background(), c, func(r observation) error { got[r.CaseID] = r; return nil }); err != nil {
		t.Fatal(err)
	}
	if followed.Load() || got[0].Status != 302 || got[1].ErrorClass != "body_limit" || got[2].ErrorClass != "client_timeout" || got[3].ErrorClass != "http_503" {
		t.Fatalf("wrong failure classification: %+v", got)
	}
	if got[1].SHA256 != "" {
		t.Fatal("truncated response treated as full hash")
	}
}

func TestOutputFailureStopsWithoutLeaking(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	c := fixture(s.URL)
	c.Schedule = []int{0, 0, 0}
	err := execute(context.Background(), c, func(observation) error { return errors.New("private sink details") })
	if err == nil || strings.Contains(err.Error(), "private sink") {
		t.Fatal("sink failure swallowed or leaked")
	}
}

func TestConfigAndExclusiveOutputs(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	dir := t.TempDir()
	input := filepath.Join(dir, "private.json")
	output := filepath.Join(dir, "result")
	c := fixture(s.URL)
	b, _ := json.Marshal(c)
	if err := os.WriteFile(input, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{input, output}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(output, "observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{input, output}); err == nil {
		t.Fatal("existing output overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(output, "observations.jsonl"))
	if string(before) != string(after) {
		t.Fatal("old evidence modified")
	}
	for _, name := range []string{input, filepath.Join(output, "observations.jsonl"), filepath.Join(output, "summary.json")} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("unsafe file permissions")
		}
	}
	if err := os.Chmod(input, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{input, filepath.Join(dir, "other")}); err == nil {
		t.Fatal("public config accepted")
	}
	for _, base := range []string{"https://127.0.0.1:80", "http://localhost:80", "http://192.0.2.1:80", "http://127.0.0.1:80/path", "http://u:p@127.0.0.1:80"} {
		bad := c
		bad.Base = base
		if bad.validate() == nil {
			t.Fatal("unsafe base accepted")
		}
	}
	bad := c
	bad.Cases = []probeCase{{ID: 0, Kind: "exact", Path: "//example.org/private"}}
	if bad.validate() == nil {
		t.Fatal("absolute case accepted")
	}
}

func TestPacedArrivalsAndMismatchedOracle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("actual")) }))
	defer s.Close()
	c := fixture(s.URL)
	c.IntervalMS = 50
	c.Schedule = []int{0, 0, 0}
	h := sha256.Sum256([]byte("different"))
	c.Cases[0].Expected = &oracle{200, 9, hex.EncodeToString(h[:]), "text/plain; charset=utf-8", ""}
	start := time.Now()
	rows := []observation{}
	if err := execute(context.Background(), c, func(r observation) error { rows = append(rows, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 100*time.Millisecond || len(rows) != 3 {
		t.Fatal("paced arrivals lost or dispatched early")
	}
	for _, r := range rows {
		if !r.OracleChecked || r.OracleMatch || r.ErrorClass != "response_mismatch" {
			t.Fatal("oracle mismatch hidden")
		}
	}
}

func TestOversizedConfigRejectsHiddenTrailingData(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	dir := t.TempDir()
	c := fixture(s.URL)
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, []byte(strings.Repeat(" ", 2<<20)+"invalid trailing data")...)
	input := filepath.Join(dir, "large.json")
	if err = os.WriteFile(input, b, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "output")
	if err = run([]string{input, output}); err == nil {
		t.Fatal("oversized config hid trailing bytes beyond decoder limit")
	}
	if _, err = os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid config created output")
	}
}
