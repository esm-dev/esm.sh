package app_dir

import (
	"os"
	"path/filepath"
	"runtime"
)

func GetAppDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	appDir := filepath.Join(homeDir, ".esm.sh")
	if runtime.GOOS == "windows" {
		appDir = filepath.Join(homeDir, "AppData\\Local\\esm.sh")
	}

	return appDir, nil
}
