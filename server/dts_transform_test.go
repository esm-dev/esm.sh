package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"esm.sh/server/storage"
)

func TestCopyDTS(t *testing.T) {
	testDir := path.Join(os.TempDir(), "esmd-testing")
	os.RemoveAll(testDir)
	ensureDir(testDir)

	err := yarnAdd(testDir, "@types/react@17.0.0")
	if err != nil {
		t.Fatal(err)
	}

	indexDTSRaw := []string{
		`// test/index.d.ts`,
		`/// <reference path="global.d.ts" />`,
		`/// <reference types="node" />`,
		`  `,
		`import {`,
		`  ReactInstance, Component, ComponentState,`,
		`  ReactElement, SFCElement, CElement,`,
		`  DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from 'react';`,
		``,
		`export type React = typeof import('react');`,
		`export { default as Anchor } from './anchor';`,
		`export { default as AutoComplete } from './auto-complete';export { default as Alert } from './alert';`,
		`/* avatar */ export { default as Avatar } from '../avatar';`,
		`declare module "test/private" {`,
		`  export const privateValue: any;`,
		`}`,
		`declare module "test" {`,
		`  export = Component;`,
		`}`,
		`declare module 'test' {`,
		`  export { privateValue } from "test/private";`,
		`  export import ReactInstance = ReactInstance;`,
		`  export import ReactElement = ReactElement;`,
		`}`,
	}
	indexDTSExpect := []string{
		`// test/index.d.ts`,
		`/// <reference path="./global.d.ts" />`,
		fmt.Sprintf(`/// <reference path="https://cdn.esm.sh/v%d/node.ns.d.ts" />`, VERSION),
		`  `,
		`import {`,
		`  ReactInstance, Component, ComponentState,`,
		`  ReactElement, SFCElement, CElement,`,
		`  DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		fmt.Sprintf(`} from 'https://cdn.esm.sh/v%d/@types/react@17.0.0/X-ESM/index.d.ts';`, VERSION),
		``,
		fmt.Sprintf(`export type React = typeof import('https://cdn.esm.sh/v%d/@types/react@17.0.0/X-ESM/index.d.ts');`, VERSION),
		`export { default as Anchor } from './anchor.d.ts';`,
		`export { default as AutoComplete } from './auto-complete.d.ts';export { default as Alert } from './alert.d.ts';`,
		`/* avatar */ export { default as Avatar } from '../avatar.d.ts';`,
		`declare module "test/private" {`,
		`  export const privateValue: any;`,
		`}`,
		`declare module "https://cdn.esm.sh/test" {`,
		`  export = Component;`,
		`}`,
		`declare module 'https://cdn.esm.sh/test' {`,
		`  export { privateValue } from "test/private";`,
		`  export import ReactInstance = ReactInstance;`,
		`  export import ReactElement = ReactElement;`,
		`}`,
		``,
		`declare module "https://cdn.esm.sh/test@*" {`,
		`  export = Component;`,
		`}`,
		`declare module "https://cdn.esm.sh/test@*" {`,
		`  export { privateValue } from "test/private";`,
		`  export import ReactInstance = ReactInstance;`,
		`  export import ReactElement = ReactElement;`,
		`}`,
	}
	ensureDir(path.Join(testDir, "node_modules", "test"))
	dtsFils := map[string]string{
		"global.d.ts":        `declear interface Event { }`,
		"anchor.d.ts":        `export default interface Anchor { }`,
		"auto-complete.d.ts": `export default interface AutoComplete { }`,
		"alert.d.ts":         `export default interface Alert { }`,
		"../avatar.d.ts":     `export default interface Avatar { }`,
		"index.d.ts":         strings.Join(indexDTSRaw, "\n"),
	}
	for name, content := range dtsFils {
		err := ioutil.WriteFile(path.Join(testDir, "node_modules", "test", name), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	config = &Config{
		cdnDomain: "cdn.esm.sh",
	}

	fs, err = storage.OpenFS(fmt.Sprintf("local:%s", testDir))
	if err != nil {
		t.Fatal(err)
	}
	db, err = storage.OpenDB(fmt.Sprintf("postdb:%s/test.db", testDir))
	if err != nil {
		t.Fatal(err)
	}
	node, err = checkNode(testDir)
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}

	err = CopyDTS(testDir, "X-ESM/", "test/index.d.ts")
	if err != nil && os.IsExist(err) {
		t.Fatal(err)
	}

	file, _, err := fs.ReadFile(fmt.Sprintf("types/v%d/test/X-ESM/index.d.ts", VERSION))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(string(data)) != strings.Join(indexDTSExpect, "\n") {
		t.Fatalf("unexpected index.d.ts:\n%s", string(data))
	}
}
