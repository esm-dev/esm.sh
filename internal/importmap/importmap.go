package importmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
)

// Config represents the configuration of an import map.
type Config struct {
	CDN    string `json:"cdn,omitempty"`
	Target string `json:"target,omitempty"`
}

// Imports represents a map of imports.
type Imports struct {
	lock    sync.RWMutex
	imports map[string]string
}

// Len returns the length of the imports map.
func (i *Imports) Len() int {
	i.lock.RLock()
	defer i.lock.RUnlock()
	return len(i.imports)
}

// Keys returns the keys of the imports map.
func (i *Imports) Keys() []string {
	i.lock.RLock()
	defer i.lock.RUnlock()
	keys := make([]string, len(i.imports))
	idx := 0
	for key := range i.imports {
		keys[idx] = key
		idx++
	}
	return keys
}

// Get returns the value of the key in the imports map.
func (i *Imports) Get(specifier string) (string, bool) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	url, ok := i.imports[specifier]
	return url, ok
}

// Set sets the value of the key in the imports map.
func (i *Imports) Set(specifier string, url string) {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.imports[specifier] = url
}

// Delete deletes the value of the key in the imports map.
func (i *Imports) Delete(specifier string) {
	i.lock.Lock()
	defer i.lock.Unlock()
	delete(i.imports, specifier)
}

// Range ranges over the imports map.
func (i *Imports) Range(fn func(specifier string, url string) bool) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	for specifier, url := range i.imports {
		if !fn(specifier, url) {
			break
		}
	}
}

// ImportMapJson represents the JSON structure of an import map.
type ImportMapJson struct {
	Config    Config                       `json:"config"`
	Imports   map[string]string            `json:"imports,omitempty"`
	Scopes    map[string]map[string]string `json:"scopes,omitempty"`
	Integrity map[string]string            `json:"integrity,omitempty"`
}

// ImportMap represents an import maps that follows the import maps specification:
// https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/script/type/importmap
type ImportMap struct {
	config    Config
	Imports   *Imports
	scopes    map[string]*Imports
	integrity *Imports
	baseUrl   *url.URL
	lock      sync.RWMutex
}

// Blank creates a new import map with empty imports and scopes.
func Blank() *ImportMap {
	return &ImportMap{
		Imports:   newImports(nil),
		scopes:    make(map[string]*Imports),
		integrity: newImports(nil),
	}
}

// Parse parses an importmap from a JSON string.
func Parse(baseUrl *url.URL, data []byte) (im *ImportMap, err error) {
	var importMapRaw ImportMapJson
	if err = json.Unmarshal(data, &importMapRaw); err != nil {
		return
	}
	scopes := make(map[string]*Imports)
	for scope, imports := range importMapRaw.Scopes {
		scopes[scope] = newImports(imports)
	}
	im = &ImportMap{
		baseUrl:   baseUrl,
		config:    importMapRaw.Config,
		Imports:   newImports(importMapRaw.Imports),
		scopes:    scopes,
		integrity: newImports(importMapRaw.Integrity),
	}
	return
}

// Config returns the config of the import map.
func (im *ImportMap) Config() Config {
	return im.config
}

// SetConfig sets the config of the import map.
func (im *ImportMap) SetConfig(config Config) {
	im.config = config
}

// Integrity returns the integrity of the import map.
func (im *ImportMap) Integrity() *Imports {
	return im.integrity
}

// SetIntegrity sets the integrity of the import map.
func (im *ImportMap) SetIntegrity(integrity *Imports) {
	im.integrity = integrity
}

// GetScopeImports returns the imports of the given scope.
func (im *ImportMap) GetScopeImports(scope string) (*Imports, bool) {
	im.lock.RLock()
	imports, ok := im.scopes[scope]
	im.lock.RUnlock()
	return imports, ok
}

// SetScopeImports sets the imports of the given scope.
func (im *ImportMap) SetScopeImports(scope string, imports *Imports) {
	im.lock.Lock()
	im.scopes[scope] = imports
	im.lock.Unlock()
}

// RangeScopes ranges over the scopes of the import map.
func (im *ImportMap) RangeScopes(fn func(scope string, imports *Imports) bool) {
	im.lock.RLock()
	defer im.lock.RUnlock()
	for scope, imports := range im.scopes {
		if !fn(scope, imports) {
			break
		}
	}
}

