package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

const cjsModuleLexerPkg = "@esm.sh/cjs-module-lexer@1.0.1"

// use `require()` to get the module's exports that are not statically analyzable by @esm.sh/cjs-module-lexer
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

func initCJSModuleLexer() (err error) {
	wd := path.Join(config.WorkDir, "npm", cjsModuleLexerPkg)
	err = ensureDir(wd)
	if err != nil {
		return err
	}

	err = os.WriteFile(path.Join(wd, "package.json"), []byte(`{"dependencies":{"@esm.sh/cjs-module-lexer":"npm:`+cjsModuleLexerPkg+`"}}`), 0644)
	if err != nil {
		return
	}

	cmd := exec.Command("pnpm", "i", "--prefer-offline")
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := err.Error()
		if len(output) > 0 {
			msg = string(output)
		}
		err = fmt.Errorf("install %s: %v", cjsModuleLexerPkg, msg)
		return
	}

	js, err := embedFS.ReadFile("server/embed/internal/cjs_module_lexer.js")
	if err != nil {
		return
	}

	minJs, err := minify(string(js), api.LoaderJS, api.ESNext)
	if err != nil {
		return
	}

	err = os.WriteFile(path.Join(wd, "cjs_module_lexer.js"), minJs, 0644)
	return
}

type cjsModuleLexerResult struct {
	ReExport         string   `json:"reexport"`
	HasDefaultExport bool     `json:"hasDefaultExport"`
	NamedExports     []string `json:"namedExports"`
	Error            string   `json:"error"`
	Stack            string   `json:"stack"`
}

func cjsModuleLexer(npmrc *NpmRC, pkgName string, wd string, specifier string, nodeEnv string) (ret cjsModuleLexerResult, err error) {
	h := sha256.New()
	h.Write([]byte(cjsModuleLexerPkg))
	h.Write([]byte(pkgName))
	h.Write([]byte(wd))
	h.Write([]byte(specifier))
	h.Write([]byte(nodeEnv))
	cacheFileName := path.Join(wd, ".cjs_module_lexer", base64.RawURLEncoding.EncodeToString(h.Sum(nil))+".json")

	// check the cache first
	if existsFile(cacheFileName) && utils.ParseJSONFile(cacheFileName, &ret) == nil {
		return
	}

	// change the args order carefully, the order is used in ./embed/cjs_module_lexer.js
	args := []interface{}{
		pkgName,
		wd,
		specifier,
		nodeEnv,
	}
	for _, name := range requireModeAllowList {
		if pkgName == name || specifier == name || strings.HasPrefix(specifier, name+"/") {
			args = append(args, true)
			break
		}
	}

	stdin := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	err = json.NewEncoder(stdin).Encode(args)
	if err != nil {
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"node",
		"--experimental-permission",
		"--allow-fs-read="+npmrc.StoreDir(),
		"cjs_module_lexer.js",
	)
	cmd.Dir = path.Join(config.WorkDir, "npm", cjsModuleLexerPkg)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("cjsModuleLexer: %s", stderr.String())
		}
		return
	}

	err = json.Unmarshal(stdout.Bytes(), &ret)
	if err != nil {
		return
	}

	if ret.Error != "" {
		if ret.Stack != "" {
			log.Errorf("[cjsModuleLexer] %s\n---\nArguments: %v\n%s\na---", ret.Error, args, ret.Stack)
		} else {
			log.Errorf("[cjsModuleLexer] %s\nArguments: %v", ret.Error, args)
		}
	} else {
		go func() {
			if ensureDir(path.Dir(cacheFileName)) == nil {
				os.WriteFile(cacheFileName, stdout.Bytes(), 0644)
			}
		}()
		log.Debugf("[cjsModuleLexer] parse %s in %s", path.Join(pkgName, specifier), time.Since(start))
	}

	return
}
