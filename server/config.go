package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Config represents the configuration of esm.sh server.
type Config struct {
	Port               uint16                 `json:"port"`
	TlsPort            uint16                 `json:"tlsPort"`
	WorkDir            string                 `json:"workDir"`
	AuthSecret         string                 `json:"authSecret"`
	AllowList          AllowList              `json:"allowList"`
	BanList            BanList                `json:"banList"`
	BuildConcurrency   uint16                 `json:"buildConcurrency"`
	BuildWaitTime      uint16                 `json:"buildWaitTime"`
	BuildStorage       string                 `json:"buildStorage"`
	Minify             json.RawMessage        `json:"minify"`
	DisableSourceMap   bool                   `json:"disableSourceMap"`
	DisableCompression bool                   `json:"disableCompression"`
	Database           string                 `json:"database"`
	LogDir             string                 `json:"logDir"`
	LogLevel           string                 `json:"logLevel"`
	NpmRegistry        string                 `json:"npmRegistry"`
	NpmToken           string                 `json:"npmToken"`
	NpmUser            string                 `json:"npmUser"`
	NpmPassword        string                 `json:"npmPassword"`
	NpmRegistries      map[string]NpmRegistry `json:"npmRegistries"`
}

type BanList struct {
	Packages []string   `json:"packages"`
	Scopes   []BanScope `json:"scopes"`
}

type BanScope struct {
	Name     string   `json:"name"`
	Excludes []string `json:"excludes"`
}

type AllowList struct {
	Packages []string     `json:"packages"`
	Scopes   []AllowScope `json:"scopes"`
}

type AllowScope struct {
	Name string `json:"name"`
}

// LoadConfig loads config from the given file. Panic if failed to load.
func LoadConfig(filename string) (cfg *Config, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("fail to read config file: %w", err)
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("fail to parse config: %w", err)
	}

	// ensure `workDir`
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

func DefaultConfig() *Config {
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
	if c.AuthSecret == "" {
		c.AuthSecret = os.Getenv("AUTH_SECRET")
	}
	if !c.DisableCompression {
		c.DisableCompression = os.Getenv("DISABLE_COMPRESSION") == "true"
	}
	if !c.DisableSourceMap {
		c.DisableSourceMap = os.Getenv("DISABLE_SOURCEMAP") == "true"
	}
	if c.Minify == nil && os.Getenv("MINIFY") == "false" {
		c.Minify = []byte("false")
	}
	if c.BuildConcurrency == 0 {
		c.BuildConcurrency = uint16(runtime.NumCPU())
	}
	if c.BuildWaitTime == 0 {
		c.BuildWaitTime = 30 // seconds
	}
	if c.Database == "" {
		c.Database = fmt.Sprintf("bolt://%s", path.Join(c.WorkDir, "esm.db"))
	}
	if c.BuildStorage == "" {
		c.BuildStorage = fmt.Sprintf("fs://%s", path.Join(c.WorkDir, "storage"))
	}
	if c.LogDir == "" {
		c.LogDir = path.Join(c.WorkDir, "log")
	}
	if c.LogLevel == "" {
		c.LogLevel = os.Getenv("LOG_LEVEL")
		if c.LogLevel == "" {
			c.LogLevel = "info"
		}
	}
	if c.NpmRegistry != "" {
		if isHttpSepcifier(c.NpmRegistry) {
			c.NpmRegistry = strings.TrimRight(c.NpmRegistry, "/") + "/"
		}
	} else {
		v := os.Getenv("NPM_REGISTRY")
		if v != "" && isHttpSepcifier(v) {
			c.NpmRegistry = strings.TrimRight(v, "/") + "/"
		} else {
			c.NpmRegistry = npmRegistry
		}
	}
	if c.NpmToken == "" {
		c.NpmToken = os.Getenv("NPM_TOKEN")
	}
	if c.NpmUser == "" {
		c.NpmUser = os.Getenv("NPM_USER")
	}
	if c.NpmPassword == "" {
		c.NpmPassword = os.Getenv("NPM_PASSWORD")
	}
	if len(c.NpmRegistries) > 0 {
		regs := make(map[string]NpmRegistry)
		for scope, rc := range c.NpmRegistries {
			if strings.HasPrefix(scope, "@") && isHttpSepcifier(rc.Registry) {
				rc.Registry = strings.TrimRight(rc.Registry, "/") + "/"
				regs[scope] = rc
			} else {
				fmt.Printf("[error] invalid npm registry for scope %s: %s\n", scope, rc.Registry)
			}
		}
		c.NpmRegistries = regs
	}
	return c
}

// extractPackageName Will take a packageName as input extract key parts and return them
//
// fullNameWithoutVersion  e.g. @github/faker
// scope                   e.g. @github
// nameWithoutVersionScope e.g. faker
func extractPackageName(packageName string) (fullNameWithoutVersion string, scope string, nameWithoutVersionScope string) {
	paths := strings.Split(packageName, "/")
	if strings.HasPrefix(packageName, "@") {
		scope = paths[0]
		nameWithoutVersionScope = strings.Split(paths[1], "@")[0]
		fullNameWithoutVersion = fmt.Sprintf("%s/%s", scope, nameWithoutVersionScope)
	} else {
		// the package has no scope prefix
		nameWithoutVersionScope = strings.Split(paths[0], "@")[0]
		fullNameWithoutVersion = nameWithoutVersionScope
	}

	return fullNameWithoutVersion, scope, nameWithoutVersionScope
}

// IsPackageBanned Checking if the package is banned.
// The `packages` list is the highest priority ban rule to match,
// so the `excludes` list in the `scopes` list won't take effect if the package is banned in `packages` list
func (banList *BanList) IsPackageBanned(fullName string) bool {
	fullNameWithoutVersion, scope, nameWithoutVersionScope := extractPackageName(fullName)

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

// IsPackageAllowed Checking if the package is allowed.
// The `packages` list is the highest priority allow rule to match,
// so the `includes` list in the `scopes` list won't take effect if the package is allowed in `packages` list
func (allowList *AllowList) IsPackageAllowed(fullName string) bool {
	if len(allowList.Packages) == 0 && len(allowList.Scopes) == 0 {
		return true
	}

	fullNameWithoutVersion, scope, _ := extractPackageName(fullName)

	for _, p := range allowList.Packages {
		if fullNameWithoutVersion == p {
			return true
		}
	}

	for _, s := range allowList.Scopes {
		if scope == s.Name {
			return true
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
