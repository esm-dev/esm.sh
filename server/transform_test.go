package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestParseESModuleExports(t *testing.T) {
	exportRaw := []string{
		`export * from './react.js';`,
	}
	reactRaw := []string{
		`export {`,
		`    Component, ReactNode`,
		`} from 'react';`,
	}
	expect := []string{"Component", "ReactNode"}

	testDir := path.Join(os.TempDir(), "test")
	ensureDir(testDir)

	err := ioutil.WriteFile(path.Join(testDir, "react.js"), []byte(strings.Join(reactRaw, "\n")), 0644)
	if err != nil {
		t.Fatal(err)
	}

	fp := path.Join(testDir, "exports.js")
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

func TestCopyDTS(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	os.RemoveAll(testDir)
	ensureDir(testDir)

	nmDir := path.Join(testDir, "node_modules")
	saveDir := path.Join(testDir, "types")
	os.RemoveAll(saveDir)
	ensureDir(saveDir)

	err := os.Chdir(testDir)
	if err != nil {
		t.Fatal(err)
	}

	err = yarnAdd("@types/react@17.0.0")
	if err != nil {
		t.Fatal(err)
	}

	indexDTSRaw := []string{
		`// dts test`,
		`/// <reference path="global.d.ts" />`,
		`/// <reference types="node" />`,
		`  `,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from 'react';`,
		``,
		`export type React = typeof import('react');`,
		`export { default as Anchor } from './anchor';`,
		`export { default as AutoComplete } from './auto-complete';export { default as Alert } from './alert';`,
		`/* avatar */ export { default as Avatar } from '../avatar';`,
		`declare module "test" {`,
		`    export = Component;`,
		`}`,
		`declare module 'test' {`,
		`    export import ReactInstance = ReactInstance;`,
		`    export import ReactElement = ReactElement;`,
		`}`,
	}
	indexDTSExcept := []string{
		`// dts test`,
		`/// <reference path="./global.d.ts" />`,
		fmt.Sprintf(`/// <reference path="https://cdn.esm.sh/v%d/_node.ns.d.ts" />`, buildVersion),
		`  `,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from '/v1/@types/react@17.0.0/index.d.ts';`,
		``,
		`export type React = typeof import('/v1/@types/react@17.0.0/index.d.ts');`,
		`export { default as Anchor } from './anchor.d.ts';`,
		`export { default as AutoComplete } from './auto-complete.d.ts';export { default as Alert } from './alert.d.ts';`,
		`/* avatar */ export { default as Avatar } from '../avatar.d.ts';`,
		`declare module "https://cdn.esm.sh/test" {`,
		`    export = Component;`,
		`}`,
		`declare module 'https://cdn.esm.sh/test' {`,
		`    export import ReactInstance = ReactInstance;`,
		`    export import ReactElement = ReactElement;`,
		`}`,
		``,
		`declare module "https://cdn.esm.sh/test@*" {`,
		`    export = Component;`,
		`}`,
		`declare module "https://cdn.esm.sh/test@*" {`,
		`    export import ReactInstance = ReactInstance;`,
		`    export import ReactElement = ReactElement;`,
		`}`,
	}
	ensureDir(path.Join(nmDir, "test"))
	dtsFils := map[string]string{
		"global.d.ts":        `declear interface Event { }`,
		"anchor.d.ts":        `export default interface Anchor { }`,
		"auto-complete.d.ts": `export default interface AutoComplete { }`,
		"alert.d.ts":         `export default interface Alert { }`,
		"../avatar.d.ts":     `export default interface Avatar { }`,
		"index.d.ts":         strings.Join(indexDTSRaw, "\n"),
	}
	for name, content := range dtsFils {
		err = ioutil.WriteFile(path.Join(nmDir, "test", name), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = copyDTS(moduleSlice{}, "cdn.esm.sh", nmDir, saveDir, "test/index.d.ts")
	if err != nil && os.IsExist(err) {
		t.Fatal(err)
	}

	data, err := ioutil.ReadFile(path.Join(saveDir, "test/index.d.ts"))
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(string(data)) != strings.Join(indexDTSExcept, "\n") {
		t.Fatal("unexpected index.d.ts", string(data))
	}
}
