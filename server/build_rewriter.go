package server

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"regexp"
)

var regReadTailwindPreflightCSS = regexp.MustCompile(`[a-zA-Z.]+\.readFileSync\(.+?/preflight\.css"\),\s*"utf-?8"\)`)

// force to use `npm:` specifier for `denonext` target to support node native module or fix `createRequire` issue
var forceNpmSpecifiers = map[string]bool{
	"@achingbrain/ssdp": true,
	"aws-crt":           true,
	"default-gateway":   true,
	"fsevent":           true,
	"lightningcss":      true,
	"re2":               true,
	"zlib-sync":         true,
	"css-tree":          true,
}

func (ctx *BuildContext) rewriteJS(in []byte) (out []byte, dropSourceMap bool) {
	switch ctx.specifier.PkgName {
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
		if ctx.isDenoTarget() && semverLessThan(ctx.specifier.PkgVersion, "0.5.0") {
			old := "__Process$.versions.node"
			new := "__Process$.versions.nope"
			return bytes.Replace(in, []byte(old), []byte(new), 1), false
		}
	}
	return in, false
}

func (ctx *BuildContext) rewriteDTS(dts string, in []byte) []byte {
	// fix preact/compat types
	if ctx.specifier.PkgName == "preact" && dts == "./compat/src/index.d.ts" {
		if !bytes.Contains(in, []byte("export type PropsWithChildren")) {
			return bytes.ReplaceAll(
				in,
				[]byte("export import ComponentProps = preact.ComponentProps;"),
				[]byte("export import ComponentProps = preact.ComponentProps;\n\n// added by esm.sh\nexport type PropsWithChildren<P = unknown> = P & { children?: preact.ComponentChildren };"),
			)
		}
	}
	return in
}
