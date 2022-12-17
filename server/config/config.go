package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var cfgs *Configs

type Configs struct {
	BanList BanList `json:"ban_list"`
}

type BanList struct {
	Packages []string   `json:"packages"`
	Scopes   []BanScope `json:"scopes"`
}

type BanScope struct {
	Name     string   `json:"name"`
	Excludes []string `json:"excludes"`
}

// MustLoadConfigs Loading configs from local `.configs.yml` file. Panic if failed to load.
func MustLoadConfigs() {
	var (
		err      error
		cfgFile  *os.File
		cfgBytes []byte
	)

	if cfgFile, err = os.Open(".configs.json"); err != nil {
		panic(fmt.Errorf("fatal error open config file: %w", err))
	}
	defer cfgFile.Close()

	if cfgBytes, err = ioutil.ReadAll(cfgFile); err != nil {
		panic(fmt.Errorf("fatal error read config file: %w", err))
	}

	if err = json.Unmarshal(cfgBytes, &cfgs); err != nil {
		panic(fmt.Errorf("fatal error parse config: %w", err))
	}
}

func Get() *Configs {
	return cfgs
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
