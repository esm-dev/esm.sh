package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCustomLandingPageResponse(t *testing.T) {
	modified := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		upstreamCode int
		since        string
		etag         string
		wantCode     int
	}{
		{"unconditional", 200, "", "", 200},
		{"modified", 200, modified.Add(-time.Hour).Format(http.TimeFormat), "", 200},
		{"unchanged", 200, modified.Format(http.TimeFormat), "", 304},
		{"future", 200, modified.Add(time.Hour).Format(http.TimeFormat), "", 304},
		{"invalid date", 200, "invalid", "", 200},
		{"matching etag", 200, "", `"version"`, 304},
		{"missing asset", 404, "", "", 404},
		{"error with matching date", 503, modified.Format(http.TimeFormat), "", 503},
		{"error with matching etag", 503, "", `"version"`, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
				w.Header().Set("Etag", test.etag)
				w.WriteHeader(test.upstreamCode)
				io.WriteString(w, "landing page")
			}))
			defer upstream.Close()
			handler := customLandingPage(&LandingPageOptions{Origin: upstream.URL, Assets: []string{"/asset"}}, http.NotFoundHandler())
			r := httptest.NewRequest(http.MethodGet, "/asset", nil)
			r.Header.Set("If-Modified-Since", test.since)
			r.Header.Set("If-None-Match", test.etag)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, test.wantCode)
			}
			wantBody := "landing page"
			if test.wantCode == http.StatusNotModified {
				wantBody = ""
			}
			if w.Body.String() != wantBody {
				t.Fatalf("body = %q, want %q", w.Body.String(), wantBody)
			}
		})
	}
}
