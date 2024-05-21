package server

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

type Pkg struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	SubPath    string `json:"subPath"`
	SubModule  string `json:"subModule"`
	FromGithub bool   `json:"fromGithub"`
}

func validatePkgPath(rc *NpmRC, pathname string) (pkg Pkg, extraQuery string, caretVersion bool, hasTarget bool, err error) {
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

	pkgName, maybeVersion, subPath, hasTarget := splitPkgPath(strings.TrimPrefix(pathname, "/"))
	if !validatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
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
	}

	// workaround for es5-ext weird "/#/" path
	if pkg.SubModule != "" && pkg.Name == "es5-ext" {
		pkg.SubModule = strings.ReplaceAll(pkg.SubModule, "/%23/", "/#/")
	}

	if fromGithub {
		// strip the leading `@`
		pkg.Name = pkg.Name[1:]
		if (valid.IsHexString(pkg.Version) && len(pkg.Version) >= 7) || regexpFullVersion.MatchString(strings.TrimPrefix(pkg.Version, "v")) {
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
					pkg.Version = ref.Sha[:16]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+pkg.Version || ref.Ref == "refs/heads/"+pkg.Version {
					pkg.Version = ref.Sha[:16]
					return
				}
			}
			// try to find the semver tag
			var c *semver.Constraints
			c, err = semver.NewConstraint(strings.TrimPrefix(pkg.Version, "semver:"))
			if err == nil {
				vs := make([]*semver.Version, len(refs))
				i := 0
				for _, ref := range refs {
					if strings.HasPrefix(ref.Ref, "refs/tags/") {
						v, e := semver.NewVersion(strings.TrimPrefix(ref.Ref, "refs/tags/"))
						if e == nil && c.Check(v) {
							vs[i] = v
							i++
						}
					}
				}
				if i > 0 {
					vs = vs[:i]
					if i > 1 {
						sort.Sort(semver.Collection(vs))
					}
					pkg.Version = vs[i-1].String()
					return
				}
			}
		}
		err = fmt.Errorf("tag or branch not found")
		return
	}

	caretVersion = strings.HasPrefix(pkg.Version, "^") && regexpFullVersion.MatchString(pkg.Version[1:])

	if caretVersion || !regexpFullVersion.MatchString(pkg.Version) {
		var p PackageJSON
		p, err = rc.fetchPackageInfo(pkgName, version)
		if err == nil {
			pkg.Version = p.Version
		}
	}
	return
}

func (pkg Pkg) ghPrefix() string {
	if pkg.FromGithub {
		return "gh/"
	}
	return ""
}

func (pkg Pkg) FullName() string {
	if pkg.FromGithub {
		return "gh/" + pkg.Name + "@" + pkg.Version
	}
	return pkg.Name + "@" + pkg.Version
}

func (pkg Pkg) String() string {
	s := pkg.FullName()
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

func splitPkgPath(specifier string) (pkgName string, version string, subPath string, hasTarget bool) {
	a := strings.Split(specifier, "/")
	nameAndVersion := ""
	if strings.HasPrefix(specifier, "@") && len(a) > 1 {
		nameAndVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
		if endsWith(subPath, ".js", ".mjs", ".css") {
			hasTarget = hasTargetSegment(a[2:])
		}
	} else {
		nameAndVersion = a[0]
		subPath = strings.Join(a[1:], "/")
		if endsWith(subPath, ".js", ".mjs", ".css") {
			hasTarget = hasTargetSegment(a[1:])
		}
	}
	if len(nameAndVersion) > 0 && nameAndVersion[0] == '@' {
		pkgName, version = utils.SplitByLastByte(nameAndVersion[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, version = utils.SplitByLastByte(nameAndVersion, '@')
	}
	return
}

func hasTargetSegment(segments []string) bool {
	if len(segments) < 2 {
		return false
	}
	if strings.HasPrefix(segments[0], "X-") && len(segments) > 2 {
		_, ok := targets[segments[1]]
		return ok
	}
	_, ok := targets[segments[0]]
	return ok
}

func getPkgName(specifier string) string {
	name, _, _, _ := splitPkgPath(specifier)
	return name
}
