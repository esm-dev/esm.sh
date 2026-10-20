package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWritePathResolutionErrorDeadline(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/react", nil)
	res := httptest.NewRecorder()
	if !writePathResolutionError(res, req, context.DeadlineExceeded, false) {
		t.Fatal("deadline was not handled")
	}
	if res.Code != http.StatusRequestTimeout || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response = HTTP %d, Cache-Control %q", res.Code, res.Header().Get("Cache-Control"))
	}
}

func TestWritePathResolutionErrorLeavesOtherErrorsToCaller(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/react", nil)
	res := httptest.NewRecorder()
	if writePathResolutionError(res, req, errors.New("package metadata unavailable"), false) {
		t.Fatal("non-timeout error was handled")
	}
	if res.Code != http.StatusOK || res.Body.Len() != 0 {
		t.Fatalf("handler wrote HTTP %d with body %q", res.Code, res.Body.String())
	}
}
