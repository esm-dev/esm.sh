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

	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

type LoaderWorker struct {
	lock      sync.Mutex
	stdin     io.Writer
	stdout    io.Reader
	outReader *bufio.Reader
}

func (l *LoaderWorker) Start(wd string, loaderJS []byte) (err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	jsPath := filepath.Join(homeDir, ".esm.sh", "run", fmt.Sprintf("loader@%d.js", VERSION))
	fi, err := os.Stat(jsPath)
	if (err != nil && os.IsNotExist(err)) || (err == nil && fi.Size() != int64(len(loaderJS))) || debug {
		os.MkdirAll(filepath.Dir(jsPath), 0755)
		err = os.WriteFile(jsPath, loaderJS, 0644)
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
	cmd.Dir = wd
	cmd.Stdin, l.stdin = io.Pipe()
	l.stdout, cmd.Stdout = io.Pipe()

	err = cmd.Start()
	if err != nil {
		l.stdin = nil
		l.stdout = nil
	} else {
		l.outReader = bufio.NewReader(l.stdout)
		if debug {
			denoVersion, _ := exec.Command(denoPath, "-v").Output()
			fmt.Println(term.Dim(fmt.Sprintf("[debug] loader process started (runtime: %s)", strings.TrimSpace(string(denoVersion)))))
		}
	}

	// pre-install npm deps
	cmd = exec.Command(denoPath, "cache", "npm:@esm.sh/unocss@0.4.2", "npm:@esm.sh/tsx@1.0.5", "npm:@esm.sh/vue-compiler@1.0.1")
	cmd.Start()
	return
}

func (l *LoaderWorker) Load(loaderType string, args []any) (lang string, code string, err error) {
	// only one load can be invoked at a time
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.outReader == nil {
		err = errors.New("loader not started")
		return
	}

	if debug {
		start := time.Now()
		defer func() {
			if loaderType == "unocss" {
				fmt.Println(term.Dim(fmt.Sprintf("[debug] load '/@uno.css' in %s (loader: unocss)", time.Since(start))))
			} else {
				fmt.Println(term.Dim(fmt.Sprintf("[debug] load '%s' in %s (loader: %s)", args[0], time.Since(start), loaderType)))
			}
		}()
	}

	loaderArgs := make([]any, len(args)+1)
	loaderArgs[0] = loaderType
	copy(loaderArgs[1:], args)
	err = json.NewEncoder(l.stdin).Encode(loaderArgs)
	if err != nil {
		return
	}
	for {
		var line []byte
		line, err = l.outReader.ReadBytes('\n')
		if err != nil {
			return
		}
		if len(line) > 3 {
			if bytes.HasPrefix(line, []byte(">>>")) {
				var s string
				t, d := utils.SplitByFirstByte(string(line[3:]), ':')
				err = json.Unmarshal([]byte(d), &s)
				if err != nil {
					return
				}
				if t == "debug" {
					fmt.Println(term.Dim(s))
					continue
				}
				if t == "error" {
					err = errors.New(s)
				} else {
					lang = t
					code = s
				}
				return
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
