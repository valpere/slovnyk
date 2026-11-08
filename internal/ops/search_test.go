package ops

import (
	"context"
	"net"
	"strings"
	"sync"
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
	timing := &searchTiming{}
	ctx := context.WithValue(context.Background(), searchTimingKey{}, timing)
	_, err = backend(ctx, lookup.Request{From: "uk-ua", To: "en-us", Q: "x"})
	elapsed := time.Since(start)
	listener.Close()
	if c := <-accepted; c != nil {
		c.Close()
	}
	if timing.failureStage != "connect" || timing.searchResult != "deadline" || timing.connectMS < 1700 || timing.lookupMS != 0 {
		t.Fatalf("incorrect connection timing: %+v", timing)
	}
	if err != lookup.ErrUnavailable || elapsed < 1800*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("connection deadline: %v elapsed %s", err, elapsed)
	}
}

// With all eight slots held in handshakes, the ninth request must spend its
// budget in admission without attempting another connection.
func TestSearchTimingAdmissionDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 8; i++ {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			accepted <- c
		}
	}()
	signer, err := lookup.NewSigner("test", map[string][]byte{"test": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	backend := searchBackend(Config{HTTPDSN: "postgres://synthetic:synthetic@" + listener.Addr().String() + "/synthetic?sslmode=disable", CursorSigner: signer})
	holders, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	defer func() {
		cancel()
		listener.Close()
		wg.Wait()
		<-done
		close(accepted)
		for c := range accepted {
			c.Close()
		}
	}()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = backend(holders, lookup.Request{}) }()
	}
	var connections []net.Conn
	defer func() {
		for _, c := range connections {
			c.Close()
		}
	}()
	for i := 0; i < 8; i++ {
		select {
		case c := <-accepted:
			connections = append(connections, c)
		case <-time.After(time.Second):
			t.Fatal("slots not occupied")
		}
	}
	timing := &searchTiming{}
	parent, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	_, err = backend(context.WithValue(parent, searchTimingKey{}, timing), lookup.Request{Q: "private*"})
	if err != lookup.ErrUnavailable || timing.failureStage != "admission" || timing.searchResult != "deadline" || timing.admissionMS < 40 || timing.connectMS != 0 || timing.lookupMS != 0 || timing.queryKind != "mask" {
		t.Fatalf("incorrect admission result: %+v %v", timing, err)
	}
}