// Resolve resolves a specifier to a URL.
// It returns the URL and a boolean indicating if the specifier was found.
// This function follows the import maps specification:
// https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/script/type/importmap
func (im *ImportMap) Resolve(specifier string, referrer *url.URL) (string, bool) {
	if im.baseUrl == nil {
		im.baseUrl, _ = url.Parse("file:///")
	}

	var hash string
	specifier, hash = utils.SplitByFirstByte(specifier, '#')
	if hash != "" {
		hash = "#" + hash
	}

	var query string
	specifier, query = utils.SplitByFirstByte(specifier, '?')
	if query != "" {
		query = "?" + query
	}

	if referrer != nil && len(im.scopes) > 0 {
		scopeKeys := make(ScopeKeys, 0, len(im.scopes))
		for prefix := range im.scopes {
			scopeKeys = append(scopeKeys, prefix)
		}
		sort.Sort(scopeKeys)
		for _, scopeKey := range scopeKeys {
			if strings.HasPrefix(referrer.String(), scopeKey) {
				imports, _ := im.GetScopeImports(scopeKey)
				ret, ok := im.resolveWith(specifier, imports)
				if ok {
					return ret + query + hash, true
				}
			}
		}
	}

	ret, ok := im.resolveWith(specifier, im.Imports)
	if ok {
		return ret + query + hash, true
	}

	return specifier + query + hash, false
}

func (im *ImportMap) resolveWith(specifier string, imports *Imports) (string, bool) {
	if len(imports.imports) == 0 {
		return "", false
	}
	if url, ok := imports.Get(specifier); ok {
		return normalizeUrl(im.baseUrl, url), true
	}
	// try to match tailing slash specifier
	if strings.ContainsRune(specifier, '/') {
		var matchedUrl string
		imports.Range(func(k string, v string) bool {
			if strings.HasSuffix(k, "/") && strings.HasPrefix(specifier, k) {
				matchedUrl = normalizeUrl(im.baseUrl, v+specifier[len(k):])
				return false
			}
			return true
		})
		if matchedUrl != "" {
			return matchedUrl, true
		}
	}
	return "", false
}

// ParseImport gets the import metadata from a specifier.
// Currently, it supports the following specifiers:
// - npm:package[@semver][/subpath]
// - jsr:scope/package[@semver][/subpath]
// - github:owner/repo[@<branch|tag|commit>][/subpath]
func (im *ImportMap) ParseImport(specifier string) (meta ImportMeta, err error) {
	var imp Import
	var scopeName string
	if strings.HasPrefix(specifier, "gh:") {
		imp.Github = true
		specifier = specifier[3:]
	} else if strings.HasPrefix(specifier, "jsr:") {
		imp.Jsr = true
		specifier = specifier[4:]
	}
	if len(specifier) > 0 && (specifier[0] == '@' || imp.Github) {
		scopeName, specifier = utils.SplitByFirstByte(specifier, '/')
	}
	imp.Name, imp.SubPath = utils.SplitByFirstByte(specifier, '/')
	if imp.Name == "" {
		// ignore empty package name
		return
	}
	imp.Name, imp.Version = utils.SplitByFirstByte(imp.Name, '@')
	if imp.Name == "" || !npm.Naming.Match(imp.Name) || !(scopeName == "" || npm.Naming.Match(strings.TrimPrefix(scopeName, "@"))) || !(imp.Version == "" || npm.Versioning.Match(imp.Version)) {
		err = fmt.Errorf("invalid package name or version: %s", specifier)
		return
	}
	if scopeName != "" {
		imp.Name = scopeName + "/" + imp.Name
	}
	return fetchImportMeta(im.cdnOrigin(), imp, im.config.Target)
}

func (im *ImportMap) FetchImportMeta(imp Import) (meta ImportMeta, err error) {
	return fetchImportMeta(im.cdnOrigin(), imp, im.config.Target)
}

// AddImportFromSpecifier adds an import from a specifier to the import map.
func (im *ImportMap) AddImportFromSpecifier(specifier string, noSRI bool) (warnings []string, errors []error) {
	imp, err := im.ParseImport(specifier)
	if err != nil {
		errors = append(errors, err)
		return
	}
	return im.AddImport(imp, noSRI)
}

// AddImport adds an import to the import map.
func (im *ImportMap) AddImport(imp ImportMeta, noSRI bool) (warnings []string, errors []error) {
	return im.addImport(set.New[string](), imp, false, nil, noSRI)
}

