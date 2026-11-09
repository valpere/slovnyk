package httpui

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestBundledFontDelivery(t *testing.T) {
	h := New(nil, nil)
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/assets/rubik-v1.ttf", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "font/ttf" || w.Header().Get("Content-Length") != strconv.Itoa(len(rubikFont)) {
			t.Fatalf("%s: status=%d headers=%v", method, w.Code, w.Header())
		}
		if method == "HEAD" && w.Body.Len() != 0 || method == "GET" && w.Body.Len() != len(rubikFont) {
			t.Fatalf("%s: unexpected body length %d", method, w.Body.Len())
		}
		if len(w.Result().Cookies()) != 0 || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: font must not set preferences and must prevent MIME sniffing", method)
		}
	}
	r := httptest.NewRequest("GET", "/assets/rubik-v1.ttf", nil)
	r.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusPartialContent || w.Body.Len() != 4 {
		t.Fatalf("font range: status=%d length=%d", w.Code, w.Body.Len())
	}
}
