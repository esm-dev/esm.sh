package server

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestParseCJSModuleExports(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	os.RemoveAll(testDir)
	ensureDir(testDir)

	err := yarnAdd(testDir, "react")
	if err != nil {
		t.Fatal(err)
	}

	exports, err := parseCJSModuleExports(testDir, "react", "development")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(exports)
}

func TestParseESModuleExports(t *testing.T) {
	exportRaw := []string{
		`export * from './react.js';`,
	}
	reactRaw := []string{
		`export {`,
		`    Component, ReactNode, useState`,
		`} from 'react';`,
	}

	tmpDir := os.TempDir()
	ensureDir(path.Join(tmpDir, "node_modules"))
	err := ioutil.WriteFile(path.Join(tmpDir, "node_modules", "react.js"), []byte(strings.Join(reactRaw, "\n")), 0644)
	if err != nil {
		t.Fatal(err)
	}

	fp := path.Join(tmpDir, "node_modules", "exports.js")
	err = ioutil.WriteFile(fp, []byte(strings.Join(exportRaw, "\n")), 0644)
	if err != nil {
		t.Fatal(err)
	}

	exports, _, err := parseESModuleExports(tmpDir, "exports")
	if err != nil {
		t.Fatal(err)
	}

	if len(exports) != 3 {
		t.Fatalf("unexpected exports.js: %s", strings.Join(exports, ","))
	}
}
