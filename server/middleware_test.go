package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestCompressNegotiation(t *testing.T) {
	body := strings.Repeat("export const value = 1;\n", 100)
	handler := withCompress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Vary", "Origin")
		io.WriteString(w, body)
	}))
	for _, test := range []struct {
		accept   []string
		encoding string
	}{
		{nil, ""},
		{[]string{"gzip"}, "gzip"},
		{[]string{"br, gzip"}, "br"},
		{[]string{"gzip, br;q=0"}, "gzip"},
		{[]string{"gzip;q=0, identity;q=1"}, ""},
		{[]string{"br;q=0, gzip;q=0"}, ""},
		{[]string{"br;q=0.2, gzip;q=0.8"}, "gzip"},
		{[]string{"gzip;q=0.5, identity;q=1"}, ""},
		{[]string{"*"}, "br"},
		{[]string{"br;q=0, *;q=1"}, "gzip"},
		{[]string{"*;q=1, br;q=0, gzip;q=0"}, ""},
		{[]string{"*;q=0, gzip"}, "gzip"},
		{[]string{"xbr, gzipx"}, ""},
		{[]string{"br;q=invalid, gzip;q=2"}, ""},
		{[]string{"gzip", "br;q=0"}, "gzip"},
	} {
		t.Run(strings.Join(test.accept, "/"), func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/module.js", nil)
			for _, value := range test.accept {
				r.Header.Add("Accept-Encoding", value)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if got := w.Header().Get("Content-Encoding"); got != test.encoding {
				t.Fatalf("Content-Encoding = %q, want %q", got, test.encoding)
			}
			if got := w.Header().Get("Vary"); got != "Origin, Accept-Encoding" {
				t.Errorf("Vary = %q", got)
			}
			var reader io.Reader = w.Body
			switch test.encoding {
			case "gzip":
				gz, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				reader = gz
			case "br":
				reader = brotli.NewReader(w.Body)
			}
			data, err := io.ReadAll(reader)
			if err != nil || string(data) != body {
				t.Fatalf("unexpected response body: %v", err)
			}
		})
	}
}
