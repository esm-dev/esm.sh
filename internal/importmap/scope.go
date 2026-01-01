package importmap

import "strings"

type ScopeKeys []string

// Len implements the sort.Interface interface.
func (s ScopeKeys) Len() int {
	return len(s)
}

// Less implements the sort.Interface interface.
// sort by the number of slashes in the key
func (s ScopeKeys) Less(i, j int) bool {
	iStr := s[i]
	jStr := s[j]
	iLen := strings.Count(iStr, "/")
	jLen := strings.Count(jStr, "/")
	if iLen == jLen {
		return iStr > jStr
	}
	return iLen > jLen
}

// Swap implements the sort.Interface interface.
func (s ScopeKeys) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
