package npm

import (
	"errors" 
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
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
	Url      string
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
	// http dependencies
	if strings.HasPrefix(v, "https:") || strings.HasPrefix(v, "http:") {
		u, e := url.Parse(v)
		if e != nil || !strings.ContainsRune(u.Host, '.') {
			return Package{}, errors.New("unsupported http dependency")
		}
		if u.Host == "pkg.pr.new" {
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
		return Package{
			Url: v,
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

// ParseAtParam parses the at query parameter and returns a normalized timestamp string.
// Supports formats:
// - yyyy[-mm[-dd]] (date format, defaults to beginning of period)  
// - <number> (unix timestamp in seconds)
func ParseAtParam(at string) (string, error) {
	if at == "" {
		return "", errors.New("at parameter is empty")
	}

	// Handle date formats first (yyyy[-mm[-dd]])
	dateRegex := regexp.MustCompile(`^(\d{4})(?:-(\d{1,2})(?:-(\d{1,2}))?)?$`)
	matches := dateRegex.FindStringSubmatch(at)
	if matches != nil {
		year := matches[1]
		month := "01"
		day := "01"

		if matches[2] != "" {
			month = matches[2]
			if len(month) == 1 {
				month = "0" + month
			}
		}
		if matches[3] != "" {
			day = matches[3]
			if len(day) == 1 {
				day = "0" + day
			}
		}

		// Parse and validate the date
		dateStr := year + "-" + month + "-" + day + "T00:00:00Z"
		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return "", errors.New("invalid date format")
		}

		return strconv.FormatInt(t.Unix(), 10) + "s", nil
	}

	// Handle numeric timestamp (seconds since epoch) - only if it's not a 4-digit year
	if regexp.MustCompile(`^\d+$`).MatchString(at) && len(at) != 4 {
		_, err := strconv.ParseInt(at, 10, 64)
		if err != nil {
			return "", errors.New("invalid timestamp")
		}
		return at + "s", nil
	}

	return "", errors.New("invalid at format, expected yyyy[-mm[-dd]] or unix timestamp")
}

// ResolveVersionByTime finds the latest version published before or at the given timestamp.
// The atTimestamp should be in format "<unix_seconds>s".
func ResolveVersionByTime(metadata *PackageMetadata, atTimestamp string) (string, error) {
	if !strings.HasSuffix(atTimestamp, "s") {
		return "", errors.New("timestamp must end with 's'")
	}

	targetUnix, err := strconv.ParseInt(strings.TrimSuffix(atTimestamp, "s"), 10, 64)
	if err != nil {
		return "", errors.New("invalid timestamp format")
	}
	targetTime := time.Unix(targetUnix, 0)

	type versionTime struct {
		version string
		time    time.Time
	}

	var validVersions []versionTime
	for version, timeStr := range metadata.Time {
		// Skip special entries like "created", "modified"
		if version == "created" || version == "modified" {
			continue
		}
		// Only include versions that exist in the versions map
		if _, exists := metadata.Versions[version]; !exists {
			continue
		}

		publishTime, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue // Skip invalid timestamps
		}

		// Only include versions published before or at the target time
		if publishTime.Before(targetTime) || publishTime.Equal(targetTime) {
			validVersions = append(validVersions, versionTime{
				version: version,
				time:    publishTime,
			})
		}
	}

	if len(validVersions) == 0 {
		return "", errors.New("no versions found for the specified date")
	}

	// Sort by publish time, latest first
	sort.Slice(validVersions, func(i, j int) bool {
		return validVersions[i].time.After(validVersions[j].time)
	})

	return validVersions[0].version, nil
}

// ResolveVersionByTimeWithConstraint finds the latest version published before or at the given timestamp
// that also satisfies the given version constraint.
func ResolveVersionByTimeWithConstraint(metadata *PackageMetadata, atTimestamp string, versionConstraint string) (string, error) {
	if !strings.HasSuffix(atTimestamp, "s") {
		return "", errors.New("timestamp must end with 's'")
	}

	targetUnix, err := strconv.ParseInt(strings.TrimSuffix(atTimestamp, "s"), 10, 64)
	if err != nil {
		return "", errors.New("invalid timestamp format")
	}
	targetTime := time.Unix(targetUnix, 0)

	type versionTime struct {
		version string
		time    time.Time
	}

	var validVersions []versionTime
	
	// Handle dist tags first
	if distVersion, ok := metadata.DistTags[versionConstraint]; ok {
		if timeStr, timeExists := metadata.Time[distVersion]; timeExists {
			if publishTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
				if publishTime.Before(targetTime) || publishTime.Equal(targetTime) {
					if _, exists := metadata.Versions[distVersion]; exists {
						return distVersion, nil
					}
				}
			}
		}
		// If dist tag doesn't satisfy time constraint, fall back to semver matching
		versionConstraint = "latest"
	}

	// Parse version constraint
	var constraint *semver.Constraints
	if versionConstraint == "latest" || versionConstraint == "" {
		// For latest, we want all versions
		constraint = nil
	} else {
		constraint, err = semver.NewConstraint(versionConstraint)
		if err != nil {
			// If constraint is invalid, fall back to latest
			constraint = nil
		}
	}

	for version, timeStr := range metadata.Time {
		// Skip special entries like "created", "modified"
		if version == "created" || version == "modified" {
			continue
		}
		// Only include versions that exist in the versions map
		if _, exists := metadata.Versions[version]; !exists {
			continue
		}

		publishTime, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			continue // Skip invalid timestamps
		}

		// Only include versions published before or at the target time
		if !(publishTime.Before(targetTime) || publishTime.Equal(targetTime)) {
			continue
		}

		// Filter out prerelease versions (beta, alpha, rc, etc.) unless explicitly requested
		if strings.ContainsRune(version, '-') {
			// If there's a version constraint that includes prereleases, allow them
			if constraint != nil && strings.ContainsRune(versionConstraint, '-') {
				// Version constraint explicitly includes prereleases, so allow them
			} else {
				// Skip prerelease versions for date-based resolution to prefer stable releases
				continue
			}
		}

		// If we have a version constraint, check if this version satisfies it
		if constraint != nil {
			
			ver, err := semver.NewVersion(version)
			if err != nil {
				continue // Skip invalid semver versions
			}
			
			if !constraint.Check(ver) {
				continue // Version doesn't satisfy constraint
			}
		}

		validVersions = append(validVersions, versionTime{
			version: version,
			time:    publishTime,
		})
	}

	if len(validVersions) == 0 {
		if constraint != nil {
			return "", errors.New("no versions found for the specified date and version constraint")
		}
		return "", errors.New("no versions found for the specified date")
	}

	// Sort by publish time, latest first
	sort.Slice(validVersions, func(i, j int) bool {
		return validVersions[i].time.After(validVersions[j].time)
	})

	return validVersions[0].version, nil
}
