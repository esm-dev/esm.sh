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

	"github.com/esm-dev/esm.sh/server/common"
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

	jsPath := filepath.Join(homeDir, ".esmd", "run", fmt.Sprintf("loader@%d.js", VERSION))
	fi, err := os.Stat(jsPath)
	if (err != nil && os.IsNotExist(err)) || (err == nil && fi.Size() != int64(len(loaderJS))) || DEBUG {
		os.MkdirAll(filepath.Dir(jsPath), 0755)
		err = os.WriteFile(jsPath, loaderJS, 0644)
		if err != nil {
			return
		}
	}

	denoPath, err := common.GetDenoPath("")
	if err != nil {
		err = errors.New("deno not found, please install deno first")
		return
	}

	cmd := exec.Command(denoPath, "run", "--no-config", "--no-lock", "-A", "--quiet", jsPath)
	cmd.Dir = wd
	cmd.Stdin, l.stdin = io.Pipe()
	l.stdout, cmd.Stdout = io.Pipe()

	err = cmd.Start()
	if err != nil {
		l.stdin = nil
		l.stdout = nil
	} else {
		l.outReader = bufio.NewReader(l.stdout)
		if DEBUG {
			denoVersion, _ := exec.Command(denoPath, "-v").Output()
			fmt.Println(term.Dim(fmt.Sprintf("[debug] loader worker started (runtime: %s)", strings.TrimSpace(string(denoVersion)))))
		}
	}

	return
}

func (l *LoaderWorker) Load(loaderType string, args []any) (lang string, code string, err error) {
	// only one load call can be invoked at a time
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.outReader == nil {
		err = errors.New("loader worker not started")
		return
	}

	if DEBUG {
		start := time.Now()
		defer func() {
			fmt.Println(term.Dim(fmt.Sprintf("[debug] load '%s' in %s (loader: %s)", args[0], time.Since(start), loaderType)))
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
