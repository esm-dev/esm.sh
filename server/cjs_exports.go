package server

import (
	"encoding/json"
	"fmt"
)

type cjsExportsResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsExportsResult, err error) {
	data := <-invokeNodeService("cjsExports", map[string]interface{}{
		"buildDir":   buildDir,
		"importPath": importPath,
		"nodeEnv":    nodeEnv,
	})

	err = json.Unmarshal(data, &ret)
	fmt.Println(ret)
	return
}
