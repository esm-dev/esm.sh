package server

import (
	"encoding/json"
)

type cjsExportsResult struct {
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Error         string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsExportsResult, err error) {
	data := invokeNodeService("parseCjsExports", map[string]interface{}{
		"buildDir":   buildDir,
		"importPath": importPath,
		"nodeEnv":    nodeEnv,
	})

	err = json.Unmarshal(data, &ret)
	return
}
