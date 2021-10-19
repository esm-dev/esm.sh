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
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const nsApp = `
	const readline = require('readline')
	const rl = readline.createInterface({
		input: process.stdin,
		history: 0,
		crlfDelay: Infinity,
		terminal: false,
	})
	const services = {
		test: async input => ({ foo: input.foo })
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
`

type NSTask struct {
	service string
	input   map[string]interface{}
	output  chan []byte
}

var invokeIndex uint32 = 0
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

func startNodeServices(quiteSignal chan bool, wd string, services []string) (err error) {
	ensureDir(wd)

	// install services
	servicesInject := "[]"
	if len(services) > 0 {
		cmd := exec.Command("yarn", append([]string{"add"}, services...)...)
		cmd.Dir = wd
		var output []byte
		output, err = cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("install services: %s", string(output))
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
	pidFile := path.Join(wd, "ns.pid")
	if data, err := ioutil.ReadFile(pidFile); err == nil {
		if i, err := strconv.Atoi(string(data)); err == nil {
			if p, err := os.FindProcess(i); err == nil {
				p.Kill()
			}
		}
	}

	errBuf := bytes.NewBuffer(nil)
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

	var tasks sync.Map

	go func() {
		for {
			nsTask := <-nsChannel
			invokeId := atomic.AddUint32(&invokeIndex, 1)
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
		}
	}()

	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Bytes()
			llen := len(line)
			// if llen == 9 && line[0] == '$' {
			// 	invokeId := string(line[1:])
			// 	v, ok := tasks.LoadAndDelete(invokeId)
			// 	if ok {
			// 		v.(chan []byte) <- nil // end
			// 	}
			// }
			if llen > 8 {
				invokeId := string(line[:8])
				v, ok := tasks.Load(invokeId)
				if ok {
					v.(chan []byte) <- line[8:]
				}
			}
		}
	}()

	if quiteSignal != nil {
		go func() {
			<-quiteSignal
			cmd.Process.Kill()
		}()
	}

	// wait the process to exit
	cmd.Wait()

	if errBuf.Len() > 0 {
		err = errors.New(strings.TrimSpace(errBuf.String()))
	}
	return
}
