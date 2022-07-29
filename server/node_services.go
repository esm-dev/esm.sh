package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const nsApp = `
	const queue = require('async/queue')
  const readline = require('readline')
  const rl = readline.createInterface({
    input: process.stdin,
    historySize: 0,
    crlfDelay: Infinity
  })
  const services = {
    test: async input => ({ ...input })
  }
  const register = %s
	const q = queue(async ({ service, invokeId, input }) => {
		let output = null
		if (typeof service === 'string' && service in services) {
			try {
				output = await services[service](input)
			} catch(e) {
				output = { error: e.message, stack: e.stack }
			}
		} else {
			output = { error: 'service not found' }
		}
		process.stdout.write(invokeId)
		process.stdout.write(JSON.stringify(output))
		process.stdout.write('\n')
	}, %d)

  for (const name of register) {
    Object.assign(services, require(name))
  }

  rl.on('line', async line => {
    if (line.charAt(0) === '{' && line.charAt(line.length-1) === '}') {
      try {
        const { service, invokeId, input } = JSON.parse(line)
        if (typeof invokeId === 'string') {
					q.push({ service, invokeId, input })
        }
      } catch(_) {}
    }
  })

  setTimeout(() => {
    process.stdout.write('READY\n')
  }, 0)
`

type NSTask struct {
	invokeId string
	service  string
	input    map[string]interface{}
	output   chan []byte
}

var nsTasks sync.Map
var nsReady bool
var nsInvokeIndex uint32 = 0
var nsChannel = make(chan *NSTask, 5120)
var stopNodeServices = func() {}

func newInvokeId() string {
	i := atomic.AddUint32(&nsInvokeIndex, 1)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, i)
	return hex.EncodeToString(buf)
}

func invokeNodeService(serviceName string, input map[string]interface{}) []byte {
	task := &NSTask{
		invokeId: newInvokeId(),
		service:  serviceName,
		input:    input,
		output:   make(chan []byte, 1),
	}
	nsChannel <- task
	select {
	case out := <-task.output:
		return out
	case <-time.After(15 * time.Second):
		nsTasks.Delete(task.invokeId)
		return []byte(`{"error": "ns timeout"}`)
	}
}

func startNodeServices(ctx context.Context, wd string, services []string) (err error) {
	pidFile := path.Join(wd, "ns.pid")
	errBuf := bytes.NewBuffer(nil)
	servicesInject := "[]"

	// install services
	if len(services) > 0 {
		cmd := exec.Command("yarn", append([]string{"add", "async"}, services...)...)
		cmd.Dir = wd
		var output []byte
		output, err = cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("install services: %v %s", err, string(output))
			return
		}
		data, _ := json.Marshal(services)
		servicesInject = string(data)
		log.Debug("node services", services, "installed")
	}

	// create ns script
	err = ioutil.WriteFile(
		path.Join(wd, "ns.js"),
		[]byte(fmt.Sprintf(nsApp, servicesInject, runtime.NumCPU())),
		0644,
	)
	if err != nil {
		return
	}

	// kill previous node process if exists
	kill(pidFile)

	cmd := exec.Command("node", "ns.js")
	cmd.Dir = wd
	cmd.Stderr = errBuf

	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	defer in.Close()

	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	defer out.Close()

	err = cmd.Start()
	if err != nil {
		return
	}

	log.Debug("node services process started, pid is", cmd.Process.Pid)

	// store node process pid
	ioutil.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	go func() {
	loop:
		for {
			if nsReady {
				select {
				case <-ctx.Done():
					cmd.Process.Kill()
					break loop
				case nsTask := <-nsChannel:
					invokeId := nsTask.invokeId
					data, err := json.Marshal(map[string]interface{}{
						"invokeId": invokeId,
						"service":  nsTask.service,
						"input":    nsTask.input,
					})
					if err == nil {
						nsTasks.Store(invokeId, nsTask.output)
						_, err = in.Write(data)
						if err != nil {
							nsTasks.Delete(invokeId)
						}
						_, err = in.Write([]byte{'\n'})
						if err != nil {
							nsTasks.Delete(invokeId)
						}
					}
				}
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Bytes()
			if string(line) == "READY" {
				nsReady = true
			} else if len(line) > 8 {
				invokeId := string(line[:8])
				v, ok := nsTasks.LoadAndDelete(invokeId)
				if ok {
					v.(chan []byte) <- line[8:]
				}
			}
		}
	}()

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
	"lz-string",
	"safe-buffer",
	"stream-http",
	"typescript",
	"seedrandom",
	"lru_map",
	"keycode",
	"vscode-oniguruma",
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

	data := invokeNodeService("parseCjsExports", args)
	err = json.Unmarshal(data, &ret)
	if err == nil && ret.Error != "" {
		if ret.Stack != "" {
			log.Errorf("[ns] parseCJSModuleExports: %s\n---\n%s\n---", ret.Error, ret.Stack)
		} else {
			log.Errorf("[ns] parseCJSModuleExports: %s", ret.Error)
		}
	}
	return
}
