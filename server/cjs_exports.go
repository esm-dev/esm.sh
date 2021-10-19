package server

import (
	"encoding/json"
)

type cjsExportsResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(cjsFile string, nodeEnv string) (ret cjsExportsResult, err error) {
	data := <-invokeNodeService("cjsExports", map[string]interface{}{
		"cjsFile": cjsFile,
		"nodeEnv": nodeEnv,
	})

	err = json.Unmarshal(data, &ret)
	return
}
