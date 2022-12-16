package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

var cfgs *Configs

type Configs struct {
	BanList BanList `mapstructure:"ban-list"`
}

type BanList struct {
	Packages []string
	Scopes   []BanScope
}

type BanScope struct {
	Name     string
	Excludes []string
}

// MustLoadConfigs Loading configs from local `.configs.yml` file. Panic if failed to load.
func MustLoadConfigs() {
	v := viper.New()
	v.SetConfigName(".configs")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")    // optionally look for config in the working directory
	err := v.ReadInConfig() // Find and read the config file
	if err != nil {         // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	if err = v.Unmarshal(&cfgs); err != nil {
		panic(fmt.Errorf("fatal error parse config: %w", err))
	}
}

func Get() *Configs {
	return cfgs
}

// IsPackageBaned Checking if the package is banned.
// The `packages` list is the highest priority ban rule to match,
// so the `excludes` list in the `scopes` list won't take effect if the package is banned in `packages` list
func (banList *BanList) IsPackageBaned(fullName string) bool {
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
