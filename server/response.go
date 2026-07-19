package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// writeStatus replies to the request with the given status code and a plain-text message.
func writeStatus(w http.ResponseWriter, code int, message string) {
	h := w.Header()
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "text/plain; charset=utf-8")
	}
	h.Set("Content-Length", strconv.Itoa(len(message)))
	w.WriteHeader(code)
	w.Write([]byte(message))
}

// writeBody replies to the request with the given data using the headers already
// set by the caller.
func writeBody(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// writeReader replies to the request with the content of the given reader,
// then closes it.
func writeReader(w http.ResponseWriter, r io.ReadCloser) {
	defer r.Close()
	io.Copy(w, r)
}

// writeJSON replies to the request with the given value encoded as JSON.
func writeJSON(w http.ResponseWriter, code int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeStatus(w, 500, "failed to encode json")
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctJSON)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(code)
	w.Write(data)
}

// writeJSONError replies to the request with the error message encoded as
// `{"code": code, "message": message}`.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{"code": code, "message": message})
}
