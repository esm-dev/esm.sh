package server

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

var cjsModuleLexerVersion = "1.0.6"
var cjsModuleLexerIgnoredPackages = set.New[string](
	"@babel/types",
	"cheerio",
	"graceful-fs",
	"he",
	"lodash",
	"lz-string",
	"maplibre-gl",
	"pako",
	"postcss-selector-parser",
	"react-draggable",
	"safe-buffer",
	"stream-browserify",
	"typescript",
	"vscode-oniguruma",
	"web-streams-ponyfill",
)

type cjsModuleLexerResult struct {
	Exports  []string `json:"exports,omitempty"`
	Reexport string   `json:"reexport,omitempty"`
}

func cjsModuleLexer(ctx *BuildContext, cjsEntry string) (ret cjsModuleLexerResult, err error) {
	h := sha1.New()
	h.Write([]byte(cjsModuleLexerVersion))
	h.Write([]byte(cjsEntry))
	h.Write([]byte(ctx.getNodeEnv()))
	cacheFileName := path.Join(ctx.wd, ".cache", "cml-"+base64.RawURLEncoding.EncodeToString(h.Sum(nil))+".json")

	// check the cache first
	if existsFile(cacheFileName) && utils.ParseJSONFile(cacheFileName, &ret) == nil {
		return
	}

	start := time.Now()
	defer func() {
		if err == nil {
			if DEBUG {
				ctx.logger.Debugf("[cjsModuleLexer] parse %s in %s", path.Join(ctx.esm.PkgName, cjsEntry), time.Since(start))
			}
			if !existsFile(cacheFileName) {
				ensureDir(path.Dir(cacheFileName))
				utils.WriteJSONFile(cacheFileName, ret, "")
			}
		}
	}()

	if cjsModuleLexerIgnoredPackages.Has(ctx.esm.PkgName) {
		js := path.Join(ctx.wd, "reveal_"+strings.ReplaceAll(cjsEntry[2:], "/", "_"))
		err = os.WriteFile(js, []byte(fmt.Sprintf(`console.log(JSON.stringify(Object.keys((await import("npm:%s")).default)))`, path.Join(ctx.esm.Name(), cjsEntry))), 0644)
		if err != nil {
			return
		}
		var data []byte
		data, err = run("deno", "run", "--no-config", "--no-lock", "--no-prompt", "--quiet", js)
		if err != nil {
			return
		}
		var namedExports []string
		err = json.Unmarshal(data, &namedExports)
		if err != nil {
			return
		}
		for _, name := range namedExports {
			if !isJsReservedWord(name) {
				ret.Exports = append(ret.Exports, name)
			}
		}
		return
	}

	worthToRetry := true
RETRY:

	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(c, "cjs-module-lexer", path.Join(ctx.esm.PkgName, cjsEntry))
	stdout, recycle := NewBuffer()
	defer recycle()
	stderr, recycle := NewBuffer()
	defer recycle()
	cmd.Dir = ctx.wd
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "NODE_ENV="+ctx.getNodeEnv())

	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			msg := stderr.String()
			if strings.HasPrefix(msg, "thread 'main' panicked at") {
				formattedMessage := strings.Split(msg, "\n")[1]
				if strings.HasPrefix(formattedMessage, "failed to resolve reexport: NotFound(") && worthToRetry {
					worthToRetry = false
					// install dependencies and retry
					ctx.npmrc.installDependencies(ctx.wd, ctx.pkgJson, true, nil)
					goto RETRY
				}
				err = fmt.Errorf("cjsModuleLexer: %s", formattedMessage)
			} else {
				err = fmt.Errorf("cjsModuleLexer: %s", msg)
			}
		}
		return
	}

	r := bufio.NewReader(stdout)
	for {
		line, e := r.ReadString('\n')
		if e != nil {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			ret.Reexport = line[1:]
			break
		} else if isJsIdentifier(line) && !isJsReservedWord(line) {
			ret.Exports = append(ret.Exports, line)
		}
	}

	return
}

func installCommonJSModuleLexer() (err error) {
	binDir := path.Join(config.WorkDir, "bin")

	// use dev version of cjs-module-lexer if exists
	// clone https://github.com/esm-dev/cjs-module-lexer to the same directory of esm.sh and run `cargo build --release -p native`
	if devCML := "../cjs-module-lexer/target/release/native"; existsFile(devCML) {
		ensureDir(binDir)
		_, err = utils.CopyFile(devCML, path.Join(binDir, "cjs-module-lexer"))
		if err == nil {
			cjsModuleLexerVersion = "dev"
		}
		return
	}

	if existsFile(path.Join(binDir, "cjs-module-lexer")) {
		return
	}

	url, err := getCommonJSModuleLexerDownloadURL()
	if err != nil {
		return
	}

	if DEBUG {
		fmt.Println(term.Dim(fmt.Sprintf("Downloading %s...", path.Base(url))))
	}

	res, err := http.Get(url)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download cjs-module-lexer: %s", res.Status)
	}

	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		return fmt.Errorf("failed to decompress cjs-module-lexer: %v", err)
	}
	defer gr.Close()

	ensureDir(binDir)
	f, err := os.OpenFile(path.Join(binDir, "cjs-module-lexer"), os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("failed to create cjs-module-lexer: %v", err)
	}
	defer f.Close()

	_, err = io.Copy(f, gr)
	return
}

func getCommonJSModuleLexerDownloadURL() (string, error) {
	var arch string
	var os string

	switch runtime.GOARCH {
	case "arm64":
		arch = "aarch64"
	case "amd64", "386":
		arch = "x86_64"
	default:
		return "", errors.New("unsupported architecture: " + runtime.GOARCH)
	}

	switch runtime.GOOS {
	case "darwin":
		os = "apple-darwin"
	case "linux":
		os = "unknown-linux-gnu"
	default:
		return "", errors.New("unsupported os: " + runtime.GOOS)
	}

	return fmt.Sprintf("https://github.com/esm-dev/cjs-module-lexer/releases/download/v%s/cjs-module-lexer-%s-%s.gz", cjsModuleLexerVersion, arch, os), nil
}
