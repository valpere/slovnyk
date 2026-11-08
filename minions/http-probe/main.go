// http-probe runs the assistant's bounded loopback HTTP diagnostics.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"
)

type oracle struct {
	Status       int    `json:"status"`
	Bytes        int    `json:"bytes"`
	SHA256       string `json:"sha256"`
	ContentType  string `json:"content_type"`
	CacheControl string `json:"cache_control"`
}
type probeCase struct {
	ID       int     `json:"id"`
	Kind     string  `json:"kind"`
	Path     string  `json:"path"`
	Expected *oracle `json:"expected,omitempty"`
}
type config struct {
	Base           string      `json:"base"`
	Phase          string      `json:"phase"`
	Workers        int         `json:"workers"`
	IntervalMS     int         `json:"interval_ms"`
	TimeoutMS      int         `json:"timeout_ms"`
	MaximumSeconds int         `json:"maximum_seconds"`
	BodyLimit      int64       `json:"body_limit"`
	Cases          []probeCase `json:"cases"`
	Schedule       []int       `json:"schedule"`
}
type observation struct {
	Sequence int    `json:"sequence"`
	CaseID   int    `json:"case_id"`
	Kind     string `json:"kind"`
	Phase    string `json:"phase"`
	oracle
	OracleChecked         bool    `json:"oracle_checked"`
	OracleMatch           bool    `json:"oracle_match"`
	ErrorClass            string  `json:"error_class,omitempty"`
	ElapsedMS             float64 `json:"elapsed_ms"`
	DispatchLagMS         float64 `json:"dispatch_lag_ms"`
	ScheduledToCompleteMS float64 `json:"scheduled_to_complete_ms"`
}

var label = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

func (c config) validate() error {
	u, err := url.Parse(c.Base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("invalid loopback base")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		return errors.New("numeric loopback address and port required")
	}
	if !label.MatchString(c.Phase) || c.Workers < 1 || c.Workers > 16 || c.IntervalMS < 0 || (c.IntervalMS > 0 && c.IntervalMS < 50) || c.IntervalMS > 10000 || c.TimeoutMS < 1 || c.TimeoutMS > 10000 || c.MaximumSeconds < 1 || c.MaximumSeconds > 1800 || c.BodyLimit < 1 || c.BodyLimit > 8<<20 || len(c.Schedule) < 1 || len(c.Schedule) > 5000 || len(c.Cases) < 1 || len(c.Cases) > 1000 {
		return errors.New("invalid bounded workload")
	}
	if int64(len(c.Schedule)-1)*int64(c.IntervalMS) >= int64(c.MaximumSeconds)*1000 {
		return errors.New("schedule exceeds deadline")
	}
	ids := map[int]bool{}
	for _, v := range c.Cases {
		u, err := url.Parse(v.Path)
		if v.ID < 0 || ids[v.ID] || !label.MatchString(v.Kind) || err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || len(v.Path) > 16384 || len(v.Path) < 1 || v.Path[0] != '/' {
			return errors.New("invalid private case")
		}
		ids[v.ID] = true
		if v.Expected != nil {
			h, err := hex.DecodeString(v.Expected.SHA256)
			if err != nil || len(h) != 32 || v.Expected.Status < 100 || v.Expected.Status > 599 || v.Expected.Bytes < 0 || int64(v.Expected.Bytes) > c.BodyLimit {
				return errors.New("invalid response oracle")
			}
		}
	}
	for _, id := range c.Schedule {
		if !ids[id] {
			return errors.New("unknown scheduled case")
		}
	}
	return nil
}

func measure(ctx context.Context, client *http.Client, c config, v probeCase, seq int, scheduled time.Time) observation {
	started := time.Now()
	r := observation{Sequence: seq, CaseID: v.ID, Kind: v.Kind, Phase: c.Phase, OracleChecked: v.Expected != nil, DispatchLagMS: float64(started.Sub(scheduled)) / float64(time.Millisecond)}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutMS)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.Base+v.Path, nil)
	if err == nil {
		var response *http.Response
		response, err = client.Do(req)
		if err == nil {
			r.Status = response.StatusCode
			r.ContentType = response.Header.Get("Content-Type")
			r.CacheControl = response.Header.Get("Cache-Control")
			var body []byte
			body, err = io.ReadAll(io.LimitReader(response.Body, c.BodyLimit+1))
			response.Body.Close()
			r.Bytes = len(body)
			if err == nil && int64(len(body)) > c.BodyLimit {
				r.ErrorClass = "body_limit"
			} else if err == nil {
				h := sha256.Sum256(body)
				r.SHA256 = hex.EncodeToString(h[:])
				if v.Expected != nil {
					r.OracleMatch = r.oracle == *v.Expected
					if !r.OracleMatch {
						r.ErrorClass = "response_mismatch"
					}
				}
				if r.Status >= 400 && r.ErrorClass == "" {
					r.ErrorClass = fmt.Sprintf("http_%d", r.Status)
				}
			}
		}
	}
	if err != nil {
		r.ErrorClass = "transport"
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			r.ErrorClass = "client_timeout"
		}
		if errors.Is(err, context.Canceled) {
			r.ErrorClass = "cancelled"
		}
	}
	completed := time.Now()
	r.ElapsedMS = float64(completed.Sub(started)) / float64(time.Millisecond)
	r.ScheduledToCompleteMS = float64(completed.Sub(scheduled)) / float64(time.Millisecond)
	return r
}

