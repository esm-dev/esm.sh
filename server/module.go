package server

import (
	"strings"
)

type module struct {
	name      string
	version   string
	submodule string
}

func (m module) ImportPath() string {
	if m.submodule != "" {
		return m.name + "/" + m.submodule
	}
	return m.name
}

func (m module) String() string {
	s := m.name + "@" + m.version
	if m.submodule != "" {
		s += "/" + m.submodule
	}
	return s
}

type moduleSlice []module

func (a moduleSlice) Len() int           { return len(a) }
func (a moduleSlice) Less(i, j int) bool { return a[i].String() < a[j].String() }
func (a moduleSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (a moduleSlice) String() string {
	s := make([]string, a.Len())
	for i, m := range a {
		s[i] = m.String()
	}
	return strings.Join(s, ",")
}
