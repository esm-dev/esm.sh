package server

import (
	"fmt"
	"strings"

	"github.com/ije/gox/utils"
)

type Pkg struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Submodule     string `json:"submodule"`
	FullSubmodule string `json:"fullsubmodule"`
}

func validatePkgPath(pathname string) (Pkg, string, error) {
	pkgName, submodule := splitPkgPath(pathname)
	fullSubmodule := submodule
	if submodule != "" {
		submodule = strings.TrimSuffix(submodule, ".js")
		submodule = strings.TrimSuffix(submodule, ".mjs")
	}
	name, maybeVersion := utils.SplitByLastByte(pkgName, '@')
	if strings.HasPrefix(pkgName, "@") {
		name, maybeVersion = utils.SplitByLastByte(pkgName[1:], '@')
		name = "@" + name
	}

	if !validatePackageName(name) {
		return Pkg{}, "", fmt.Errorf("invalid package name '%s'", name)
	}

	version, q := utils.SplitByFirstByte(maybeVersion, '&')
	if regexpFullVersion.MatchString(version) {
		for prefix, ver := range fixedPkgVersions {
			if strings.HasPrefix(name+"@"+version, prefix) {
				version = ver
			}
		}
		return Pkg{
			Name:          name,
			Version:       version,
			Submodule:     submodule,
			FullSubmodule: fullSubmodule,
		}, q, nil
	}

	info, _, err := getPackageInfo("", name, version)
	if err != nil {
		return Pkg{}, "", err
	}

	return Pkg{
		Name:          name,
		Version:       info.Version,
		Submodule:     submodule,
		FullSubmodule: fullSubmodule,
	}, q, nil
}

func (m Pkg) Equels(other Pkg) bool {
	return m.Name == other.Name && m.Version == other.Version && m.Submodule == other.Submodule
}

func (m Pkg) ImportPath() string {
	if m.Submodule != "" {
		return m.Name + "/" + m.Submodule
	}
	return m.Name
}

func (m Pkg) String() string {
	s := m.Name + "@" + m.Version
	if m.Submodule != "" {
		s += "/" + m.Submodule
	}
	return s
}

// sortable pkg slice
type PkgSlice []Pkg

func (a PkgSlice) Len() int           { return len(a) }
func (a PkgSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PkgSlice) Less(i, j int) bool { return a[i].String() < a[j].String() }

func (a PkgSlice) Has(name string) bool {
	for _, m := range a {
		if m.Name == name {
			return false
		}
	}
	return false
}

func (a PkgSlice) Get(name string) (Pkg, bool) {
	for _, m := range a {
		if m.Name == name {
			return m, true
		}
	}
	return Pkg{}, false
}

func (a PkgSlice) String() string {
	s := make([]string, a.Len())
	for i, m := range a {
		s[i] = m.String()
	}
	return strings.Join(s, ",")
}
