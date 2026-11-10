package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type alertState struct {
	Passed   bool      `json:"passed"`
	LastSent time.Time `json:"last_sent"`
}

func needsAlert(previous *alertState, passed bool, now time.Time) bool {
	if previous == nil {
		return !passed
	}
	return previous.Passed != passed || (!passed && now.Sub(previous.LastSent) >= 6*time.Hour)
}

var telegramToken = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

func validateTelegram(token, chat string) error {
	chatID, err := strconv.ParseInt(chat, 10, 64)
	if !telegramToken.MatchString(token) || err != nil || chatID == 0 {
		return errors.New("telegram_config_invalid")
	}
	return nil
}

func notifyTelegram(ctx context.Context, path string, passed bool, now time.Time, token, chat string, send func(context.Context, string) error) error {
	// Validate even when a healthy check does not send a message.
	if err := validateTelegram(token, chat); err != nil {
		return err
	}
	return notify(ctx, path, passed, now, send)
}

func sendTelegram(ctx context.Context, client *http.Client, token, chat, text string) error {
	bad := errors.New("telegram_delivery_failed")
	if err := validateTelegram(token, chat); err != nil {
		return err
	}
	body := url.Values{"chat_id": {chat}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", strings.NewReader(body.Encode()))
	if err != nil {
		return bad
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return bad
	} // net errors include the URL/token; never return them.
	defer response.Body.Close()
	var result struct {
		OK bool `json:"ok"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result) != nil || !result.OK {
		return bad
	}
	return nil
}

func notify(ctx context.Context, path string, passed bool, now time.Time, send func(context.Context, string) error) error {
	if !filepath.IsAbs(path) {
		return errors.New("alert_state_path_invalid")
	}
	var previous *alertState
	b, err := os.ReadFile(path)
	if err == nil {
		previous = &alertState{}
		if json.Unmarshal(b, previous) != nil {
			return errors.New("alert_state_invalid")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("alert_state_unreadable")
	}
	if !needsAlert(previous, passed, now) && previous != nil {
		return nil
	}
	next := alertState{Passed: passed}
	if needsAlert(previous, passed, now) {
		text := "Slovnyk: readiness and disk checks recovered."
		if !passed {
			text = "Slovnyk ALERT: readiness or free disk check failed. Inspect slovnyk-check.service on the server."
		}
		if err := send(ctx, text); err != nil {
			return err
		}
		next.LastSent = now
	}
	b, _ = json.Marshal(next)
	f, err := os.CreateTemp(filepath.Dir(path), ".alert-state-*")
	if err != nil {
		return errors.New("alert_state_write_failed")
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(b)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil || os.Rename(f.Name(), path) != nil {
		return errors.New("alert_state_write_failed")
	}
	return nil
}
