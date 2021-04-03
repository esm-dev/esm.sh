package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestCopyDTS(t *testing.T) {
	testDir := path.Join(os.TempDir(), "testcopydts")
	nmDir := path.Join(testDir, "node_modules")
	os.RemoveAll(testDir)
	ensureDir(testDir)

	err := yarnAdd(testDir, "@types/react@17.0.0")
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
		fmt.Sprintf(`/// <reference path="https://cdn.esm.sh/v%d/_node.ns.d.ts" />`, VERSION),
		`  `,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		fmt.Sprintf(`} from '/v%d/@types/react@17.0.0/index.d.ts';`, VERSION),
		``,
		fmt.Sprintf(`export type React = typeof import('/v%d/@types/react@17.0.0/index.d.ts');`, VERSION),
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

	err = copyDTS(config{
		storageDir: testDir,
		domain:     "cdn.esm.sh",
	}, nmDir, "test/index.d.ts")
	if err != nil && os.IsExist(err) {
		t.Fatal(err)
	}

	data, err := ioutil.ReadFile(path.Join(testDir, fmt.Sprintf("types/v%d/test/index.d.ts", VERSION)))
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(string(data)) != strings.Join(indexDTSExcept, "\n") {
		t.Fatal("unexpected index.d.ts", string(data))
	}
}
