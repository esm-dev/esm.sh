package importmap

import (
	"crypto/sha256"
	"encoding/hex"
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

// Import represents an import from esm.sh CDN.
type Import struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	SubPath  string `json:"subpath"`
	Github   bool   `json:"-"`
	Jsr      bool   `json:"-"`
	External bool   `json:"-"`
	Dev      bool   `json:"-"`
}

func (im Import) Specifier(withVersion bool) string {
	b := strings.Builder{}
	if im.Github {
		b.WriteString("gh:")
	} else if im.Jsr {
		b.WriteString("jsr:")
	}
	b.WriteString(im.Name)
	if withVersion && im.Version != "" {
		b.WriteByte('@')
		b.WriteString(im.Version)
	}
	if im.SubPath != "" {
		b.WriteByte('/')
		b.WriteString(im.SubPath)
	}
	return b.String()
}

func (im Import) RegistryPrefix() string {
	if im.Github {
		return "gh/"
	}
	if im.Jsr {
		return "jsr/"
	}
	return ""
}

// ImportMeta represents the import metadata of a import.
type ImportMeta struct {
	Import
	Integrity   string   `json:"integrity"` // "sha384-..."
	Exports     []string `json:"exports"`
	Imports     []string `json:"imports"`
	PeerImports []string `json:"peerImports"`
}

// HasExternalImports returns true if the import has external imports.
func (imp *ImportMeta) HasExternalImports() bool {
	if len(imp.PeerImports) > 0 {
		return true
	}
	for _, importPath := range imp.Imports {
		if !strings.HasPrefix(importPath, "/node/") && !strings.HasPrefix(importPath, "/"+imp.Name+"@") {
			return true
		}
	}
	return false
}

// EsmSpecifier returns the esm specifier of the import meta.
func (imp *ImportMeta) EsmSpecifier() string {
	b := strings.Builder{}
	if imp.Github {
		b.WriteString("gh/")
	} else if imp.Jsr {
		b.WriteString("jsr/")
	}
	if imp.HasExternalImports() {
		b.WriteString("*") // add "external all" modifier of esm.sh
	}
	b.WriteString(imp.Name)
	b.WriteByte('@')
	b.WriteString(imp.Version)
	return b.String()
}

// fetchImportMeta fetches the import metadata from the esm.sh CDN.
func fetchImportMeta(cdnOrigin string, imp Import, target string) (meta ImportMeta, err error) {
	asteriskPrefix := ""
	version := ""
	subPath := ""
	if imp.External {
		asteriskPrefix = "*"
	}
	if imp.Version != "" {
		version = "@" + imp.Version
	}
	if imp.SubPath != "" {
		subPath = "/" + imp.SubPath
	}
	url := fmt.Sprintf("%s/%s%s%s%s%s?meta", cdnOrigin, asteriskPrefix, imp.RegistryPrefix(), imp.Name, version, subPath)
	if target != "" && target != "es2022" {
		url += "&target=" + target
	}

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

	sha := sha256.Sum256([]byte(url))
	cachePath := filepath.Join(appDir, "meta", hex.EncodeToString(sha[:]))

	// if the version is exact, check the cache on disk
	if npm.IsExactVersion(imp.Version) {
		f, err := os.Open(cachePath)
		if err == nil {
			defer f.Close()
			err = json.NewDecoder(f).Decode(&meta)
			if err == nil {
				meta.Name = imp.Name
				meta.Github = imp.Github
				meta.Jsr = imp.Jsr
				fetchCache.Store(url, meta)
				return meta, nil
			}
			// if decode error, remove the cache file and try to fetch again
			_ = os.Remove(cachePath)
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		err = fmt.Errorf("package not found: %s", imp.Specifier(true))
		return
	}

	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("unexpected http status %d: %s", resp.StatusCode, msg)
		return
	}

	jsonData, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("could not read %s: %s", url, err.Error())
		return
	}

	err = json.Unmarshal(jsonData, &meta)
	if err != nil {
		err = fmt.Errorf("could not decode %s: %s", url, err.Error())
		return
	}

	meta.Name = imp.Name
	meta.Github = imp.Github
	meta.Jsr = imp.Jsr

	// cache the metadata on disk
	dirname := filepath.Dir(cachePath)
	if _, err := os.Lstat(dirname); err != nil && os.IsNotExist(err) {
		os.MkdirAll(dirname, 0755)
	}
	os.WriteFile(cachePath, jsonData, 0644)

	// cache the metadata in memory
	fetchCache.Store(url, meta)
	if meta.Version != imp.Version {
		// cache the exact version as well
		cacheKey := fmt.Sprintf("%s/%s%s%s@%s%s?meta", cdnOrigin, asteriskPrefix, imp.RegistryPrefix(), imp.Name, meta.Version, subPath)
		if target != "" {
			cacheKey += "&target=" + target
		}
		fetchCache.Store(cacheKey, meta)
	}
	return
}