// addImport adds an import to the import map.
func (im *ImportMap) addImport(mark *set.Set[string], imp ImportMeta, indirect bool, targetImports *Imports, noSRI bool) (warnings []string, errors []error) {
	specifier := imp.Specifier(false)
	if mark.Has(specifier) {
		return
	}
	mark.Add(specifier)

	cdnOrigin := im.cdnOrigin()
	cdnScopeImportsMap, cdnScoped := im.GetScopeImports(cdnOrigin + "/")
	if !cdnScoped {
		cdnScopeImportsMap = &Imports{imports: map[string]string{}}
		im.SetScopeImports(cdnOrigin+"/", cdnScopeImportsMap)
	}

	imports := im.Imports
	if indirect {
		if targetImports != nil {
			imports = targetImports
		} else {
			imports = cdnScopeImportsMap
		}
	}

	var target string
	switch v := im.config.Target; v {
	case "es2015", "es2016", "es2017", "es2018", "es2019", "es2020", "es2021", "es2022", "es2023", "es2024", "esnext":
		target = v
	default:
		target = "es2022"
	}

	moduleUrl := cdnOrigin + "/" + imp.EsmSpecifier() + "/"
	moduleUrl += target + "/"
	if imp.SubPath != "" {
		if imp.Dev || imp.SubPath == "jsx-dev-runtime" {
			moduleUrl += imp.SubPath + ".development.mjs"
		} else {
			moduleUrl += imp.SubPath + ".mjs"
		}
	} else {
		if strings.ContainsRune(imp.Name, '/') {
			_, name := utils.SplitByFirstByte(imp.Name, '/')
			moduleUrl += name + ".mjs"
		} else {
			moduleUrl += imp.Name + ".mjs"
		}
	}

	imports.Set(specifier, moduleUrl)
	if !indirect {
		cdnScopeImportsMap.Delete(specifier)
	}
	if !noSRI {
		if !imp.HasExternalImports() {
			if imp.Integrity != "" {
				im.integrity.Set(moduleUrl, imp.Integrity)
			}
		} else {
			meta, err := im.FetchImportMeta(Import{
				Name:     imp.Name,
				Version:  imp.Version,
				SubPath:  imp.SubPath,
				Github:   imp.Github,
				Jsr:      imp.Jsr,
				Dev:      imp.Dev,
				External: true,
			})
			if err != nil {
				errors = append(errors, err)
				return
			}
			if meta.Integrity != "" {
				im.integrity.Set(moduleUrl, meta.Integrity)
			}
		}
	} else {
		im.integrity.Delete(moduleUrl)
	}

	if imp.HasExternalImports() {
		peerImportsLen := len(imp.PeerImports)
		importsLen := len(imp.Imports)
		allImports := make([]string, len(imp.PeerImports)+len(imp.Imports))
		if peerImportsLen > 0 {
			copy(allImports, imp.PeerImports)
		}
		if importsLen > 0 {
			copy(allImports[peerImportsLen:], imp.Imports)
		}
		wg := sync.WaitGroup{}
		for i, pathname := range allImports {
			isPeer := i < peerImportsLen
			wg.Go(func() {
				if strings.HasPrefix(pathname, "/node/") {
					// ignore node built-in modules
					return
				}
				depImport, err := ParseEsmPath(pathname)
				if err != nil {
					errors = append(errors, err)
					return
				}
				// if the dependency is the same as the current import, use the version of the current import
				if depImport.Name == imp.Name {
					depImport.Version = imp.Version
				}
				depImportSpecifier := depImport.Specifier(false)
				addedUrl, exists := im.Imports.Get(depImportSpecifier)
				if !exists {
					addedUrl, exists = cdnScopeImportsMap.Get(depImportSpecifier)
				}
				var targetImports *Imports
				if exists && strings.HasPrefix(addedUrl, cdnOrigin+"/") {
					addedImport, err := ParseEsmPath(addedUrl)
					if err == nil && npm.IsExactVersion(addedImport.Version) {
						if depImport.Version == addedImport.Version {
							// the version of the dependency is exact and equals to the version in the import map
							return
						}
						// if the version of the dependency is not exact,
						// check if it is satisfied with the version in the import map
						// or create a new scope for the dependency
						if !npm.IsExactVersion(depImport.Version) {
							c, err := semver.NewConstraint(depImport.Version)
							if err == nil && c.Check(semver.MustParse(addedImport.Version)) {
								// the version of the dependency is exact and satisfied with the version in the import map
								return
							}
							if isPeer {
								warnings = append(warnings, "incorrect peer dependency "+depImport.Name+"@"+addedImport.Version+term.Dim("(unmet "+depImport.Version+")"))
								return
							}
							var ok bool
							scope := cdnOrigin + "/" + imp.EsmSpecifier() + "/"
							targetImports, ok = im.GetScopeImports(scope)
							if !ok {
								targetImports = newImports(nil)
								im.SetScopeImports(scope, targetImports)
							}
						}
					}
				}
				meta, err := im.FetchImportMeta(depImport)
				if err != nil {
					errors = append(errors, err)
					return
				}
				warns, errs := im.addImport(mark, meta, !isPeer, targetImports, noSRI)
				warnings = append(warnings, warns...)
				errors = append(errors, errs...)
			})
		}
		wg.Wait()
	}

	return
}

