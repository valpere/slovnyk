//go:build linux

package artifacts

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRefuseFIFOWithoutOpening(t *testing.T) {
	root := scratch(t)
	if e := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	done := make(chan error, 1)
	go func() { _, e := s.ReaderSize("pipe", 4096); done <- e }()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("FIFO admitted")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked opening FIFO")
	}
	// The digest path takes the same regular-file guard.
	if e = s.Check(context.Background(), Ref{Path: "pipe", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}); e == nil {
		t.Fatal("FIFO admitted by checksum")
	}
}
