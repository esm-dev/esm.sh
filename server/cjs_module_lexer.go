package server

import (
	"bufio"
	"bytes"
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

	"github.com/esm-dev/esm.sh/internal/deno"
	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

var cjsModuleLexerVersion = "1.0.8"
var cjsModuleLexerIgnoredPackages = set.New(
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

func cjsModuleLexer(b *BuildContext, cjsEntry string) (ret cjsModuleLexerResult, err error) {
	h := sha1.New()
	h.Write([]byte(cjsModuleLexerVersion))
	h.Write([]byte(cjsEntry))
	h.Write([]byte(b.getNodeEnv()))
	cacheFileName := path.Join(b.wd, ".cjs-module-lexer", base64.RawURLEncoding.EncodeToString(h.Sum(nil))+".json")

	// check the cache first
	if existsFile(cacheFileName) && utils.ParseJSONFile(cacheFileName, &ret) == nil {
		return
	}

	start := time.Now()
	defer func() {
		if err == nil {
			if DEBUG {
				b.logger.Debugf("[cjsModuleLexer] parse %s in %s", path.Join(b.esmPath.PkgName, cjsEntry), time.Since(start))
			}
			if !existsFile(cacheFileName) {
				ensureDir(path.Dir(cacheFileName))
				utils.WriteJSONFile(cacheFileName, ret, "")
			}
		}
	}()

	if cjsModuleLexerIgnoredPackages.Has(b.esmPath.PkgName) {
		denoPath := deno.ResolveDenoPath(config.WorkDir)
		err = doOnce("check-deno", func() (err error) {
			return deno.CheckDenoPath(denoPath)
		})
		if err != nil {
			return
		}
		js := path.Join(b.wd, "reveal_"+strings.ReplaceAll(cjsEntry[2:], "/", "_"))
		err = os.WriteFile(js, fmt.Appendf(nil, `console.log(JSON.stringify(Object.keys((await import("npm:%s")).default)))`, path.Join(b.esmPath.PackageId(), cjsEntry)), 0644)
		if err != nil {
			return
		}
		cancelCtx, cancel := context.WithTimeout(b.Context(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(cancelCtx,
			denoPath,
			"run",
			"--allow-env",
			"--no-prompt",
			"--no-config",
			"--no-lock",
			"--quiet",
			js)
		cmd.Env = append(os.Environ(), "DENO_NO_UPDATE_CHECK=1")
		var data []byte
		data, err = cmd.Output()
		if err != nil {
			err = errors.New("cjsModuleLexer(fallback mode): " + err.Error())
			return
		}
		var namedExports []string
		err = json.Unmarshal(data, &namedExports)
		if err != nil {
			err = errors.New("cjsModuleLexer(fallback mode): " + err.Error())
			return
		}
		for _, name := range namedExports {
			if !isJsReservedWord(name) {
				ret.Exports = append(ret.Exports, name)
			}
		}
		return
	}

	err = doOnce("install-cjs-module-lexer", func() (err error) {
		err = installCjsModuleLexerContext(b.Context())
		return
	})
	if err != nil {
		return
	}

	retried := false
RETRY:

	ctx, cancel := context.WithTimeout(b.Context(), time.Minute)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	defer cancel()

	cmd := exec.CommandContext(ctx, path.Join(config.WorkDir, fmt.Sprintf("bin/cjs-module-lexer-%s", cjsModuleLexerVersion)), path.Join(b.esmPath.PkgName, cjsEntry))
	cmd.Dir = b.wd
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "NODE_ENV="+b.getNodeEnv())

	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			msg := stderr.String()
			if strings.HasPrefix(msg, "thread 'main' panicked at") {
				formattedMessage := strings.Split(msg, "\n")[1]
				if strings.HasPrefix(formattedMessage, "failed to resolve reexport: NotFound(") && !retried {
					retried = true
					// install dependencies and retry
					err = b.npmrc.installDependenciesContext(b.Context(), b.wd, b.pkgJson, true, nil)
					if err != nil {
						return
					}
					goto RETRY
				}
				err = fmt.Errorf("cjsModuleLexer: %s", formattedMessage)
			} else {
				err = fmt.Errorf("cjsModuleLexer: %s", msg)
			}
		} else {
			err = fmt.Errorf("cjsModuleLexer: %v", err)
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

func installCjsModuleLexerContext(ctx context.Context) (err error) {
	installDir := path.Join(config.WorkDir, "bin")
	installPath := path.Join(installDir, fmt.Sprintf("cjs-module-lexer-%s", cjsModuleLexerVersion))

	// use dev version of cjs-module-lexer if exists
	// clone https://github.com/esm-dev/cjs-module-lexer to the same directory of esm.sh and run `cargo build --release -p native`
	if DEBUG {
		localBuild := "../cjs-module-lexer/target/release/native"
		if existsFile(localBuild) {
			ensureDir(installDir)
			_, err = utils.CopyFile(localBuild, installPath)
			if err == nil {
				cjsModuleLexerVersion = "dev"
			}
			return
		}
	}

	if existsFile(installPath) {
		return
	}

	url, err := getCjsModuleLexerDownloadURL()
	if err != nil {
		return
	}

	if DEBUG {
		fmt.Println(term.Dim(fmt.Sprintf("Downloading %s...", path.Base(url))))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download cjs-module-lexer: %s", res.Status)
	}

	gr, err := gzip.NewReader(&contextReader{ctx: ctx, reader: res.Body})
	if err != nil {
		return fmt.Errorf("failed to decompress cjs-module-lexer: %v", err)
	}
	defer gr.Close()

	ensureDir(installDir)
	f, err := os.OpenFile(installPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("failed to create cjs-module-lexer: %v", err)
	}
	defer f.Close()

	_, err = io.Copy(f, &contextReader{ctx: ctx, reader: gr})
	return
}

func getCjsModuleLexerDownloadURL() (string, error) {
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
