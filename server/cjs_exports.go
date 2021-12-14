package server

import (
	"encoding/json"
	"time"
)

type cjsExportsResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsExportsResult, err error) {
	data := invokeNodeService("parseCjsExports", map[string]interface{}{
		"buildDir":   buildDir,
		"importPath": importPath,
		"nodeEnv":    nodeEnv,
	}, 10*time.Second)

	err = json.Unmarshal(data, &ret)
	return
}
