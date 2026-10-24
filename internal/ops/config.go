package ops

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/valpere/slovnyk/internal/lookup"
)

// Config separates read-only HTTP storage from operator and migration credentials.
type Config struct {
	CursorSigner    *lookup.Signer
	HTTPDSN         string
	ArtifactRoot    string
	HTTPAddr        string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// LoadConfig never includes environment values in errors.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{HTTPAddr: "127.0.0.1:8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second}
	if value := getenv("SLOVNYK_HTTP_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, errors.New("invalid SLOVNYK_HTTP_ADDR: expected host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return Config{}, errors.New("invalid SLOVNYK_HTTP_ADDR: expected numeric port 0..65535")
	}
	if value := getenv("SLOVNYK_LOG_LEVEL"); value != "" {
		switch value {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return Config{}, errors.New("invalid SLOVNYK_LOG_LEVEL: expected debug, info, warn or error")
		}
	}
	if value := getenv("SLOVNYK_SHUTDOWN_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return Config{}, errors.New("invalid SLOVNYK_SHUTDOWN_TIMEOUT: expected positive duration")
		}
		cfg.ShutdownTimeout = duration
	}
	cfg.HTTPDSN = getenv("SLOVNYK_HTTP_DSN")
	cfg.ArtifactRoot = getenv("SLOVNYK_ARTIFACT_ROOT")
	if (cfg.HTTPDSN == "") != (cfg.ArtifactRoot == "") {
		return Config{}, errors.New("SLOVNYK_HTTP_DSN and SLOVNYK_ARTIFACT_ROOT must be set together")
	}
	keysText, active := getenv("SLOVNYK_CURSOR_KEYS"), getenv("SLOVNYK_CURSOR_ACTIVE_KID")
	if keysText != "" || active != "" || cfg.HTTPDSN != "" {
		safeError := errors.New("invalid SLOVNYK_CURSOR_KEYS or SLOVNYK_CURSOR_ACTIVE_KID: configure a bounded keyring and active key")
		if len(keysText) > 4096 {
			return Config{}, safeError
		}
		keys := map[string][]byte{}
		for _, item := range strings.Split(keysText, ",") {
			kid, encoded, ok := strings.Cut(item, ":")
			key, err := hex.DecodeString(encoded)
			if !ok || err != nil || keys[kid] != nil {
				return Config{}, safeError
			}
			keys[kid] = key
		}
		signer, err := lookup.NewSigner(active, keys)
		if err != nil {
			return Config{}, safeError
		}
		cfg.CursorSigner = signer
	}
	return cfg, nil
}
