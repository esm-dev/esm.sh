package server

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/ije/gox/log"
)

// the minimum content size to enable compression
const compressMinSize = 1024

// withRecovery returns a middleware that sets the `Server` header and recovers
// from panics in the next handlers.
func withRecovery(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				logger.Errorf("[panic] %v\n%s", v, debug.Stack())
				writeStatus(w, 500, "Internal Server Error")
			}
		}()
		w.Header().Set("Server", "esm.sh")
		next.ServeHTTP(w, r)
	})
}

// withAccessLog returns a middleware that logs the request/response to the given logger.
func withAccessLog(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		wr := &loggedResponseWriter{ResponseWriter: w, code: 200}
		startTime := time.Now()
		next.ServeHTTP(wr, r)
		ref := r.Referer()
		if ref == "" {
			ref = "-"
		}
		logger.Printf(
			`%s %s %s %s %s %d %s "%s" %d %d %dms`,
			remoteIP(r),
			r.Host,
			r.Proto,
			r.Method,
			r.RequestURI,
			r.ContentLength,
			ref,
			strings.ReplaceAll(r.UserAgent(), `"`, `\"`),
			wr.code,
			wr.written,
			time.Since(startTime)/time.Millisecond,
		)
	})
}

// loggedResponseWriter records the status code and the number of bytes written
// for access logging.
type loggedResponseWriter struct {
	http.ResponseWriter
	code       int
	written    int
	headerSent bool
}

func (w *loggedResponseWriter) WriteHeader(code int) {
	if !w.headerSent {
		w.headerSent = true
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggedResponseWriter) Write(p []byte) (n int, err error) {
	w.headerSent = true
	n, err = w.ResponseWriter.Write(p)
	w.written += n
	return
}

func (w *loggedResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// withCompress returns a middleware that compresses text responses with brotli
// or gzip if the client accepts it.
func withCompress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var encoding string
		if acceptEncoding := r.Header.Get("Accept-Encoding"); strings.Contains(acceptEncoding, "br") {
			encoding = "br"
		} else if strings.Contains(acceptEncoding, "gzip") {
			encoding = "gzip"
		}
		if encoding == "" {
			next.ServeHTTP(w, r)
			return
		}
		wr := &compressResponseWriter{ResponseWriter: w, encoding: encoding}
		defer wr.Close()
		next.ServeHTTP(wr, r)
	})
}

// compressResponseWriter compresses the response body with brotli or gzip. The
// compression is enabled on the first write if the content is text and either
// the size is unknown or at least `compressMinSize`.
type compressResponseWriter struct {
	http.ResponseWriter
	encoding   string
	zWriter    io.WriteCloser
	headerSent bool
}

func (w *compressResponseWriter) WriteHeader(code int) {
	if w.headerSent {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.headerSent = true
	h := w.Header()
	if code >= 200 && code != http.StatusNoContent && code != http.StatusNotModified && h.Get("Content-Encoding") == "" && isCompressibleContentType(h.Get("Content-Type")) {
		size := -1
		if cl := h.Get("Content-Length"); cl != "" {
			size, _ = strconv.Atoi(cl)
		}
		if size < 0 || size >= compressMinSize {
			appendVaryHeader(h, "Accept-Encoding")
			h.Set("Content-Encoding", w.encoding)
			h.Del("Content-Length")
			if w.encoding == "br" {
				w.zWriter = brotli.NewWriterLevel(w.ResponseWriter, brotli.BestSpeed)
			} else {
				w.zWriter, _ = gzip.NewWriterLevel(w.ResponseWriter, gzip.BestSpeed)
			}
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *compressResponseWriter) Write(p []byte) (int, error) {
	if !w.headerSent {
		w.WriteHeader(200)
	}
	if w.zWriter != nil {
		return w.zWriter.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *compressResponseWriter) Close() error {
	if w.zWriter != nil {
		return w.zWriter.Close()
	}
	return nil
}

func (w *compressResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func isCompressibleContentType(contentType string) bool {
	return contentType != "" && (strings.HasPrefix(contentType, "text/") ||
		strings.HasPrefix(contentType, "application/javascript") ||
		strings.HasPrefix(contentType, "application/typescript") ||
		strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "application/xml") ||
		strings.HasPrefix(contentType, "application/wasm"))
}

// remoteIP returns the remote client IP.
func remoteIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
		if ip != "" {
			ip, _, _ = strings.Cut(ip, ",")
		} else {
			ip = r.RemoteAddr
		}
	}
	ip = strings.TrimSpace(ip)
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}
