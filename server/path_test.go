package server

import (
	"net/http"
	"testing"
)

func TestParsePrPackageName(t *testing.T) {
	for _, pathname := range []string{
		"/pr/tinybench@abcdef0",
		"/pr/tinylibs/tinybench/tinybench@abcdef0",
		"/pr/owner/repo/@scope/pkg@abcdef0",
	} {
		esm, _, exact, _, _, err := parseEsmPath(nil, pathname)
		if err != nil {
			t.Errorf("parseEsmPath(%q): %v", pathname, err)
		} else if !esm.PrPrefix || !exact {
			t.Errorf("parseEsmPath(%q) did not return an exact PR package", pathname)
		}
	}

	for _, pathname := range []string{
		"/pr/../pkg@abcdef0",
		"/pr/owner//pkg@abcdef0",
		"/pr/owner/repo/..@abcdef0",
		"/pr/owner/repo/bad%name@abcdef0",
		"/pr/owner/repo/bad name@abcdef0",
		"/pr/@scope/..@abcdef0",
	} {
		if _, _, _, _, _, err := parseEsmPath(nil, pathname); err == nil {
			t.Errorf("expected PR package path %q to be rejected", pathname)
		}
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
