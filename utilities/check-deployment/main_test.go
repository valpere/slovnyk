package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ready     int
		free      uint64
		diskError bool
		pass      bool
	}{
		{"ready and enough disk", 200, 30, false, true},
		{"health alone insufficient", 503, 30, false, false},
		{"disk reserve", 200, 29, false, false},
		{"disk failure", 200, 30, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/readyz" {
					w.WriteHeader(tc.ready)
				}
			}))
			defer s.Close()
			r := check(context.Background(), s.URL, "unused", 30, func(string) (uint64, error) {
				if tc.diskError {
					return 0, errors.New("private path")
				}
				return tc.free, nil
			})
			if r.Passed != tc.pass {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestRejectRemoteAndRedirect(t *testing.T) {
	for _, base := range []string{"http://example.org", "http://user:secret@127.0.0.1", "http://127.0.0.1/search?q=private"} {
		if check(context.Background(), base, "", 1, freeBytes).Passed {
			t.Fatal("accepted unsafe base")
		}
	}
	requests := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; http.Redirect(w, r, "/unexpected", 302) }))
	defer s.Close()
	r := check(context.Background(), s.URL, "", 1, func(string) (uint64, error) { return 2, nil })
	if r.Passed || requests != 2 {
		t.Fatalf("redirect followed or passed: %+v, requests %d", r, requests)
	}
}
