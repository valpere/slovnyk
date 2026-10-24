package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInvalidCommandDoesNotLogArguments(t *testing.T) {
	var output bytes.Buffer
	code := run(context.Background(), []string{"synthetic-secret"}, func(string) string { return "" }, &output)
	assertSafeEvent(t, code, output.String(), "invalid_command")
}

func TestInvalidConfigDoesNotLogDSN(t *testing.T) {
	var output bytes.Buffer
	code := run(context.Background(), []string{"serve"}, func(key string) string {
		if key == "SLOVNYK_HTTP_ADDR" {
			return "postgres://user:synthetic-secret@host/database"
		}
		return ""
	}, &output)
	assertSafeEvent(t, code, output.String(), "invalid_config")
}

func assertSafeEvent(t *testing.T, code int, output, event string) {
	t.Helper()
	if code != 2 || strings.Contains(output, "synthetic-secret") || strings.Contains(output, "postgres://") {
		t.Fatalf("unsafe error or wrong exit code: %d %s", code, output)
	}
	var log map[string]any
	if err := json.Unmarshal([]byte(output), &log); err != nil {
		t.Fatal(err)
	}
	if log["event"] != event {
		t.Fatalf("unexpected event: %v", log)
	}
}

func TestMigrateConfigErrorsDoNotLogSecrets(t *testing.T) {
	for _, dsn := range []string{"", "postgres://operator:synthetic-migration-secret@host:invalid/db"} {
		var output bytes.Buffer
		code := run(context.Background(), []string{"migrate"}, func(key string) string {
			if key == "SLOVNYK_MIGRATION_DSN" {
				return dsn
			}
			return ""
		}, &output)
		if code != 2 || strings.Contains(output.String(), "synthetic-migration-secret") {
			t.Fatalf("invalid migration config: exit=%d", code)
		}
	}
}
