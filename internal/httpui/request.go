// Package httpui adapts the lookup contract to bounded HTTP responses.
package httpui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/valpere/slovnyk/internal/lookup"
	"github.com/valpere/slovnyk/internal/normalization"
)

const MaxTargetBytes = 32 << 10

type request struct {
	Search lookup.Request
	Plain  bool
	Empty  bool
}

// parse decodes the query exactly once, including ignored parameters. It does
// not resolve locales, normalize keys, or select a dataset.
func parse(r *http.Request, signer *lookup.Signer) (request, int) {
	out := request{Search: lookup.Request{Mode: normalization.Auto, Limit: 50}}
	if len(r.RequestURI) > MaxTargetBytes {
		return out, http.StatusRequestURITooLong
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return out, 400
	}
	for k, vs := range values {
		if !validText(k) {
			return out, 400
		}
		for _, v := range vs {
			if !validText(v) {
				return out, 400
			}
		}
	}
	trimmed, err := normalization.Validate(values.Get("q"))
	if err != nil {
		return out, 400
	}
	for _, k := range []string{"from", "to", "q", "mode", "limit", "cursor", "format", "ui"} {
		if len(values[k]) > 1 {
			return out, 400
		}
	}
	if vs, ok := values["mode"]; ok {
		out.Search.Mode = normalization.Mode(vs[0])
		if out.Search.Mode != normalization.Auto && out.Search.Mode != normalization.Exact && out.Search.Mode != normalization.Mask {
			return out, 400
		}
	}
	if vs, ok := values["limit"]; ok {
		if vs[0] == "" {
			return out, 400
		}
		for _, c := range vs[0] {
			if c < '0' || c > '9' {
				return out, 400
			}
		}
		n, e := strconv.Atoi(vs[0])
		if e != nil || n < 1 || n > 100 {
			return out, 400
		}
		out.Search.Limit = n
	}
	if vs, ok := values["format"]; ok {
		switch vs[0] {
		case "html":
		case "plain":
			out.Plain = true
		default:
			return out, 400
		}
	}
	for _, k := range []string{"from", "to", "cursor"} {
		if vs, ok := values[k]; ok && vs[0] == "" {
			return out, 400
		}
	}
	out.Search.From, out.Search.To, out.Search.Q, out.Search.Cursor = values.Get("from"), values.Get("to"), values.Get("q"), values.Get("cursor")
	if out.Search.Cursor != "" {
		if _, e := signer.Decode(out.Search.Cursor); e != nil || trimmed == "" {
			return out, 400
		}
	}
	out.Empty = trimmed == ""
	if !out.Empty && (out.Search.From == "" || out.Search.To == "" || len(out.Search.From) > 64 || len(out.Search.To) > 64) {
		return out, 400
	}
	return out, 200
}

func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

// nextURL uses validated, decoded values only; unknown query keys are discarded.
func nextURL(r request, cursor string) string {
	q := url.Values{"from": {r.Search.From}, "to": {r.Search.To}, "q": {r.Search.Q}, "mode": {string(r.Search.Mode)}, "limit": {strconv.Itoa(r.Search.Limit)}, "cursor": {cursor}}
	if r.Plain {
		q.Set("format", "plain")
	}
	return "/search?" + q.Encode()
}
