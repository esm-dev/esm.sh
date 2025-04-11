package env

import (
	"os"
	"path/filepath"
	"runtime"
)

const Windows = runtime.GOOS == "windows"

// GetAppDir returns the application data directory for the given app name.
func GetAppDataDir(appName string) (string, error) {
	if Windows {
		appDataDir := os.Getenv("LOCALAPPDATA")
		if appDataDir != "" {
			return filepath.Join(appDataDir, appName), nil
		}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if Windows {
		return filepath.Join(homeDir, "AppData", "Local", appName), nil
	}
	return filepath.Join(homeDir, "."+appName), nil
}

// GetCachDir returns the cache directory for the given app name.
func GetCachDir(appName string) (string, error) {
	if Windows {
		appDataDir := os.Getenv("LOCALAPPDATA")
		if appDataDir != "" {
			return filepath.Join(appDataDir, appName, "Cache"), nil
		}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if Windows {
		return filepath.Join(homeDir, "AppData", "Local", appName, "Cache"), nil
	}
	return filepath.Join(homeDir, ".cache", appName), nil
}
