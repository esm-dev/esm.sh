package server

import (
	"bytes"

	"github.com/Masterminds/semver/v3"
)

func rewriteJS(task *BuildTask, js []byte) []byte {
	denoTarget := task.Target == "deno" || task.Target == "denonext"
	switch task.Pkg.Name {
	case "axios", "cross-fetch", "whatwg-fetch":
		if denoTarget {
			xhr := []byte(`import "https://deno.land/x/xhr@0.3.0/mod.ts";`)
			buf := make([]byte, len(js)+len(xhr))
			copy(buf, xhr)
			copy(buf[len(xhr):], js)
			js = buf
		}
	case "iconv-lite":
		if denoTarget && semver.MustParse(task.Pkg.Version).LessThan(semver.MustParse("0.5.0")) {
			js = bytes.Replace(js, []byte("__Process$.versions.node"), []byte("void 0"), 1)
		}
	}
	return js
}
