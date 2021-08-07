package server

import (
	"os"
	"path"
	"testing"
	"time"
)

func TestParseCJSExports(t *testing.T) {
	testDir := path.Join(os.TempDir(), "test")
	os.RemoveAll(testDir)
	ensureDir(testDir)

	err := yarnAdd(testDir, "react")
	if err != nil {
		t.Fatal(err)
	}

	config = &Config{
		cjsLexerServerPort: 8088,
	}
	go startCJSLexerServer(config.cjsLexerServerPort, true)

	time.Sleep(time.Second)

	ret, err := parseCJSModuleExports(testDir, "react")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(ret)
}
