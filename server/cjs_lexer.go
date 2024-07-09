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
)

const cjsLexerPkg = "esm-cjs-lexer@0.11.2"

// use `require()` to get the module's exports that are not statically analyzable by esm-cjs-lexer
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
	wd := path.Join(config.WorkDir, "npm", cjsLexerPkg)
	err = ensureDir(wd)
	if err != nil {
		return err
	}

	// ensure 'package.json' file to prevent read up-levels
	packageJsonFp := path.Join(wd, "package.json")
	if !existsFile(packageJsonFp) {
		err = os.WriteFile(packageJsonFp, []byte("{}"), 0644)
		if err != nil {
			return
		}
	}

	cmd := exec.Command("pnpm", "add", "--prefer-offline", cjsLexerPkg)
	cmd.Dir = wd
	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("install %s: %v", cjsLexerPkg, err)
		return
	}

	js, err := embedFS.ReadFile("server/embed/cjs_lexer.js")
	if err != nil {
		return
	}

	minJs, err := minify(string(js), api.ESNext, api.LoaderJS)
	if err != nil {
		return
	}

	err = os.WriteFile(path.Join(wd, "cjs_lexer.js"), minJs, 0644)
	return
}

type cjsLexerResult struct {
	ReExport         string   `json:"reexport,omitempty"`
	HasDefaultExport bool     `json:"hasDefaultExport"`
	NamedExports     []string `json:"namedExports"`
	Error            string   `json:"error"`
	Stack            string   `json:"stack"`
}

func cjsLexer(npmrc *NpmRC, pkgName string, wd string, specifier string, nodeEnv string) (ret cjsLexerResult, err error) {
	h := sha256.New()
	h.Write([]byte(cjsLexerPkg))
	h.Write([]byte(specifier))
	h.Write([]byte(nodeEnv))
	cacheFileName := path.Join(wd, ".cjs_lexer", base64.RawURLEncoding.EncodeToString(h.Sum(nil))+".json")

	// check the cache first
	if existsFile(cacheFileName) && parseJSONFile(cacheFileName, &ret) == nil {
		return
	}

	// change the args order carefully, the order is used in ./embed/cjs_lexer.js
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
	argsData := mustEncodeJSON(args)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var (
		outBuf bytes.Buffer
		errBuf bytes.Buffer
	)
	cmd := exec.CommandContext(
		ctx,
		"node",
		"--experimental-permission",
		"--allow-fs-read="+npmrc.NpmDir(),
		"cjs_lexer.js",
	)
	cmd.Dir = path.Join(config.WorkDir, "npm", cjsLexerPkg)
	cmd.Stdin = bytes.NewBuffer(argsData)
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
			log.Errorf("[cjsLexer] %s\n---\nArguments: %v\n%s\na---", ret.Error, args, ret.Stack)
		} else {
			log.Errorf("[cjsLexer] %s\nArguments: %v", ret.Error, args)
		}
	} else {
		go func() {
			if ensureDir(path.Dir(cacheFileName)) == nil {
				os.WriteFile(cacheFileName, outBuf.Bytes(), 0644)
			}
		}()
		log.Debugf("[cjsLexer] parse %s in %s", path.Join(pkgName, specifier), time.Since(start))
	}

	return
}
