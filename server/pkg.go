package server

import (
	"errors"
	"strings"

	"github.com/ije/gox/utils"
)

type pkg struct {
	name      string
	version   string
	submodule string
}

func parsePkg(pathname string) (*pkg, error) {
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

	if strings.HasSuffix(submodule, ".d.ts") {
		return nil, errors.New("invalid path")
	}

	name, version := utils.SplitByLastByte(packageName, '@')
	if scope != "" {
		name = scope + "/" + name
	}
	if name == "" {
		return nil, errors.New("invalid path")
	}

	if version == "" {
		version = "latest"
	}
	info, _, err := node.getPackageInfo(name, version)
	if err != nil {
		return nil, err
	}

	return &pkg{
		name:      name,
		version:   info.Version,
		submodule: strings.TrimSuffix(submodule, ".js"),
	}, nil
}

func (m pkg) Equels(other pkg) bool {
	return m.name == other.name && m.version == other.version && m.submodule == other.submodule
}

func (m pkg) ImportPath() string {
	if m.submodule != "" {
		return m.name + "/" + m.submodule
	}
	return m.name
}

func (m pkg) String() string {
	s := m.name + "@" + m.version
	if m.submodule != "" {
		s += "/" + m.submodule
	}
	return s
}

// sortable pkg slice
type pkgSlice []pkg

func (a pkgSlice) Len() int           { return len(a) }
func (a pkgSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a pkgSlice) Less(i, j int) bool { return a[i].String() < a[j].String() }

func (a pkgSlice) Has(name string) bool {
	for _, m := range a {
		if m.name == name {
			return false
		}
	}
	return false
}

func (a pkgSlice) String() string {
	s := make([]string, a.Len())
	for i, m := range a {
		s[i] = m.String()
	}
	return strings.Join(s, ",")
}
