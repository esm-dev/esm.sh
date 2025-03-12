package server

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/goccy/go-json"
	"github.com/ije/gox/term"
)

var (
	// global config
	config *Config
)

// Config represents the configuration of esm.sh server.
type Config struct {
	Port                uint16                 `json:"port"`
	TlsPort             uint16                 `json:"tlsPort"`
	LegacyServer        string                 `json:"legacyServer"` // normally you don't need to set this
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
	AccessLog           bool                   `json:"accessLog"`
	NpmRegistry         string                 `json:"npmRegistry"`
	NpmToken            string                 `json:"npmToken"`
	NpmUser             string                 `json:"npmUser"`
	NpmPassword         string                 `json:"npmPassword"`
	NpmScopedRegistries map[string]NpmRegistry `json:"npmScopedRegistries"`
	NpmQueryCacheTTL    uint32                 `json:"npmQueryCacheTTL"`
	MinifyRaw           json.RawMessage        `json:"minify"`
	SourceMapRaw        json.RawMessage        `json:"sourceMap"`
	CompressRaw         json.RawMessage        `json:"compress"`
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

	var config Config
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("fail to parse config: %w", err)
	}
	if config.WorkDir != "" && !filepath.IsAbs(config.WorkDir) {
		config.WorkDir, err = filepath.Abs(config.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("fail to get absolute path of the work directory: %w", err)
		}
	}
	normalizeConfig(&config)
	return &config, nil
}

func DefaultConfig() *Config {
	config := &Config{}
	normalizeConfig(config)
	return config
}

func normalizeConfig(config *Config) {
	if config.Port == 0 {
		config.Port = 80
		if v := os.Getenv("ESMPORT"); v != "" {
			if p, e := strconv.Atoi(v); e == nil && p >= 80 && p < 65536 {
				config.Port = uint16(p)
			}
		}
	}
	if config.WorkDir == "" {
		if v := os.Getenv("ESMDIR"); v != "" && existsDir(v) {
			config.WorkDir = v
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				homeDir = "/home"
			}
			config.WorkDir = path.Join(homeDir, ".esmd")
		}
	}
	if v := os.Getenv("CORS_ALLOW_ORIGINS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			orig := strings.TrimSpace(p)
			if orig != "" {
				u, e := url.Parse(orig)
				if e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
					config.CorsAllowOrigins = append(config.CorsAllowOrigins, u.Scheme+"://"+u.Host)
				}
			}
		}
	}
	if config.CustomLandingPage.Origin == "" {
		v := os.Getenv("CUSTOM_LANDING_PAGE_ORIGIN")
		if v != "" {
			config.CustomLandingPage.Origin = v
			if v := os.Getenv("CUSTOM_LANDING_PAGE_ASSETS"); v != "" {
				a := strings.Split(v, ",")
				for _, p := range a {
					p = strings.TrimSpace(p)
					if p != "" {
						config.CustomLandingPage.Assets = append(config.CustomLandingPage.Assets, p)
					}
				}
			}
		}
	}
	if origin := config.CustomLandingPage.Origin; origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			fmt.Println(term.Red("[error] invalid custom landing page origin: " + origin))
			config.CustomLandingPage = LandingPageOptions{}
		} else {
			config.CustomLandingPage.Origin = u.Scheme + "://" + u.Host
		}
	}
	if config.BuildConcurrency == 0 {
		config.BuildConcurrency = uint16(runtime.NumCPU())
	}
	if config.BuildWaitTime == 0 {
		config.BuildWaitTime = 30 // seconds
	}
	if config.Storage.Type == "" {
		storageType := os.Getenv("STORAGE_TYPE")
		if storageType == "" {
			storageType = "fs"
		}
		config.Storage.Type = storageType
	}
	if config.Storage.Endpoint == "" {
		storageEndpint := os.Getenv("STORAGE_ENDPOINT")
		if storageEndpint == "" {
			storageEndpint = path.Join(config.WorkDir, "storage")
		}
		config.Storage.Endpoint = storageEndpint
	}
	if config.Storage.Region == "" {
		config.Storage.Region = os.Getenv("STORAGE_REGION")
	}
	if config.Storage.AccessKeyID == "" {
		config.Storage.AccessKeyID = os.Getenv("STORAGE_ACCESS_KEY_ID")
	}
	if config.Storage.SecretAccessKey == "" {
		config.Storage.SecretAccessKey = os.Getenv("STORAGE_SECRET_ACCESS_KEY")
	}
	if config.LogDir == "" {
		config.LogDir = path.Join(config.WorkDir, "log")
	}
	if config.LogLevel == "" {
		config.LogLevel = os.Getenv("LOG_LEVEL")
		if config.LogLevel == "" {
			config.LogLevel = "info"
		}
	}
	if !config.AccessLog {
		config.AccessLog = os.Getenv("ACCESS_LOG") == "true"
	}
	if config.NpmRegistry != "" {
		if isHttpSepcifier(config.NpmRegistry) {
			config.NpmRegistry = strings.TrimRight(config.NpmRegistry, "/") + "/"
		}
	} else {
		v := os.Getenv("NPM_REGISTRY")
		if v != "" && isHttpSepcifier(v) {
			config.NpmRegistry = strings.TrimRight(v, "/") + "/"
		} else {
			config.NpmRegistry = npmRegistry
		}
	}
	if config.NpmToken == "" {
		config.NpmToken = os.Getenv("NPM_TOKEN")
	}
	if config.NpmUser == "" {
		config.NpmUser = os.Getenv("NPM_USER")
	}
	if config.NpmPassword == "" {
		config.NpmPassword = os.Getenv("NPM_PASSWORD")
	}
	if len(config.NpmScopedRegistries) > 0 {
		regs := make(map[string]NpmRegistry)
		for scope, rc := range config.NpmScopedRegistries {
			if strings.HasPrefix(scope, "@") && isHttpSepcifier(rc.Registry) {
				rc.Registry = strings.TrimRight(rc.Registry, "/") + "/"
				regs[scope] = rc
			} else {
				fmt.Printf("[error] invalid npm registry for scope %s: %s\n", scope, rc.Registry)
			}
		}
		config.NpmScopedRegistries = regs
	}
	if config.NpmQueryCacheTTL == 0 {
		v := os.Getenv("NPM_QUERY_CACHE_TTL")
		if v != "" {
			i, e := strconv.Atoi(v)
			if e == nil && i >= 0 {
				config.NpmQueryCacheTTL = uint32(i)
			} else {
				config.NpmQueryCacheTTL = 600
			}
		}
		config.NpmQueryCacheTTL = 600
	}
	config.Compress = !(bytes.Equal(config.CompressRaw, []byte("false")) || os.Getenv("COMPRESS") == "false")
	config.SourceMap = !(bytes.Equal(config.SourceMapRaw, []byte("false")) || (os.Getenv("SOURCEMAP") == "false" || os.Getenv("SOURCE_MAP") == "false"))
	config.Minify = !(bytes.Equal(config.MinifyRaw, []byte("false")) || os.Getenv("MINIFY") == "false")
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

func init() {
	config = DefaultConfig()
}
