package server

import (
	"bytes"
	"fmt"
	"testing"
)

func TestDtsWalker(t *testing.T) {
	const rawDts = `
/*
 * This is a multi-line comment.
 * @types: import("react")
 */
// This is a single-line comment. import("react")
// import React from "react";
/// <reference path="global.d.ts" />

import * as hooks from "./hooks";
import * as CSS from 'csstype';
import * as PropTypes from 'prop-types';
import type { Interaction as SchedulerInteraction } /* inline comment */ from "scheduler/tracing";
import DefaultExport, { AndNamed } from "scheduler/tracing";
import {
  client,
  server
} from "react-dom"
export {
  client,
  server
} from "react-dom"

export * from "react";
export = React;

import React = import('react');
import React = require("react");
import ReactDOM = { client: import('react-dom/client'), server: import('react-dom/server') }
`

	const expectedDts = `
/*
 * This is a multi-line comment.
 * @types: import("react")
 */
// This is a single-line comment. import("react")
// import React from "react";
/// <reference path="./global.d.ts" />

import * as hooks from "./hooks/index.d.ts";
import * as CSS from 'https://esm.sh/csstype@1.0.0/index.d.ts';
import * as PropTypes from 'https://esm.sh/prop-types@1.0.0/index.d.ts';
import type { Interaction as SchedulerInteraction } /* inline comment */ from "https://esm.sh/scheduler@1.0.0/tracing.d.ts";
import DefaultExport, { AndNamed } from "https://esm.sh/scheduler@1.0.0/tracing.d.ts";
import {
  client,
  server
} from "https://esm.sh/@types/react-dom@1.0.0/index.d.ts"
export {
  client,
  server
} from "https://esm.sh/@types/react-dom@1.0.0/index.d.ts"

export * from "https://esm.sh/@types/react@1.0.0/index.d.ts";
export = React;

import React = import('https://esm.sh/@types/react@1.0.0/index.d.ts');
import React = require("https://esm.sh/@types/react@1.0.0/index.d.ts");
import ReactDOM = { client: import('https://esm.sh/react-dom@1.0.0/client.d.ts'), server: import('https://esm.sh/react-dom@1.0.0/server.d.ts') }
`

	buf := bytes.NewBuffer(nil)
	err := parseDts(bytes.NewReader([]byte(rawDts)), buf, func(name string, kind TsImportKind, position int) (string, error) {
		if kind == TsImportFrom || kind == TsImportCall {
			if name == "react" || name == "react-dom" {
				return fmt.Sprintf("https://esm.sh/@types/%s@1.0.0/index.d.ts", name), nil
			}
			if isRelPathSpecifier(name) {
				return name + "/index.d.ts", nil
			}
			pkgName, _, subPath, _ := splitESMPath(name)
			if subPath != "" {
				return fmt.Sprintf("https://esm.sh/%s@1.0.0/%s.d.ts", pkgName, subPath), nil
			}
			return fmt.Sprintf("https://esm.sh/%s@1.0.0/index.d.ts", pkgName), nil
		}
		return name, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if buf.String() != expectedDts {
		t.Fatal("transformed dts not match, want:", expectedDts, "got:", buf.String())
	}
}
