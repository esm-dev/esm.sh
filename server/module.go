package server

import (
	"strings"

	"github.com/ije/gox/utils"
)

type module struct {
	name      string
	version   string
	submodule string
}

func parseModule(pathname string) (*module, error) {
	a := strings.Split(strings.Trim(pathname, "/"), "/")
	for i, s := range a {
		a[i] = strings.TrimSpace(s)
	}
	scope := ""
	packageName := a[0]
	submodule := strings.Join(a[1:], "/")
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		scope = a[0]
		packageName = a[1]
		submodule = strings.Join(a[2:], "/")
	}
	name, version := utils.SplitByLastByte(packageName, '@')
	if scope != "" {
		name = scope + "/" + name
	}
	if name != "" && version == "" {
		info, err := nodeEnv.getPackageLatestInfo(name)
		if err != nil {
			return nil, err
		}
		version = info.Version
	}
	return &module{
		name:      name,
		version:   version,
		submodule: submodule,
	}, nil
}

func (m module) Equels(other module) bool {
	return m.name == other.name && m.version == other.version && m.submodule == other.submodule
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
