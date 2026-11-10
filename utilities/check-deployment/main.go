// check-deployment is an operator check, also usable from a systemd timer.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"
)

type result struct {
	Passed  bool     `json:"passed"`
	Health  int      `json:"health_status"`
	Ready   int      `json:"ready_status"`
	Free    uint64   `json:"free_bytes"`
	Minimum uint64   `json:"minimum_free_bytes"`
	Errors  []string `json:"errors,omitempty"`
}

func freeBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func check(ctx context.Context, base, data string, minimum uint64, disk func(string) (uint64, error)) result {
	r := result{Minimum: minimum}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		r.Errors = append(r.Errors, "invalid_loopback_base")
		return r
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, endpoint := range []struct {
		path   string
		status *int
	}{{"/healthz", &r.Health}, {"/readyz", &r.Ready}} {
		u.Path = endpoint.path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			r.Errors = append(r.Errors, "invalid_loopback_base")
			return r
		}
		response, err := client.Do(req)
		if err != nil {
			r.Errors = append(r.Errors, "probe_unavailable")
			continue
		}
		*endpoint.status = response.StatusCode
		_ = response.Body.Close() // Never log or retain the response body.
		if response.StatusCode != http.StatusOK {
			r.Errors = append(r.Errors, "probe_not_ok")
		}
	}
	r.Free, err = disk(data)
	if err != nil {
		r.Errors = append(r.Errors, "disk_unavailable")
	} else if r.Free < minimum {
		r.Errors = append(r.Errors, "disk_below_reserve")
	}
	r.Passed = len(r.Errors) == 0
	return r
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8080", "numeric loopback HTTP origin")
	data := flag.String("data", "/srv/slovnyk", "filesystem whose free space is checked")
	minimum := flag.Uint64("min-free-gib", 30, "minimum free GiB; alert only, never deletes files")
	telegram := flag.Bool("notify-telegram", false, "send state changes to configured Telegram chat")
	state := flag.String("state", "", "absolute private state file for Telegram deduplication")
	flag.Parse()
	if flag.NArg() != 0 || *minimum == 0 || *minimum > (1<<30) {
		fmt.Fprintln(os.Stderr, "invalid arguments")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	r := check(ctx, *base, *data, *minimum<<30, freeBytes)
	cancel()
	if *telegram {
		transport := &http.Transport{Proxy: nil}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		notifyCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
		token, chat := os.Getenv("SLOVNYK_TELEGRAM_TOKEN"), os.Getenv("SLOVNYK_TELEGRAM_CHAT_ID")
		err := notifyTelegram(notifyCtx, *state, r.Passed, time.Now(), token, chat, func(ctx context.Context, text string) error {
			return sendTelegram(ctx, client, token, chat, text)
		})
		stop()
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
			r.Passed = false
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
		os.Exit(1)
	}
	if !r.Passed {
		os.Exit(1)
	}
}
