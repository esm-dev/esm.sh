package server

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const nsApp = `
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

	for (const name of register) {
		const { serviceName, main } = require(name)
		services[serviceName] = main
	}

	rl.on('line', async line => {
		if (line.charAt(0) === '{' && line.charAt(line.length-1) === '}') {
			try {
				const { service, invokeId, input } = JSON.parse(line)
				if (typeof invokeId === 'string') {
					let output = null
					try {
						if (typeof service === 'string' && service in services) {
							output = await services[service](input)
						} else {
							output = { error: 'service not found' }
						}
					} catch(e) {
						output = { error: e.message }
					}
					process.stdout.write(invokeId)
					process.stdout.write(JSON.stringify(output))
					process.stdout.write('\n')
				}
			} catch(e) {}
		}
	})

	setTimeout(() => {
		process.stdout.write('READY\n')
	}, 0)
`

type NSTask struct {
	service string
	input   map[string]interface{}
	output  chan []byte
}

var nsReady = false
var nsInvokeIndex uint32 = 0
var nsChannel = make(chan *NSTask, 64)

func invokeNodeService(serviceName string, input map[string]interface{}) chan []byte {
	task := &NSTask{
		service: serviceName,
		input:   input,
		output:  make(chan []byte, 1),
	}
	nsChannel <- task
	return task.output
}

func startNodeServices(wd string, services []string) (err error) {
	pidFile := path.Join(wd, "ns.pid")
	errBuf := bytes.NewBuffer(nil)
	servicesInject := "[]"

	// install services
	if len(services) > 0 {
		cmd := exec.Command("yarn", append([]string{"add"}, services...)...)
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

	// create ns app js
	err = ioutil.WriteFile(
		path.Join(wd, "ns.js"),
		[]byte(fmt.Sprintf(nsApp, servicesInject)),
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

	nsReady = true
	log.Debug("node services process started, pid is", cmd.Process.Pid)

	// store node process pid
	ioutil.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	var tasks sync.Map
	var ready bool

	go func() {
		for {
			if ready {
				nsTask := <-nsChannel
				invokeId := atomic.AddUint32(&nsInvokeIndex, 1)
				buf := make([]byte, 4)
				binary.LittleEndian.PutUint32(buf, invokeId)
				invokeIdHex := hex.EncodeToString(buf)
				data, err := json.Marshal(map[string]interface{}{
					"invokeId": invokeIdHex,
					"service":  nsTask.service,
					"input":    nsTask.input,
				})
				if err == nil {
					tasks.Store(invokeIdHex, nsTask.output)
					_, err = in.Write(data)
					if err != nil {
						tasks.Delete(invokeId)
					}
					_, err = in.Write([]byte{'\n'})
					if err != nil {
						tasks.Delete(invokeId)
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
			// if llen == 9 && line[0] == '$' {
			// 	invokeId := string(line[1:])
			// 	v, ok := tasks.LoadAndDelete(invokeId)
			// 	if ok {
			// 		v.(chan []byte) <- nil // end
			// 	}
			// }
			if string(line) == "READY" {
				ready = true
			} else if llen := len(line); llen > 8 {
				invokeId := string(line[:8])
				v, ok := tasks.Load(invokeId)
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
