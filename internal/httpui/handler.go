package httpui

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/valpere/slovnyk/internal/catalog"
	"github.com/valpere/slovnyk/internal/downloads"
	"github.com/valpere/slovnyk/internal/entries"
	"github.com/valpere/slovnyk/internal/i18n"
	"github.com/valpere/slovnyk/internal/lookup"
)

type SearchFunc func(context.Context, lookup.Request) (lookup.Page, error)

type CatalogFunc func(context.Context, string) (catalog.Snapshot, error)
type Option func(*options)
type options struct {
	catalog   CatalogFunc
	downloads DownloadsFunc
	export    ExportFunc
	entry     EntryFunc
	sitemap   SitemapFunc
}

func WithCatalog(read CatalogFunc) Option { return func(o *options) { o.catalog = read } }

// New serves search, language inventory, and localized no-JavaScript forms.
func New(search SearchFunc, signer *lookup.Signer, opts ...Option) http.Handler {
	config := options{}
	for _, apply := range opts {
		apply(&config)
	}
	mux := http.NewServeMux()
	serve := func(w http.ResponseWriter, r *http.Request) {
		parsed, status := parse(r, signer)
		lang, manual := i18n.Select(parsed.UI, cookie(r, uiCookie), r.Header.Get("Accept-Language"))
		data := view{Request: parsed.Search, Lang: lang, Text: i18n.Messages(lang), IsLanguages: r.URL.Path == "/languages", IsDownloads: r.URL.Path == "/downloads", UIAction: r.URL.Path}
		for _, c := range i18n.Choices() {
			data.UILocales = append(data.UILocales, choice{Code: c.Code, Label: c.Name, Selected: c.Code == lang})
		}
		var changes []*http.Cookie
		// One outer deadline includes search, catalog and both connection acquisitions.
		ctx, cancel := context.WithTimeout(r.Context(), lookup.DBTimeout)
		defer cancel()
		dataset := ""
		if status != 200 {
			data.Request = lookup.Request{}
			data.Message = data.Text["invalid"]
		} else {
			if manual {
				changes = append(changes, preference(r, uiCookie, lang))
			}
			if data.IsDownloads {
				status = downloadView(ctx, config.downloads, &data)
			}
			if !parsed.Empty && !data.IsLanguages && !data.IsDownloads {
				if search == nil {
					status = 503
					data.Message = data.Text["unavailable"]
				} else {
					page, err := search(ctx, parsed.Search)
					status = lookup.Status(err)
					if err != nil {
						data.Message = data.Text[messageKey(err)]
					} else {
						dataset = page.Dataset
						data.Groups = page.Groups
						if len(page.Groups) == 0 {
							data.Message = data.Text["no_matches"]
						}
						if page.HasMore {
							data.Next = nextURL(parsed, page.Cursor)
						}
					}
				}
			}
			if status == 200 && !data.IsDownloads && config.catalog != nil {
				inventory, err := config.catalog(ctx, dataset)
				if err != nil {
					data.Notice = data.Text["catalog_unavailable"]
					if data.IsLanguages {
						status = 503
					}
				} else {
					data.HasCatalog = true
					if parsed.Empty && !data.IsLanguages && !parsed.FromSet && !parsed.ToSet {
						from, to, saved := savedPair(r)
						if saved {
							if inventory.Has(from, to) {
								data.Request.From, data.Request.To = from, to
							} else {
								data.Notice = data.Text["pair_unavailable"]
							}
						} else {
							data.Request.From, data.Request.To = propose(inventory, r.Header.Get("Accept-Language"))
						}
					}
					if parsed.FromSet && parsed.ToSet && inventory.Has(parsed.Search.From, parsed.Search.To) {
						changes = append(changes, preference(r, pairCookie, encodePair(parsed.Search.From, parsed.Search.To)))
					}
					data.FromChoices = languageChoices(inventory.Locales, data.Request.From)
					data.ToChoices = languageChoices(inventory.Locales, data.Request.To)
					if data.IsLanguages {
						for _, p := range inventory.Pairs {
							q := url.Values{"from": {p.From}, "to": {p.To}}
							data.Pairs = append(data.Pairs, pairView{From: i18n.Native(p.From), To: i18n.Native(p.To), Count: p.Count, URL: "/?" + q.Encode()})
						}
					}
				}
			} else if status == 200 && !data.IsDownloads {
				data.Notice = data.Text["catalog_unavailable"]
				if data.IsLanguages {
					status = 503
				}
			}
		}
		data.SourceLang, data.TargetLang = contentLang(data.Request.From), contentLang(data.Request.To)
		// Language switching preserves only the parsed request, including its cursor.
		// No hidden ui on search: automatic browser selection must not become a
		// persisted manual choice merely because the visitor submits a search.
		for _, p := range []field{{"from", data.Request.From}, {"to", data.Request.To}, {"q", data.Request.Q}, {"cursor", parsed.Search.Cursor}} {
			if p.Value != "" {
				data.UIFields = append(data.UIFields, p)
			}
		}
		if data.Request.Mode != "" {
			data.UIFields = append(data.UIFields, field{"mode", string(data.Request.Mode)})
		}
		if data.Request.Limit != 0 {
			data.UIFields = append(data.UIFields, field{"limit", strconv.Itoa(data.Request.Limit)})
		}
		body := &boundedBuffer{}
		var err error
		contentType := "text/html; charset=utf-8"
		if parsed.Plain {
			contentType = "text/plain; charset=utf-8"
			err = plain(body, data)
		} else {
			err = pageTemplate.Execute(body, data)
		}
		if err != nil {
			status = 503
			contentType = "text/plain; charset=utf-8"
			body = &boundedBuffer{}
			_, _ = io.WriteString(body, data.Text["render_error"]+"\n")
			changes = nil
		}
		for _, c := range changes {
			http.SetCookie(w, c)
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
		w.Header().Set("Content-Language", lang)
		w.Header().Set("Vary", "Accept-Language, Cookie")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Robots-Tag", "noindex, follow")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.WriteHeader(status)
		if r.Method != "HEAD" {
			_, _ = w.Write(body.Bytes())
		}
	}
	mux.HandleFunc("GET /{$}", serve)
	mux.HandleFunc("GET /search", serve)
	mux.HandleFunc("GET /languages", serve)
	mux.HandleFunc("GET /downloads", serve)
	mux.HandleFunc("GET /downloads/{dataset}/{export}", exportHandler(config.export))
	mux.HandleFunc("GET /word/{id}", entryHandler(config.entry))
	mux.HandleFunc("GET /robots.txt", robotsHandler)
	sitemap := sitemapHandler(config.sitemap)
	mux.HandleFunc("GET /sitemap.xml", sitemap)
	mux.HandleFunc("GET /{sitemap}", sitemap)
	return LimitTarget(primaryDomain(mux))
}

type choice struct {
	Code, Label string
	Selected    bool
}
type field struct{ Name, Value string }
type pairView struct {
	From, To, URL string
	Count         int64
}
type view struct {
	Request                               lookup.Request
	Groups                                []lookup.Group
	Message, Notice, Next, Lang, UIAction string
	Text                                  map[string]string
	HasCatalog, IsLanguages, IsDownloads  bool
	Downloads                             downloads.Manifest
	IsEntry                               bool
	Entry                                 entries.Page
	Title, Description, Canonical         string
	SourceLang, TargetLang                string
	FromChoices, ToChoices, UILocales     []choice
	Pairs                                 []pairView
	UIFields                              []field
}

func languageChoices(codes []string, selected string) []choice {
	out := []choice{}
	found := false
	for _, code := range codes {
		out = append(out, choice{Code: code, Label: i18n.Native(code), Selected: code == selected})
		if code == selected {
			found = true
		}
	}
	if selected != "" && !found {
		out = append(out, choice{Code: selected, Label: selected, Selected: true})
	}
	return out
}

// The writer refuses excess bytes before allocating or committing any headers.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > lookup.MaxResponseBytes-b.Len() {
		return 0, errors.New("response_budget_exceeded")
	}
	return b.Buffer.Write(p)
}
func (b *boundedBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

func plain(w io.Writer, v view) error {
	if v.IsDownloads && v.Downloads.Dataset != "" {
		if err := plainDownloads(w, v.Downloads); err != nil {
			return err
		}
	}
	if v.Notice != "" {
		if _, e := io.WriteString(w, v.Notice+"\n"); e != nil {
			return e
		}
	}
	for _, p := range v.Pairs {
		if v.IsLanguages {
			if _, e := io.WriteString(w, p.From+" → "+p.To+"\t"+strconv.FormatInt(p.Count, 10)+"\n"); e != nil {
				return e
			}
		}
	}

	if v.Message != "" {
		if _, e := io.WriteString(w, v.Message+"\n"); e != nil {
			return e
		}
	}
	for _, g := range v.Groups {
		if _, e := io.WriteString(w, g.Source+"\n"); e != nil {
			return e
		}
		for _, t := range g.Translations {
			if _, e := io.WriteString(w, "\t"+t+"\n"); e != nil {
				return e
			}
		}
	}
	if v.Next != "" {
		_, e := io.WriteString(w, v.Text["next"]+": "+v.Next+"\n")
		return e
	}
	return nil
}
func messageKey(err error) string {
	switch {
	case errors.Is(err, lookup.ErrCursor):
		return "cursor"
	case errors.Is(err, lookup.ErrGone):
		return "gone"
	case errors.Is(err, lookup.ErrLocale):
		return "locale"
	case errors.Is(err, lookup.ErrPair):
		return "pair"
	case errors.Is(err, lookup.ErrInput):
		return "invalid"
	default:
		return "unavailable"
	}
}

//go:embed search.html
var pageHTML string
var pageTemplate = template.Must(template.New("search").Funcs(template.FuncMap{
	"native":   i18n.Native,
	"entryURL": func(from, to, source string) string { return entries.Path(entries.ID(from, to, source)) },
}).Parse(pageHTML))

// LimitTarget runs before routing: ServeMux may otherwise redirect an oversized
// target with a noncanonical path before the search handler can reject it.
func LimitTarget(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.RequestURI) > MaxTargetBytes {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Robots-Tag", "noindex, follow")
			http.Error(w, "Request target is too long.", http.StatusRequestURITooLong)
			return
		}
		next.ServeHTTP(w, r)
	})
}
