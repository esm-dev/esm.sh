package server

import (
	"bytes"
	"fmt"
	"testing"
)

func TestDtsWalker(t *testing.T) {
	const dts = `
// Type definitions for React 18
// Project: http://facebook.github.io/react/
/// <reference path="global.d.ts" />

import * as CSS from 'csstype';
import * as PropTypes from 'prop-types';
import { Interaction as SchedulerInteraction } from "scheduler/tracing";

export = React;

// Type definitions for ReactDOM 18
import React = import('react');
import React = require("react");
`

	buf := bytes.NewBuffer(nil)
	err := walkDts(bytes.NewReader([]byte(dts)), buf, func(name string, kind string, position int) string {
		t.Log(name, kind, position)
		if kind == "importExpr" {
			return fmt.Sprintf("https://esm.sh/%s@latest/index.d.ts", name)
		}
		if kind == "importCall" {
			return fmt.Sprintf("https://esm.sh/%s@latest/index.d.ts", name)
		}
		return name
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(buf.String())
}
