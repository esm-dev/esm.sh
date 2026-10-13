package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
)

var (
	regSanitizeHTMLExport       = regexp.MustCompile(`(?m)^ {2,4}(?:interface|type|const|function) ([\w$]+)`)
	regReadTailwindPreflightCSS = regexp.MustCompile(`[a-zA-Z.]+\.readFileSync\(.+?/preflight\.css"\),\s*"utf-?8"\)`)
)

func (ctx *BuildContext) rewriteJS(in []byte) (out []byte, dropSourceMap bool) {
	switch ctx.esmPath.PkgName {
	case "axios", "cross-fetch", "whatwg-fetch":
		if ctx.isDenoTarget() {
			xhr := []byte("\nimport \"https://deno.land/x/xhr@0.3.0/mod.ts\";")
			return concatBytes(in, xhr), false
		}

	case "tailwindcss":
		preflightCSSFile := path.Join(ctx.wd, "node_modules", "tailwindcss/src/css/preflight.css")
		if existsFile(preflightCSSFile) {
			data, err := os.ReadFile(preflightCSSFile)
			if err == nil {
				str, _ := json.Marshal(string(data))
				return regReadTailwindPreflightCSS.ReplaceAll(in, str), true // drop breaking source map
			}
		}

	case "iconv-lite":
		if ctx.isDenoTarget() && semverLessThan(ctx.esmPath.PkgVersion, "0.5.0") {
			old := "__Process$.versions.node"
			new := "__Process$.versions.nope"
			return bytes.Replace(in, []byte(old), []byte(new), 1), false
		}
	}
	return in, false
}

func (ctx *BuildContext) rewriteDTS(filename string, buf *bytes.Buffer) *bytes.Buffer {
	switch ctx.esmPath.PkgName {
	case "@types/sanitize-html":
		if filename == "./index.d.ts" {
			dts := buf.Bytes()
			_, namespace, ok := bytes.Cut(dts, []byte("\ndeclare namespace sanitize {\n"))
			if ok && bytes.Contains(dts, []byte("\nexport = sanitize;\n")) {
				buf = bytes.NewBuffer(bytes.Replace(dts, []byte("\nexport = sanitize;\n"), []byte("\nexport default sanitize;\n"), 1))
				for _, match := range regSanitizeHTMLExport.FindAllSubmatch(namespace, -1) {
					fmt.Fprintf(buf, "export import %s = sanitize.%s;\n", match[1], match[1])
				}
				return buf
			}
		}
	case "preact":
		// fix preact/compat types
		if filename == "./compat/src/index.d.ts" {
			dts := buf.Bytes()
			if !bytes.Contains(dts, []byte("export type PropsWithChildren")) {
				return bytes.NewBuffer(bytes.ReplaceAll(
					dts,
					[]byte("export import ComponentProps = preact.ComponentProps;"),
					[]byte("export import ComponentProps = preact.ComponentProps;\n\n// added by esm.sh\nexport type PropsWithChildren<P = unknown> = P & { children?: preact.ComponentChildren };"),
				))
			}
		}
	case "@thebeyondgroup/shopify-rich-text-renderer":
		if filename == "./index.d.ts" {
			prefix := "declare module '{ESM_CDN_ORIGIN}/" + ctx.esmPath.PackageId() + "/" + ctx.getBuildArgsPrefix(true) + "index.d.ts' {"
			if body, ok := bytes.CutPrefix(bytes.TrimSpace(buf.Bytes()), []byte(prefix)); ok {
				if body, ok := bytes.CutSuffix(body, []byte("}")); ok {
					return bytes.NewBuffer(body)
				}
			}
		}
	case "@rollup/plugin-commonjs":
		dts := buf.Bytes()
		// see https://github.com/denoland/deno/issues/27492
		return bytes.NewBuffer(bytes.ReplaceAll(
			dts,
			[]byte("[package: string]: ReadonlyArray<string>"),
			[]byte("[name   : string]: ReadonlyArray<string>"),
		))
	}
	return buf
}
