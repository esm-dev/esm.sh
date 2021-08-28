package server

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func TestParseCJSExports(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	ensureDir(testDir)

	config = &Config{
		cjsLexerServerPort: 8088,
	}
	go func() {
		err := startCJSLexerServer(config.cjsLexerServerPort, path.Join(testDir, "cjs-lexer.pid"), true)
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
}
