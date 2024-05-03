package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// allowlist for _invoke_ mode
var requireModeAllowList = []string{
	"@babel/types",
	"cheerio",
	"graceful-fs",
	"he",
	"jsbn",
	"netmask",
	"xml2js",
	"keycode",
	"lru_map",
	"lz-string",
	"maplibre-gl",
	"pako",
	"postcss-selector-parser",
	"react-draggable",
	"resolve",
	"safe-buffer",
	"seedrandom",
	"stream-browserify",
	"stream-http",
	"typescript",
	"vscode-oniguruma",
	"web-streams-ponyfill",
}

func initCJSLexerNodeApp() (err error) {
	wd := path.Join(cfg.WorkDir, "ns")
	err = ensureDir(wd)
	if err != nil {
		return err
	}

	// install dependencies
	cmd := exec.Command("pnpm", "i", "enhanced-resolve@5.16.0", "esm-cjs-lexer@0.10.0")
	cmd.Dir = wd
	var output []byte
	output, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("install services: %v %s", err, string(output))
		return
	}

	// create cjs_lexer.js
	js, err := embedFS.ReadFile("server/embed/cjs_lexer.js")
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(wd, "cjs_lexer.js"), js, 0644)
	return
}

type cjsLexerResult struct {
	Reexport         string   `json:"reexport,omitempty"`
	HasDefaultExport bool     `json:"hasDefaultExport"`
	NamedExports     []string `json:"namedExports"`
	Error            string   `json:"error"`
	Stack            string   `json:"stack"`
}

func cjsLexer(cwd string, specifier string, nodeEnv string) (ret cjsLexerResult, err error) {
	start := time.Now()
	args := map[string]interface{}{
		"cwd":       cwd,
		"specifier": specifier,
		"nodeEnv":   nodeEnv,
	}

	/* workaround for edge cases that can't be parsed by cjsLexer correctly */
	for _, name := range requireModeAllowList {
		if specifier == name || strings.HasPrefix(specifier, name+"/") {
			args["requireMode"] = 1
			break
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	cmd := exec.CommandContext(ctx, "node", "--experimental-permission", "--allow-fs-read=*", "cjs_lexer.js")
	cmd.Dir = path.Join(cfg.WorkDir, "ns")
	cmd.Stdin = bytes.NewBuffer(mustEncodeJSON(args))
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = fmt.Errorf("cjsLexer: %s", errBuf.String())
		}
		return
	}

	err = json.Unmarshal(outBuf.Bytes(), &ret)
	if err != nil {
		return
	}

	if ret.Error != "" {
		if ret.Stack != "" {
			log.Errorf("[cjsLexer] %s\n---\n%s\n---", ret.Error, ret.Stack)
		} else {
			log.Errorf("[cjsLexer] %s", ret.Error)
		}
	} else {
		log.Debugf("[cjsLexer] parse %s in %s", specifier, time.Since(start))
	}

	return
}
