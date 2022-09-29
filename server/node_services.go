package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const nsApp = `
const http = require('http');
const ns = require('esm-node-services');

const services = {
  test: async input => ({ ...input }),
	...ns
}

const requestListener = function (req, res) {
  if (req.method === "GET") {
    res.writeHead(200);
    res.end("READY");
  } else if (req.method === "POST") {
    let data = '';
    req.on('data', chunk => {
      data += chunk;
    });
    req.on('end', async () => {
      try {
        const { service, input } = JSON.parse(data);
        let output = null
        if (typeof service === 'string' && service in services) {
          output = await services[service](input)
        } else {
          output = { error: 'service "' + service + '" not found' }
        }
        res.writeHead(output.error ? 400 : 200);
        res.end(JSON.stringify(output));
      } catch (e) {
        res.writeHead(500);
        res.end(JSON.stringify({ error: e.message, stack: e.stack }));
      }
    });
  } else {
    res.writeHead(405);
    res.end("Method not allowed");
  }
}

const server = http.createServer(requestListener);
server.listen(%d);
`

var nsPort int
var nsPidFile string

type NSPlayload struct {
	Service string                 `json:"service"`
	Input   map[string]interface{} `json:"input"`
}

func invokeNodeService(serviceName string, input map[string]interface{}) (data []byte, err error) {
	task := &NSPlayload{
		Service: serviceName,
		Input:   input,
	}
	buf := new(bytes.Buffer)
	err = json.NewEncoder(buf).Encode(task)
	if err != nil {
		return
	}
	res, err := http.Post(fmt.Sprintf("http://localhost:%d", nsPort), "application/json", buf)
	if err != nil {
		// kill current ns process
		kill(nsPidFile)
		return
	}
	defer res.Body.Close()
	data, err = ioutil.ReadAll(res.Body)
	return
}

func startNodeServices(etcDir string, port int) (err error) {
	wd := path.Join(etcDir, "ns")
	err = clearDir(wd)
	if err != nil {
		return
	}

	nsPort = port
	nsPidFile = path.Join(etcDir, "ns.pid")

	// install services
	cmd := exec.Command("yarn", "add", "esm-node-services")
	cmd.Dir = wd
	var output []byte
	output, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("install services: %v %s", err, string(output))
		return
	}

	// create ns script
	err = ioutil.WriteFile(
		path.Join(wd, "ns.js"),
		[]byte(fmt.Sprintf(nsApp, port)),
		0644,
	)
	if err != nil {
		return
	}

	// kill previous ns process if exists
	kill(nsPidFile)

	errBuf := bytes.NewBuffer(nil)
	cmd = exec.Command("node", "ns.js")
	cmd.Dir = wd
	cmd.Stderr = errBuf

	err = cmd.Start()
	if err != nil {
		return
	}

	log.Debug("node services process started, pid is", cmd.Process.Pid)

	// store node process pid
	ioutil.WriteFile(nsPidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	// wait the process to exit
	err = cmd.Wait()
	if errBuf.Len() > 0 {
		err = errors.New(strings.TrimSpace(errBuf.String()))
	}
	return
}

type cjsExportsResult struct {
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Error         string   `json:"error"`
	Stack         string   `json:"stack"`
}

var requireModeAllowList = []string{
	"domhandler",
	"he",
	"keycode",
	"lru_map",
	"lz-string",
	"postcss-selector-parser",
	"resolve",
	"safe-buffer",
	"seedrandom",
	"stream-browserify",
	"stream-http",
	"typescript",
	"vscode-oniguruma",
	"web-streams-ponyfill",
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsExportsResult, err error) {
	args := map[string]interface{}{
		"buildDir":   buildDir,
		"importPath": importPath,
		"nodeEnv":    nodeEnv,
	}

	/* workaround for edge cases that can't be parsed by cjsLexer correctly */
	for _, name := range requireModeAllowList {
		if importPath == name || strings.HasPrefix(importPath, name+"/") {
			args["requireMode"] = 1
			break
		}
	}

	data, err := invokeNodeService("parseCjsExports", args)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &ret)
	if err != nil {
		return
	}

	if ret.Error != "" {
		if ret.Stack != "" {
			log.Errorf("[ns] parseCJSModuleExports: %s\n---\n%s\n---", ret.Error, ret.Stack)
		} else {
			log.Errorf("[ns] parseCJSModuleExports: %s", ret.Error)
		}
	}
	return
}