func execute(ctx context.Context, c config, sink func(observation) error) error {
	if err := c.validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.MaximumSeconds)*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, DialContext: (&net.Dialer{Timeout: time.Duration(c.TimeoutMS) * time.Millisecond}).DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cases := map[int]probeCase{}
	for _, v := range c.Cases {
		cases[v.ID] = v
	}
	type job struct {
		seq, id int
		at      time.Time
	}
	jobs := make(chan job, len(c.Schedule))
	results := make(chan observation, c.Workers)
	var wg sync.WaitGroup
	for i := 0; i < c.Workers; i++ {
		wg.Go(func() {
			for j := range jobs {
				results <- measure(ctx, client, c, cases[j.id], j.seq, j.at)
			}
		})
	}
	start := time.Now()
	go func() {
		defer close(jobs)
		for i, id := range c.Schedule {
			at := start.Add(time.Duration(i) * time.Duration(c.IntervalMS) * time.Millisecond)
			if delay := time.Until(at); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return
				}
			}
			select {
			case jobs <- job{i, id, at}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	var sinkErr error
	count := 0
	for r := range results {
		count++
		if sinkErr == nil {
			sinkErr = sink(r)
			if sinkErr != nil {
				cancel()
			}
		}
	}
	if sinkErr != nil {
		return errors.New("observation output failed")
	}
	if ctx.Err() != nil || count != len(c.Schedule) {
		return errors.New("workload interrupted; observations incomplete")
	}
	return nil
}

func percentile(rows []observation, field func(observation) float64, p int) float64 {
	values := make([]float64, len(rows))
	for i, r := range rows {
		values[i] = field(r)
	}
	sort.Float64s(values)
	if len(values) == 0 {
		return 0
	}
	return values[(p*len(values)+99)/100-1]
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: http-probe PRIVATE_CONFIG_JSON NEW_OUTPUT_DIRECTORY")
	}
	f, err := os.Open(args[0])
	if err != nil {
		return errors.New("cannot open private config")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 2<<20 {
		return errors.New("private config must be a regular file up to 2 MiB with mode 0600 or stricter")
	}
	var c config
	decoder := json.NewDecoder(io.LimitReader(f, 2<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return errors.New("invalid private config")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("invalid trailing config")
	}
	if err = c.validate(); err != nil {
		return err
	}
	if err = os.Mkdir(args[1], 0700); err != nil {
		return errors.New("output directory must be new")
	}
	output, err := os.OpenFile(args[1]+"/observations.jsonl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot create observations")
	}
	defer output.Close()
	rows := []observation{}
	encoder := json.NewEncoder(output)
	started := time.Now()
	if err = execute(context.Background(), c, func(r observation) error { rows = append(rows, r); return encoder.Encode(r) }); err != nil {
		return err
	}
	if output.Sync() != nil {
		return errors.New("cannot sync observations")
	}
	summary := map[string]any{"complete": true, "phase": c.Phase, "requests": len(rows), "workers": c.Workers, "interval_ms": c.IntervalMS, "elapsed_seconds": time.Since(started).Seconds(), "client_timeout_ms": c.TimeoutMS, "body_limit": c.BodyLimit, "latencies": "client observed including bounded body read; no server timer", "percentile": "nearest rank, ceil(p*N); all outcomes retained", "connection_reuse": false, "groups": []any{}}
	kinds := map[string][]observation{}
	for _, r := range rows {
		kinds[r.Kind] = append(kinds[r.Kind], r)
	}
	names := []string{}
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	groups := []any{}
	for _, k := range names {
		g := kinds[k]
		statuses := map[int]int{}
		errors := map[string]int{}
		checked, matches := 0, 0
		for _, r := range g {
			statuses[r.Status]++
			if r.ErrorClass != "" {
				errors[r.ErrorClass]++
			}
			if r.OracleChecked {
				checked++
				if r.OracleMatch {
					matches++
				}
			}
		}
		groups = append(groups, map[string]any{"kind": k, "requests": len(g), "status_counts": statuses, "error_counts": errors, "oracle_checked": checked, "oracle_matches": matches, "client_p50_ms": percentile(g, func(r observation) float64 { return r.ElapsedMS }, 50), "client_p95_ms": percentile(g, func(r observation) float64 { return r.ElapsedMS }, 95), "scheduled_to_complete_p95_ms": percentile(g, func(r observation) float64 { return r.ScheduledToCompleteMS }, 95), "dispatch_lag_p95_ms": percentile(g, func(r observation) float64 { return r.DispatchLagMS }, 95)})
	}
	summary["groups"] = groups
	s, err := os.OpenFile(args[1]+"/summary.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot create summary")
	}
	defer s.Close()
	if json.NewEncoder(s).Encode(summary) != nil || s.Sync() != nil {
		return errors.New("cannot persist summary")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
