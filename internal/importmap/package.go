package importmap

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/goccy/go-json"
	"github.com/ije/gox/sync"
)

var (
	cacheMutex sync.KeyedMutex
	cacheStore sync.Map
)

type PackageJSON struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

func fetchPackageInfo(cdnOrigin string, pkg string) (pkgJSON PackageJSON, err error) {
	url := fmt.Sprintf("%s/%s/package.json", cdnOrigin, pkg)

	// check cache first
	if v, ok := cacheStore.Load(url); ok {
		pkgJSON, _ = v.(PackageJSON)
		return
	}

	// only one fetch at a time for the same url
	unlock := cacheMutex.Lock(url)
	defer unlock()

	// check cache again after get lock
	if v, ok := cacheStore.Load(url); ok {
		pkgJSON, _ = v.(PackageJSON)
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		err = errors.New("http request failed: " + err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		err = errors.New("package not found: " + pkg)
		return
	}
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		err = errors.New(string(msg))
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&pkgJSON)
	if err != nil {
		err = errors.New("could not parse package.json")
	}
	if err == nil {
		cacheStore.Store(url, pkgJSON)
	}
	return
}

func walkPackageDependencies(pkg PackageJSON, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	if len(pkg.Dependencies) > 0 {
		walkDependencies(pkg.Dependencies, callback)
	}
	if len(pkg.PeerDependencies) > 0 {
		walkDependencies(pkg.PeerDependencies, callback)
	}
}

func walkDependencies(deps map[string]string, callback func(specifier, pkgName, pkgVersion, prefix string)) {
	for specifier, pkgVersion := range deps {
		pkgName := specifier
		pkg, err := npm.ResolveDependencyVersion(pkgVersion)
		if err == nil && pkg.Name != "" {
			pkgName = pkg.Name
			pkgVersion = pkg.Version
		}
		var prefix string
		if pkg.Github {
			prefix = "/gh"
		} else if pkg.PkgPrNew {
			prefix = "/pr"
		}
		callback(specifier, pkgName, pkgVersion, prefix)
	}
}
