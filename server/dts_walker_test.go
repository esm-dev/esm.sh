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
 * @example: import("react")
 */
// This is a single-line comment. import("react")
// import React from "react";
/// <reference path="global.d.ts" />

import * as CSS from 'csstype';
import * as PropTypes from 'prop-types';
import type { Interaction as SchedulerInteraction } /* bad comment */ from "scheduler/tracing";
import DefaultExport, { AndNamed } from "scheduler/tracing";

export * from "react";
export = React;

// todo: support: export const weird: "import('react')";

import React = import('react');
import React = require("react");
`

	const expectedDts = `
/*
 * This is a multi-line comment.
 * @example: import("react")
 */
// This is a single-line comment. import("react")
// import React from "react";
/// <reference path="./global.d.ts" />

import * as CSS from 'https://esm.sh/csstype@1.0.0/index.d.ts';
import * as PropTypes from 'https://esm.sh/prop-types@1.0.0/index.d.ts';
import type { Interaction as SchedulerInteraction } /* bad comment */ from "https://esm.sh/scheduler@1.0.0/tracing.d.ts";
import DefaultExport, { AndNamed } from "https://esm.sh/scheduler@1.0.0/tracing.d.ts";

export * from "https://esm.sh/@types/react@1.0.0/index.d.ts";
export = React;

// todo: support: export const weird: "import('react')";

import React = import('https://esm.sh/@types/react@1.0.0/index.d.ts');
import React = require("https://esm.sh/@types/react@1.0.0/index.d.ts");
`

	buf := bytes.NewBuffer(nil)
	err := walkDts(bytes.NewReader([]byte(rawDts)), buf, func(name string, kind string, position int) string {
		if kind == "importExpr" || kind == "importCall" {
			if name == "react" || name == "react-dom" {
				return fmt.Sprintf("https://esm.sh/@types/%s@1.0.0/index.d.ts", name)
			}
			pkgName, subPath := splitPkgPath(name)
			if subPath != "" {
				return fmt.Sprintf("https://esm.sh/%s@1.0.0/%s.d.ts", pkgName, subPath)
			}
			return fmt.Sprintf("https://esm.sh/%s@1.0.0/index.d.ts", pkgName)
		}
		return name
	})
	if err != nil {
		t.Fatal(err)
	}

	if buf.String() != expectedDts {
		t.Fatal("transformed dts not match, want:", expectedDts, "got:", buf.String())
	}
}
