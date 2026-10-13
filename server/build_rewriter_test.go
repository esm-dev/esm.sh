package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestRewriteSanitizeHTMLTypes(t *testing.T) {
	const input = `import type { ParserOptions } from "https://example.com/parser.d.ts";

export = sanitize;

declare function sanitize(dirty: string, options?: sanitize.IOptions): string;

declare namespace sanitize {
  interface IOptions { parser?: ParserOptions; }
  type Transformer = (value: string) => string;
  const defaults: IOptions;
  function simpleTransform(tag: string): Transformer;
}
`
	const exports = `export import IOptions = sanitize.IOptions;
export import Transformer = sanitize.Transformer;
export import defaults = sanitize.defaults;
export import simpleTransform = sanitize.simpleTransform;
`
	for _, indent := range []string{"  ", "    "} {
		source := strings.ReplaceAll(input, "\n  ", "\n"+indent)
		ctx := &BuildContext{esmPath: EsmPath{PkgName: "@types/sanitize-html"}}
		got := ctx.rewriteDTS("./index.d.ts", bytes.NewBufferString(source)).String()
		want := strings.Replace(source, "export = sanitize;", "export default sanitize;", 1) + exports
		if got != want {
			t.Fatalf("expected:\n%s\ngot:\n%s", want, got)
		}
	}
	for _, test := range []struct {
		pkg, file, source string
	}{
		{"other", "./index.d.ts", input},
		{"@types/sanitize-html", "./other.d.ts", input},
		{"@types/sanitize-html", "./index.d.ts", strings.Replace(input, "export = sanitize;", "export default sanitize;", 1)},
		{"@types/sanitize-html", "./index.d.ts", "declare const sanitize: unknown;\nexport = sanitize;\n"},
	} {
		ctx := &BuildContext{esmPath: EsmPath{PkgName: test.pkg}}
		if got := ctx.rewriteDTS(test.file, bytes.NewBufferString(test.source)).String(); got != test.source {
			t.Fatalf("unexpected rewrite of %s/%s:\n%s", test.pkg, test.file, got)
		}
	}
}
