package server

import (
	"archive/zip"
	"bytes"
	"context"
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

	"github.com/ije/gox/utils"
)

var (
	loaderRuntime        = "deno"
	loaderRuntimeVersion = "2.1.4"
)

type LoaderOutput struct {
	Lang  string `json:"lang"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func runLoader(npmrc *NpmRC, loaderName string, args []string, mainDependency Package, extraDeps ...string) (output *LoaderOutput, err error) {
	wd := path.Join(npmrc.StoreDir(), fmt.Sprintf("loader-v%d", VERSION), strings.ReplaceAll(strings.Join(append([]string{mainDependency.String()}, extraDeps...), "+"), "/", "_"))
	loaderJsFilename := path.Join(wd, "loader.mjs")
	if !existsFile(loaderJsFilename) {
		var loaderJS []byte
		loaderJS, err = embedFS.ReadFile(fmt.Sprintf("server/embed/internal/%s_loader.js", loaderName))
		if err != nil {
			err = fmt.Errorf("could not find loader: %s", loaderName)
			return
		}

		ensureDir(wd)

		stderr := bytes.NewBuffer(nil)
		cmd := exec.Command("npm", append([]string{"i", "--ignore-scripts", "--no-bin-links", mainDependency.String()}, extraDeps...)...)
		cmd.Dir = wd
		cmd.Stderr = stderr
		err = cmd.Run()
		if err != nil {
			if stderr.Len() > 0 {
				err = fmt.Errorf("could not install %s %s: %s", mainDependency.String(), strings.Join(extraDeps, " "), stderr.String())
			}
			return
		}

		err = os.WriteFile(loaderJsFilename, loaderJS, 0755)
		if err != nil {
			err = fmt.Errorf("could not write loader.js")
			return
		}
	}

	stdin := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	err = json.NewEncoder(stdin).Encode(args)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", "loader.mjs")
	cmd.Dir = wd
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("preLoad: %s", stderr.String())
		}
		return
	}

	var out LoaderOutput
	err = json.NewDecoder(stdout).Decode(&out)
	if err != nil {
		return
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	return &out, nil
}

func transformVue(npmrc *NpmRC, vueVersion string, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "vue", args, Package{Name: "@vue/compiler-sfc", Version: vueVersion}, "@esm.sh/vue-loader@1.0.3")
}

func transformSvelte(npmrc *NpmRC, svelteVersion string, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "svelte", args, Package{Name: "svelte", Version: svelteVersion})
}

func generateUnoCSS(npmrc *NpmRC, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "unocss", args, Package{Name: "@esm.sh/unocss", Version: "0.3.1"}, "@iconify/json@2.2.280")
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

	url, err := getLoaderRuntimeDownloadURL()
	if err != nil {
		return
	}

	log.Debugf("downloading %s...", path.Base(url))

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

func getLoaderRuntimeDownloadURL() (string, error) {
	var arch string
	var os string

	switch runtime.GOARCH {
	case "arm64":
		arch = "aarch64"
	case "amd64", "386":
		arch = "x86_64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	switch runtime.GOOS {
	case "darwin":
		os = "apple-darwin"
	case "linux":
		os = "unknown-linux-gnu"
	default:
		return "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}

	return fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/%s-%s-%s.zip", loaderRuntimeVersion, loaderRuntime, arch, os), nil
}
