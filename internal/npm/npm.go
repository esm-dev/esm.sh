package npm

import (
	"errors"
	"net/url"
	"strings"

	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

var (
	Naming     = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('$'), valid.Eq('!')}
	Versioning = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+')}
)

// ValidatePackageName validates the package name.
// based on https://github.com/npm/validate-npm-package-name
func ValidatePackageName(pkgName string) bool {
	if l := len(pkgName); l == 0 || l > 214 {
		return false
	}
	if strings.HasPrefix(pkgName, "@") {
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		return Naming.Match(scope[1:]) && Naming.Match(name)
	}
	return Naming.Match(pkgName)
}

type Package struct {
	Name     string
	Version  string
	Github   bool
	PkgPrNew bool
}

func (p *Package) String() string {
	s := p.Name + "@" + p.Version
	if p.Github {
		return "gh/" + s
	}
	if p.PkgPrNew {
		return "pr/" + s
	}
	return s
}

// ResolveDependencyVersion resolves the version of a dependency
// e.g. "react": "npm:react@19.0.0"
// e.g. "react": "github:facebook/react#semver:19.0.0"
// e.g. "flag": "jsr:@luca/flag@0.0.1"
// e.g. "tinybench": "https://pkg.pr.new/tinybench@a832a55"
func ResolveDependencyVersion(v string) (Package, error) {
	// ban file specifier
	if strings.HasPrefix(v, "file:") {
		return Package{}, errors.New("unsupported file dependency")
	}
	if strings.HasPrefix(v, "npm:") {
		pkgName, pkgVersion := splitPackageVersion(v[4:])
		return Package{
			Name:    pkgName,
			Version: pkgVersion,
		}, nil
	}
	if strings.HasPrefix(v, "jsr:") {
		pkgName, pkgVersion := splitPackageVersion(v[4:])
		if !strings.HasPrefix(pkgName, "@") || !strings.ContainsRune(pkgName, '/') {
			return Package{}, errors.New("invalid jsr dependency")
		}
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		return Package{
			Name:    "@jsr/" + scope[1:] + "__" + name,
			Version: pkgVersion,
		}, nil
	}
	if strings.HasPrefix(v, "github:") {
		repo, fragment := utils.SplitByLastByte(strings.TrimPrefix(v, "github:"), '#')
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(fragment), "semver:"),
		}, nil
	}
	if strings.HasPrefix(v, "git+ssh://") || strings.HasPrefix(v, "git+https://") || strings.HasPrefix(v, "git://") {
		gitUrl, e := url.Parse(v)
		if e != nil || gitUrl.Hostname() != "github.com" {
			return Package{}, errors.New("unsupported git dependency")
		}
		repo := strings.TrimSuffix(gitUrl.Path[1:], ".git")
		if gitUrl.Scheme == "git+ssh" {
			repo = gitUrl.Port() + "/" + repo
		}
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(gitUrl.Fragment), "semver:"),
		}, nil
	}
	// https://pkg.pr.new
	if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
		u, e := url.Parse(v)
		if e != nil || u.Host != "pkg.pr.new" {
			return Package{}, errors.New("unsupported http dependency")
		}
		pkgName, rest := utils.SplitByLastByte(u.Path[1:], '@')
		if rest == "" {
			return Package{}, errors.New("unsupported http dependency")
		}
		version, _ := utils.SplitByFirstByte(rest, '/')
		if version == "" {
			return Package{}, errors.New("unsupported http dependency")
		}
		return Package{
			PkgPrNew: true,
			Name:     pkgName,
			Version:  version,
		}, nil
	}
	// see https://docs.npmjs.com/cli/v10/configuring-npm/package-json#git-urls-as-dependencies
	if !strings.HasPrefix(v, "@") && strings.ContainsRune(v, '/') {
		repo, fragment := utils.SplitByLastByte(v, '#')
		return Package{
			Github:  true,
			Name:    repo,
			Version: strings.TrimPrefix(url.QueryEscape(fragment), "semver:"),
		}, nil
	}
	return Package{}, nil
}

func splitPackageVersion(v string) (string, string) {
	if strings.HasPrefix(v, "@") {
		if i := strings.IndexByte(v[1:], '@'); i > 0 {
			return v[:i+1], v[i+2:]
		}
		return v, ""
	}
	if i := strings.IndexByte(v, '@'); i > 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// IsDistTag returns true if the given version is a distribution tag.
// https://docs.npmjs.com/cli/v9/commands/npm-dist-tag
func IsDistTag(s string) bool {
	switch s {
	case "latest", "next", "beta", "alpha", "canary", "rc", "experimental":
		return true
	default:
		return false
	}
}

// IsExactVersion returns true if the given version is an exact version.
func IsExactVersion(version string) bool {
	a := strings.SplitN(version, ".", 3)
	if len(a) != 3 {
		return false
	}
	if len(a[0]) == 0 || !isNumericString(a[0]) || len(a[1]) == 0 || !isNumericString(a[1]) {
		return false
	}
	p := a[2]
	if len(p) == 0 {
		return false
	}
	patchEnd := false
	for i, c := range p {
		if !patchEnd {
			if c == '-' || c == '+' {
				if i == 0 || i == len(p)-1 {
					return false
				}
				patchEnd = true
			} else if c < '0' || c > '9' {
				return false
			}
		} else {
			if !(c == '.' || c == '_' || c == '-' || c == '+' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}

func isNumericString(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NormalizePackageVersion normalizes the package version.
// It removes the leading `=` or `v` and returns "latest" for empty or "*" versions.
func NormalizePackageVersion(version string) string {
	// strip leading `=` or `v`
	if strings.HasPrefix(version, "=") {
		version = version[1:]
	} else if strings.HasPrefix(version, "v") && IsExactVersion(version[1:]) {
		version = version[1:]
	}
	if version == "" || version == "*" {
		return "latest"
	}
	return version
}

// ToTypesPackageName converts a package name to a types package name.
// If the package name is scoped, it returns "@types/@scope__name".
func ToTypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}
