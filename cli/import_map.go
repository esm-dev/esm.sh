package cli

import (
	"net/url"
	"strings"

	"github.com/ije/gox/utils"
)

type ImportMap struct {
	Src     string                       `json:"$src,omitempty"`
	Imports map[string]string            `json:"imports,omitempty"`
	Scopes  map[string]map[string]string `json:"scopes,omitempty"`
	srcUrl  *url.URL
}

func (m ImportMap) Resolve(path string) (string, bool) {
	imports := m.Imports
	if m.srcUrl == nil && m.Src != "" {
		m.srcUrl, _ = url.Parse(m.Src)
	}
	// todo: check `scopes`
	if len(imports) > 0 {
		if v, ok := imports[path]; ok {
			return m.toAbsPath(v), true
		}
		if strings.ContainsRune(path, '/') {
			nonTrailingSlashImports := make([][2]string, 0, len(imports))
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(path, k) {
						return m.toAbsPath(v + path[len(k):]), true
					}
				} else {
					nonTrailingSlashImports = append(nonTrailingSlashImports, [2]string{k, v})
				}
			}
			// expand match
			// e.g. `"react": "https://esm.sh/react@18` -> `"react/": "https://esm.sh/react@18/`
			for _, p := range nonTrailingSlashImports {
				k, v := p[0], p[1]
				p, q := utils.SplitByLastByte(v, '?')
				if strings.HasPrefix(path, k+"/") {
					url := p + path[len(k):]
					if q != "" {
						url += "?" + q
					}
					return m.toAbsPath(url), true
				}
			}
		}
	}
	return path, false
}

func (m ImportMap) toAbsPath(path string) string {
	if isRelPathSpecifier(path) {
		if m.srcUrl != nil {
			return m.srcUrl.ResolveReference(&url.URL{Path: path}).String()
		}
		return path
	}
	return path
}
