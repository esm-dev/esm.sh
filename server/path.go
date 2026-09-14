package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
)

type EsmPath struct {
	GhPrefix   bool
	PrPrefix   bool
	PkgName    string
	PkgVersion string
	SubPath    string
}

func (p EsmPath) Package() npm.Package {
	return npm.Package{
		Github:   p.GhPrefix,
		PkgPrNew: p.PrPrefix,
		Name:     p.PkgName,
		Version:  p.PkgVersion,
	}
}

func (p EsmPath) PackageId() string {
	id := p.PkgName
	if p.PkgVersion != "" && p.PkgVersion != "*" && p.PkgVersion != "latest" {
		id += "@" + strings.ReplaceAll(p.PkgVersion, " ", "%20")
	}
	if p.GhPrefix {
		return "gh/" + id
	}
	if p.PrPrefix {
		return "pr/" + id
	}
	return id
}

func (p EsmPath) String() string {
	if p.SubPath != "" {
		return p.PackageId() + "/" + p.SubPath
	}
	return p.PackageId()
}

func parseEsmPath(npmrc *NpmRC, pathname string) (esm EsmPath, extraQuery string, exactVersion bool, target string, xArgs *BuildArgs, err error) {
	esm, extraQuery, exactVersion, target, xArgs, err = parseEsmPathSyntax(pathname)
	if err != nil || exactVersion {
		return
	}
	if esm.PrPrefix {
		esm.PkgVersion, err = resolvePrPackageVersion(esm)
		if err == nil && !isCommitish(esm.PkgVersion) {
			err = errors.New("pkg.pr.new: tag or branch not found")
		}
	} else if esm.GhPrefix {
		esm.PkgVersion, err = resolveGhPackageVersion(esm)
		if err == nil && !isCommitish(esm.PkgVersion) {
			err = errors.New("github: tag or branch not found")
		}
	} else {
		var date time.Time
		var isDateVersion bool
		date, isDateVersion, err = npm.IsDateVersion(esm.PkgVersion)
		if err != nil {
			return
		}
		var p *npm.PackageJSON
		if isDateVersion {
			p, err = npmrc.getPackageInfoByDate(esm.PkgName, date)
		} else {
			p, err = npmrc.getPackageInfo(esm.PkgName, esm.PkgVersion)
		}
		if err == nil {
			esm.PkgVersion = p.Version
		}
	}
	return
}

