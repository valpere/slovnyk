package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramRequiresAPIAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"ok":true}`, true}, {`{"ok":false}`, false}, {`bad`, false}} {
		client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.telegram.org" || r.URL.Path != "/bot123:synthetic/sendMessage" {
				t.Fatal("unexpected endpoint")
			}
			if err := r.ParseForm(); err != nil || r.Form.Get("chat_id") != "456" || r.Form.Get("text") != "safe status" {
				t.Fatal("bad payload")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})}
		if got := sendTelegram(context.Background(), client, "123:synthetic", "456", "safe status"); (got == nil) != tc.want {
			t.Fatalf("got %v", got)
		}
	}
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("SECRET_TOKEN") })}
	if err := sendTelegram(context.Background(), client, "123:synthetic", "456", "safe status"); err == nil || strings.Contains(err.Error(), "SECRET_TOKEN") {
		t.Fatal("unsafe error")
	}
}

func TestAlertTransitionsAndDeliveryRetry(t *testing.T) {
	path := filepath.Join(scratch(t), "state.json")
	now := time.Now()
	calls := 0
	send := func(context.Context, string) error { calls++; return nil }
	if err := notify(context.Background(), path, true, now, send); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("healthy startup should not send")
	}
	for _, tc := range []struct {
		passed bool
		after  time.Duration
		want   int
	}{{false, time.Minute, 1}, {false, 2 * time.Minute, 1}, {false, 7 * time.Hour, 2}, {true, 8 * time.Hour, 3}} {
		if err := notify(context.Background(), path, tc.passed, now.Add(tc.after), send); err != nil {
			t.Fatal(err)
		}
		if calls != tc.want {
			t.Fatalf("calls %d want %d", calls, tc.want)
		}
	}
	fail := func(context.Context, string) error { return errors.New("delivery_failed") }
	if err := notify(context.Background(), path, false, now.Add(9*time.Hour), fail); err == nil {
		t.Fatal("delivery failure hidden")
	}
	if err := notify(context.Background(), path, false, now.Add(9*time.Hour), send); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatal("unacknowledged alert was not retried")
	}
}

func TestInvalidTelegramConfigOnHealthyCheck(t *testing.T) {
	for _, tc := range [][2]string{{"", "1"}, {"123:token", ""}, {"123:token", "0"}, {"REPLACE_TOKEN", "1"}} {
		err := notifyTelegram(context.Background(), filepath.Join(scratch(t), "state.json"), true, time.Now(), tc[0], tc[1], func(context.Context, string) error {
			t.Fatal("invalid config attempted delivery")
			return nil
		})
		if err == nil || err.Error() != "telegram_config_invalid" {
			t.Fatalf("got %v", err)
		}
	}
}

func scratch(t *testing.T) string {
	t.Helper()
	const root = "../../tmp/utilities-tests"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(root, "deployment-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })
	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitialFailureAndPrivateState(t *testing.T) {
	path := filepath.Join(scratch(t), "state.json")
	calls := 0
	send := func(context.Context, string) error { calls++; return nil }
	if err := notify(context.Background(), path, false, time.Now(), send); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("initial failure was not reported")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("state is not private")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := notify(context.Background(), path, true, time.Now(), send); err == nil || err.Error() != "alert_state_invalid" {
		t.Fatalf("invalid state silently accepted: %v", err)
	}
	if calls != 1 {
		t.Fatal("corrupt state sent misleading recovery")
	}
}
