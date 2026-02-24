package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/app_dir"
	"github.com/esm-dev/esm.sh/internal/deno"
	"github.com/ije/gox/term"
)

type JSWorker struct {
	wd        string
	config    string
	script    string
	lock      sync.Mutex
	stdin     io.Writer
	stdout    io.Reader
	process   *os.Process
	outReader *bufio.Reader
}

func (jsw *JSWorker) Start() (err error) {
	js, err := efs.ReadFile("internal/" + jsw.script)
	if err != nil {
		return
	}

	appDir, err := app_dir.GetAppDir()
	if err != nil {
		return
	}

	jsPath := filepath.Join(appDir, jsw.script)
	os.MkdirAll(appDir, 0755)
	err = os.WriteFile(jsPath, js, 0644)
	if err != nil {
		return
	}

	denoPath := deno.ResolveDenoPath(appDir)
	err = deno.CheckDenoPath(denoPath)
	if err != nil {
		return
	}

	args := []string{
		"run",
		"--allow-read",
		"--allow-write",
		"--allow-env",
		"--allow-net",
		"--allow-sys",
		"--no-prompt",
		"--no-lock",
		"--quiet",
	}
	if jsw.config != "" {
		args = append(args, "--config", jsw.config)
	} else {
		args = append(args, "--no-config")
	}
	args = append(args, jsPath)
	cmd := exec.Command(denoPath, args...)
	cmd.Env = append(os.Environ(), "DENO_NO_UPDATE_CHECK=1", "DENO_NO_PACKAGE_JSON=1")
	cmd.Dir = jsw.wd
	cmd.Stdin, jsw.stdin = io.Pipe()
	jsw.stdout, cmd.Stdout = io.Pipe()

	err = cmd.Start()
	if err != nil {
		jsw.stdin = nil
		jsw.stdout = nil
		return
	}

	jsw.process = cmd.Process
	jsw.outReader = bufio.NewReader(jsw.stdout)
	if DEBUG {
		cmd := exec.Command(denoPath, "-v")
		cmd.Env = append(os.Environ(), "DENO_NO_UPDATE_CHECK=1")
		denoVersion, _ := cmd.Output()
		fmt.Println(term.Dim(fmt.Sprintf("[debug] js worker started (runtime: %s)", strings.TrimSpace(string(denoVersion)))))
	}
	return
}

func (jsw *JSWorker) Stop() (err error) {
	if jsw.process != nil {
		jsw.process.Kill()
		jsw.process = nil
	}
	jsw.stdin = nil
	jsw.stdout = nil
	jsw.outReader = nil
	if DEBUG {
		fmt.Println(term.Dim(fmt.Sprintf("[debug] js worker stopped (runtime: %s)", jsw.script)))
	}
	return
}

func (jsw *JSWorker) Call(args ...any) (format string, output string, err error) {
	// only one load call can be invoked at a time
	jsw.lock.Lock()
	defer jsw.lock.Unlock()

	if jsw.outReader == nil {
		err = errors.New("js worker not started")
		return
	}

	if DEBUG {
		start := time.Now()
		defer func() {
			fmt.Println(term.Dim(fmt.Sprintf("[debug] call %s#%s(%s) in %s", jsw.script, args[0], args[1], time.Since(start))))
		}()
	}

	err = json.NewEncoder(jsw.stdin).Encode(args)
	if err != nil {
		return
	}
	for {
		var line []byte
		line, err = jsw.outReader.ReadBytes('\n')
		if err != nil {
			return
		}
		if len(line) > 3 && bytes.HasPrefix(line, []byte{'>', '>', '>'}) {
			data := line[3:]
			index := bytes.IndexByte(data, ':')
			if index == -1 {
				// ignore invalid message
				continue
			}
			var str string
			err = json.Unmarshal(data[index+1:], &str)
			if err != nil {
				// ignore invalid message
				return
			}
			flag := string(data[:index])
			switch flag {
			case "debug":
				fmt.Println(term.Dim(str))
			case "error":
				err = errors.New(str)
				return
			default:
				format = flag
				output = str
				return
			}
		}
	}
}
