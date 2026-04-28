package server

import (
	"net/http"
	"testing"
)

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
