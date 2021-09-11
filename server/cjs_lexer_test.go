package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseCJSExports(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	pidFile := path.Join(testDir, "cjs-lexer.pid")
	ensureDir(testDir)

	config = &Config{
		cjsLexerServerPort: 8088,
	}
	go func() {
		err := startCJSLexerServer(config.cjsLexerServerPort, pidFile, true)
		if err != nil {
			fmt.Println("startCJSLexerServer:", err)
		}
	}()

	err := yarnAdd(testDir, "react@17", "path-browserify")
	if err != nil {
		t.Fatal(err)
	}

	// wait cjs-lexer server
	time.Sleep(time.Second / 2)

	ret, err := parseCJSModuleExports(testDir, "react", "development")
	if err != nil {
		t.Fatal(err)
	}
	if ret.Error != "" {
		t.Fatal(ret.Error)
	}
	if !strings.Contains(strings.Join(ret.Exports, ","), "createElement") {
		t.Fatal("missing `createElement` export")
	}
	t.Log(ret.Exports)

	ret, err = parseCJSModuleExports(testDir, "path-browserify", "development")
	if err != nil {
		t.Fatal(err)
	}
	if ret.Error != "" {
		t.Fatal(ret.Error)
	}
	if !strings.Contains(strings.Join(ret.Exports, ","), "basename") {
		t.Fatal("missing `basename` export")
	}
	t.Log(ret.Exports)

	// kill the cjs-lexer node process
	if data, err := ioutil.ReadFile(pidFile); err == nil {
		if i, err := strconv.Atoi(string(data)); err == nil {
			if p, err := os.FindProcess(i); err == nil {
				p.Kill()
			}
		}
	}
}
