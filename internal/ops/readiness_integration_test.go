//go:build integration

package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/downloads"
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
	backend := httpBackends(Config{HTTPDSN: httpDSN, CursorSigner: signer, ArtifactRoot: root}, func() bool { return true })
	search, readCatalog := backend.search, backend.catalog
	handler := httpui.New(search, signer, httpui.WithCatalog(readCatalog), httpui.WithDownloads(backend.downloads, backend.export))
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest("GET", "/search?from=uk-ua&to=en-us&q="+url.QueryEscape("слово")+"&limit=1&format=plain", nil))
	if first.Code != 200 || !strings.Contains(first.Body.String(), "More translations: /search?") {
		t.Fatal("HTTP search failed", first.Code, first.Body.String())
	}
	_, continuation, ok := strings.Cut(first.Body.String(), "More translations: ")
	if !ok {
		t.Fatal("missing continuation")
	}
	next, e := url.Parse(strings.TrimSpace(continuation))
	if e != nil {
		t.Fatal(e)
	}
	params := next.Query()
	params.Set("format", "html")
	params.Set("ui", "uk")
	next.RawQuery = params.Encode()
	secondPage := httptest.NewRecorder()
	handler.ServeHTTP(secondPage, httptest.NewRequest("GET", next.String(), nil))
	if secondPage.Header().Get("Content-Language") != "uk" || secondPage.Code != 200 || !strings.Contains(secondPage.Body.String(), "word") || strings.Contains(secondPage.Body.String(), `rel="next"`) {
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

	// Inventory reads use the same restricted HTTP role. The lookup pin must
	// remain usable without consulting active_dataset during a publication lock.
	inventory, err := readCatalog(ctx, report.Dataset)
	if err != nil || inventory.Dataset != report.Dataset || !inventory.Has("uk-ua", "en-us") {
		t.Fatal("published catalog unavailable to HTTP role", err)
	}
	languages := httptest.NewRecorder()
	handler.ServeHTTP(languages, httptest.NewRequest("GET", "/languages?ui=uk", nil))
	if languages.Code != 200 || !strings.Contains(languages.Body.String(), "uk-ua") || languages.Header().Get("Content-Language") != "uk" {
		t.Fatal("localized catalog failed", languages.Code)
	}
	if len(languages.Result().Cookies()) != 1 || languages.Result().Cookies()[0].Name != "slovnyk_ui" {
		t.Fatal("manual UI preference was not persisted")
	}
	form := httptest.NewRecorder()
	browser := httptest.NewRequest("GET", "/", nil)
	browser.Header.Set("Accept-Language", "uk-UA")
	handler.ServeHTTP(form, browser)
	if form.Code != 200 || !strings.Contains(form.Body.String(), `value="uk-ua" selected`) || !strings.Contains(form.Body.String(), `value="en-us" selected`) || len(form.Result().Cookies()) != 0 {
		t.Fatal("browser pair proposal failed", form.Code)
	}
	lock, err := connect().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err = lock.Exec(ctx, "LOCK TABLE slovnyk.active_dataset IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err = readCatalog(ctx, report.Dataset); err != nil {
		t.Fatal("pinned inventory consulted active pointer", err)
	}
	blocked, stopBlocked := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = readCatalog(blocked, "")
	stopBlocked()
	if !errors.Is(err, lookup.ErrUnavailable) {
		t.Fatal("initial inventory did not honor deadline", err)
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = readCatalog(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, lookup.ErrUnavailable) {
		t.Fatal("unknown dataset exposed", err)
	}

	manifest, err := backend.downloads(ctx)
	if err != nil || manifest.Dataset != report.Dataset || len(manifest.Exports) != len(report.Pairs) {
		t.Fatal("download manifest under HTTP role", err)
	}
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest("GET", "/downloads?ui=uk", nil))
	if index.Code != 200 || !strings.Contains(index.Body.String(), report.Dataset) {
		t.Fatal("download index", index.Code)
	}
	for _, p := range report.Pairs {
		fileURL := "/downloads/" + report.Dataset + "/" + p.ExportID
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", fileURL, nil))
		sum := sha256.Sum256(response.Body.Bytes())
		if response.Code != 200 || hex.EncodeToString(sum[:]) != p.Artifact.SHA256 || int64(response.Body.Len()) != p.Artifact.Bytes {
			t.Fatal("download bytes/hash mismatch", response.Code)
		}
		parser, err := importdata.NewGzipReader(bytes.NewReader(response.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		var rows int64
		for {
			record, err := parser.Next()
			if err == io.EOF {
				break
			}
			if err != nil || record.Reason != "" {
				t.Fatal("invalid downloaded CSV", err)
			}
			rows++
		}
		if err := parser.Close(); err != nil {
			t.Fatal(err)
		}
		if rows != p.Rows {
			t.Fatal("download logical row count", rows, p.Rows)
		}
		partial := httptest.NewRecorder()
		req := httptest.NewRequest("GET", fileURL, nil)
		req.Header.Set("Range", "bytes=0-7")
		handler.ServeHTTP(partial, req)
		if partial.Code != 206 || !bytes.Equal(partial.Body.Bytes(), response.Body.Bytes()[:8]) {
			t.Fatal("download resume failed", partial.Code)
		}
		head := httptest.NewRecorder()
		handler.ServeHTTP(head, httptest.NewRequest("HEAD", fileURL, nil))
		if head.Code != 200 || head.Body.Len() != 0 {
			t.Fatal("HEAD download", head.Code)
		}
	}
	// Ready alone is insufficient: an unpublished version must not be exposed.
	m.Source.Authority = "download-unpublished-integration"
	unpublished, err := importer.Run(ctx, connect(), m, "../../testdata/import", importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := publication.Verify(ctx, connect(), unpublished.Dataset, "../../testdata/import", root, publication.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = downloads.Resolve(ctx, connect(), pending.Dataset, pending.Pairs[0].ExportID); !errors.Is(err, downloads.ErrNotFound) {
		t.Fatal("unpublished export exposed", err)
	}
	// Publish B, then fetch retained A at exactly the same URL and bytes.
	if _, err = publication.Publish(ctx, connect(), pending.Dataset, root, n+1, "download test", false); err != nil {
		t.Fatal(err)
	}
	retained := httptest.NewRecorder()
	handler.ServeHTTP(retained, httptest.NewRequest("GET", "/downloads/"+report.Dataset+"/"+report.Pairs[0].ExportID, nil))
	if retained.Code != 200 {
		t.Fatal("retained download lost", retained.Code)
	}
	sum := sha256.Sum256(retained.Body.Bytes())
	if hex.EncodeToString(sum[:]) != report.Pairs[0].Artifact.SHA256 {
		t.Fatal("retained download changed")
	}
	if _, err = publication.Publish(ctx, connect(), report.Dataset, root, n+2, "download test rollback", true); err != nil {
		t.Fatal(err)
	}
	// Availability and filesystem confinement are separate from DB identity.
	blockedBackend := httpBackends(Config{HTTPDSN: httpDSN, CursorSigner: signer, ArtifactRoot: root}, func() bool { return false })
	if _, err = blockedBackend.downloads(ctx); !errors.Is(err, lookup.ErrUnavailable) {
		t.Fatal("download bypassed readiness", err)
	}
	p := report.Pairs[0]
	artifactFile := filepath.Join(root, p.Artifact.Path)
	if err = os.Rename(artifactFile, artifactFile+"-missing"); err != nil {
		t.Fatal(err)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest("GET", "/downloads/"+report.Dataset+"/"+p.ExportID, nil))
	if missing.Code != 503 {
		t.Fatal("missing file presented as success", missing.Code)
	}
	if err = os.Symlink(artifactFile+"-missing", artifactFile); err != nil {
		t.Fatal(err)
	}
	linked := httptest.NewRecorder()
	handler.ServeHTTP(linked, httptest.NewRequest("GET", "/downloads/"+report.Dataset+"/"+p.ExportID, nil))
	if linked.Code != 503 {
		t.Fatal("symlink export exposed", linked.Code)
	}
	if err = os.Remove(artifactFile); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(artifactFile+"-missing", artifactFile); err != nil {
		t.Fatal(err)
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
