package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePrPath(t *testing.T) {
	for _, pathname := range []string{
		"/pr/tinybench@a832a55",
		"/pr/sveltejs/svelte@a832a55",
		"/pr/tinylibs/tinybench/tinybench@a832a55",
		"/pr/owner/repo/@scope/package@a832a55",
	} {
		if _, _, _, _, _, err := parseEsmPath(nil, pathname); err != nil {
			t.Errorf("expected %q to be valid: %v", pathname, err)
		}
	}

	for _, pathname := range []string{
		"/pr/@a832a55",
		"/pr/.@a832a55",
		"/pr/..@a832a55",
		"/pr/owner/../package@a832a55",
		"/pr/owner/repo/package/extra/name@a832a55",
		"/pr/owner/repo/package%name@a832a55",
	} {
		if _, _, _, _, _, err := parseEsmPath(nil, pathname); err == nil {
			t.Errorf("expected %q to be rejected", pathname)
		}
	}

	pathname := httptest.NewRequest(http.MethodGet, "/pr/owner/%2e%2e/package@a832a55", nil).URL.Path
	if _, _, _, _, _, err := parseEsmPath(nil, pathname); err == nil {
		t.Errorf("expected percent-encoded traversal path %q to be rejected", pathname)
	}
}

func TestPrCommitFromHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "full sha",
			header:   "mcp-use:mcp-use:dffcf42d399d18d19ac0ca766e60b0ef13309181",
			expected: "dffcf42",
		},
		{
			name:     "short sha",
			header:   "tinylibs:tinybench:a832a55",
			expected: "a832a55",
		},
		{
			name:     "missing sha",
			header:   "tinylibs:tinybench:",
			expected: "",
		},
		{
			name:     "non hex sha",
			header:   "tinylibs:tinybench:not-a-sha",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			header.Set("x-commit-key", test.header)

			if commit := prCommitFromHeader(header); commit != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, commit)
			}
		})
	}
}
