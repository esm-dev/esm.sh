package server

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

var (
	loaderRuntime        = "deno"
	loaderRuntimeVersion = "2.1.4"
	compileSyncMap       = sync.Map{}
	bufferPool           = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
)

type LoaderOutput struct {
	Lang  string `json:"lang"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func runLoader(loaderJsPath string, filename string, code string) (output *LoaderOutput, err error) {
	outBuf := bufferPool.Get().(*bytes.Buffer)
	errBuf := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		outBuf.Reset()
		errBuf.Reset()
		bufferPool.Put(outBuf)
		bufferPool.Put(errBuf)
	}()
	c := exec.Command(
		path.Join(config.WorkDir, "bin", loaderRuntime), "run",
		"--no-config",
		"--no-lock",
		"--cached-only",
		"--no-prompt",
		"--allow-read=.",
		"--quiet",
		loaderJsPath,
		filename, // args[0]
	)
	c.Dir = os.TempDir()
	c.Stdin = strings.NewReader(code)
	c.Stdout = outBuf
	c.Stderr = errBuf
	err = c.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = errors.New(errBuf.String())
		}
		return
	}
	if outBuf.Len() < 2 {
		err = errors.New("bad loader output")
		return
	}
	data := outBuf.Bytes()
	if data[0] != '1' && data[0] != '2' {
		err = errors.New(string(data[2:]))
		return
	}
	lang := "js"
	if data[0] == '2' {
		lang = "ts"
	}
	return &LoaderOutput{Lang: lang, Code: string(data[2:])}, nil
}

func buildLoader(wd, loaderJs, outfile string) (err error) {
	ret := esbuild.Build(esbuild.BuildOptions{
		Outfile:           outfile,
		Stdin:             &esbuild.StdinOptions{Contents: loaderJs, ResolveDir: wd},
		Platform:          esbuild.PlatformBrowser,
		Format:            esbuild.FormatESModule,
		Target:            esbuild.ESNext,
		Bundle:            true,
		MinifySyntax:      true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		Write:             true,
		PreserveSymlinks:  true,
		Plugins: []esbuild.Plugin{{
			Name: "resolver",
			Setup: func(build esbuild.PluginBuild) {
				build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if strings.HasPrefix(args.Path, "node:") || nodeBuiltinModules[args.Path] {
						return esbuild.OnResolveResult{Path: "node:" + strings.TrimPrefix(args.Path, "node:"), External: true}, nil
					}
					if strings.HasPrefix(args.Path, "jsr:") {
						return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
					}
					return esbuild.OnResolveResult{}, nil
				})
			},
		}},
	})
	if len(ret.Errors) > 0 {
		err = errors.New(ret.Errors[0].Text)
	}
	return
}

func installLoaderRuntime() (err error) {
	binDir := path.Join(config.WorkDir, "bin")
	err = ensureDir(binDir)
	if err != nil {
		return err
	}

	// check local installed deno
	installedRuntime, err := exec.LookPath(loaderRuntime)
	if err == nil {
		output, err := run(installedRuntime, "eval", "console.log(Deno.version.deno)")
		if err == nil {
			version := strings.TrimSpace(string(output))
			if !semverLessThan(version, "1.45") {
				_, err = utils.CopyFile(installedRuntime, path.Join(binDir, loaderRuntime))
				if err == nil {
					loaderRuntimeVersion = version
				}
				return err
			}
		}
	}

	if existsFile(path.Join(binDir, loaderRuntime)) {
		output, err := run(path.Join(binDir, loaderRuntime), "eval", "console.log(Deno.version.deno)")
		if err == nil {
			version := strings.TrimSpace(string(output))
			if !semverLessThan(version, loaderRuntimeVersion) {
				return nil
			}
		}
	}

	url, err := getLoaderRuntimeInstallURL()
	if err != nil {
		return
	}

	if debug {
		log.Debugf("downloading %s...", path.Base(url))
	}

	res, err := http.Get(url)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download %s: %s", loaderRuntime, res.Status)
	}

	tmpFile := path.Join(binDir, loaderRuntime+".zip")
	defer os.Remove(tmpFile)

	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, err = io.Copy(f, res.Body)
	if err != nil {
		return
	}

	zr, err := zip.OpenReader(tmpFile)
	if err != nil {
		return
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if zf.Name == loaderRuntime {
			r, err := zf.Open()
			if err != nil {
				return err
			}
			defer r.Close()

			f, err := os.OpenFile(path.Join(binDir, loaderRuntime), os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, r)
			if err != nil {
				return err
			}
			break
		}
	}

	return
}

func getLoaderRuntimeInstallURL() (string, error) {
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
	// case "windows":
	// 	os = "pc-windows-msvc"
	default:
		return "", errors.New("unsupported os: " + runtime.GOOS)
	}

	return fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/%s-%s-%s.zip", loaderRuntimeVersion, loaderRuntime, arch, os), nil
}
