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

	cmd := exec.Command("npm", "i", "--no-package-lock")
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

	err = os.WriteFile(path.Join(wd, "cjs_module_lexer.cjs"), minJs, 0644)
	return
}

type cjsModuleLexerResult struct {
	ReExport         string   `json:"reexport"`
	HasDefaultExport bool     `json:"hasDefaultExport"`
	NamedExports     []string `json:"namedExports"`
	Error            string   `json:"error"`
	Stack            string   `json:"stack"`
}

func (ctx *BuildContext) cjsModuleLexer(specifier string, nodeEnv string) (ret cjsModuleLexerResult, err error) {
	h := sha256.New()
	h.Write([]byte(cjsModuleLexerPkg))
	h.Write([]byte(ctx.esmPath.PkgName))
	h.Write([]byte(specifier))
	h.Write([]byte(nodeEnv))
	cacheFileName := path.Join(ctx.wd, ".cjs_module_lexer", base64.RawURLEncoding.EncodeToString(h.Sum(nil))+".json")

	// check the cache first
	if existsFile(cacheFileName) && utils.ParseJSONFile(cacheFileName, &ret) == nil {
		return
	}

	lexerWd := path.Join(config.WorkDir, "npm", cjsModuleLexerPkg)
	if !existsFile(path.Join(lexerWd, "cjs_module_lexer.cjs")) {
		err = initCJSModuleLexer()
		if err != nil {
			return
		}
	}

	requireMode := false
	for _, name := range requireModeAllowList {
		if ctx.esmPath.PkgName == name || specifier == name || strings.HasPrefix(specifier, name+"/") {
			requireMode = true
			break
		}
	}

	// change the args order carefully, the order is used in ./embed/internal/cjs_module_lexer.js
	args := []interface{}{
		ctx.wd,
		ctx.esmPath.PkgName,
		specifier,
		nodeEnv,
	}
	nodeArgs := []string{
		"--experimental-permission",
		"--allow-fs-read=" + lexerWd,
		"--allow-fs-read=" + ctx.npmrc.StoreDir(),
		"cjs_module_lexer.cjs",
	}
	if requireMode {
		args = append(args, true)
		// install dependencies & peerDependencies for require mode
		ctx.installDependencies(ctx.packageJson, true)
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, "node", nodeArgs...)
	stdin := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cmd.Dir = lexerWd
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	start := time.Now()
	json.NewEncoder(stdin).Encode(args)
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
		log.Debugf("[cjsModuleLexer] parse %s in %s", path.Join(ctx.esmPath.PkgName, specifier), time.Since(start))
	}
	return
}
