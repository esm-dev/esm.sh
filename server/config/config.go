package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	Port             uint16  `json:"port,omitempty"`
	TlsPort          uint16  `json:"tlsPort,omitempty"`
	NsPort           uint16  `json:"nsPort,omitempty"`
	BuildConcurrency uint16  `json:"buildConcurrency,omitempty"`
	BanList          BanList `json:"banList,omitempty"`
	WorkDir          string  `json:"workDir,omitempty"`
	Cache            string  `json:"cache,omitempty"`
	Database         string  `json:"database,omitempty"`
	Storage          string  `json:"storage,omitempty"`
	LogLevel         string  `json:"logLevel,omitempty"`
	LogDir           string  `json:"logDir,omitempty"`
	Origin           string  `json:"origin,omitempty"`
	BasePath         string  `json:"basePath,omitempty"`
	NpmRegistry      string  `json:"npmRegistry,omitempty"`
	NpmToken         string  `json:"npmToken,omitempty"`
	NpmCDN           string  `json:"npmCDN,omitempty"`
	BackupNpmCDN     string  `json:"backupNpmCDN,omitempty"`
	NoCompress       bool    `json:"noCompress,omitempty"`
}

type BanList struct {
	Packages []string   `json:"packages"`
	Scopes   []BanScope `json:"scopes"`
}

type BanScope struct {
	Name     string   `json:"name"`
	Excludes []string `json:"excludes"`
}

// Load loads config from the given file. Panic if failed to load.
func Load(filename string) (*Config, error) {
	var (
		cfg     *Config
		cfgFile *os.File
		err     error
	)

	cfgFile, err = os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("fail to read config file: %w", err)
	}
	defer cfgFile.Close()

	err = json.NewDecoder(cfgFile).Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("fail to parse config: %w", err)
	}

	// fix config
	if cfg.WorkDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("fail to get current user home directory: %w", err)
		}
		cfg.WorkDir = path.Join(homeDir, ".esmd")
	} else {
		cfg.WorkDir, err = filepath.Abs(cfg.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("fail to get absolute path of the work directory: %w", err)
		}
	}
	if cfg.BasePath != "" {
		a := strings.Split(cfg.BasePath, "/")
		path := make([]string, len(a))
		n := 0
		for i := 0; i < len(a); i++ {
			if a[i] != "" {
				path[n] = a[i]
				n++
			}
		}
		if n > 0 {
			cfg.BasePath = "/" + strings.Join(path[:n], "/")
		} else {
			cfg.BasePath = ""
		}
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.NsPort == 0 {
		cfg.NsPort = 8088
	}
	if cfg.BuildConcurrency == 0 {
		cfg.BuildConcurrency = uint16(runtime.NumCPU())
	}
	if cfg.Cache == "" {
		cfg.Cache = "memory:default"
	}
	if cfg.Database == "" {
		cfg.Database = fmt.Sprintf("bolt:%s", path.Join(cfg.WorkDir, "esm.db"))
	}
	if cfg.Storage == "" {
		cfg.Storage = fmt.Sprintf("local:%s", path.Join(cfg.WorkDir, "storage"))
	}
	if cfg.LogDir == "" {
		cfg.LogDir = path.Join(cfg.WorkDir, "log")
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.NpmCDN == "" {
		cfg.NpmCDN = "https://esm.sh"
	}

	return cfg, nil
}

func Default() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	workDir := path.Join(homeDir, ".esmd")
	return &Config{
		Port:             8080,
		NsPort:           8088,
		BuildConcurrency: uint16(runtime.NumCPU()),
		WorkDir:          workDir,
		Cache:            "memory:default",
		Database:         fmt.Sprintf("bolt:%s", path.Join(workDir, "esm.db")),
		Storage:          fmt.Sprintf("local:%s", path.Join(workDir, "storage")),
		LogDir:           path.Join(workDir, "log"),
		LogLevel:         "info",
		NpmCDN:           "https://esm.sh",
	}
}

// IsPackageBanned Checking if the package is banned.
// The `packages` list is the highest priority ban rule to match,
// so the `excludes` list in the `scopes` list won't take effect if the package is banned in `packages` list
func (banList *BanList) IsPackageBanned(fullName string) bool {
	var (
		fullNameWithoutVersion  string // e.g. @github/faker
		scope                   string // e.g. @github
		nameWithoutVersionScope string // e.g. faker
	)
	paths := strings.Split(fullName, "/")
	if len(paths) < 2 {
		// the package has no scope prefix
		nameWithoutVersionScope = strings.Split(paths[0], "@")[0]
		fullNameWithoutVersion = nameWithoutVersionScope
	} else {
		scope = paths[0]
		nameWithoutVersionScope = strings.Split(paths[1], "@")[0]
		fullNameWithoutVersion = fmt.Sprintf("%s/%s", scope, nameWithoutVersionScope)
	}

	for _, p := range banList.Packages {
		if fullNameWithoutVersion == p {
			return true
		}
	}

	for _, s := range banList.Scopes {
		if scope == s.Name {
			return !isPackageExcluded(nameWithoutVersionScope, s.Excludes)
		}
	}

	return false
}

func isPackageExcluded(name string, excludes []string) bool {
	for _, exclude := range excludes {
		if name == exclude {
			return true
		}
	}

	return false
}
