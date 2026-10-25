//go:build integration

// Package serviceflow exercises the built service over its CLI and loopback HTTP.
package serviceflow

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/importdata"
	"github.com/valpere/slovnyk/internal/importer"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/sitemaps"
)

func cleanEnv() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "SLOVNYK_") {
			env = append(env, value)
		}
	}
	return env
}

func TestServiceLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(root, "tmp", "serviceflow")
	if err = os.MkdirAll(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	binary := filepath.Join(dir, "slovnyk")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "./cmd/slovnyk")
	build.Dir, build.Env = root, cleanEnv()
	if _, err = build.CombinedOutput(); err != nil {
		t.Fatal("build failed", err)
	}

	// Only the disposable runner DSN is accepted; never fall back to app config.
	adminDSN := os.Getenv("SLOVNYK_TEST_DSN")
	u, err := url.Parse(adminDSN)
	if err != nil || u.Scheme != "postgresql" || u.Hostname() != "127.0.0.1" || u.Path != "/slovnyk_test" {
		t.Fatal("use make test-integration with its disposable loopback DB")
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	db, operator, reader := "slovnyk_flow_"+suffix, "flow_operator_"+suffix, "flow_http_"+suffix
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{db}.Sanitize()+" TEMPLATE template0 ENCODING 'UTF8'"); err != nil {
		t.Fatal("cannot create isolated database", err)
	}
	// Register cleanup immediately, including partial setup failures. Nothing from
	// the runner's existing database or its fixed group roles is removed.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, e := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{db}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error("isolated database cleanup failed")
		}
		for _, role := range []string{operator, reader} {
			if _, e := admin.Exec(cleanup, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize()); e != nil {
				t.Error("test login cleanup failed")
			}
		}
	})
	u.Path = "/" + db
	migrationDSN := u.String()
	cli := func(want int, env []string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir, cmd.Env = root, append(cleanEnv(), env...)
		out, e := cmd.CombinedOutput()
		code := 0
		if e != nil {
			if exit, ok := e.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal("CLI could not execute")
			}
		}
		if code != want {
			t.Fatalf("%s exit %d, want %d (diagnostics withheld)", args[0], code, want)
		}
		return out
	}
	cli(0, []string{"SLOVNYK_MIGRATION_DSN=" + migrationDSN}, "migrate")
	// Real LOGINs inherit only the service groups, rather than connecting as the
	// migration owner and relying on SET ROLE for the application process.
	for role, group := range map[string]string{operator: "slovnyk_operator", reader: "slovnyk_http"} {
		if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN PASSWORD 'synthetic-flow-only' IN ROLE "+pgx.Identifier{group}.Sanitize()); err != nil {
			t.Fatal("test login setup failed")
		}
	}
	roleDSN := func(role string) string {
		v := *u
		v.User = url.UserPassword(role, "synthetic-flow-only")
		return v.String()
	}
	operatorDSN, httpDSN := roleDSN(operator), roleDSN(reader)
	operatorEnv := []string{"SLOVNYK_OPERATOR_DSN=" + operatorDSN}
	probe, err := pgx.Connect(ctx, httpDSN)
	if err != nil {
		t.Fatal("HTTP login unavailable")
	}
	var current string
	var super bool
	err = probe.QueryRow(ctx, `SELECT current_user,rolsuper FROM pg_roles WHERE rolname=current_user`).Scan(&current, &super)
	if err != nil || current != reader || super {
		t.Fatal("HTTP is not a restricted login")
	}
	if _, err = probe.Exec(ctx, `UPDATE slovnyk.active_dataset SET generation=generation+1`); err == nil {
		t.Fatal("HTTP login can mutate active pointer")
	}
	_ = probe.Close(ctx)

	artifactRoot := filepath.Join(dir, "artifacts")
	if err = os.Mkdir(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := func() (string, func()) {
		t.Helper()
		log, e := os.CreateTemp(dir, "server-*.log")
		if e != nil {
			t.Fatal(e)
		}
		cmd := exec.CommandContext(ctx, binary, "serve")
		cmd.Dir = root
		cmd.Env = append(cleanEnv(), "SLOVNYK_HTTP_ADDR=127.0.0.1:0", "SLOVNYK_HTTP_DSN="+httpDSN, "SLOVNYK_ARTIFACT_ROOT="+artifactRoot, "SLOVNYK_CURSOR_ACTIVE_KID=flow", "SLOVNYK_CURSOR_KEYS=flow:"+strings.Repeat("78", 32), "SLOVNYK_SHUTDOWN_TIMEOUT=2s")
		cmd.Stderr = log
		if e = cmd.Start(); e != nil {
			_ = log.Close()
			t.Fatal("serve start failed")
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		stopped := false
		stop := func() {
			t.Helper()
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case e = <-done:
				if e != nil {
					t.Error("serve exit failed")
				}
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Error("serve shutdown timeout")
			}
			_ = log.Close()
			raw, e := os.ReadFile(log.Name())
			if e != nil {
				t.Error("cannot read process log")
				return
			}
			for _, secret := range []string{"flow-private-word", "synthetic-flow-only", "postgresql://", "q="} {
				if bytes.Contains(raw, []byte(secret)) {
					t.Error("private input in process log")
				}
			}
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			raw, _ := os.ReadFile(log.Name())
			line, _, _ := bytes.Cut(raw, []byte{'\n'})
			var event struct{ Event, Address string }
			if json.Unmarshal(line, &event) == nil && event.Event == "http_started" {
				host, port, e := net.SplitHostPort(event.Address)
				if e != nil || host != "127.0.0.1" || port == "0" {
					t.Fatal("non-loopback startup")
				}
				return "http://" + event.Address, stop
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("startup timeout")
		return "", stop
	}
	base, stop := start()
	get := func(path string, want int) ([]byte, http.Header) {
		t.Helper()
		res, e := client.Get(base + path)
		if e != nil {
			t.Fatal("HTTP request failed", e)
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
		closeErr := res.Body.Close()
		if e != nil || closeErr != nil || len(b) > 2<<20 || res.StatusCode != want {
			t.Fatalf("HTTP %s status %d, want %d", strings.SplitN(path, "?", 2)[0], res.StatusCode, want)
		}
		return b, res.Header
	}
	waitStatus := func(status int) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			res, e := client.Get(base + "/readyz")
			if e == nil {
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode == status {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("readiness did not reach %d", status)
	}
	waitReady := func() { t.Helper(); waitStatus(200) }
	get("/healthz", 200)
	get("/readyz", 503)
	get("/search?from=en-us&to=uk-ua&q=flow-private-word", 503)

	raw, err := os.ReadFile(filepath.Join(root, "testdata/import/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := importdata.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Locales = []string{"en-us", "uk-ua"}
	manifest.ExpectedPairs = []importdata.Pair{{From: "en-us", To: "uk-ua"}}
	sitemapReports := map[string]sitemaps.Result{}
	stage := func(name string) publication.Report {
		t.Helper()
		input := filepath.Join(dir, name)
		if e := os.Mkdir(input, 0700); e != nil {
			t.Fatal(e)
		}
		var corpus bytes.Buffer
		z := gzip.NewWriter(&corpus)
		w := csv.NewWriter(z)
		for i := range 53 {
			if e := w.Write([]string{"en-us", "uk-ua", "flow-private-word", fmt.Sprintf("%s translation %03d", name, i)}); e != nil {
				t.Fatal(e)
			}
		}
		w.Flush()
		if w.Error() != nil {
			t.Fatal(w.Error())
		}
		if e := z.Close(); e != nil {
			t.Fatal(e)
		}
		hash := sha256.Sum256(corpus.Bytes())
		m := manifest
		m.Source.Authority = "serviceflow-" + name
		m.Artifacts = []importdata.Artifact{{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(corpus.Len()), Name: "corpus.csv.gz", Format: "csv4.gz", MultiPair: true}}
		b, e := json.Marshal(m)
		if e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(input, "manifest.json")
		if e = os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(input, "corpus.csv.gz"), corpus.Bytes(), 0600); e != nil {
			t.Fatal(e)
		}
		var imported importer.Report
		if e = json.Unmarshal(cli(0, operatorEnv, "import", "run", "--manifest", path, "--source", input, "--batch-size", "7"), &imported); e != nil || imported.Dataset == "" || imported.Error != "" {
			t.Fatal("invalid import report")
		}
		var verified publication.Report
		if e = json.Unmarshal(cli(0, operatorEnv, "dataset", "verify", imported.Dataset, "--source", input, "--artifacts", artifactRoot), &verified); e != nil || verified.Unique != 53 || verified.Rejected != 0 || len(verified.Pairs) != 1 {
			t.Fatal("invalid verification report")
		}
		var sitemap sitemaps.Result
		if e = json.Unmarshal(cli(0, operatorEnv, "dataset", "sitemap", imported.Dataset, "--artifacts", artifactRoot), &sitemap); e != nil || sitemap.URLs != 1 || len(sitemap.Shards) != 1 {
			t.Fatal("sitemap CLI inventory", e)
		}
		sitemapReports[verified.Dataset] = sitemap

		return verified
	}
	publish := func(command string, r publication.Report, generation int64, want int) {
		t.Helper()
		out := cli(want, operatorEnv, "dataset", command, r.Dataset, "--artifacts", artifactRoot, "--expected-generation", strconv.FormatInt(generation, 10), "--operator-ref", "serviceflow")
		if want == 0 {
			var receipt publication.Receipt
			if json.Unmarshal(out, &receipt) != nil || receipt.Dataset != r.Dataset || receipt.Generation != generation+1 {
				t.Fatal("invalid publication receipt")
			}
		} else if !bytes.Contains(out, []byte("publication_generation_conflict")) {
			t.Fatal("wrong CAS failure")
		}
	}
	checkSitemap := func(dataset string) {
		t.Helper()
		inventory := sitemapReports[dataset]
		waitStatus(200)
		index, headers := get("/sitemap.xml", 200)
		sum := sha256.Sum256(index)
		if hex.EncodeToString(sum[:]) != inventory.Index.SHA256 || headers.Get("Cache-Control") != "no-store" {
			t.Fatal("active sitemap index")
		}
		shard := inventory.Shards[0]
		body, headers := get("/sitemap-"+dataset+"-"+shard.Artifact.SHA256+".xml", 200)
		sum = sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != shard.Artifact.SHA256 || !strings.Contains(string(body), "https://slovnyk.org/word/") || !strings.Contains(headers.Get("Cache-Control"), "immutable") {
			t.Fatal("pinned sitemap shard")
		}
	}
	a := stage("A")
	get("/sitemap-"+a.Dataset+"-"+sitemapReports[a.Dataset].Shards[0].Artifact.SHA256+".xml", 503)
	publish("publish", a, 0, 0)
	checkSitemap(a.Dataset)
	waitReady()
	search := "/search?from=en-us&to=uk-ua&q=flow-private-word&limit=1&format=plain&ui=en"
	first, _ := get(search, 200)
	continuation := func(body []byte) string {
		t.Helper()
		_, next, ok := strings.Cut(string(body), "More translations: ")
		if !ok {
			t.Fatal("missing continuation")
		}
		return strings.TrimSpace(next)
	}
	nextA := continuation(first)
	if !bytes.Contains(first, []byte("A translation 000")) {
		t.Fatal("wrong initial search result")
	}
	mask, _ := get("/search?from=en-us&to=uk-ua&q=flow-private-*&format=plain&ui=en", 200)
	if !bytes.Contains(mask, []byte("A translation 000")) {
		t.Fatal("mask did not match")
	}
	entry := entries.Path(entries.ID("en-us", "uk-ua", "flow-private-word"))
	article, headers := get(entry, 200)
	if headers.Get("X-Robots-Tag") != "" || !bytes.Contains(article, []byte(`rel="canonical"`)) {
		t.Fatal("dictionary article not indexable")
	}
	marker := `class="next" href="`
	_, link, ok := strings.Cut(string(article), marker)
	if !ok {
		t.Fatal("entry continuation missing")
	}
	entryNext, _, _ := strings.Cut(link, `"`)
	entryTail, _ := get(entryNext, 200)
	if !bytes.Contains(entryTail, []byte("A translation 052")) {
		t.Fatal("entry tail missing")
	}
	fileURL := "/downloads/" + a.Dataset + "/" + a.Pairs[0].ExportID
	download := func(path string, pair publication.PairReport) []byte {
		t.Helper()
		b, _ := get(path, 200)
		hash := sha256.Sum256(b)
		if hex.EncodeToString(hash[:]) != pair.Artifact.SHA256 || int64(len(b)) != pair.Artifact.Bytes {
			t.Fatal("download bytes differ from verification")
		}
		z, e := gzip.NewReader(bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		rows, e := csv.NewReader(z).ReadAll()
		closeErr := z.Close()
		if e != nil || closeErr != nil || len(rows) != 53 || rows[0][3] != "A translation 000" {
			t.Fatal("download CSV content mismatch")
		}
		return b
	}
	originalFile := download(fileURL, a.Pairs[0])
	listing, _ := get("/downloads", 200)
	if !bytes.Contains(listing, []byte(fileURL)) {
		t.Fatal("manifest does not link verified export")
	}
	get("/languages", 200)
	get(search+"&cursor=invalid", 400)
	b := stage("B")
	get("/sitemap-"+b.Dataset+"-"+sitemapReports[b.Dataset].Shards[0].Artifact.SHA256+".xml", 404)
	publish("publish", b, 0, 1)
	publish("publish", b, 1, 0)
	checkSitemap(b.Dataset)
	get("/sitemap-"+a.Dataset+"-"+sitemapReports[a.Dataset].Shards[0].Artifact.SHA256+".xml", 200)
	waitReady()
	active, _ := get(search, 200)
	if !bytes.Contains(active, []byte("B translation 000")) {
		t.Fatal("search did not follow B")
	}
	activeArticle, _ := get(entry, 200)
	if !bytes.Contains(activeArticle, []byte("B translation 000")) || bytes.Contains(activeArticle, []byte("A translation")) {
		t.Fatal("stable article did not follow B")
	}
	retained, _ := get(nextA, 200)
	if !bytes.Contains(retained, []byte("A translation 001")) || bytes.Contains(retained, []byte("B translation")) {
		t.Fatal("search continuation lost A")
	}
	retainedEntry, h := get(entryNext, 200)
	if !bytes.Equal(retainedEntry, entryTail) || h.Get("X-Robots-Tag") != "noindex, follow" {
		t.Fatal("entry continuation lost A")
	}
	if !bytes.Equal(download(fileURL, a.Pairs[0]), originalFile) {
		t.Fatal("retained download changed")
	}
	publish("rollback", a, 2, 0)
	checkSitemap(a.Dataset)
	waitReady()
	restored, _ := get(search, 200)
	if !bytes.Equal(first, restored) {
		t.Fatal("rollback did not restore A")
	}
	restoredArticle, _ := get(entry, 200)
	if !bytes.Equal(article, restoredArticle) {
		t.Fatal("rollback did not restore dictionary article")
	}
	// Refuse connections only to this test's database and terminate its existing
	// sessions. The control connection stays in the disposable runner database.
	if _, err = admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{db}.Sanitize()+" ALLOW_CONNECTIONS false"); err != nil {
		t.Fatal("cannot inject isolated database outage")
	}
	if _, err = admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, db); err != nil {
		t.Fatal("cannot close isolated database sessions")
	}
	waitStatus(503)
	get("/healthz", 200)
	get(search, 503)
	get("/sitemap.xml", 503)
	if _, err = admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{db}.Sanitize()+" ALLOW_CONNECTIONS true"); err != nil {
		t.Fatal("cannot restore isolated database connections")
	}
	waitReady()
	recovered, _ := get(search, 200)
	if !bytes.Equal(first, recovered) {
		t.Fatal("database recovery changed published results")
	}
	stop()
	base, stop = start()
	waitReady()
	checkSitemap(a.Dataset)
	restarted, _ := get(nextA, 200)
	if !bytes.Equal(retained, restarted) {
		t.Fatal("restart lost cursor/keyring or retained data")
	}
	download(fileURL, a.Pairs[0])
	stop()
	// Freeze all fixture writers, dump the DB and copy immutable files, then
	// recover into a fresh DB/root. Never reuse the original DB for restored HTTP.
	restoredDB, restoredRoot := restoreFixture(t, ctx, admin, migrationDSN, artifactRoot, dir)
	originalDSN, originalRoot := httpDSN, artifactRoot
	restoreURL, err := url.Parse(httpDSN)
	if err != nil {
		t.Fatal("invalid fixture HTTP configuration")
	}
	restoreURL.Path = "/" + restoredDB
	httpDSN = restoreURL.String()
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{db}.Sanitize()+" ALLOW_CONNECTIONS false"); err != nil {
		t.Fatal("cannot isolate original database during restore acceptance")
	}
	if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, db); err != nil {
		t.Fatal("cannot close original database sessions")
	}
	// A DB-only restore must not become ready without the referenced artifacts.
	artifactRoot = filepath.Join(dir, "missing-restored-artifacts")
	if err := os.Mkdir(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	base, stop = start()
	time.Sleep(1200 * time.Millisecond)
	get("/healthz", 200)
	get("/readyz", 503)
	get("/sitemap.xml", 503)
	get(fileURL, 503)
	stop()
	artifactRoot = restoredRoot
	base, stop = start()
	waitReady()
	checkSitemap(a.Dataset)
	for path, want := range map[string][]byte{
		search: first, nextA: retained, entry: article, entryNext: entryTail,
		"/search?from=en-us&to=uk-ua&q=flow-private-*&format=plain&ui=en": mask,
	} {
		body, _ := get(path, 200)
		if !bytes.Equal(body, want) {
			t.Fatal("restored HTTP response differs", strings.SplitN(path, "?", 2)[0])
		}
	}
	if !bytes.Equal(download(fileURL, a.Pairs[0]), originalFile) {
		t.Fatal("restored download differs")
	}
	retainedB, _ := get("/downloads/"+b.Dataset+"/"+b.Pairs[0].ExportID, 200)
	retainedBHash := sha256.Sum256(retainedB)
	if hex.EncodeToString(retainedBHash[:]) != b.Pairs[0].Artifact.SHA256 {
		t.Fatal("restored retained B download differs")
	}
	get("/sitemap-"+b.Dataset+"-"+sitemapReports[b.Dataset].Shards[0].Artifact.SHA256+".xml", 200)
	probe, err = pgx.Connect(ctx, httpDSN)
	if err != nil {
		t.Fatal("restored HTTP login unavailable")
	}
	if _, err := probe.Exec(ctx, `UPDATE slovnyk.active_dataset SET generation=generation+1`); err == nil {
		t.Fatal("restored HTTP login can mutate active pointer")
	}
	_ = probe.Close(ctx)
	restoreURL, err = url.Parse(operatorDSN)
	if err != nil {
		t.Fatal("invalid fixture operator configuration")
	}
	restoreURL.Path = "/" + restoredDB
	var repeated sitemaps.Result
	if err := json.Unmarshal(cli(0, []string{"SLOVNYK_OPERATOR_DSN=" + restoreURL.String()}, "dataset", "sitemap", a.Dataset, "--artifacts", restoredRoot), &repeated); err != nil || repeated.Report != sitemapReports[a.Dataset].Report {
		t.Fatal("restored operator cannot retry registered sitemap")
	}
	stop()
	// Subsequent corruption injection still uses only the original test fixture.
	httpDSN, artifactRoot = originalDSN, originalRoot
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{db}.Sanitize()+" ALLOW_CONNECTIONS true"); err != nil {
		t.Fatal("cannot restore original fixture connections")
	}
	// A same-size corrupt registered shard must block readiness on restart.
	shardPath := filepath.Join(artifactRoot, sitemapReports[a.Dataset].Shards[0].Artifact.Path)
	corrupt, err := os.ReadFile(shardPath)
	if err != nil {
		t.Fatal(err)
	}
	corrupt[len(corrupt)-1] ^= 1
	if err := os.Chmod(shardPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shardPath, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	base, stop = start()
	get("/healthz", 200)
	time.Sleep(1200 * time.Millisecond)
	get("/readyz", 503)
	get("/sitemap.xml", 503)
	stop()
	t.Log("PASS: empty DB → CLI migrate/import/verify/publish A → real HTTP exact/mask/word/download/sitemap → stale CAS rejection → publish B → retained A → rollback A → DB outage/recovery → process restart → local backup/restore to fresh DB/root with original DB unavailable; restricted LOGINs and private query logs checked")
}
