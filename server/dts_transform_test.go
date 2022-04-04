package server

import (
	"bytes"
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
		`// test/index.d.ts `,
		`/// <reference path="global.d.ts" /> `,
		`/// <reference types="node" /> `,
		`  `,
		`import {`,
		`  ReactInstance, Component, ComponentState,`,
		`  ReactElement, SFCElement, CElement,`,
		`  DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from 'react';`,
		``,
		`export type React = typeof import('react');`,
		`export { default as Anchor /* anchor */ } from './anchor';`,
		`export { default as AutoComplete } from './auto-complete';export { default as Alert } from './alert';`,
		`/* export Avatar start */ export { default as Avatar /*`,
		`  * export Avatar as default`,
		`  */ } from '../avatar'; /* export Avatar end */`,
		`/* export Form */ export { default as Form } from '../form';`,
		`/* export Uploader start */ export { default as Uploader } from '../uploader'; /* export Uploader end */`,
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
		`export { default as Anchor /* anchor */ } from './anchor.d.ts';`,
		`export { default as AutoComplete } from './auto-complete.d.ts';export { default as Alert } from './alert.d.ts';`,
		`/* export Avatar start */ export { default as Avatar /*`,
		`  * export Avatar as default`,
		`  */ } from '../avatar.d.ts'; /* export Avatar end */`,
		`/* export Form */ export { default as Form } from '../form.d.ts';`,
		`/* export Uploader start */ export { default as Uploader } from '../uploader.d.ts'; /* export Uploader end */`,
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
		"../form.d.ts":       `export default interface Form { }`,
		"../uploader.d.ts":   `export default interface Uploader { }`,
		"index.d.ts":         strings.Join(indexDTSRaw, "\n"),
	}
	for name, content := range dtsFils {
		err := ioutil.WriteFile(path.Join(testDir, "node_modules", "test", name), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	cdnDomain = "cdn.esm.sh"

	cache, err = storage.OpenCache("memory:main")
	if err != nil {
		t.Fatal(err)
	}
	fs, err = storage.OpenFS(fmt.Sprintf("local:%s", testDir))
	if err != nil {
		t.Fatal(err)
	}
	db, err = storage.OpenDB(fmt.Sprintf("postdb:%s", path.Join(testDir, "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	node, err = checkNode(testDir)
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}

	task := BuildTask{
		Pkg:  Pkg{},
		Deps: PkgSlice{},
		wd:   testDir,
	}
	err = task.copyDTS("test/index.d.ts", VERSION, "X-ESM/", newStringSet())
	if err != nil && os.IsExist(err) {
		t.Fatal(err)
	}

	file, err := fs.ReadFile(fmt.Sprintf("types/v%d/test/X-ESM/index.d.ts", VERSION))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.TrimSpace(data)

	expectDts := strings.Join(indexDTSExpect, "\n")
	for i, c := range data {
		e := expectDts[i]
		if e != c {
			t.Fatalf("unexpected index.d.ts at %d: %s<<<>>>%s", i, data[:i], data[i:])
		}
	}
}