// ParseEsmPath parses an import from a pathname or URL.
func ParseEsmPath(pathnameOrUrl string) (imp Import, err error) {
	var pathname string
	if strings.HasPrefix(pathnameOrUrl, "https://") || strings.HasPrefix(pathnameOrUrl, "http://") {
		var u *url.URL
		u, err = url.Parse(pathnameOrUrl)
		if err != nil {
			return
		}
		pathname = u.Path
	} else if strings.HasPrefix(pathnameOrUrl, "/") {
		var u *url.URL
		u, err = url.Parse("https://esm.sh" + pathnameOrUrl)
		if err != nil {
			return
		}
		pathname = u.Path
	} else {
		err = fmt.Errorf("invalid pathname or url: %s", pathnameOrUrl)
		return
	}
	if strings.HasPrefix(pathname, "/gh/") {
		imp.Github = true
		pathname = pathname[3:]
	} else if strings.HasPrefix(pathname, "/jsr/") {
		imp.Jsr = true
		pathname = pathname[4:]
	}
	segs := strings.Split(utils.NormalizePathname(pathname)[1:], "/")
	if len(segs) == 0 {
		err = fmt.Errorf("invalid pathname: %s", pathname)
		return
	}
	if strings.HasPrefix(segs[0], "@") {
		if len(segs) == 1 || segs[1] == "" {
			err = fmt.Errorf("invalid pathname: %s", pathname)
			return
		}
		name, version := utils.SplitByLastByte(segs[1], '@')
		imp.Name = segs[0] + "/" + name
		imp.Version = version
		segs = segs[2:]
	} else {
		imp.Name, imp.Version = utils.SplitByLastByte(segs[0], '@')
		segs = segs[1:]
	}
	// remove the leading `*` from the package name if it is from esm.sh
	imp.Name = strings.TrimPrefix(imp.Name, "*")
	if len(segs) > 0 {
		var hasTargetSegment bool
		switch segs[0] {
		case "es2015", "es2016", "es2017", "es2018", "es2019", "es2020", "es2021", "es2022", "es2023", "es2024", "esnext", "denonext", "deno", "node":
			// remove the target segment of esm.sh
			segs = segs[1:]
			hasTargetSegment = true
		}
		if len(segs) > 0 {
			if hasTargetSegment && strings.HasSuffix(pathname, ".mjs") {
				subPath := strings.TrimSuffix(strings.Join(segs, "/"), ".mjs")
				if strings.HasSuffix(subPath, ".development") {
					subPath = strings.TrimSuffix(subPath, ".development")
					imp.Dev = true
				}
				if strings.ContainsRune(subPath, '/') || (subPath != imp.Name && !strings.HasSuffix(imp.Name, "/"+subPath)) {
					imp.SubPath = subPath
				}
			} else {
				imp.SubPath = strings.Join(segs, "/")
			}
		}
	}
	return
}
