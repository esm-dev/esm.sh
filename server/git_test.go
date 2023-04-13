package server

import (
	"testing"
)

func TestListRepoRefs(t *testing.T) {
	refs, err := listRepoRefs("https://github.com/esm-dev/esm.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(refs)
}
