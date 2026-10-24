package ops

import (
	"errors"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// Config contains only HTTP process settings. Storage is introduced separately.
type Config struct {
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
	return cfg, nil
}
