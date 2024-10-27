package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type LoaderWorker struct {
	stdin     io.Writer
	stdout    io.Reader
	outReader *bufio.Reader
	lock      sync.Mutex
}

func (lw *LoaderWorker) Start(loaderjs []byte) (err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	jsPath := filepath.Join(homeDir, ".esm.sh", "run", fmt.Sprintf("loader@%d.js", VERSION))
	fi, err := os.Stat(jsPath)
	if (err != nil && os.IsNotExist(err)) || (err == nil && fi.Size() != int64(len(loaderjs))) || os.Getenv("DEBUG") == "1" {
		os.MkdirAll(filepath.Dir(jsPath), 0755)
		err = os.WriteFile(jsPath, loaderjs, 0644)
		if err != nil {
			return
		}
	}
	denoPath, err := getDenoPath()
	if err != nil {
		err = errors.New("deno not found, please install deno first")
		return
	}

	cmd := exec.Command(denoPath, "run", "--no-lock", "-A", jsPath)
	cmd.Stdin, lw.stdin = io.Pipe()
	lw.stdout, cmd.Stdout = io.Pipe()
	err = cmd.Start()
	if err != nil {
		lw.stdin = nil
		lw.stdout = nil
	} else {
		lw.outReader = bufio.NewReader(lw.stdout)
		if os.Getenv("DEBUG") == "1" {
			denoVersion, _ := exec.Command(denoPath, "-v").Output()
			fmt.Printf("Loader started (runtime: %s)\n", strings.TrimSpace(string(denoVersion)))
		}
	}
	return
}

func (lw *LoaderWorker) Load(loaderType string, args ...any) (code string, err error) {
	// only one load can be invoked at a time
	lw.lock.Lock()
	defer lw.lock.Unlock()
	if lw.outReader == nil {
		return "", errors.New("loader not started")
	}
	if os.Getenv("DEBUG") == "1" {
		start := time.Now()
		defer func() {
			fmt.Printf("Loader.Load(%s) took %s\n", loaderType, time.Since(start))
		}()
	}
	loaderArgs := make([]any, len(args)+1)
	loaderArgs[0] = loaderType
	copy(loaderArgs[1:], args)
	err = json.NewEncoder(lw.stdin).Encode(loaderArgs)
	if err != nil {
		return
	}
	for {
		line, err := lw.outReader.ReadBytes('\n')
		if err != nil {
			return "", err
		}
		if len(line) > 3 {
			if bytes.HasPrefix(line, []byte(">>>\"")) || bytes.HasPrefix(line, []byte(">>!\"")) {
				var ret string
				err = json.Unmarshal(line[3:], &ret)
				if err != nil {
					return "", err
				}
				if line[2] == '!' {
					return "", errors.New(ret)
				}
				return ret, nil
			}
		}
	}
}

var lock sync.Mutex

func getDenoPath() (denoPath string, err error) {
	lock.Lock()
	defer lock.Unlock()

	denoPath, err = exec.LookPath("deno")
	if err != nil {
		fmt.Println("Installing deno...")
		denoPath, err = installDeno()
	}
	return
}

func installDeno() (string, error) {
	isWin := runtime.GOOS == "windows"
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if !isWin {
		denoPath := filepath.Join(homeDir, ".deno/bin/deno")
		fi, err := os.Stat(denoPath)
		if err == nil && fi.Mode().IsRegular() {
			return denoPath, nil
		}
	}
	installScriptUrl := "https://deno.land/install.sh"
	scriptExe := "sh"
	if isWin {
		installScriptUrl = "https://deno.land/install.ps1"
		scriptExe = "iex"
	}
	res, err := http.Get(installScriptUrl)
	if err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", errors.New("failed to get latest deno version")
	}
	defer res.Body.Close()
	cmd := exec.Command(scriptExe)
	cmd.Stdin = res.Body
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	if isWin {
		return exec.LookPath("deno")
	}
	return filepath.Join(homeDir, ".deno/bin/deno"), nil
}