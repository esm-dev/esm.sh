package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/jsonc"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
	syncx "github.com/ije/gox/sync"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

const (
	npmRegistry = "https://registry.npmjs.org/"
	jsrRegistry = "https://npm.jsr.io/"
)

var (
	defaultNpmRC *NpmRC
	installMutex syncx.KeyedMutex
)

type NpmRegistry struct {
	Registry string `json:"registry"`
	Token    string `json:"token"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type NpmRC struct {
	NpmRegistry
	ScopedRegistries map[string]NpmRegistry `json:"scopedRegistries"`
	zoneId           string
}

func DefaultNpmRC() *NpmRC {
	if defaultNpmRC != nil {
		return defaultNpmRC
	}
	defaultNpmRC = &NpmRC{
		NpmRegistry: NpmRegistry{
			Registry: config.NpmRegistry,
			Token:    config.NpmToken,
			User:     config.NpmUser,
			Password: config.NpmPassword,
		},
		ScopedRegistries: map[string]NpmRegistry{
			"@jsr": {
				Registry: jsrRegistry,
			},
		},
	}
	if len(config.NpmScopedRegistries) > 0 {
		for scope, reg := range config.NpmScopedRegistries {
			defaultNpmRC.ScopedRegistries[scope] = NpmRegistry{
				Registry: reg.Registry,
				Token:    reg.Token,
				User:     reg.User,
				Password: reg.Password,
			}
		}
	}
	return defaultNpmRC
}

func NewNpmRcFromJSON(jsonData []byte) (npmrc *NpmRC, err error) {
	var rc NpmRC
	err = json.Unmarshal(jsonData, &rc)
	if err != nil {
		return nil, err
	}
	if rc.zoneId != "" {
		if !valid.IsDomain(rc.zoneId) {
			return nil, errors.New("invalid zoneId: must be a valid domain")
		}
	}
	if rc.Registry == "" {
		rc.Registry = config.NpmRegistry
	} else if !strings.HasSuffix(rc.Registry, "/") {
		rc.Registry += "/"
	}
	if rc.ScopedRegistries == nil {
		rc.ScopedRegistries = map[string]NpmRegistry{}
	}
	if _, ok := rc.ScopedRegistries["@jsr"]; !ok {
		rc.ScopedRegistries["@jsr"] = NpmRegistry{
			Registry: jsrRegistry,
		}
	}
	for _, reg := range rc.ScopedRegistries {
		if reg.Registry != "" && !strings.HasSuffix(reg.Registry, "/") {
			reg.Registry += "/"
		}
	}
	return &rc, nil
}
