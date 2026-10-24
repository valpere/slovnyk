package ops

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/catalog"
	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func searchBackend(cfg Config) httpui.SearchFunc { search, _ := httpBackends(cfg); return search }

// Search and catalog share eight dedicated connection slots. Admission,
// connection establishment and SQL honor the caller's outer two-second budget.
func httpBackends(cfg Config) (httpui.SearchFunc, httpui.CatalogFunc) {
	service, err := lookup.New(cfg.CursorSigner)
	if err != nil || cfg.HTTPDSN == "" {
		return nil, nil
	}
	slots := make(chan struct{}, 8)
	borrow := func(ctx context.Context) (*pgx.Conn, func(), error) {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return nil, nil, lookup.ErrUnavailable
		}
		c, err := postgres.Connect(ctx, cfg.HTTPDSN)
		if err != nil {
			<-slots
			return nil, nil, lookup.ErrUnavailable
		}
		release := func() {
			defer func() { <-slots }()
			cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = c.Close(cleanup)
		}
		return c, release, nil
	}
	search := func(parent context.Context, r lookup.Request) (lookup.Page, error) {
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		c, release, err := borrow(ctx)
		if err != nil {
			return lookup.Page{}, err
		}
		defer release()
		return service.Search(ctx, c, r)
	}
	read := func(parent context.Context, dataset string) (catalog.Snapshot, error) {
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		c, release, err := borrow(ctx)
		if err != nil {
			return catalog.Snapshot{}, err
		}
		defer release()
		return catalog.Read(ctx, c, dataset)
	}
	return search, read
}
