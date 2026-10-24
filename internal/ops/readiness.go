package ops

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/publication"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

type readiness struct {
	ready atomic.Bool
	done  chan struct{}
}

// Retained data/files are immutable and must be mounted read-only to HTTP.
// Verify all references on connection recovery and each new generation. Cheap
// pointer probes run every second; HTTP requests never perform disk scans.
func startReadiness(ctx context.Context, cfg Config) *readiness {
	state := &readiness{done: make(chan struct{})}
	go func() {
		defer close(state.done)
		if cfg.HTTPDSN == "" {
			return
		}
		var c *pgx.Conn
		var cached publication.Receipt
		closeDB := func() {
			if c != nil {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
				_ = c.Close(cleanup)
				cancel()
				c = nil
			}
			cached = publication.Receipt{}
		}
		defer closeDB()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			if ctx.Err() != nil {
				state.ready.Store(false)
				return
			}
			if c == nil {
				var e error
				c, e = postgres.Connect(ctx, cfg.HTTPDSN)
				if e != nil {
					state.ready.Store(false)
				}
			}
			if c != nil {
				var current publication.Receipt
				probe, cancel := context.WithTimeout(ctx, 2*time.Second)
				e := c.QueryRow(probe, "SELECT coalesce(dataset_id::text,''),generation FROM slovnyk.active_dataset WHERE singleton=1").Scan(&current.Dataset, &current.Generation)
				cancel()
				if e != nil {
					state.ready.Store(false)
					closeDB()
				} else if current.Dataset == "" {
					state.ready.Store(false)
				} else if current != cached {
					state.ready.Store(false)
					checked, err := publication.CheckActive(ctx, c, cfg.ArtifactRoot)
					if err != nil {
						closeDB()
					} else {
						cached = checked
						state.ready.Store(true)
					}
				} else {
					state.ready.Store(true)
				}
			}
			select {
			case <-ctx.Done():
				state.ready.Store(false)
				return
			case <-tick.C:
			}
		}
	}()
	return state
}
