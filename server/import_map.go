package server

import "strings"

type ImportMap struct {
	Src     string                       `json:"$src,omitempty"`
	Support bool                         `json:"$support,omitempty"`
	Imports map[string]string            `json:"imports,omitempty"`
	Scopes  map[string]map[string]string `json:"scopes,omitempty"`
}

func (m ImportMap) Resolve(path string) (string, bool) {
	imports := m.Imports
	// todo: check `scopes`
	if len(imports) > 0 {
		if v, ok := imports[path]; ok {
			if m.Support {
				return path, true
			}
			return v, true
		}
		if strings.ContainsRune(path, '/') {
			nonTrailingSlashImports := make([][2]string, 0, len(imports))
			for k, v := range imports {
				if strings.HasSuffix(k, "/") {
					if strings.HasPrefix(path, k) {
						if m.Support {
							return path, true
						}
						return v + path[len(k):], true
					}
				} else {
					nonTrailingSlashImports = append(nonTrailingSlashImports, [2]string{k, v})
				}
			}
			// expand match
			// e.g. `"react": "https://esm.sh/react@18` -> `"react/": "https://esm.sh/react@18/`
			for _, p := range nonTrailingSlashImports {
				k, v := p[0], p[1]
				if strings.HasPrefix(path, k+"/") {
					return v + path[len(k):], true
				}
			}
		}
	}
	return path, false
}
