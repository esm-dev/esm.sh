package deno

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ije/gox/utils"
)

const DenoVersion = "2.4.4"

func GetDenoPath(workDir string) (denoPath string, err error) {
	if workDir == "" {
		return "", errors.New("workDir is empty")
	}

	denoPath = filepath.Join(workDir, "bin/deno")
	if runtime.GOOS == "windows" {
		denoPath += ".exe"
	}

	fi, err := os.Lstat(denoPath)
	if err == nil && !fi.IsDir() {
		return denoPath, nil
	}

	err = installDeno(denoPath, DenoVersion)
	if err != nil {
		return "", err
	}

	return denoPath, nil
}

func installDeno(installPath string, version string) (err error) {
	// ensure install dir
	os.MkdirAll(filepath.Dir(installPath), 0755)

	// check local installed deno
	systemDenoPath, err := exec.LookPath("deno")
	if err == nil {
		cmd := exec.Command(systemDenoPath, "eval", "console.log(Deno.version.deno)")
		output, err := cmd.Output()
		if err == nil {
			version := strings.Split(strings.TrimSpace(string(output)), ".")
			if len(version) == 3 {
				major, _ := strconv.Atoi(version[0])
				minor, _ := strconv.Atoi(version[1])
				// check if the installed deno version is greater than or equal to 1.45
				if major > 1 || (major == 1 && minor >= 45) {
					if runtime.GOOS == "windows" {
						_, err = utils.CopyFile(systemDenoPath, installPath)
					} else {
						err = os.Symlink(systemDenoPath, installPath)
					}
					if err != nil {
						return err
					}
					return nil
				}
			}
		}
	}

	url, err := getDenoDotZipURL(version)
	if err != nil {
		return
	}

	res, err := http.Get(url)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download Deno install package: %s", res.Status)
	}

	tmpFile := filepath.Join(os.TempDir(), "deno.zip")
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
		if zf.Name == "deno" || zf.Name == "deno.exe" {
			r, err := zf.Open()
			if err != nil {
				return err
			}
			defer r.Close()

			f, err := os.OpenFile(installPath, os.O_CREATE|os.O_WRONLY, 0755)
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

	return nil
}

func getDenoDotZipURL(version string) (string, error) {
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
	case "windows":
		os = "pc-windows-msvc"
	default:
		return "", errors.New("unsupported os: " + runtime.GOOS)
	}

	return fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/deno-%s-%s.zip", version, arch, os), nil
}
