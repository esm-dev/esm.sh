package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestRemoteIPTrustedProxies(t *testing.T) {
	previous := config.TrustedProxies
	t.Cleanup(func() { config.TrustedProxies = previous })
	for _, test := range []struct {
		name  string
		peer  string
		proxy bool
		xff   string
		xreal string
		want  string
	}{
		{"untrusted headers", "192.0.2.1:1234", false, "203.0.113.1", "203.0.113.2", "192.0.2.1"},
		{"untrusted peer", "198.51.100.1:1234", true, "203.0.113.1", "203.0.113.2", "198.51.100.1"},
		{"trusted proxy", "192.0.2.1:1234", true, "198.51.100.1", "", "198.51.100.1"},
		{"spoofed first hop", "192.0.2.1:1234", true, "203.0.113.9, 198.51.100.1", "203.0.113.2", "198.51.100.1"},
		{"proxy chain", "192.0.2.1:1234", true, "198.51.100.1, 192.0.2.2", "", "198.51.100.1"},
		{"real ip fallback", "192.0.2.1:1234", true, "", "198.51.100.1", "198.51.100.1"},
		{"invalid forwarded ip", "192.0.2.1:1234", true, "invalid", "198.51.100.1", "192.0.2.1"},
		{"missing headers", "192.0.2.1:1234", true, "", "", "192.0.2.1"},
		{"ipv6", "[2001:db8:1::1]:1234", true, "2001:db8:2::1", "", "2001:db8:2::1"},
		{"mapped ipv4", "[::ffff:192.0.2.1]:1234", true, "::ffff:198.51.100.1", "", "198.51.100.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config.TrustedProxies = nil
			if test.proxy {
				config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("2001:db8:1::/48")}
			}
			req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
			req.RemoteAddr = test.peer
			req.Header.Set("X-Forwarded-For", test.xff)
			req.Header.Set("X-Real-IP", test.xreal)
			if got := remoteIP(req); got != test.want {
				t.Fatalf("remoteIP = %q, want %q", got, test.want)
			}
		})
	}
}

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
