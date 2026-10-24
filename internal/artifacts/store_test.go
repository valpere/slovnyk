package artifacts

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func scratch(t *testing.T) string {
	t.Helper()
	base := "../../tmp/m1-05"
	if e := os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	p, e := os.MkdirTemp(base, "store-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(p) })
	return p
}
func install(s *Store) (Ref, error) {
	return s.Install(context.Background(), "datasets/test/exports/one", ".csv.gz", func(w io.Writer) error { _, e := io.WriteString(w, "complete immutable bytes"); return e })
}
func TestNoClobberAndConfinement(t *testing.T) {
	root := scratch(t)
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a, e := install(s)
	if e != nil {
		t.Fatal(e)
	}
	before, e := os.Stat(filepath.Join(root, a.Path))
	if e != nil {
		t.Fatal(e)
	}
	b, e := install(s)
	if e != nil || a != b {
		t.Fatalf("repeat: %+v %v", b, e)
	}
	after, e := os.Stat(filepath.Join(root, a.Path))
	if e != nil || !os.SameFile(before, after) {
		t.Fatal("existing inode replaced", e)
	}
	if e = s.Check(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink("datasets", filepath.Join(root, "link")); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"../outside", "/absolute", "link/test/exports/one", "datasets/../escape"} {
		if _, e = s.Install(context.Background(), p, ".json", func(w io.Writer) error { return nil }); e == nil {
			t.Fatalf("admitted %q", p)
		}
	}
	if e = os.Chmod(filepath.Join(root, a.Path), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, a.Path), []byte(strings.Repeat("x", int(a.Bytes))), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = install(s); !errors.Is(e, ErrArtifact) {
		t.Fatalf("clobbered corrupt existing artifact: %v", e)
	}
}
func TestArtifactCrashProcess(t *testing.T) {
	point := os.Getenv("SLOVNYK_TEST_CRASH_POINT")
	if point == "" {
		return
	}
	s, e := Open(os.Getenv("SLOVNYK_TEST_CRASH_ROOT"))
	if e != nil {
		os.Exit(91)
	}
	s.fault = func(p string) error {
		if p == point {
			os.Exit(42)
		}
		return nil
	}
	_, _ = install(s)
	os.Exit(92)
}
func TestProcessCrashAndRetry(t *testing.T) {
	for _, point := range []string{"written", "file_synced", "linked", "directory_synced"} {
		t.Run(point, func(t *testing.T) {
			root := scratch(t)
			absolute, e := filepath.Abs(root)
			if e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestArtifactCrashProcess$")
			cmd.Env = append(os.Environ(), "SLOVNYK_TEST_CRASH_POINT="+point, "SLOVNYK_TEST_CRASH_ROOT="+absolute)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatalf("helper: %v", err)
			}
			finals, e := filepath.Glob(filepath.Join(root, "datasets/test/exports/one/*.csv.gz"))
			if e != nil {
				t.Fatal(e)
			}
			linked := point == "linked" || point == "directory_synced"
			if (len(finals) == 1) != linked {
				t.Fatalf("visibility after %s: %v", point, finals)
			}
			s, e := Open(root)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			ref, e := install(s)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.Check(context.Background(), ref); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestFailedWriteNeverInstalls(t *testing.T) {
	root := scratch(t)
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_, e = s.Install(context.Background(), "incomplete", ".json", func(w io.Writer) error { _, _ = io.WriteString(w, "partial"); return io.ErrUnexpectedEOF })
	if !errors.Is(e, io.ErrUnexpectedEOF) {
		t.Fatal(e)
	}
	files, e := os.ReadDir(filepath.Join(root, "incomplete"))
	if e != nil || len(files) != 0 {
		t.Fatalf("partial files: %v %v", files, e)
	}
}

func TestInstallationFaultsReturnFailure(t *testing.T) {
	for _, point := range []string{"written", "file_synced", "linked", "directory_synced"} {
		t.Run(point, func(t *testing.T) {
			s, e := Open(scratch(t))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			s.fault = func(p string) error {
				if p == point {
					return io.ErrClosedPipe
				}
				return nil
			}
			if _, e = install(s); !errors.Is(e, io.ErrClosedPipe) {
				t.Fatalf("failure swallowed at %s: %v", point, e)
			}
			s.fault = nil
			if _, e = install(s); e != nil {
				t.Fatal(e)
			}
		})
	}
}
