package server

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/gox/utils"
)

type ESMPathKind uint8

const (
	// module entry
	ESMEntry ESMPathKind = iota
	// js/css build
	ESMBuild
	// source map
	ESMSourceMap
	// *.d.ts
	ESMDts
	// package raw file
	RawFile
)

type ESMPath struct {
	GhPrefix    bool
	PrPrefix    bool
	PkgName     string
	PkgVersion  string
	SubPath     string
	SubBareName string
}

func (path ESMPath) PackageName() string {
	s := path.PkgName
	if path.PkgVersion != "" && path.PkgVersion != "*" && path.PkgVersion != "latest" {
		s += "@" + path.PkgVersion
	}
	if path.GhPrefix {
		return "gh/" + s
	}
	if path.PrPrefix {
		return "pr/" + s
	}
	return s
}

func (path ESMPath) String() string {
	s := path.PackageName()
	if path.SubBareName != "" {
		s += "/" + path.SubBareName
	}
	return s
}

func praseESMPath(npmrc *NpmRC, pathname string) (esm ESMPath, extraQuery string, isFixedVersion bool, isBuildDist bool, err error) {
	// see https://pkg.pr.new
	if strings.HasPrefix(pathname, "/pr/") || strings.HasPrefix(pathname, "/pkg.pr.new/") {
		if strings.HasPrefix(pathname, "/pr/") {
			pathname = pathname[4:]
		} else {
			pathname = pathname[12:]
		}
		pkgName, rest := utils.SplitByLastByte(pathname, '@')
		if rest == "" {
			err = errors.New("invalid path")
			return
		}
		version, subPath := utils.SplitByFirstByte(rest, '/')
		if version == "" || !regexpVersion.MatchString(version) {
			err = errors.New("invalid path")
			return
		}
		isBuildDist = validateBuildDist(strings.Split(subPath, "/"))
		isFixedVersion = true
		esm = ESMPath{
			PkgName:     pkgName,
			PkgVersion:  version,
			SubPath:     subPath,
			SubBareName: toModuleBareName(subPath, !isBuildDist),
			PrPrefix:    true,
		}
		return
	}

	var ghPrefix bool
	if strings.HasPrefix(pathname, "/gh/") {
		if !strings.ContainsRune(pathname[4:], '/') {
			err = errors.New("invalid path")
			return
		}
		// add a leading `@` to the package name
		pathname = "/@" + pathname[4:]
		ghPrefix = true
	} else if strings.HasPrefix(pathname, "/github.com/") {
		if !strings.ContainsRune(pathname[12:], '/') {
			err = errors.New("invalid path")
			return
		}
		// add a leading `@` to the package name
		pathname = "/@" + pathname[12:]
		ghPrefix = true
	} else if strings.HasPrefix(pathname, "/jsr/") {
		segs := strings.Split(pathname[5:], "/")
		if len(segs) < 2 || !strings.HasPrefix(segs[0], "@") {
			err = errors.New("invalid path")
			return
		}
		pathname = "/@jsr/" + segs[0][1:] + "__" + segs[1]
		if len(segs) > 2 {
			pathname += "/" + strings.Join(segs[2:], "/")
		}
	}

	pkgName, maybeVersion, subPath, isBuildDist := splitESMPath(pathname)
	if !validatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	// strip the leading `@` added before
	if ghPrefix {
		pkgName = pkgName[1:]
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}

	esm = ESMPath{
		PkgName:     pkgName,
		PkgVersion:  version,
		SubPath:     subPath,
		SubBareName: toModuleBareName(subPath, !isBuildDist),
		GhPrefix:    ghPrefix,
	}

	// workaround for es5-ext "../#/.." path
	if esm.SubBareName != "" && esm.PkgName == "es5-ext" {
		esm.SubBareName = strings.ReplaceAll(esm.SubBareName, "/%23/", "/#/")
	}

	if ghPrefix {
		if isCommitish(esm.PkgVersion) || regexpVersionStrict.MatchString(strings.TrimPrefix(esm.PkgVersion, "v")) {
			isFixedVersion = true
			return
		}
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", esm.PkgName))
		if err != nil {
			return
		}
		if esm.PkgVersion == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					esm.PkgVersion = ref.Sha[:7]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+esm.PkgVersion || ref.Ref == "refs/heads/"+esm.PkgVersion {
					esm.PkgVersion = ref.Sha[:7]
					return
				}
			}
			// try to find the semver tag
			var c *semver.Constraints
			c, err = semver.NewConstraint(strings.TrimPrefix(esm.PkgVersion, "semver:"))
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
					esm.PkgVersion = vs[i-1].String()
					return
				}
			}
		}
		err = errors.New("tag or branch not found")
		return
	}

	isFixedVersion = regexpVersionStrict.MatchString(esm.PkgVersion)
	if !isFixedVersion {
		var p *PackageJSON
		p, err = npmrc.fetchPackageInfo(pkgName, esm.PkgVersion)
		if err == nil {
			esm.PkgVersion = p.Version
		}
	}
	return
}

func splitESMPath(pathname string) (pkgName string, version string, subPath string, isBuildDist bool) {
	a := strings.Split(strings.TrimPrefix(pathname, "/"), "/")
	nameAndVersion := ""
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		nameAndVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
		isBuildDist = validateBuildDist(a[2:])
	} else {
		nameAndVersion = a[0]
		subPath = strings.Join(a[1:], "/")
		isBuildDist = validateBuildDist(a[1:])
	}
	if len(nameAndVersion) > 0 && nameAndVersion[0] == '@' {
		pkgName, version = utils.SplitByFirstByte(nameAndVersion[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, version = utils.SplitByFirstByte(nameAndVersion, '@')
	}
	if version != "" {
		version = strings.TrimSpace(version)
	}
	return
}

func validateBuildDist(segments []string) bool {
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

func toPackageName(specifier string) string {
	name, _, _, _ := splitESMPath(specifier)
	return name
}
