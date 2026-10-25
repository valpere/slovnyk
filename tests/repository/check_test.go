package repository

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func scratch(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll("../../tmp/utilities-tests", 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("../../tmp/utilities-tests", "repository-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestMarkdownLinksAndEncoding(t *testing.T) {
	root := scratch(t)
	if err := os.WriteFile(filepath.Join(root, "has space.md"), []byte("# target\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "README.md")
	for _, tc := range []struct {
		body string
		want string
	}{
		{"[ok](has%20space.md#heading) [ok](<has space.md>) [web](https://example.com)\n", ""},
		{"[bad](missing.md)\n", "missing link"}, {"bad \n", "trailing whitespace"}, {string([]byte{255}), "Invalid UTF-8"},
	} {
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(markdown(path, "README.md"), "\n")
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Fatalf("%q: %s", tc.body, got)
		}
	}
}
func TestIgnoreChecksIncludeTrackedSourcesAndNewlines(t *testing.T) {
	root := scratch(t)
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git: %s %v", b, e)
		}
	}
	run("init", "-q")
	for _, dir := range []string{"data/experiments", "utilities/tool"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"README.md", "data/README.md", "data/experiments/a\nb", "utilities/tool/main.go"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte("ok\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "utilities/tool/main.go")
	writeIgnore := func(body string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeIgnore("/data/experiments/\n")
	if r := check(root, []string{"README.md"}); !r.Passed || r.IgnoredDataFiles != 0 || len(r.IgnoredDataRoots) != 1 {
		t.Fatalf("%+v", r)
	}
	if paths, err := ignored(root, []string{"data/experiments/a\nb"}); err != nil || !paths["data/experiments/a\nb"] {
		t.Fatalf("newline path lost: %v %v", paths, err)
	}
	writeIgnore("/utilities/\n")
	r := check(root, []string{"README.md"})
	errors := strings.Join(r.Errors, "\n")
	if r.Passed || !strings.Contains(errors, "Local data is not ignored") || !strings.Contains(errors, "Source ignored: utilities/tool/main.go") {
		t.Fatalf("%+v", r)
	}
}

func TestPrivateDataRootAndUnignoredChild(t *testing.T) {
	root := scratch(t)
	git := exec.Command("git", "init", "-q")
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", output, err)
	}
	for _, name := range []string{"README.md", "data/README.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("ok\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	private := filepath.Join(root, "data/experiments/.pgdata")
	if err := os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(private, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0700) })
	ignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(ignore, []byte("/data/experiments/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := check(root, []string{"README.md"})
	if !r.Passed || len(r.IgnoredDataRoots) != 1 || r.IgnoredDataRoots[0] != "data/experiments/" || r.IgnoredDataFiles != 0 {
		t.Fatalf("private data root was traversed: %+v", r)
	}
	// If the parent is not excluded, a child exception must still be inspected.
	if err := os.Chmod(private, 0700); err != nil {
		t.Fatal(err)
	}
	visible := filepath.Join(root, "data/experiments/visible")
	if err := os.Mkdir(visible, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(visible, "leak.csv"), []byte("synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte("/data/experiments/*\n!/data/experiments/visible/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r = check(root, []string{"README.md"})
	if r.Passed || len(r.IgnoredDataRoots) != 0 || !strings.Contains(strings.Join(r.Errors, "\n"), "Local data is not ignored: data/experiments/visible/leak.csv") {
		t.Fatalf("unignored child hidden: %+v", r)
	}
}

// TestRepository is a project check used directly by CI, independent of minions.
func TestRepository(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	r := check(root, nil)
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "tmp", "tests")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "repository-check.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
	if !r.Passed {
		t.Fatal("repository checks failed")
	}
}
