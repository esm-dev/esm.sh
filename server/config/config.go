package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ije/gox/utils"
)

const MinBuildConcurrency = 4

type Config struct {
	Port             uint16  `json:"port,omitempty"`
	TlsPort          uint16  `json:"tlsPort,omitempty"`
	NsPort           uint16  `json:"nsPort,omitempty"`
	BuildConcurrency uint16  `json:"buildConcurrency,omitempty"`
	BanList          BanList `json:"banList,omitempty"`
	AuthSecret       string  `json:"authSecret,omitempty"`
	WorkDir          string  `json:"workDir,omitempty"`
	Cache            string  `json:"cache,omitempty"`
	Database         string  `json:"database,omitempty"`
	Storage          string  `json:"storage,omitempty"`
	LogLevel         string  `json:"logLevel,omitempty"`
	LogDir           string  `json:"logDir,omitempty"`
	CdnOrigin        string  `json:"cdnOrigin,omitempty"`
	CdnBasePath      string  `json:"cdnBasePath,omitempty"`
	NpmRegistry      string  `json:"npmRegistry,omitempty"`
	NpmToken         string  `json:"npmToken,omitempty"`
	NpmRegistryScope string  `json:"npmRegistryScope,omitempty"`
	NpmUser          string  `json:"npmUser,omitempty"`
	NpmPassword      string  `json:"npmPassword,omitempty"`
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
	return fixConfig(cfg), nil
}

func Default() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return fixConfig(&Config{
		WorkDir: path.Join(homeDir, ".esmd"),
	})
}

func fixConfig(c *Config) *Config {
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.NsPort == 0 {
		c.NsPort = 8088
	}
	if c.CdnOrigin != "" {
		_, e := url.Parse(c.NpmRegistry)
		if e != nil {
			panic("invalid Cdnorigin url: " + e.Error())
		}
		c.CdnOrigin = strings.TrimRight(c.CdnOrigin, "/")
	} else {
		v := os.Getenv("CDN_ORIGIN")
		if v != "" {
			if _, e := url.Parse(v); e == nil {
				c.CdnOrigin = strings.TrimRight(v, "/")
			}
		}
	}
	if c.CdnBasePath != "" {
		a := strings.Split(c.CdnBasePath, "/")
		path := make([]string, len(a))
		n := 0
		for _, p := range a {
			if p != "" && p != "." {
				path[n] = p
				n++
			}
		}
		if n > 0 {
			c.CdnBasePath = "/" + strings.Join(path[:n], "/")
		} else {
			c.CdnBasePath = ""
		}
	} else {
		v := os.Getenv("CDN_BASE_PATH")
		if v != "" {
			c.CdnBasePath = utils.CleanPath(v)
		}
	}
	if c.BuildConcurrency == 0 {
		c.BuildConcurrency = uint16(2 * runtime.NumCPU())
	}
	if c.BuildConcurrency < MinBuildConcurrency {
		c.BuildConcurrency = MinBuildConcurrency
	}
	if c.Cache == "" {
		c.Cache = "memory:default"
	}
	if c.Database == "" {
		c.Database = fmt.Sprintf("bolt:%s", path.Join(c.WorkDir, "esm.db"))
	}
	if c.Storage == "" {
		c.Storage = fmt.Sprintf("local:%s", path.Join(c.WorkDir, "storage"))
	}
	if c.LogDir == "" {
		c.LogDir = path.Join(c.WorkDir, "log")
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.NpmRegistry != "" {
		_, e := url.Parse(c.NpmRegistry)
		if e != nil {
			panic("invalid npm registry url: " + e.Error())
		}
		c.NpmRegistry = strings.TrimRight(c.NpmRegistry, "/") + "/"
	} else {
		v := os.Getenv("NPM_REGISTRY")
		if v != "" {
			if _, e := url.Parse(v); e == nil {
				c.NpmRegistry = strings.TrimRight(v, "/") + "/"
			}
		}
	}
	if c.NpmToken == "" {
		c.NpmToken = os.Getenv("NPM_TOKEN")
	}
	if c.NpmRegistryScope == "" {
		c.NpmRegistryScope = os.Getenv("NPM_REGISTRY_SCOPE")
	}
	if c.NpmUser == "" {
		c.NpmUser = os.Getenv("NPM_USER")
	}
	if c.NpmPassword == "" {
		c.NpmPassword = os.Getenv("NPM_PASSWORD")
	}
	if c.AuthSecret == "" {
		c.AuthSecret = os.Getenv("SERVER_AUTH_SECRET")
	}
	return c
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
