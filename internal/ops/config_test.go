package ops

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(func(string) string { return "" })
	if err != nil || cfg.HTTPAddr != "127.0.0.1:8080" || cfg.LogLevel != slog.LevelInfo || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
	values := map[string]string{"SLOVNYK_HTTP_ADDR": "[::1]:9090", "SLOVNYK_LOG_LEVEL": "debug", "SLOVNYK_SHUTDOWN_TIMEOUT": "500ms"}
	cfg, err = LoadConfig(func(key string) string { return values[key] })
	if err != nil || cfg.HTTPAddr != "[::1]:9090" || cfg.LogLevel != slog.LevelDebug || cfg.ShutdownTimeout != 500*time.Millisecond {
		t.Fatalf("overrides: %+v, %v", cfg, err)
	}
}

func TestConfigErrorsNeverContainValues(t *testing.T) {
	for key, values := range map[string][]string{
		"SLOVNYK_HTTP_ADDR":        {"postgres://user:synthetic-secret@host/db", "localhost:nope", "localhost:65536", "localhost:-1"},
		"SLOVNYK_LOG_LEVEL":        {"synthetic-secret"},
		"SLOVNYK_SHUTDOWN_TIMEOUT": {"synthetic-secret", "0s", "-1s"},
	} {
		for _, value := range values {
			t.Run(key+"/"+value, func(t *testing.T) {
				_, err := LoadConfig(func(name string) string {
					if name == key {
						return value
					}
					return ""
				})
				if err == nil || !strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "synthetic-secret") {
					t.Fatalf("expected safe configuration error, got %v", err)
				}
			})
		}
	}
}
