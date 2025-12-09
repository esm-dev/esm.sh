package web

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/deno"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

type JSWorker struct {
	wd        string
	config    string
	script    string
	lock      sync.Mutex
	process   *os.Process
	stdin     io.Writer
	stdout    io.Reader
	outReader *bufio.Reader
}

func (jsw *JSWorker) Start() (err error) {
	js, err := efs.ReadFile("internal/" + jsw.script)
	if err != nil {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	workDir := filepath.Join(homeDir, ".esm.sh")
	if runtime.GOOS == "windows" {
		workDir = filepath.Join(homeDir, "AppData\\Local\\esm.sh")
	}

	jsPath := filepath.Join(workDir, jsw.script)

	os.MkdirAll(workDir, 0755)
	err = os.WriteFile(jsPath, js, 0644)
	if err != nil {
		return
	}

	denoPath := deno.GetDenoPath(workDir)
	err = deno.CheckDeno(denoPath)
	if err != nil {
		return
	}

	args := []string{
		"run",
		"--allow-read=" + homeDir,
		"--allow-write=" + homeDir,
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
		var line string
		line, err = jsw.outReader.ReadString('\n')
		if err != nil {
			return
		}
		if len(line) > 3 && strings.HasPrefix(line, ">>>") {
			flag, data := utils.SplitByFirstByte(line[3:], ':')
			if flag == "debug" || flag == "error" {
				var msg string
				err = json.Unmarshal([]byte(data), &msg)
				if err != nil {
					return
				}
				if flag == "debug" {
					fmt.Println(term.Dim(msg))
					continue
				}
				err = errors.New(msg)
			} else {
				format = flag
				output = data
			}
			return
		}
	}
}
