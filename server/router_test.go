package server

import (
	"testing"
)

func TestCSSEntryRedirectURL(t *testing.T) {
	origin := "https://esm.sh"
	esmPath := EsmPath{
		PkgName:    "@material/web",
		PkgVersion: "2.4.2-nightly.95dd57c.0",
	}

	tests := []struct {
		cssEntry string
		expected string
	}{
		{
			cssEntry: "./labs/gb/components/ripple/ripple.css",
			expected: "https://esm.sh/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
		{
			cssEntry: "labs/gb/components/ripple/ripple.css",
			expected: "https://esm.sh/@material/web@2.4.2-nightly.95dd57c.0/labs/gb/components/ripple/ripple.css",
		},
	}

	for _, test := range tests {
		// Calling the actual helper function from router.go
		url := getCSSEntryRedirectURL(origin, esmPath, test.cssEntry)
		if url != test.expected {
			t.Errorf("For CSSEntry %q, expected %q, but got %q", test.cssEntry, test.expected, url)
		}
	}
}
