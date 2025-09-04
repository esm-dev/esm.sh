package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
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

func (p EsmPath) Package() npm.Package {
	return npm.Package{
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

func parseEsmPath(npmrc *NpmRC, pathname string) (esm EsmPath, extraQuery string, exactVersion bool, hasTargetSegment bool, err error) {
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
		if version == "" || !npm.Versioning.Match(version) {
			err = errors.New("invalid path")
			return
		}
		hasTargetSegment = validateTargetSegment(strings.Split(subPath, "/"))
		esm = EsmPath{
			PkgName:       pkgName,
			PkgVersion:    version,
			SubPath:       subPath,
			SubModuleName: stripEntryModuleExt(subPath),
			PrPrefix:      true,
		}
		if isCommitish(esm.PkgVersion) {
			exactVersion = true
			return
		}
		esm.PkgVersion, err = resolvePrPackageVersion(esm)
		if err != nil {
			return
		}
		if !isCommitish(esm.PkgVersion) {
			err = errors.New("pkg.pr.new: tag or branch not found")
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
	} else if strings.HasPrefix(pathname, "/jsr.io/") {
		segs := strings.Split(pathname[8:], "/")
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
	if !npm.ValidatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	// strip the leading `@` added before
	if ghPrefix {
		pkgName = pkgName[1:]
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.PathUnescape(version); e == nil {
		version = v
	}

	esm = EsmPath{
		PkgName:       pkgName,
		PkgVersion:    version,
		SubPath:       subPath,
		SubModuleName: stripEntryModuleExt(subPath),
		GhPrefix:      ghPrefix,
	}

	// workaround for es5-ext "../#/.." path
	if esm.SubModuleName != "" && esm.PkgName == "es5-ext" {
		esm.SubModuleName = strings.ReplaceAll(esm.SubModuleName, "/%23/", "/#/")
	}

	if ghPrefix {
		if npm.IsExactVersion(strings.TrimPrefix(esm.PkgVersion, "v")) || isCommitish(esm.PkgVersion) {
			exactVersion = true
			return
		}

		esm.PkgVersion, err = resolveGhPackageVersion(esm)
		if err != nil {
			return
		}

		if !isCommitish(esm.PkgVersion) {
			err = errors.New("github: tag or branch not found")
		}
		return
	}

	originalExactVersion := len(esm.PkgVersion) > 0 && npm.IsExactVersion(esm.PkgVersion)
	exactVersion = originalExactVersion

	// Check if version is a date format (yyyy-mm-dd)
	isDateVersion := npm.IsDateVersion(esm.PkgVersion)

	if !originalExactVersion {
		var p *npm.PackageJSON
		if isDateVersion {
			// For date versions, resolve directly to exact version using date-based resolution
			p, err = npmrc.getPackageInfoByDate(pkgName, esm.PkgVersion)
		} else {
			// Normal semver resolution
			p, err = npmrc.getPackageInfo(pkgName, esm.PkgVersion)
		}
		if err == nil {
			esm.PkgVersion = p.Version
			// Keep exactVersion as false for redirect logic even after resolution
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

// isPackageInExternalNamespace checks if a package belongs to an external namespace
// For example, if "@radix-ui" is in external, then "@radix-ui/react-dropdown" would match
func isPackageInExternalNamespace(pkgName string, external set.ReadOnlySet[string]) bool {
	for _, ext := range external.Values() {
		// Check if ext is a namespace (starts with @ and has no /)
		if strings.HasPrefix(ext, "@") && !strings.Contains(ext[1:], "/") {
			// Check if the package belongs to this namespace
			if strings.HasPrefix(pkgName, ext+"/") {
				return true
			}
		}
	}
	return false
}

func resolveGhPackageVersion(esm EsmPath) (version string, err error) {
	return withCache("gh/"+esm.PkgName+"@"+esm.PkgVersion, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (version string, aliasKey string, err error) {
		var refs []GitRef
		refs, err = listGhRepoRefs(fmt.Sprintf("https://github.com/%s", esm.PkgName))
		if err != nil {
			return
		}
		if esm.PkgVersion == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					version = ref.Sha[:7]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+esm.PkgVersion || ref.Ref == "refs/heads/"+esm.PkgVersion {
					version = ref.Sha[:7]
					return
				}
			}
			// try to find the 'semver' tag
			if semv, erro := semver.NewConstraint(strings.TrimPrefix(esm.PkgVersion, "semver:")); erro == nil {
				semtags := make([]*semver.Version, len(refs))
				i := 0
				for _, ref := range refs {
					if after, ok := strings.CutPrefix(ref.Ref, "refs/tags/"); ok {
						v, e := semver.NewVersion(after)
						if e == nil && semv.Check(v) {
							semtags[i] = v
							i++
						}
					}
				}
				if i > 0 {
					semtags = semtags[:i]
					if i > 1 {
						sort.Sort(semver.Collection(semtags))
					}
					version = semtags[i-1].String()
					return
				}
			}
		}
		version = esm.PkgVersion
		return
	})
}

func resolvePrPackageVersion(esm EsmPath) (version string, err error) {
	return withCache("pr/"+esm.PkgName+"@"+esm.PkgVersion, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (version string, aliasKey string, err error) {
		u, err := url.Parse(fmt.Sprintf("https://pkg.pr.new/%s@%s", esm.PkgName, esm.PkgVersion))
		if err != nil {
			return
		}
		versionRegex := regexp.MustCompile(`[^/]@([\da-f]{7,})$`)
		client, recycle := fetch.NewClient("esmd/"+VERSION, 30, false, nil)
		version = esm.PkgVersion
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			match := versionRegex.FindStringSubmatch(req.URL.Path)
			if len(match) > 1 {
				version = match[1][:7]
			}
			if len(via) >= 6 {
				return errors.New("too many redirects")
			}
			return nil
		}
		defer recycle()

		_, err = client.Fetch(u, nil)
		return
	})
}
