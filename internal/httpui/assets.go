package httpui

import (
	"bytes"
	_ "embed"
	"net/http"
	"time"
)

// Only the locally bundled font is added to the page policy; scripts and remote
// resources remain blocked. Keep search and indexable entry pages consistent.
const pagePolicy = "default-src 'none'; style-src 'unsafe-inline'; font-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

//go:embed assets/rubik.ttf
var rubikFont []byte

func fontHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "rubik-v1.ttf", time.Time{}, bytes.NewReader(rubikFont))
}
