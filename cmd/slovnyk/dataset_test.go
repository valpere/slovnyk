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
