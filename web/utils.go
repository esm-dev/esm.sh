package web

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// isModulePath checks if the given string is a module path.
func isModulePath(s string) bool {
	switch path.Ext(s) {
	case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".svelte", ".vue":
		return true
	default:
		return false
	}
}

// isHttpSepcifier returns true if the specifier is a remote URL.
func isHttpSepcifier(specifier string) bool {
	return strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "http://")
}

// isRelPathSpecifier returns true if the specifier is a local path.
func isRelPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}

// isAbsPathSpecifier returns true if the specifier is an absolute path.
func isAbsPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "file://")
}

// encodeUrl converts a url.URL to a string without escaping the path.
func encodeUrl(u *url.URL) string {
	var buf strings.Builder
	n := len(u.Scheme) + 3 + len(u.Host) + len(u.Path) + len(u.RawQuery)
	if u.RawQuery != "" {
		n++ // '?'
	}
	buf.Grow(n)
	buf.WriteString(u.Scheme)
	buf.Write([]byte{':', '/', '/'})
	buf.WriteString(u.Host)
	buf.WriteString(u.Path)
	if u.RawQuery != "" {
		buf.WriteByte('?')
		buf.WriteString(u.RawQuery)
	}
	return buf.String()
}

// dummyResponseWriter is a dummy http.ResponseWriter that does nothing.
type dummyResponseWriter struct {
	header http.Header
}

func (w *dummyResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *dummyResponseWriter) WriteHeader(statusCode int) {
}

func (w *dummyResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}
