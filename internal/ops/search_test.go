package ops

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/valpere/slovnyk/internal/lookup"
)

func TestSearchCanceledBeforeConnection(t *testing.T) {
	signer, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	backend := searchBackend(Config{HTTPDSN: "postgres://127.0.0.1:1/unreachable?sslmode=disable", CursorSigner: signer})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err = backend(ctx, lookup.Request{From: "uk-ua", To: "en-us", Q: "x"})
	if err != lookup.ErrUnavailable || time.Since(started) > time.Second {
		t.Fatal("canceled acquisition did not fail safely", err)
	}
	if searchBackend(Config{}) != nil {
		t.Fatal("unconfigured backend must not connect")
	}
}

// A TCP peer that never completes the PostgreSQL handshake proves the HTTP
// budget also bounds connection establishment (the storage default is 5 s).
func TestSearchConnectionSharesTwoSecondBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, e := listener.Accept()
		if e == nil {
			accepted <- c
		}
		close(accepted)
	}()
	signer, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	backend := searchBackend(Config{HTTPDSN: "postgres://synthetic:synthetic@" + listener.Addr().String() + "/synthetic?sslmode=disable", CursorSigner: signer})
	start := time.Now()
	_, err = backend(context.Background(), lookup.Request{From: "uk-ua", To: "en-us", Q: "x"})
	elapsed := time.Since(start)
	listener.Close()
	if c := <-accepted; c != nil {
		c.Close()
	}
	if err != lookup.ErrUnavailable || elapsed < 1800*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("connection deadline: %v elapsed %s", err, elapsed)
	}
}
