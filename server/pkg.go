package server

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

type Pkg struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Submodule     string `json:"submodule"`
	FullSubmodule string `json:"fullsubmodule"`
	FromGithub    bool   `json:"fromGithub"`
}

func validatePkgPath(pathname string) (pkg Pkg, query string, err error) {
	fromGithub := strings.HasPrefix(pathname, "/gh/") && strings.Count(pathname, "/") >= 3
	if fromGithub {
		pathname = "/@" + pathname[4:]
	}
	pkgName, fullSubmodule := splitPkgPath(pathname)
	name, maybeVersion := utils.SplitByLastByte(pkgName, '@')
	if strings.HasPrefix(pkgName, "@") {
		name, maybeVersion = utils.SplitByLastByte(pkgName[1:], '@')
		name = "@" + name
	}
	if !validatePackageName(name) {
		return Pkg{}, "", fmt.Errorf("invalid package name '%s'", name)
	}

	version, query := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}

	submodule := fullSubmodule
	if submodule != "" {
		submodule = strings.TrimSuffix(submodule, ".js")
		submodule = strings.TrimSuffix(submodule, ".mjs")
	}

	pkg = Pkg{
		Name:          name,
		Version:       version,
		Submodule:     submodule,
		FullSubmodule: fullSubmodule,
		FromGithub:    fromGithub,
	}

	if fromGithub {
		// strip the leading `@`
		pkg.Name = pkg.Name[1:]
		if valid.IsHexString(pkg.Version) && len(pkg.Version) >= 10 {
			return
		}
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", pkg.Name))
		if err != nil {
			return
		}
		if pkg.Version == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					pkg.Version = ref.Sha[:10]
					return
				}
			}
		} else if strings.HasPrefix(pkg.Version, "semver:") {
			// TODO: support semver
		} else {
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+pkg.Version || ref.Ref == "refs/heads/"+pkg.Version {
					pkg.Version = ref.Sha[:10]
					return
				}
			}
		}
		err = fmt.Errorf("tag or branch not found")
		return
	}

	// use fixed version
	for prefix, fixedVersion := range fixedPkgVersions {
		if strings.HasPrefix(name+"@"+version, prefix) {
			pkg.Version = fixedVersion
			return
		}
	}

	if regexpFullVersion.MatchString(version) {
		return
	}

	p, _, err := getPackageInfo("", name, version)
	if err == nil {
		pkg.Version = p.Version
	}
	return
}

func (pkg Pkg) Equels(other Pkg) bool {
	return pkg.Name == other.Name && pkg.Version == other.Version && pkg.Submodule == other.Submodule
}

func (pkg Pkg) ImportPath() string {
	if pkg.Submodule != "" {
		return pkg.Name + "/" + pkg.Submodule
	}
	return pkg.Name
}

func (pkg Pkg) VersionName() string {
	s := pkg.Name + "@" + pkg.Version
	if pkg.FromGithub {
		s = "gh/" + s
	}
	return s
}

func (pkg Pkg) String() string {
	s := pkg.VersionName()
	if pkg.Submodule != "" {
		s += "/" + pkg.Submodule
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

func splitPkgPath(pathname string) (pkgName string, submodule string) {
	a := strings.Split(strings.Trim(pathname, "/"), "/")
	pkgName = a[0]
	submodule = strings.Join(a[1:], "/")
	if strings.HasPrefix(pkgName, "@") && len(a) > 1 {
		pkgName = a[0] + "/" + a[1]
		submodule = strings.Join(a[2:], "/")
	}
	return
}
