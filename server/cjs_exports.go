package server

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"strings"
)

type cjsExportsResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsExportsResult, err error) {
	if strings.HasSuffix(importPath, ".json") {
		var data []byte
		data, err = ioutil.ReadFile(path.Join(buildDir, importPath))
		if err != nil {
			return
		}

		var m map[string]interface{}
		if json.Unmarshal(data, &m) == nil {
			var exports []string
			var i int
			exports = make([]string, len(m))
			for key := range m {
				exports[i] = key
				i++
			}
			ret = cjsExportsResult{Exports: exports}
		}

		return
	}

	data := invokeNodeService("parseCjsExports", map[string]interface{}{
		"buildDir":   buildDir,
		"importPath": importPath,
		"nodeEnv":    nodeEnv,
	})

	err = json.Unmarshal(data, &ret)
	return
}
