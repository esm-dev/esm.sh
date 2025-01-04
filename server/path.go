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

type EsmPath struct {
	GhPrefix      bool
	PrPrefix      bool
	PkgName       string
	PkgVersion    string
	SubPath       string
	SubModuleName string
}

func (p EsmPath) Package() Package {
	return Package{
		Github:   p.GhPrefix,
		PkgPrNew: p.PrPrefix,
		Name:     p.PkgName,
		Version:  p.PkgVersion,
	}
}

func (p EsmPath) Name() string {
	name := p.PkgName
	if p.PkgVersion != "" && p.PkgVersion != "*" && p.PkgVersion != "latest" {
		name += "@" + p.PkgVersion
	}
	if p.GhPrefix {
		return "gh/" + name
	}
	if p.PrPrefix {
		return "pr/" + name
	}
	return name
}

func (p EsmPath) Specifier() string {
	if p.SubModuleName != "" {
		return p.Name() + "/" + p.SubModuleName
	}
	return p.Name()
}

func praseEsmPath(npmrc *NpmRC, pathname string) (esmPath EsmPath, extraQuery string, isFixedVersion bool, hasTargetSegment bool, err error) {
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
		hasTargetSegment = validateTargetSegment(strings.Split(subPath, "/"))
		isFixedVersion = true
		esmPath = EsmPath{
			PkgName:       pkgName,
			PkgVersion:    version,
			SubPath:       subPath,
			SubModuleName: stripEntryModuleExt(subPath),
			PrPrefix:      true,
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

	pkgName, maybeVersion, subPath, hasTargetSegment := splitEsmPath(pathname)
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

	esmPath = EsmPath{
		PkgName:       pkgName,
		PkgVersion:    version,
		SubPath:       subPath,
		SubModuleName: stripEntryModuleExt(subPath),
		GhPrefix:      ghPrefix,
	}

	// workaround for es5-ext "../#/.." path
	if esmPath.SubModuleName != "" && esmPath.PkgName == "es5-ext" {
		esmPath.SubModuleName = strings.ReplaceAll(esmPath.SubModuleName, "/%23/", "/#/")
	}

	if ghPrefix {
		if isCommitish(esmPath.PkgVersion) || regexpVersionStrict.MatchString(strings.TrimPrefix(esmPath.PkgVersion, "v")) {
			isFixedVersion = true
			return
		}
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", esmPath.PkgName))
		if err != nil {
			return
		}
		if esmPath.PkgVersion == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					esmPath.PkgVersion = ref.Sha[:7]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+esmPath.PkgVersion || ref.Ref == "refs/heads/"+esmPath.PkgVersion {
					esmPath.PkgVersion = ref.Sha[:7]
					return
				}
			}
			// try to find the semver tag
			var c *semver.Constraints
			c, err = semver.NewConstraint(strings.TrimPrefix(esmPath.PkgVersion, "semver:"))
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
					esmPath.PkgVersion = vs[i-1].String()
					return
				}
			}
		}
		err = errors.New("tag or branch not found")
		return
	}

	isFixedVersion = regexpVersionStrict.MatchString(esmPath.PkgVersion)
	if !isFixedVersion {
		var p *PackageJSON
		p, err = npmrc.fetchPackageInfo(pkgName, esmPath.PkgVersion)
		if err == nil {
			esmPath.PkgVersion = p.Version
		}
	}
	return
}

func splitEsmPath(pathname string) (pkgName string, version string, subPath string, hasTargetSegment bool) {
	a := strings.Split(strings.TrimPrefix(pathname, "/"), "/")
	nameAndVersion := ""
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		nameAndVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
		hasTargetSegment = validateTargetSegment(a[2:])
	} else {
		nameAndVersion = a[0]
		subPath = strings.Join(a[1:], "/")
		hasTargetSegment = validateTargetSegment(a[1:])
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

func validateTargetSegment(segments []string) bool {
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
	name, _, _, _ := splitEsmPath(specifier)
	return name
}
