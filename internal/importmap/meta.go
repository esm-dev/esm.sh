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

type ImportMeta struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Exports []string `json:"exports"`
	Imports []string `json:"imports"`
	SubPath string   `json:"subpath"`
	Github  bool     `json:"-"`
	Jsr     bool     `json:"-"`
}

func (meta ImportMeta) Specifier() string {
	b := strings.Builder{}
	if meta.Jsr {
		b.WriteString("jsr:")
	}
	b.WriteString(meta.Name)
	if meta.SubPath != "" {
		b.WriteByte('/')
		b.WriteString(meta.SubPath)
	}
	return b.String()
}

func (meta ImportMeta) String() string {
	b := strings.Builder{}
	if meta.Github {
		b.WriteString("gh/")
	} else if meta.Jsr {
		b.WriteString("jsr/")
	}
	if len(meta.Imports) > 0 {
		b.WriteString("*") // add "external all" modifier of esm.sh
	}
	b.WriteString(meta.Name)
	b.WriteByte('@')
	b.WriteString(meta.Version)
	return b.String()
}

func FetchImportMeta(cdnOrigin string, regPrefix string, pkgName string, pkgVersion string, subpath string) (meta ImportMeta, err error) {
	url := fmt.Sprintf("%s/%s%s@%s?meta", cdnOrigin, regPrefix, pkgName, pkgVersion)

	// check memory cache first
	if v, ok := fetchCache.Load(url); ok {
		meta, _ = v.(ImportMeta)
		return
	}

	// only one fetch at a time for the same url
	unlock := keyedMutex.Lock(url)
	defer unlock()

	// check memory cache again after acquiring the lock state
	if v, ok := fetchCache.Load(url); ok {
		meta, _ = v.(ImportMeta)
		return
	}

	appDir, err := app_dir.GetAppDir()
	if err != nil {
		err = fmt.Errorf("could not get app directory: %s", err.Error())
		return
	}

	// if the version is exact, check the cache on disk
	if npm.IsExactVersion(pkgVersion) && appDir != "" {
		name := pkgName + "@" + pkgVersion
		if subpath != "" {
			name += "/" + subpath
		}
		cachePath := filepath.Join(appDir, "meta", regPrefix, name+".json")
		dirname := filepath.Dir(cachePath)
		if _, err := os.Lstat(dirname); err != nil && os.IsNotExist(err) {
			_ = os.MkdirAll(dirname, 0755)
		}
		f, err := os.Open(cachePath)
		if err == nil {
			defer f.Close()
			err = json.NewDecoder(f).Decode(&meta)
			if err == nil {
				switch regPrefix {
				case "gh/":
					meta.Github = true
				case "jsr/":
					meta.Name = pkgName
					meta.Jsr = true
				}
				fetchCache.Store(url, meta)
				return meta, nil
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

	err = json.Unmarshal(jsonData, &meta)
	if err != nil {
		err = fmt.Errorf("could not decode %s@%s/package.json: %s", pkgName, pkgVersion, err.Error())
		return
	}

	switch regPrefix {
	case "gh/":
		meta.Github = true
	case "jsr/":
		meta.Name = pkgName
		meta.Jsr = true
	}

	// cache the package.json on disk
	if appDir != "" {
		cachePath := filepath.Join(appDir, "registry", regPrefix, pkgName+"@"+meta.Version+".json")
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, jsonData, 0644)
	}

	// cache the package info in memory
	fetchCache.Store(url, meta)
	if meta.Version != pkgVersion {
		fetchCache.Store(fmt.Sprintf("%s/%s%s@%s/package.json", cdnOrigin, regPrefix, pkgName, meta.Version), meta)
	}
	return
}

func ParseEsmPath(urlRaw string) (pkgInfo ImportMeta, err error) {
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
