package server

import (
	"net/http"
	"testing"
	"time"
)

func TestParseEsmPathGithubVersion(t *testing.T) {
	const repo = "path-test/semver-tags"
	key := "git ls-remote https://github.com/" + repo
	setCacheItem(key, []GitRef{
		{Ref: "HEAD", Sha: "abcdef1234567890"},
		{Ref: "refs/heads/main", Sha: "abcdef1234567890"},
		{Ref: "refs/tags/v1.10.0", Sha: "1111111234567890"},
		{Ref: "refs/tags/1.2.0", Sha: "2222222234567890"},
		{Ref: "refs/tags/v2.0.0-beta.1", Sha: "3333333234567890"},
		{Ref: "refs/tags/invalid", Sha: "4444444234567890"},
	}, time.Minute)
	t.Cleanup(func() { deleteCacheItem(key) })
	for _, test := range []struct {
		version string
		want    string
		exact   bool
	}{
		{"", "abcdef1", false},
		{"main", "abcdef1", false},
		{"^1", "1111111", false},
		{"semver:^1", "1111111", false},
		{"~1.2", "2222222", false},
		{"*", "1111111", false},
		{"v1.10.0", "v1.10.0", true},
		{"abcdef1", "abcdef1", true},
		{"^3", "", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			t.Cleanup(func() { deleteCacheItem("gh/" + repo + "@" + test.version) })
			esm, _, exact, _, _, err := parseEsmPath(nil, "/gh/"+repo+"@"+test.version)
			if test.want == "" {
				if err == nil {
					t.Fatal("expected an unmatched version error")
				}
				return
			}
			if err != nil || esm.PkgVersion != test.want || exact != test.exact {
				t.Fatalf("version %q = %q (exact %v), %v; want %q (exact %v)", test.version, esm.PkgVersion, exact, err, test.want, test.exact)
			}
		})
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
