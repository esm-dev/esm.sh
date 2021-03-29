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

	err := os.Chdir(testDir)
	if err != nil {
		t.Fatal(err)
	}

	err = yarnAdd("react")
	if err != nil {
		t.Fatal(err)
	}

	exports, err := parseCJSModuleExports(testDir, "react")
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
	expect := []string{"Component", "ReactNode", "useState"}

	tmpDir := os.TempDir()
	err := ioutil.WriteFile(path.Join(tmpDir, "react.js"), []byte(strings.Join(reactRaw, "\n")), 0644)
	if err != nil {
		t.Fatal(err)
	}

	fp := path.Join(tmpDir, "exports.js")
	err = ioutil.WriteFile(fp, []byte(strings.Join(exportRaw, "\n")), 0644)
	if err != nil {
		t.Fatal(err)
	}

	exports, _, err := parseESModuleExports(".", fp)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(exports, ",") != strings.Join(expect, ",") {
		t.Fatalf("unexpected exports.js: %s", strings.Join(exports, ","))
	}
}