// parseEsmPathSyntax parses a package URL without consulting resolution caches.
func parseEsmPathSyntax(pathname string) (esm EsmPath, extraQuery string, exactVersion bool, target string, xArgs *BuildArgs, err error) {
	// see https://pkg.pr.new
	if strings.HasPrefix(pathname, "/pr/") || strings.HasPrefix(pathname, "/pkg.pr.new/") {
		if strings.HasPrefix(pathname, "/pr/") {
			pathname = pathname[4:]
		} else {
			pathname = pathname[12:]
		}
		pkgName, rest := utils.SplitByLastByte(pathname, '@')
		if rest == "" || !validatePrPackageName(pkgName) {
			err = errors.New("invalid path")
			return
		}
		version, subPath := utils.SplitByFirstByte(rest, '/')
		if version == "" || !npm.Versioning.Match(version) {
			err = errors.New("invalid path")
			return
		}
		if subPath != "" {
			subPath, target, xArgs = parseSubPath(subPath)
		}
		esm = EsmPath{
			PkgName:    pkgName,
			PkgVersion: version,
			SubPath:    stripEntryModuleExt(subPath),
			PrPrefix:   true,
		}
		exactVersion = isCommitish(esm.PkgVersion)
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

	pkgName, maybeVersion, subPathRaw := splitEsmPath(pathname)
	if !npm.ValidatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	var subPath string
	subPath, target, xArgs = parseSubPath(subPathRaw)

	// strip the leading `@` added before
	if ghPrefix {
		pkgName = pkgName[1:]
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.PathUnescape(version); e == nil {
		version = v
	}

	// workaround for es5-ext "../#/.." path
	if subPath != "" && pkgName == "es5-ext" {
		subPath = strings.ReplaceAll(subPath, "/%23/", "/#/")
	}

	esm = EsmPath{
		PkgName:    pkgName,
		PkgVersion: version,
		SubPath:    stripEntryModuleExt(subPath),
		GhPrefix:   ghPrefix,
	}

	if ghPrefix {
		exactVersion = npm.IsExactVersion(strings.TrimPrefix(esm.PkgVersion, "v")) || isCommitish(esm.PkgVersion)
	} else {
		exactVersion = len(esm.PkgVersion) > 0 && npm.IsExactVersion(esm.PkgVersion)
	}
	return
}

func parseSubPath(subPathRaw string) (subPath string, target string, xArgs *BuildArgs) {
	segments := strings.Split(subPathRaw, "/")
	if l := len(segments); l >= 2 {
		el0 := segments[0]
		el1 := segments[1]
		if strings.HasPrefix(el0, "X-") {
			args, err := decodeBuildArgs(el0)
			if err == nil {
				if _, ok := targets[el1]; ok {
					return strings.Join(segments[2:], "/"), el1, &args
				}
				return strings.Join(segments[1:], "/"), "", &args
			}
		}
		_, ok := targets[el0]
		if ok {
			return strings.Join(segments[1:], "/"), el0, nil
		}
	}
	return strings.Join(segments, "/"), "", nil
}

func splitEsmPath(pathname string) (pkgName string, pkgVersion string, subPath string) {
	pathname = strings.TrimPrefix(pathname, "/")
	if strings.HasPrefix(pathname, "@") {
		scopeName, rest := utils.SplitByFirstByte(pathname, '/')
		pkgName, subPath = utils.SplitByFirstByte(rest, '/')
		pkgName = scopeName + "/" + pkgName
	} else {
		pkgName, subPath = utils.SplitByFirstByte(pathname, '/')
	}
	if len(pkgName) > 0 && pkgName[0] == '@' {
		pkgName, pkgVersion = utils.SplitByFirstByte(pkgName[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, pkgVersion = utils.SplitByFirstByte(pkgName, '@')
	}
	if pkgVersion != "" {
		pkgVersion = strings.TrimSpace(pkgVersion)
	}
	return
}

// pkg.pr.new accepts a package name, owner/repo, or owner/repo/package name.
func validatePrPackageName(name string) bool {
	if npm.ValidatePackageName(name) {
		return true
	}
	owner, rest, _ := strings.Cut(name, "/")
	repo, pkg, hasPkg := strings.Cut(rest, "/")
	return npm.ValidatePackageName("@"+owner+"/"+repo) && (!hasPkg || npm.ValidatePackageName(pkg))
}

func toPackageName(specifier string) string {
	name, _, _ := splitEsmPath(specifier)
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
				var latest *semver.Version
				for _, ref := range refs {
					if after, ok := strings.CutPrefix(ref.Ref, "refs/tags/"); ok {
						v, e := semver.NewVersion(after)
						if e == nil && semv.Check(v) && (latest == nil || v.GreaterThan(latest)) {
							latest = v
							version = ref.Sha[:7]
						}
					}
				}
				if latest != nil {
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
		client := newFetchClient("esmd/"+VERSION, 30)
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

		resp, err := client.Fetch(u, nil)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		if err == nil && resp != nil {
			if commit := prCommitFromHeader(resp.Header); commit != "" {
				version = commit
			}
		}
		return
	})
}

func prCommitFromHeader(header http.Header) string {
	key := header.Get("x-commit-key")
	if key == "" {
		return ""
	}
	idx := strings.LastIndexByte(key, ':')
	if idx < 0 || idx == len(key)-1 {
		return ""
	}
	commit := key[idx+1:]
	if !isCommitish(commit) {
		return ""
	}
	return commit[:7]
}
