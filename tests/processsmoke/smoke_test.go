//go:build smoke

// Process smoke tests probe only their own loopback HTTP process and shutdown.
package processsmoke

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type logBuffer struct {
	mu    sync.Mutex
	body  []byte
	first chan []byte
	sent  bool
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.body)+len(p) > 64<<10 {
		return 0, errors.New("lifecycle logs too large")
	}
	b.body = append(b.body, p...)
	if !b.sent {
		if i := bytes.IndexByte(b.body, '\n'); i >= 0 {
			b.first <- append([]byte(nil), b.body[:i]...)
			b.sent = true
		}
	}
	return len(p), nil
}
func (b *logBuffer) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.body...)
}

type httpCheck struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
}
type result struct {
	Signal             string      `json:"signal"`
	ExitCode           int         `json:"exit_code"`
	HTTP               []httpCheck `json:"http"`
	Events             []string    `json:"events"`
	QueryTextNotLogged bool        `json:"query_text_not_logged"`
}
type report struct {
	Passed  bool     `json:"passed"`
	Results []result `json:"results"`
	Error   string   `json:"error,omitempty"`
}

func probe(root, binary string, signal syscall.Signal) (result, error) {
	r := result{Signal: map[syscall.Signal]string{syscall.SIGTERM: "SIGTERM", syscall.SIGINT: "SIGINT"}[signal]}
	logs := &logBuffer{first: make(chan []byte, 1)}
	cmd := exec.Command(binary, "serve")
	cmd.Dir = root
	cmd.WaitDelay = time.Second
	// The smoke must never inherit a local or production storage configuration.
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "SLOVNYK_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "SLOVNYK_HTTP_ADDR=127.0.0.1:0", "SLOVNYK_LOG_LEVEL=info", "SLOVNYK_SHUTDOWN_TIMEOUT=2s")
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		return r, errors.New("process_start_failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	var first []byte
	select {
	case first = <-logs.first:
	case <-done:
		waited = true
		return r, errors.New("process_exited_before_startup")
	case <-time.After(5 * time.Second):
		return r, errors.New("startup_timeout")
	}
	var startup struct {
		Event   string `json:"event"`
		Address string `json:"address"`
	}
	if json.Unmarshal(first, &startup) != nil || startup.Event != "http_started" {
		return r, errors.New("invalid_startup_event")
	}
	host, port, err := net.SplitHostPort(startup.Address)
	n, e := strconv.Atoi(port)
	if err != nil || e != nil || host != "127.0.0.1" || n < 1 || n > 65535 {
		return r, errors.New("invalid_loopback_address")
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/healthz?q=synthetic-secret", 200}, {"HEAD", "/healthz", 200}, {"GET", "/readyz", 503}} {
		req, err := http.NewRequest(tc.method, "http://"+startup.Address+tc.path, nil)
		if err != nil {
			return r, errors.New("request_failed")
		}
		response, err := client.Do(req)
		if err != nil {
			return r, errors.New("request_failed")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || len(body) > 64<<10 || response.StatusCode != tc.status || tc.method == "HEAD" && len(body) != 0 {
			return r, errors.New("unexpected_http_response")
		}
		if tc.path == "/readyz" {
			var readiness struct {
				Reason string `json:"reason"`
			}
			if json.Unmarshal(body, &readiness) != nil || readiness.Reason != "storage_not_initialized" {
				return r, errors.New("unexpected_readiness_reason")
			}
		}
		r.HTTP = append(r.HTTP, httpCheck{tc.method, strings.SplitN(tc.path, "?", 2)[0], response.StatusCode})
	}
	if err = cmd.Process.Signal(signal); err != nil {
		return r, errors.New("signal_failed")
	}
	select {
	case err = <-done:
		waited = true
		if err != nil {
			return r, errors.New("shutdown_failed")
		}
	case <-time.After(5 * time.Second):
		return r, errors.New("shutdown_timeout")
	}
	raw := logs.snapshot()
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var event struct {
			Event string `json:"event"`
		}
		if json.Unmarshal(line, &event) != nil {
			return r, errors.New("invalid_lifecycle_log")
		}
		r.Events = append(r.Events, event.Event)
	}
	if strings.Join(r.Events, ",") != "http_started,http_draining,http_stopped" {
		return r, errors.New("unexpected_lifecycle_events")
	}
	if bytes.Contains(raw, []byte("synthetic-secret")) || bytes.Contains(raw, []byte("q=")) {
		return r, errors.New("query_text_logged")
	}
	r.QueryTextNotLogged = true
	return r, nil
}
func TestProcessSmoke(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	r := report{Results: []result{}}
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		item, e := probe(root, filepath.Join(root, "bin/slovnyk"), signal)
		if e != nil {
			r.Error = e.Error()
			break
		}
		r.Results = append(r.Results, item)
	}
	r.Passed = r.Error == ""
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "tmp", "tests")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "http-smoke.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
	if !r.Passed {
		t.Fatal("process smoke failed")
	}
}
