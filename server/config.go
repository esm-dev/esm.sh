package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/ije/gox/term"
)

var (
	// global config
	config Config
)

// Config represents the configuration of esm.sh server.
type Config struct {
	Port                uint16                 `json:"port"`
	TlsPort             uint16                 `json:"tlsPort"`
	CustomLandingPage   LandingPageOptions     `json:"customLandingPage"`
	WorkDir             string                 `json:"workDir"`
	CorsAllowOrigins    []string               `json:"corsAllowOrigins"`
	AllowList           AllowList              `json:"allowList"`
	BanList             BanList                `json:"banList"`
	BuildConcurrency    uint16                 `json:"buildConcurrency"`
	BuildWaitTime       uint16                 `json:"buildWaitTime"`
	Storage             storage.StorageOptions `json:"storage"`
	CacheRawFile        bool                   `json:"cacheRawFile"`
	LogDir              string                 `json:"logDir"`
	LogLevel            string                 `json:"logLevel"`
	NpmRegistry         string                 `json:"npmRegistry"`
	NpmToken            string                 `json:"npmToken"`
	NpmUser             string                 `json:"npmUser"`
	NpmPassword         string                 `json:"npmPassword"`
	NpmScopedRegistries map[string]NpmRegistry `json:"npmScopedRegistries"`
	NpmQueryCacheTTL    uint32                 `json:"npmQueryCacheTTL"`
	MinifyRaw           json.RawMessage        `json:"minify"`
	SourceMapRaw        json.RawMessage        `json:"sourceMap"`
	CompressRaw         json.RawMessage        `json:"compress"`
	LegacyServer        string                 `json:"legacyServer"` // normally you don't need to set this
	Minify              bool                   `json:"-"`
	SourceMap           bool                   `json:"-"`
	Compress            bool                   `json:"-"`
}

type LandingPageOptions struct {
	Origin string   `json:"origin"`
	Assets []string `json:"assets"`
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
func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("fail to read config file: %w", err)
	}
	defer file.Close()

	var c Config
	err = json.NewDecoder(file).Decode(&c)
	if err != nil {
		return nil, fmt.Errorf("fail to parse config: %w", err)
	}

	if c.WorkDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/home/esmd"
		}
		c.WorkDir = path.Join(homeDir, ".esmd")
	} else {
		c.WorkDir, err = filepath.Abs(c.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("fail to get absolute path of the work directory: %w", err)
		}
	}

	normalizeConfig(&c)
	return &c, nil
}

func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/home/esmd"
	}
	c := &Config{WorkDir: path.Join(homeDir, ".esmd")}
	normalizeConfig(c)
	return c
}

func normalizeConfig(c *Config) {
	if c.Port == 0 {
		c.Port = 80
	}
	if v := os.Getenv("CORS_ALLOW_ORIGINS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			orig := strings.TrimSpace(p)
			if orig != "" {
				u, e := url.Parse(orig)
				if e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
					c.CorsAllowOrigins = append(c.CorsAllowOrigins, u.Scheme+"://"+u.Host)
				}
			}
		}
	}
	if c.CustomLandingPage.Origin == "" {
		v := os.Getenv("CUSTOM_LANDING_PAGE_ORIGIN")
		if v != "" {
			c.CustomLandingPage.Origin = v
			if v := os.Getenv("CUSTOM_LANDING_PAGE_ASSETS"); v != "" {
				a := strings.Split(v, ",")
				for _, p := range a {
					p = strings.TrimSpace(p)
					if p != "" {
						c.CustomLandingPage.Assets = append(c.CustomLandingPage.Assets, p)
					}
				}
			}
		}
	}
	if origin := c.CustomLandingPage.Origin; origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			fmt.Println(term.Red("[error] invalid custom landing page origin: " + origin))
			c.CustomLandingPage = LandingPageOptions{}
		} else {
			c.CustomLandingPage.Origin = u.Scheme + "://" + u.Host
		}
	}
	if c.BuildConcurrency == 0 {
		c.BuildConcurrency = uint16(runtime.NumCPU())
	}
	if c.BuildWaitTime == 0 {
		c.BuildWaitTime = 30 // seconds
	}
	if c.Storage.Type == "" {
		storageType := os.Getenv("STORAGE_TYPE")
		if storageType == "" {
			storageType = "fs"
		}
		c.Storage.Type = storageType
	}
	if c.Storage.Endpoint == "" {
		storageEndpint := os.Getenv("STORAGE_ENDPOINT")
		if storageEndpint == "" {
			storageEndpint = path.Join(c.WorkDir, "storage")
		}
		c.Storage.Endpoint = storageEndpint
	}
	if c.Storage.Region == "" {
		c.Storage.Region = os.Getenv("STORAGE_REGION")
	}
	if c.Storage.AccessKeyID == "" {
		c.Storage.AccessKeyID = os.Getenv("STORAGE_ACCESS_KEY_ID")
	}
	if c.Storage.SecretAccessKey == "" {
		c.Storage.SecretAccessKey = os.Getenv("STORAGE_SECRET_ACCESS_KEY")
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
	if len(c.NpmScopedRegistries) > 0 {
		regs := make(map[string]NpmRegistry)
		for scope, rc := range c.NpmScopedRegistries {
			if strings.HasPrefix(scope, "@") && isHttpSepcifier(rc.Registry) {
				rc.Registry = strings.TrimRight(rc.Registry, "/") + "/"
				regs[scope] = rc
			} else {
				fmt.Printf("[error] invalid npm registry for scope %s: %s\n", scope, rc.Registry)
			}
		}
		c.NpmScopedRegistries = regs
	}
	if c.NpmQueryCacheTTL == 0 {
		v := os.Getenv("NPM_QUERY_CACHE_TTL")
		if v != "" {
			i, e := strconv.Atoi(v)
			if e == nil && i >= 0 {
				c.NpmQueryCacheTTL = uint32(i)
			} else {
				c.NpmQueryCacheTTL = 600
			}
		}
		c.NpmQueryCacheTTL = 600
	}
	c.Compress = !(bytes.Equal(c.CompressRaw, []byte("false")) || os.Getenv("COMPRESS") == "false")
	c.SourceMap = !(bytes.Equal(c.SourceMapRaw, []byte("false")) || (os.Getenv("SOURCEMAP") == "false" || os.Getenv("SOURCE_MAP") == "false"))
	c.Minify = !(bytes.Equal(c.MinifyRaw, []byte("false")) || os.Getenv("MINIFY") == "false")
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
