package npm_replacements

import "testing"

func TestNoLossyReplacements(t *testing.T) {
	for _, name := range []string{"define-properties", "deep-extend"} {
		if _, ok := Get(name); ok {
			t.Errorf("%s must use the upstream package", name)
		}
	}
}
