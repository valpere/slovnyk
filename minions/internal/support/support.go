// Package support provides repository discovery for assistant-only shortcuts.
package support

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Root() (string, error) {
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	_, source, _, _ := runtime.Caller(0)
	for _, start := range []string{cwd, filepath.Dir(exe), filepath.Dir(source)} {
		for p := start; p != ""; p = filepath.Dir(p) {
			if b, err := os.ReadFile(filepath.Join(p, "go.mod")); err == nil && strings.HasPrefix(string(b), "module github.com/valpere/slovnyk\n") {
				return p, nil
			}
			if filepath.Dir(p) == p {
				break
			}
		}
	}
	return "", errors.New("repository root not found")
}
