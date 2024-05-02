package server

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"regexp"
)

var regReadTailwindPreflightCSS = regexp.MustCompile(`[a-zA-Z.]+\.readFileSync\(.+?/preflight\.css"\),\s*"utf-?8"\)`)

func (task *BuildTask) rewriteJS(js []byte) (ret []byte, dropSourceMap bool) {
	switch task.pkg.Name {
	case "axios", "cross-fetch", "whatwg-fetch":
		if task.isDenoTarget() {
			xhr := []byte("\nimport \"https://deno.land/x/xhr@0.3.0/mod.ts\";")
			return concatBytes(js, xhr), false
		}

	case "tailwindcss":
		preflightCSSFile := path.Join(task.wd, "node_modules", "tailwindcss/src/css/preflight.css")
		if existsFile(preflightCSSFile) {
			data, err := os.ReadFile(preflightCSSFile)
			if err == nil {
				str, _ := json.Marshal(string(data))
				return regReadTailwindPreflightCSS.ReplaceAll(js, str), true // drop breaking source map
			}
		}

	case "iconv-lite":
		if task.isDenoTarget() && semverLessThan(task.pkg.Version, "0.5.0") {
			old := "__Process$.versions.node"
			new := "__Process$.versions.nope"
			return bytes.Replace(js, []byte(old), []byte(new), 1), false
		}
	}
	return nil, false
}
