package ops

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthAndReadiness(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/healthz", 200}, {"GET", "/readyz", 503},
		{"HEAD", "/healthz", 200}, {"HEAD", "/readyz", 503},
		{"POST", "/healthz", 405}, {"GET", "/", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			newHandler().ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
			if r.Code != tc.code {
				t.Fatalf("status %d, want %d", r.Code, tc.code)
			}
			if tc.method == "GET" && (tc.code == 200 || tc.code == 503) {
				var body map[string]string
				if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if r.Header().Get("Cache-Control") != "no-store" || r.Header().Get("Content-Type") != "application/json" {
					t.Fatal("missing probe headers")
				}
				if tc.code == 503 && body["reason"] != "storage_not_initialized" {
					t.Fatalf("unexpected readiness reason: %v", body)
				}
			}
		})
	}
}

func TestShutdownDrainsInFlightRequest(t *testing.T) {
	testShutdown(t, false)
}

func TestShutdownDeadlineClosesConnections(t *testing.T) {
	testShutdown(t, true)
}

func testShutdown(t *testing.T, expire bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		if expire {
			<-r.Context().Done()
			return
		}
		<-release
		_, _ = io.WriteString(w, "completed")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	timeout := 2 * time.Second
	if expire {
		timeout = 30 * time.Millisecond
	}
	go func() {
		result <- serve(ctx, listener, server, timeout, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	clientResult := make(chan error, 1)
	client := &http.Client{Timeout: 3 * time.Second}
	go func() {
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			err = readErr
			if !expire && string(body) != "completed" {
				err = io.ErrUnexpectedEOF
			}
		}
		clientResult <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	if !expire {
		// The server must wait while this request is still blocked.
		select {
		case err := <-result:
			t.Fatalf("shutdown returned before request completion: %v", err)
		case <-time.After(30 * time.Millisecond):
		}
		release <- struct{}{}
	}
	select {
	case err := <-result:
		if expire && (err == nil || !strings.Contains(err.Error(), "deadline")) {
			t.Fatalf("expected shutdown timeout, got %v", err)
		}
		if !expire && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown hung")
	}
	select {
	case err := <-clientResult:
		if !expire && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request hung")
	}
}

func TestListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	err = Run(context.Background(), Config{HTTPAddr: listener.Addr().String()}, slog.Default())
	if err == nil || err.Error() != "HTTP listener unavailable" {
		t.Fatalf("expected safe listener error, got %v", err)
	}
}
