package server

import (
	"bytes"
)

// ensure the length of the returned `js` is not changed to avoid source map mapping issue
func rewriteJS(task *BuildTask, js []byte) []byte {
	// most of npm packages check for the `window` object to detect browser environment, but Deno also has the `window` object
	// so we need to replace `window` with `Deno`
	if task.isDenoTarget() {
		for _, r := range [][2]string{
			{
				"typeof window !== \"undefined\"",
				"typeof Deno   !== \"undefined\"",
			},
			{
				`typeof window<"u"`,
				`typeof Deno  <"u"`,
			},
		} {
			js = bytes.Replace(js, []byte(r[0]), []byte(r[1]), -1)
		}
	}
	switch task.Pkg.Name {
	case "axios", "cross-fetch", "whatwg-fetch":
		if task.isDenoTarget() {
			xhr := []byte("\nimport \"https://deno.land/x/xhr@0.3.0/mod.ts\";")
			js = concatBytes(js, xhr)
		}
	case "iconv-lite":
		if task.isDenoTarget() && semverLessThan(task.Pkg.Version, "0.5.0") {
			old := "__Process$.versions.node"
			new := "__Process$.versions.nope"
			js = bytes.Replace(js, []byte(old), []byte(new), 1)
		}
	}
	return js
}
