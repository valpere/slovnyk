package ops

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/valpere/slovnyk/internal/artifacts"
	"github.com/valpere/slovnyk/internal/catalog"
	"github.com/valpere/slovnyk/internal/downloads"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/httpui"
	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func searchBackend(cfg Config) httpui.SearchFunc { return httpBackends(cfg, nil).search }

type backends struct {
	search    httpui.SearchFunc
	catalog   httpui.CatalogFunc
	downloads httpui.DownloadsFunc
	export    httpui.ExportFunc
	entry     httpui.EntryFunc
}

// Search, catalog, dictionary pages and download metadata share eight slots. Admission,
// connection establishment and SQL honor the caller's outer two-second budget.
func httpBackends(cfg Config, ready func() bool) backends {
	service, err := lookup.New(cfg.CursorSigner)
	if err != nil || cfg.HTTPDSN == "" {
		return backends{}
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
	list := func(parent context.Context) (downloads.Manifest, error) {
		if ready == nil || !ready() {
			return downloads.Manifest{}, lookup.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		c, release, err := borrow(ctx)
		if err != nil {
			return downloads.Manifest{}, err
		}
		defer release()
		return downloads.List(ctx, c)
	}
	open := func(parent context.Context, dataset, id string) (downloads.Export, *os.File, error) {
		if ready == nil || !ready() {
			return downloads.Export{}, nil, lookup.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		c, release, err := borrow(ctx)
		if err != nil {
			return downloads.Export{}, nil, err
		}
		e, err := downloads.Resolve(ctx, c, dataset, id)
		release() // Streaming must not hold a database connection slot.
		if err != nil {
			return downloads.Export{}, nil, err
		}
		store, err := artifacts.Open(cfg.ArtifactRoot)
		if err != nil {
			return downloads.Export{}, nil, lookup.ErrUnavailable
		}
		defer store.Close()
		f, err := store.Reader(e.Artifact)
		if err != nil {
			return downloads.Export{}, nil, lookup.ErrUnavailable
		}
		return e, f, nil
	}
	entry := func(parent context.Context, id, cursor string) (entries.Page, error) {
		if ready == nil || !ready() {
			return entries.Page{}, lookup.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(parent, lookup.DBTimeout)
		defer cancel()
		c, release, err := borrow(ctx)
		if err != nil {
			return entries.Page{}, err
		}
		defer release()
		return entries.Read(ctx, c, cfg.CursorSigner, id, cursor)
	}
	return backends{search: search, catalog: read, downloads: list, export: open, entry: entry}
}
