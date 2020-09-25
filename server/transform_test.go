package server

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestToRequire(t *testing.T) {
	raw := []string{
		`// dts test`,
		`/// <reference path="global.d.ts" />`,
		`  `,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement as CE,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from 'react';`,
		``,
		`import { default as Anchor } from './anchor';`,
		`import { default as AutoComplete } from './auto-complete';import { default as Alert , AlertOptions } from './alert';`,
		`/* avatar */ import { default as Avatar } from '../avatar';`,
	}
	expect := []string{
		`// dts test`,
		`/// <reference path="global.d.ts" />`,
		``,
		`const {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement: CE,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} = require('react');`,
		``,
		`const { default: Anchor } = require('./anchor');`,
		`const { default: AutoComplete } = require('./auto-complete');const { default: Alert , AlertOptions } = require('./alert');`,
		`/* avatar */ const { default: Avatar } = require('../avatar');`,
	}

	data := toRequire([]byte(strings.Join(raw, "\n")))
	if strings.TrimSpace(string(data)) != strings.Join(expect, "\n") {
		t.Fatal("unexpected index.d.ts", string(data))
	}
}

func TestCopyDTS(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	ensureDir(testDir)

	nmDir := path.Join(testDir, "node_modules")
	saveDir := path.Join(testDir, "types")
	os.RemoveAll(saveDir)
	ensureDir(saveDir)

	err := os.Chdir(testDir)
	if err != nil {
		t.Fatal(err)
	}

	err = yarnAdd("@types/react@16.9.49")
	if err != nil {
		t.Fatal(err)
	}

	indexDTSRaw := []string{
		`// dts test`,
		`/// <reference path="global.d.ts" />`,
		`  `,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from 'react';`,
		``,
		`export { default as Anchor } from './anchor';`,
		`export { default as AutoComplete } from './auto-complete';export { default as Alert } from './alert';`,
		`/* avatar */ export { default as Avatar } from '../avatar';`,
	}
	indexDTSExcept := []string{
		`// dts test`,
		`/// <reference path="./global.d.ts" />`,
		``,
		`import {`,
		`    ReactInstance, Component, ComponentState,`,
		`    ReactElement, SFCElement, CElement,`,
		`    DOMAttributes, DOMElement, ReactNode, ReactPortal`,
		`} from '/@types/react@16.9.49/index.d.ts';`,
		``,
		`export { default as Anchor } from './anchor.d.ts';`,
		`export { default as AutoComplete } from './auto-complete.d.ts';export { default as Alert } from './alert.d.ts';`,
		`/* avatar */ export { default as Avatar } from '../avatar.d.ts';`,
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

	err = copyDTS(nmDir, saveDir, "test/index.d.ts")
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
