package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparePreservesFilesAndRejectsFileRoot(t *testing.T) {
	if e := os.MkdirAll("../../tmp/utilities-tests", 0700); e != nil {
		t.Fatal(e)
	}
	base, e := os.MkdirTemp("../../tmp/utilities-tests", "artifacts-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	root := filepath.Join(base, "nested", "artifacts")
	absolute, e := prepare(root)
	if e != nil || !filepath.IsAbs(absolute) {
		t.Fatal(absolute, e)
	}
	file := filepath.Join(root, "keep")
	if e = os.WriteFile(file, []byte("unchanged"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = prepare(root); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(file); e != nil || string(b) != "unchanged" {
		t.Fatal(string(b), e)
	}
	if _, e = prepare(file); e == nil {
		t.Fatal("accepted file as directory")
	}
}
