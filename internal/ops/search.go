package ops

import (
	"context"
	"time"

	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

// searchBackend bounds concurrent dedicated connections. Waiting for admission,
// connection establishment and every lookup query share one two-second budget.
func searchBackend(cfg Config) httpui.SearchFunc {
	service, err := lookup.New(cfg.CursorSigner)
	if err != nil || cfg.HTTPDSN == "" {
		return nil
	}
	slots := make(chan struct{}, 8)
	return func(parent context.Context, r lookup.Request) (lookup.Page, error) {
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return lookup.Page{}, lookup.ErrUnavailable
		}
		defer func() { <-slots }()
		conn, err := postgres.Connect(ctx, cfg.HTTPDSN)
		if err != nil {
			return lookup.Page{}, lookup.ErrUnavailable
		}
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			_ = conn.Close(cleanup)
		}()
		return service.Search(ctx, conn, r)
	}
}
