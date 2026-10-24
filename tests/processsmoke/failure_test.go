//go:build smoke

package processsmoke

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SMOKE_TEST_HELPER") == "1" && len(os.Args) > 1 && os.Args[1] == "serve" {
		_ = os.WriteFile(os.Getenv("SMOKE_TEST_PID"), []byte(strconv.Itoa(os.Getpid())), 0600)
		// Never emit inherited values, even when a test detects an environment leak.
		for _, e := range os.Environ() {
			if strings.HasPrefix(e, "SLOVNYK_HTTP_DSN=") {
				fmt.Fprintln(os.Stderr, `{"event":"unexpected_storage"}`)
				time.Sleep(time.Minute)
				os.Exit(1)
			}
		}
		fmt.Fprintln(os.Stderr, `{"event":"http_started","address":"192.0.2.1:8080"}`)
		time.Sleep(time.Minute)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
func TestRejectExternalAddressAndReapChild(t *testing.T) {
	if err := os.MkdirAll("../../tmp/utilities-tests", 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("../../tmp/utilities-tests", "smoke-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(root, "pid")
	t.Setenv("SMOKE_TEST_HELPER", "1")
	t.Setenv("SMOKE_TEST_PID", pidFile)
	t.Setenv("SLOVNYK_HTTP_DSN", "synthetic-do-not-use")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	_, err = probe(root, binary, syscall.SIGTERM)
	if err == nil || err.Error() != "invalid_loopback_address" {
		t.Fatal(err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("child not reaped: %v", err)
	}
}
