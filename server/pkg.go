package server

import (
	"fmt"
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
	if strings.HasPrefix(packageName, "@") && len(a) > 1 {
		scope = packageName[1:]
		packageName = a[1]
		submodule = strings.Join(a[2:], "/")
	}

	// ref https://github.com/npm/validate-npm-package-name
	if scope != "" && (len(scope) > 214 || !npmNaming.Is(scope)) {
		return nil, fmt.Errorf("invalid scope '%s'", scope)
	}

	name, version := utils.SplitByLastByte(packageName, '@')
	if name != "" && (len(name) > 214 || !npmNaming.Is(name)) {
		return nil, fmt.Errorf("invalid package name '%s'", name)
	}

	if scope != "" {
		name = fmt.Sprintf("@%s/%s", scope, name)
	}
	if version == "" {
		version = "latest"
	}
	info, _, _, err := node.getPackageInfo("", name, version)
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
