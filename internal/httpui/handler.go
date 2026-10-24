package httpui

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strconv"

	"github.com/valpere/slovnyk/internal/lookup"
)

type SearchFunc func(context.Context, lookup.Request) (lookup.Page, error)

// New exposes the first HTTP slice of M1-07. This English development UI is not
// the completed localization/catalog/SEO/downloads milestone.
func New(search SearchFunc, signer *lookup.Signer) http.Handler {
	mux := http.NewServeMux()
	serve := func(w http.ResponseWriter, r *http.Request) {
		parsed, status := parse(r, signer)
		data := view{Request: parsed.Search}
		if status != 200 {
			data = view{Message: "Invalid search parameters. Check the query and language pair."}
		} else if !parsed.Empty {
			if search == nil {
				status = 503
				data.Message = message(lookup.ErrUnavailable)
			} else {
				page, err := search(r.Context(), parsed.Search)
				status = lookup.Status(err)
				if err != nil {
					data.Message = message(err)
				} else {
					data.Groups = page.Groups
					if len(page.Groups) == 0 {
						data.Message = "No matches. Try a mask: * matches any sequence; ? matches one character."
					}
					if page.HasMore {
						data.Next = nextURL(parsed, page.Cursor)
					}
				}
			}
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
			_, _ = io.WriteString(body, "Response unavailable. Please try again.\n")
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
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
	return LimitTarget(mux)
}

type view struct {
	Request       lookup.Request
	Groups        []lookup.Group
	Message, Next string
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
		_, e := io.WriteString(w, "Next: "+v.Next+"\n")
		return e
	}
	return nil
}
func message(err error) string {
	switch {
	case errors.Is(err, lookup.ErrCursor):
		return "Invalid continuation link. Start a new search."
	case errors.Is(err, lookup.ErrGone):
		return "This dictionary version is unavailable. Start a new search."
	case errors.Is(err, lookup.ErrLocale):
		return "Unknown language code. Check the language pair."
	case errors.Is(err, lookup.ErrPair):
		return "This translation direction is unavailable. Choose another language pair."
	case errors.Is(err, lookup.ErrInput):
		return "Invalid search parameters. Check the query and language pair."
	default:
		return "Search is temporarily unavailable. Please try again."
	}
}

var pageTemplate = template.Must(template.New("search").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Slovnyk · Dictionary</title>
<style>
:root{font-family:system-ui,sans-serif;color:#172b36;background:#f5f7f8;line-height:1.6}*{box-sizing:border-box}body{margin:0}main{max-width:52rem;margin:4rem auto;padding:0 1.25rem}h1{letter-spacing:-.04em;font-size:2.8rem;line-height:1.1}p{color:#425563}form,section{background:#fff;border:1px solid #d4dfe4;border-radius:1rem;padding:1.5rem;margin:1.5rem 0}label{display:block;font-weight:600}input{width:100%;padding:.75rem;border:1px solid #738b98;border-radius:.5rem;font:inherit;margin:.3rem 0 1rem}.pair{display:flex;gap:1rem}.pair label{flex:1;min-width:0}button,.next{display:inline-block;background:#075985;color:#fff;border:0;border-radius:.5rem;padding:.8rem 1.4rem;font:inherit;font-weight:600;cursor:pointer}a{color:#075985}button:focus-visible,input:focus-visible,a:focus-visible{outline:3px solid #c35d00;outline-offset:3px}.word{white-space:pre-wrap;overflow-wrap:anywhere}h2{font-size:1.3rem}li{margin:.5rem 0}@media(max-width:32rem){main{margin:2rem auto}.pair{display:block}}
</style></head><body><main><h1>Slovnyk</h1><p>A multilingual dictionary. Find a word or explore with a mask.</p>
<form action="/search" method="get"><label for="q">Word or mask</label><input id="q" name="q" value="{{.Request.Q}}" maxlength="1024" autofocus>
<div class="pair"><label>From language code<input name="from" value="{{.Request.From}}" placeholder="uk-ua" required></label><label>To language code<input name="to" value="{{.Request.To}}" placeholder="en-gb" required></label></div>
<input type="hidden" name="mode" value="{{if .Request.Mode}}{{.Request.Mode}}{{else}}auto{{end}}"><input type="hidden" name="limit" value="{{if .Request.Limit}}{{.Request.Limit}}{{else}}50{{end}}"><button type="submit">Translate</button></form>
{{if .Message}}<p role="status">{{.Message}}</p>{{end}}
{{range .Groups}}<section><h2 class="word">{{.Source}}</h2><ul>{{range .Translations}}<li class="word">{{.}}</li>{{end}}</ul></section>{{end}}
{{if .Next}}<a class="next" href="{{.Next}}" rel="next">More translations</a>{{end}}
</main></body></html>`))

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
