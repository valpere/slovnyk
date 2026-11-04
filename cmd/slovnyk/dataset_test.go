package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatasetArgumentsAndSecrets(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	for _, args := range [][]string{
		{"dataset"}, {"dataset", "publish", id, "--artifacts", "tmp", "--operator-ref", "me"},
		{"dataset", "rollback", id, "--artifacts", "tmp", "--expected-generation", "0"},
		{"dataset", "export", id, "--artifacts", "tmp", "--source", "sensitive-source"},
		{"dataset", "verify", id, "--artifacts", "tmp", "--source", "sensitive-source"},
		{"dataset", "publish", "sensitive-invalid-id"},
		{"dataset", "sitemap", id, "--artifacts", "tmp", "--source", "sensitive-source"},
		{"dataset", "sitemap", id, "--artifacts", "tmp", "--expected-generation", "0"},
		{"dataset", "sitemap", id, "--artifacts", "tmp"},
		{"dataset", "reconcile", id},
		{"dataset", "reconcile", id, "--source", "sensitive-source"},
		{"dataset", "reconcile", id, "--source", "sensitive-source", "--artifacts", "tmp"},
		{"dataset", "reconcile", id, "--source", "sensitive-source", "--expected-generation", "0"},
		{"dataset", "reconcile", id, "--source", "sensitive-source", "--operator-ref", "me"},
	} {
		var out bytes.Buffer
		code := run(context.Background(), args, func(key string) string {
			if key == "SLOVNYK_OPERATOR_DSN" {
				return "invalid-secret-dsn"
			}
			return ""
		}, &out)
		if code == 0 || strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "sensitive") {
			t.Fatalf("%v: %d %s", args, code, out.String())
		}
	}
}

func TestReconcileRejectsInvalidFlagsBeforeConnecting(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	for _, flags := range [][]string{
		nil,
		{"--source", ""},
		{"--source", "snapshot", "--artifacts", ""},
		{"--source", "snapshot", "--expected-generation", "0"},
		{"--source", "snapshot", "--operator-ref", "owner"},
		{"--source", "snapshot", "extra"},
		{"--source", "snapshot", "--loss-approval", "sensitive-approval"},
	} {
		args := append([]string{"reconcile", id}, flags...)
		var out bytes.Buffer
		readConfig := false
		code := runDataset(context.Background(), args, func(string) string {
			readConfig = true
			return ""
		}, &out)
		if code != 2 || readConfig || !strings.Contains(out.String(), "invalid_dataset_arguments") {
			t.Fatalf("invalid flags reached connection setup: %v: %d %s", args, code, out.String())
		}
	}
}

func TestApprovalFileErrorsBeforeConnecting(t *testing.T) {
	if e := os.MkdirAll("../../tmp", 0700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp("../../tmp", "approval-cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "invalid.json")
	if e = os.WriteFile(file, []byte(`{"synthetic-secret":"not an approval"}`), 0600); e != nil {
		t.Fatal(e)
	}
	for _, file := range []string{file, filepath.Join(dir, "sensitive-absent.json"), ""} {
		args := []string{"verify", "00000000-0000-0000-0000-000000000001", "--source", "snapshot", "--artifacts", "artifacts", "--loss-approval", file}
		var out bytes.Buffer
		readConfig := false
		code := runDataset(context.Background(), args, func(string) string { readConfig = true; return "" }, &out)
		if code != 2 || readConfig || strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "sensitive") {
			t.Fatalf("unsafe approval error: %d %s", code, out.String())
		}
	}
}
