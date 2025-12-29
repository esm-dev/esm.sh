package importmap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/esm-dev/esm.sh/internal/app_dir"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/sync"
	"github.com/ije/gox/utils"
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
	Name    string
	Version string
	Peer    bool
	Github  bool
	Jsr     bool
}

func (pkg PackageInfo) String() string {
	b := strings.Builder{}
	if pkg.Github {
		b.WriteString("gh/")
	} else if pkg.Jsr {
		b.WriteString("jsr/")
	}
	if len(pkg.Dependencies) > 0 || len(pkg.PeerDependencies) > 0 {
		b.WriteString("*") // add external-all modifier of esm.sh
	}
	b.WriteString(pkg.Name)
	b.WriteByte('@')
	b.WriteString(pkg.Version)
	return b.String()
}

func fetchPackageInfo(cdnOrigin string, regPrefix string, pkgName string, pkgVersion string) (pkgInfo PackageInfo, err error) {
	url := fmt.Sprintf("%s/%s%s@%s/package.json", cdnOrigin, regPrefix, pkgName, pkgVersion)

	// check memory cache first
	if v, ok := fetchCache.Load(url); ok {
		pkgInfo, _ = v.(PackageInfo)
		return
	}

	// only one fetch at a time for the same url
	unlock := keyedMutex.Lock(url)
	defer unlock()

	// check memory cache again after acquiring the lock state
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
		cachePath := filepath.Join(appDir, "registry", regPrefix, pkgName+"@"+pkgInfo.Version+".json")
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, jsonData, 0644)
	}

	// cache the package info in memory
	fetchCache.Store(url, pkgInfo)
	if pkgInfo.Version != pkgVersion {
		fetchCache.Store(fmt.Sprintf("%s/%s%s@%s/package.json", cdnOrigin, regPrefix, pkgName, pkgInfo.Version), pkgInfo)
	}
	return
}

func resolveDependency(cdnOrigin string, dep Dependency) (pkgInfo PackageInfo, err error) {
	var regPrefix string
	if dep.Github {
		regPrefix = "gh/"
	} else if dep.Jsr {
		regPrefix = "jsr/"
	}
	return fetchPackageInfo(cdnOrigin, regPrefix, dep.Name, dep.Version)
}

func getPackageInfoFromUrl(urlRaw string) (pkgInfo PackageInfo, err error) {
	u, err := url.Parse(urlRaw)
	if err != nil {
		err = fmt.Errorf("invalid url: %s", urlRaw)
		return
	}
	pathname := u.Path
	if strings.HasPrefix(pathname, "/jsr/") {
		pkgInfo.Jsr = true
		pathname = pathname[4:]
	} else if strings.HasPrefix(pathname, "/gh/") {
		pkgInfo.Github = true
		pathname = pathname[3:]
	}
	segs := strings.Split(pathname[1:], "/")
	if len(segs) == 0 {
		err = fmt.Errorf("invalid url: %s", urlRaw)
		return
	}
	if strings.HasPrefix(segs[0], "@") {
		if len(segs) == 1 || segs[1] == "" {
			err = fmt.Errorf("invalid url: %s", urlRaw)
			return
		}
		name, version := utils.SplitByLastByte(segs[1], '@')
		pkgInfo.Name = segs[0] + "/" + name
		pkgInfo.Version = version
	} else {
		pkgInfo.Name, pkgInfo.Version = utils.SplitByLastByte(segs[0], '@')
	}
	// remove the leading `*` from the package name if it is from esm.sh
	if len(pkgInfo.Name) > 0 && pkgInfo.Name[0] == '*' {
		pkgInfo.Name = strings.TrimPrefix(pkgInfo.Name, "*")
	}
	return
}

func resolvePackageDependencies(pkg PackageInfo) (deps map[string]Dependency, err error) {
	peerDeps, err := resovleDependencies(pkg.PeerDependencies, true)
	if err != nil {
		return nil, err
	}
	pkgDeps, err := resovleDependencies(pkg.Dependencies, false)
	if err != nil {
		return nil, err
	}
	deps = make(map[string]Dependency, len(peerDeps)+len(pkgDeps))
	for _, dep := range peerDeps {
		deps[dep.Name] = dep
	}
	for _, dep := range pkgDeps {
		deps[dep.Name] = dep
	}
	return
}

func resovleDependencies(deps map[string]string, peer bool) ([]Dependency, error) {
	rdeps := make([]Dependency, 0, len(deps))
	for name, version := range deps {
		dep := Dependency{
			Name:    name,
			Version: version,
			Peer:    peer,
		}
		pkg, err := npm.ResolveDependencyVersion(version)
		if err != nil {
			return nil, err
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
		rdeps = append(rdeps, dep)
	}
	return rdeps, nil
}
