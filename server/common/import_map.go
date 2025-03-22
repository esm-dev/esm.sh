package common

import (
	"net/url"
	"strings"

	"github.com/ije/gox/utils"
)

type ImportMap struct {
	Src       string                       `json:"$src,omitempty"`
	Imports   map[string]string            `json:"imports,omitempty"`
	Scopes    map[string]map[string]string `json:"scopes,omitempty"`
	Integrity map[string]string            `json:"integrity,omitempty"`
	Routes    map[string]string            `json:"routes,omitempty"`
	srcUrl    *url.URL
}

func (m ImportMap) Resolve(path string) (string, bool) {
	var query string
	path, query = utils.SplitByFirstByte(path, '?')
	if query != "" {
		query = "?" + query
	}
	imports := m.Imports
	if m.srcUrl == nil && m.Src != "" {
		m.srcUrl, _ = url.Parse(m.Src)
	}
	// todo: check `scopes`
	if len(imports) > 0 {
		if v, ok := imports[path]; ok {
			return m.toAbsPath(v) + query, true
		}
		if strings.ContainsRune(path, '/') {
			nonTrailingSlashImports := make([][2]string, 0, len(imports))
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(path, k) {
						return m.toAbsPath(v+path[len(k):]) + query, true
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
				if q != "" {
					q = "?" + q
					if query != "" {
						q += "&" + query[1:]
					}
				} else if query != "" {
					q = query
				}
				if strings.HasPrefix(path, k+"/") {
					return m.toAbsPath(p+path[len(k):]) + q, true
				}
			}
		}
	}
	return path + query, false
}

func (m ImportMap) toAbsPath(path string) string {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		if m.srcUrl != nil {
			return m.srcUrl.ResolveReference(&url.URL{Path: path}).String()
		}
		return path
	}
	return path
}
