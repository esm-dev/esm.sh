package server

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ije/gox/utils"
)

const denoVersion = "2.7.13"

func resolveDenoPath(workDir string) string {
	denoPath := filepath.Join(workDir, "bin/deno")
	if runtime.GOOS == "windows" {
		denoPath += ".exe"
	}
	return denoPath
}

func checkDenoPath(denoPath string) (err error) {
	fi, err := os.Lstat(denoPath)
	if err == nil {
		if !fi.IsDir() && validateDenoVersion(denoPath, denoVersion) == nil {
			return nil
		}
		// remove the invalid deno path and install a new one
		os.RemoveAll(denoPath)
	}
	return installDeno(denoPath, denoVersion)
}

func installDeno(installPath string, version string) (err error) {
	// ensure install dir
	os.MkdirAll(filepath.Dir(installPath), 0755)

	// check system installed deno
	systemDenoPath, err := exec.LookPath("deno")
	if err == nil {
		err = validateDenoVersion(systemDenoPath, version)
		if err == nil {
			if runtime.GOOS == "windows" {
				_, err = utils.CopyFile(systemDenoPath, installPath)
			} else {
				err = os.Symlink(systemDenoPath, installPath)
			}
			return
		}
	}

	url, err := getDenoDownloadURL(version)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download Deno install package: %s", res.Status)
	}

	installDir := filepath.Dir(installPath)
	zf, err := os.CreateTemp(installDir, ".deno-*.zip")
	if err != nil {
		return
	}
	defer os.Remove(zf.Name())
	defer zf.Close()

	size, err := io.Copy(zf, res.Body)
	if err != nil {
		return
	}

	zr, err := zip.NewReader(zf, size)
	if err != nil {
		return
	}

	for _, entry := range zr.File {
		if entry.Name == "deno" || entry.Name == "deno.exe" {
			r, err := entry.Open()
			if err != nil {
				return err
			}
			defer r.Close()

			f, err := os.CreateTemp(installDir, ".deno-*")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			defer f.Close()

			if _, err = io.Copy(f, r); err != nil {
				return err
			}
			if err = f.Chmod(0755); err != nil {
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			return os.Rename(f.Name(), installPath)
		}
	}

	return errors.New("deno binary not found in the install package")
}

func getDenoDownloadURL(version string) (string, error) {
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

func validateDenoVersion(denoPath, want string) error {
	cmd := exec.Command(denoPath, "eval", "console.log(Deno.version.deno)")
	cmd.Env = append(os.Environ(), "DENO_NO_UPDATE_CHECK=1")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) == want {
		return nil
	}
	return errors.New("invalid deno version: expected " + want)
}
