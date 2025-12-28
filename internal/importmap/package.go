package importmap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/esm-dev/esm.sh/internal/app_dir"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/sync"
)

var (
	keyedMutex sync.KeyedMutex
	fetchCache sync.Map
)

type PackageInfo struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
	Github           bool              `json:"-"`
	Jsr              bool              `json:"-"`
}

type Dependency struct {
	Specifier string
	Name      string
	Version   string
	Peer      bool
	Github    bool
	Jsr       bool
}

func (pkg PackageInfo) String() string {
	b := strings.Builder{}
	if pkg.Github {
		b.WriteString("gh/")
	} else if pkg.Jsr {
		b.WriteString("jsr/")
	}
	b.WriteString(pkg.Name)
	b.WriteByte('@')
	b.WriteString(pkg.Version)
	return b.String()
}

func fetchPackageInfo(cdnOrigin string, regPrefix string, pkgName string, pkgVersion string) (pkgInfo PackageInfo, err error) {
	url := fmt.Sprintf("%s/%s%s@%s/package.json", cdnOrigin, regPrefix, pkgName, pkgVersion)

	// check cache first
	if v, ok := fetchCache.Load(url); ok {
		pkgInfo, _ = v.(PackageInfo)
		return
	}

	// only one fetch at a time for the same url
	unlock := keyedMutex.Lock(url)
	defer unlock()

	// check cache again after acquiring the lock state
	if v, ok := fetchCache.Load(url); ok {
		pkgInfo, _ = v.(PackageInfo)
		return
	}

	appDir, _ := app_dir.GetAppDir()

	// if the version is exact, check the cache on disk
	if npm.IsExactVersion(pkgVersion) && appDir != "" {
		cachePath := filepath.Join(appDir, "registry", regPrefix, pkgName+"@"+pkgVersion+".json")
		f, err := os.Open(cachePath)
		if err == nil {
			defer f.Close()
			err = json.NewDecoder(f).Decode(&pkgInfo)
			if err == nil {
				switch regPrefix {
				case "gh/":
					pkgInfo.Github = true
				case "jsr/":
					pkgInfo.Name = pkgName
					pkgInfo.Jsr = true
				}
				fetchCache.Store(url, pkgInfo)
				return pkgInfo, nil
			}
			// if decode error, remove the cache file
			// and try to fetch again
			_ = os.Remove(cachePath)
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		err = fmt.Errorf("package not found: %s@%s", pkgName, pkgVersion)
		return
	}

	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("unexpected http status %d: %s", resp.StatusCode, msg)
		return
	}

	jsonData, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("could not read %s@%s/package.json: %s", pkgName, pkgVersion, err.Error())
		return
	}

	err = json.Unmarshal(jsonData, &pkgInfo)
	if err != nil {
		err = fmt.Errorf("could not decode %s@%s/package.json: %s", pkgName, pkgVersion, err.Error())
		return
	}

	switch regPrefix {
	case "gh/":
		pkgInfo.Github = true
	case "jsr/":
		pkgInfo.Name = pkgName
		pkgInfo.Jsr = true
	}

	// cache the package.json on disk
	if appDir != "" {
		cachePath := filepath.Join(appDir, "registry", regPrefix, pkgName+"@"+pkgVersion+".json")
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, jsonData, 0644)
	}

	// cache the package info in memory
	fetchCache.Store(url, pkgInfo)
	return
}

func walkPackageDependencies(pkg PackageInfo, callback func(dep Dependency)) error {
	if len(pkg.PeerDependencies) > 0 {
		err := walkDependencies(pkg.PeerDependencies, true, callback)
		if err != nil {
			return err
		}
	}
	if len(pkg.Dependencies) > 0 {
		err := walkDependencies(pkg.Dependencies, false, callback)
		if err != nil {
			return err
		}
	}
	return nil
}

func walkDependencies(deps map[string]string, peer bool, callback func(dep Dependency)) error {
	for specifier, version := range deps {
		pkg, err := npm.ResolveDependencyVersion(version)
		if err != nil {
			return err
		}
		dep := Dependency{
			Specifier: specifier,
			Name:      specifier,
			Version:   version,
			Peer:      peer,
		}
		if pkg.Name != "" {
			dep.Name = pkg.Name
			dep.Version = pkg.Version
			dep.Github = pkg.Github
		}
		if strings.HasPrefix(pkg.Name, "@jsr/") {
			dep.Name = "@" + strings.Replace(dep.Name[5:], "__", "/", 1)
			dep.Jsr = true
		}
		callback(dep)
	}
	return nil
}