func (im *ImportMap) cdnOrigin() string {
	cdn := im.config.CDN
	if strings.HasPrefix(cdn, "https://") || strings.HasPrefix(cdn, "http://") {
		return cdn
	}
	return "https://esm.sh"
}

// MarshalJSON implements the json.Marshaler interface.
func (im *ImportMap) MarshalJSON() ([]byte, error) {
	return []byte(im.FormatJSON(0)), nil
}

// FormatJSON formats the import map as a JSON string.
func (im *ImportMap) FormatJSON(indent int) string {
	buf := strings.Builder{}
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent+1)
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteString("{\n")
	if cf := im.config; cf.CDN != "" || cf.Target != "" {
		buf.Write(indentStr)
		buf.WriteString("\"config\": {\n")
		if cf.CDN != "" {
			buf.Write(indentStr)
			buf.WriteString("  \"cdn\": \"")
			buf.WriteString(cf.CDN)
			buf.WriteString("\"")
			if cf.Target != "" {
				buf.WriteString(",\n")
			} else {
				buf.WriteByte('\n')
			}
		}
		if cf.Target != "" {
			buf.Write(indentStr)
			buf.WriteString("  \"target\": \"")
			buf.WriteString(cf.Target)
			buf.WriteString("\"\n")
		}
		buf.Write(indentStr)
		buf.WriteString("},\n")
	}
	buf.Write(indentStr)
	buf.WriteString("\"imports\": {")
	if im.Imports.Len() > 0 {
		buf.WriteByte('\n')
		formatImports(&buf, im.Imports, indent+2)
		buf.Write(indentStr)
		buf.WriteByte('}')
	} else {
		buf.WriteByte('}')
	}
	scopes := make([]string, 0, len(im.scopes))
	if len(im.scopes) > 0 {
		for key, imports := range im.scopes {
			if imports.Len() > 0 {
				scopes = append(scopes, key)
			}
		}
	}
	sort.Strings(scopes)
	if len(scopes) > 0 {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"scopes\": {\n")
		i := 0
		for _, scope := range scopes {
			imports, _ := im.GetScopeImports(scope)
			if imports.Len() == 0 {
				continue
			}
			buf.Write(indentStr)
			buf.WriteString("  \"")
			buf.WriteString(scope)
			buf.WriteString("\": {\n")
			formatImports(&buf, imports, indent+3)
			buf.Write(indentStr)
			buf.WriteString("  }")
			if len(scopes) > 1 && i < len(scopes)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
			i++
		}
		buf.Write(indentStr)
		buf.WriteByte('}')
	}
	if im.integrity.Len() > 0 {
		buf.WriteString(",\n")
		buf.Write(indentStr)
		buf.WriteString("\"integrity\": {\n")
		formatMap(&buf, im.integrity, indent+2)
		buf.Write(indentStr)
		buf.WriteByte('}')
	}
	buf.WriteByte('\n')
	buf.Write(indentStr[0 : 2*indent])
	buf.WriteByte('}')
	return buf.String()
}

func formatImports(buf *strings.Builder, imports *Imports, indent int) {
	keys := imports.Keys()
	sort.Strings(keys)
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent)
	for i, key := range keys {
		url, ok := imports.Get(key)
		if !ok || url == "" {
			// ignore empty values
			continue
		}
		buf.Write(indentStr)
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString("\": \"")
		buf.WriteString(url)
		buf.WriteByte('"')
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
}

func formatMap(buf *strings.Builder, m *Imports, indent int) {
	keys := m.Keys()
	sort.Strings(keys)
	indentStr := bytes.Repeat([]byte{' ', ' '}, indent)
	for i, key := range keys {
		value, ok := m.Get(key)
		if !ok || value == "" {
			// ignore empty values
			continue
		}
		buf.Write(indentStr)
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString("\": \"")
		buf.WriteString(value)
		buf.WriteByte('"')
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
}

func newImports(imports map[string]string) *Imports {
	if imports == nil {
		imports = map[string]string{}
	}
	return &Imports{imports: imports}
}

func normalizeUrl(baseUrl *url.URL, path string) string {
	if baseUrl != nil && (strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")) {
		return baseUrl.ResolveReference(&url.URL{Path: path}).String()
	}
	return path
}
