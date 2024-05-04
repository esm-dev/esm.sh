package server

import (
	"fmt"
	"strings"
	"testing"
)

func TestGetNodejsLatestVersion(t *testing.T) {
	version, err := getNodejsLatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, fmt.Sprintf("v%d.", nodejsMinVersion)) {
		t.Fatalf("bad version %s", version)
	}
}
