//go:build integration

package ops

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func TestReadinessRecoveryWithHTTPRole(t *testing.T) {
	dsn := os.Getenv("SLOVNYK_TEST_DSN")
	if dsn == "" {
		t.Fatal("use make test-integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	connect := func() *pgx.Conn {
		c, e := postgres.Connect(ctx, dsn)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			_ = c.Close(cleanup)
		})
		return c
	}
	if e := os.MkdirAll("../../tmp/m1-05", 0700); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp("../../tmp/m1-05", "readiness-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(root)
	b, e := os.ReadFile("../../testdata/import/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := importdata.ParseManifest(b)
	if e != nil {
		t.Fatal(e)
	}
	m.Source.Authority = "readiness-integration"
	staged, e := importer.Run(ctx, connect(), m, "../../testdata/import", importer.Options{})
	if e != nil {
		t.Fatal(e)
	}
	report, e := publication.Verify(ctx, connect(), staged.Dataset, "../../testdata/import", root, publication.Options{})
	if e != nil {
		t.Fatal(e)
	}
	var n int64
	if e = connect().QueryRow(ctx, "SELECT generation FROM slovnyk.active_dataset").Scan(&n); e != nil {
		t.Fatal(e)
	}
	if _, e = publication.Publish(ctx, connect(), report.Dataset, root, n, "readiness test", false); e != nil {
		t.Fatal(e)
	}
	httpURL, e := url.Parse(dsn)
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	query := httpURL.Query()
	query.Set("role", "slovnyk_http")
	httpURL.RawQuery = query.Encode()
	httpDSN := httpURL.String()
	probe, e := postgres.Connect(ctx, httpDSN)
	if e != nil {
		t.Fatal(e)
	}
	var role string
	if e = probe.QueryRow(ctx, "SELECT current_user").Scan(&role); e != nil {
		t.Fatal(e)
	}
	_ = probe.Close(ctx)
	if role != "slovnyk_http" {
		t.Fatal("test must use HTTP credentials")
	}
	// Exercise the real HTTP adapter with the restricted role and a published
	// synthetic dataset. Continuation switches format without changing its pin.
	signer, e := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if e != nil {
		t.Fatal(e)
	}
	handler := httpui.New(searchBackend(Config{HTTPDSN: httpDSN, CursorSigner: signer}), signer)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest("GET", "/search?from=uk-ua&to=en-us&q="+url.QueryEscape("слово")+"&limit=1&format=plain", nil))
	if first.Code != 200 || !strings.Contains(first.Body.String(), "Next: /search?") {
		t.Fatal("HTTP search failed", first.Code, first.Body.String())
	}
	_, continuation, ok := strings.Cut(first.Body.String(), "Next: ")
	if !ok {
		t.Fatal("missing continuation")
	}
	next, e := url.Parse(strings.TrimSpace(continuation))
	if e != nil {
		t.Fatal(e)
	}
	params := next.Query()
	params.Set("format", "html")
	next.RawQuery = params.Encode()
	secondPage := httptest.NewRecorder()
	handler.ServeHTTP(secondPage, httptest.NewRequest("GET", next.String(), nil))
	if secondPage.Code != 200 || !strings.Contains(secondPage.Body.String(), "word") || strings.Contains(secondPage.Body.String(), `rel="next"`) {
		t.Fatal("HTTP continuation failed", secondPage.Code, secondPage.Body.String())
	}
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"from=missing&to=en-us&q=x", 400}, {"from=en-us&to=en-us&q=x", 404}, {"from=uk-ua&to=en-us&q=not-in-fixture", 200},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/search?"+tc.query, nil))
		if response.Code != tc.status {
			t.Fatal("HTTP catalog status", tc.query, response.Code)
		}
	}

	renamed := root + "-offline"
	if e = os.Rename(root, renamed); e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(renamed)
	status := startReadiness(ctx, Config{HTTPDSN: httpDSN, ArtifactRoot: root})
	defer func() { cancel(); <-status.done }()
	check := func(want int) {
		t.Helper()
		r := httptest.NewRecorder()
		readinessHandler(status.ready.Load).ServeHTTP(r, httptest.NewRequest("GET", "/readyz", nil))
		if r.Code != want {
			t.Fatalf("HTTP %d, want %d", r.Code, want)
		}
	}
	check(503)
	if e = os.Rename(renamed, root); e != nil {
		t.Fatal(e)
	}
	wait := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			if status.ready.Load() == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("ready never became %v", want)
	}
	wait(true)
	check(200)
	cancel()
	<-status.done
	// Restart verifies bytes again, even though generation is unchanged.
	file := filepath.Join(root, report.Pairs[0].Artifact.Path)
	if e = os.Chmod(file, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, []byte("corrupted"), 0600); e != nil {
		t.Fatal(e)
	}
	secondCtx, stop := context.WithCancel(context.Background())
	defer stop()
	second := startReadiness(secondCtx, Config{HTTPDSN: httpDSN, ArtifactRoot: root})
	defer func() { stop(); <-second.done }()
	time.Sleep(1200 * time.Millisecond)
	if second.ready.Load() {
		t.Fatal("corrupted artifact admitted after restart")
	}
}
