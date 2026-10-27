package main

import (
	"bytes"
	"context"
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
