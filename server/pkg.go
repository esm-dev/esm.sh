package server

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

type Pkg struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	SubPath    string `json:"subPath"`
	SubModule  string `json:"subModule"`
	FromGithub bool   `json:"fromGithub"`
	FromEsmsh  bool   `json:"fromEsmsh"`
}

func validatePkgPath(pathname string) (pkg Pkg, extraQuery string, err error) {
	fromGithub := strings.HasPrefix(pathname, "/gh/") && strings.Count(pathname, "/") >= 3
	if fromGithub {
		pathname = "/@" + pathname[4:]
	} else if strings.HasPrefix(pathname, "/jsr/@") && strings.Count(pathname, "/") >= 3 {
		segs := strings.Split(pathname, "/")
		pathname = "/@jsr/" + segs[2][1:] + "__" + segs[3]
		if len(segs) > 4 {
			pathname += "/" + strings.Join(segs[4:], "/")
		}
	}

	pkgName, maybeVersion, subPath := splitPkgPath(pathname)
	fromEsmsh := strings.HasPrefix(pkgName, "~") && valid.IsHexString(pkgName[1:])
	if !fromEsmsh && !validatePackageName(pkgName) {
		return Pkg{}, "", fmt.Errorf("invalid package name '%s'", pkgName)
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}

	pkg = Pkg{
		Name:       pkgName,
		Version:    version,
		SubPath:    subPath,
		SubModule:  toModuleBareName(subPath, true),
		FromGithub: fromGithub,
		FromEsmsh:  fromEsmsh,
	}

	if fromEsmsh {
		pkg.Version = "0.0.0"
		return
	}

	if fromGithub {
		// strip the leading `@`
		pkg.Name = pkg.Name[1:]
		if (valid.IsHexString(pkg.Version) && len(pkg.Version) >= 10) || regexpFullVersion.MatchString(strings.TrimPrefix(pkg.Version, "v")) {
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

	if !regexpFullVersion.MatchString(pkg.Version) && cfg != nil {
		var p NpmPackageInfo
		p, err = fetchPackageInfo(pkgName, version)
		if err == nil {
			pkg.Version = p.Version
		}
	}
	return
}

func (pkg Pkg) ImportPath() string {
	if pkg.SubModule != "" {
		return pkg.Name + "/" + pkg.SubModule
	}
	return pkg.Name
}

func (pkg Pkg) VersionName() string {
	if pkg.FromGithub {
		return "gh/" + pkg.Name + "@" + pkg.Version
	}
	return pkg.Name + "@" + pkg.Version
}

func (pkg Pkg) String() string {
	s := pkg.VersionName()
	if pkg.SubModule != "" {
		s += "/" + pkg.SubModule
	}
	return s
}

type PathSlice []string

func (a PathSlice) Len() int      { return len(a) }
func (a PathSlice) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a PathSlice) Less(i, j int) bool {
	return len(strings.Split(a[i], "/")) < len(strings.Split(a[j], "/"))
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

func toModuleBareName(path string, stripIndexSuffier bool) string {
	if path != "" {
		subModule := path
		if strings.HasSuffix(subModule, ".mjs") {
			subModule = strings.TrimSuffix(subModule, ".mjs")
		} else if strings.HasSuffix(subModule, ".cjs") {
			subModule = strings.TrimSuffix(subModule, ".cjs")
		} else {
			subModule = strings.TrimSuffix(subModule, ".js")
		}
		if stripIndexSuffier {
			subModule = strings.TrimSuffix(subModule, "/index")
		}
		return subModule
	}
	return ""
}

func splitPkgPath(specifier string) (pkgName string, version string, subPath string) {
	a := strings.Split(strings.TrimPrefix(specifier, "/"), "/")
	pkgNameWithVersion := a[0]
	subPath = strings.Join(a[1:], "/")
	if strings.HasPrefix(pkgNameWithVersion, "@") && len(a) > 1 {
		pkgNameWithVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
	}
	if len(pkgNameWithVersion) > 0 && pkgNameWithVersion[0] == '@' {
		pkgName, version = utils.SplitByLastByte(pkgNameWithVersion[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, version = utils.SplitByLastByte(pkgNameWithVersion, '@')
	}
	return
}

func getPkgName(specifier string) string {
	name, _, _ := splitPkgPath(specifier)
	return name
}
