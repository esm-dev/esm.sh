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
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/utils"
)

var (
	fetchCache sync.Map
	fetchLocks sync.Map
	fetchLimit = make(chan struct{}, 8)
	httpClient = &http.Client{Timeout: time.Minute}
)

// Import represents an import from esm.sh CDN.
type Import struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	SubPath  string `json:"-"`
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

func (im Import) npmName() string {
	if im.Jsr && strings.HasPrefix(im.Name, "@") {
		return "@jsr/" + strings.Replace(im.Name[1:], "/", "__", 1)
	}
	return im.Name
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
	Module      string   `json:"module"`
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
		if !strings.HasPrefix(importPath, "/node/") && !strings.HasPrefix(importPath, "/"+imp.npmName()+"@") {
			return true
		}
	}
	return false
}

// EsmSpecifier returns the esm specifier of the import meta.
func (imp *ImportMeta) EsmSpecifier() string {
	b := strings.Builder{}
	if imp.HasExternalImports() {
		b.WriteString("*") // add "external all" modifier of esm.sh
	}
	// [warning] the server doesn't support the "jsr/*" form, the "*" must come before the registry prefix
	b.WriteString(imp.RegistryPrefix())
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
	if imp.Dev {
		url += "&dev"
	}

	// check memory cache first
	if v, ok := fetchCache.Load(url); ok {
		meta, _ = v.(ImportMeta)
		return
	}

	// only one fetch at a time for the same url
	done := make(chan struct{})
	for {
		v, loaded := fetchLocks.LoadOrStore(url, done)
		if !loaded {
			break
		}
		<-v.(chan struct{})
	}
	defer func() {
		fetchLocks.Delete(url)
		close(done)
	}()

	// check memory cache again after acquiring the lock
	if v, ok := fetchCache.Load(url); ok {
		meta, _ = v.(ImportMeta)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("could not get app directory: %s", err.Error())
		return
	}
	appDir := filepath.Join(homeDir, ".esm.sh")
	if runtime.GOOS == "windows" {
		appDir = filepath.Join(homeDir, "AppData", "Local", "esm.sh")
	}

	sha := sha256.Sum256([]byte(url))
	cachePath := filepath.Join(appDir, "meta", hex.EncodeToString(sha[:]))

	// if the version is exact, check the cache on disk
	var jsonData []byte
	if npm.IsExactVersion(imp.Version) {
		jsonData, _ = os.ReadFile(cachePath)
	}
	if jsonData == nil || json.Unmarshal(jsonData, &meta) != nil {
		meta = ImportMeta{}
		fetchLimit <- struct{}{}
		defer func() { <-fetchLimit }()

		var resp *http.Response
		resp, err = httpClient.Get(url)
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

		jsonData, err = io.ReadAll(resp.Body)
		if err != nil {
			err = fmt.Errorf("could not read %s: %s", url, err.Error())
			return
		}

		err = json.Unmarshal(jsonData, &meta)
		if err != nil {
			err = fmt.Errorf("could not decode %s: %s", url, err.Error())
			return
		}

		// cache the metadata on disk
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, jsonData, 0644)
	}

	// [warning] keep the requested name, the CDN returns a jsr package as "@jsr/scope__name"
	meta.Name = imp.Name
	meta.SubPath = imp.SubPath
	meta.Github = imp.Github
	meta.Jsr = imp.Jsr
	meta.External = imp.External
	meta.Dev = imp.Dev

	// cache the metadata in memory
	fetchCache.Store(url, meta)
	if meta.Version != imp.Version {
		// cache the exact version as well
		cacheKey := fmt.Sprintf("%s/%s%s%s@%s%s?meta", cdnOrigin, asteriskPrefix, imp.RegistryPrefix(), imp.Name, meta.Version, subPath)
		if target != "" && target != "es2022" {
			cacheKey += "&target=" + target
		}
		if imp.Dev {
			cacheKey += "&dev"
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
	if strings.HasPrefix(pathname, "/*") {
		pathname = "/" + pathname[2:]
	}
	if strings.HasPrefix(pathname, "/gh/") {
		imp.Github = true
		pathname = pathname[3:]
	} else if strings.HasPrefix(pathname, "/jsr/") {
		imp.Jsr = true
		pathname = pathname[4:]
	}
	segs := strings.Split(utils.NormalizePathname(pathname)[1:], "/")
	segs[0] = strings.TrimPrefix(segs[0], "*")
	if segs[0] == "" {
		err = fmt.Errorf("invalid pathname: %s", pathname)
		return
	}
	if imp.Github || strings.HasPrefix(segs[0], "@") {
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
				subPath, imp.Dev = strings.CutSuffix(subPath, ".development")
				if strings.ContainsRune(subPath, '/') || subPath != path.Base(imp.npmName()) {
					imp.SubPath = subPath
				}
			} else {
				imp.SubPath = strings.Join(segs, "/")
			}
		}
	}
	return
}
