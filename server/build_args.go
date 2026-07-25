package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
)

const (
	maxBuildArgBytes        = 4096
	maxBuildArgItems        = 32
	maxEncodedBuildArgBytes = 4*maxBuildArgBytes + 16
)

type BuildArgs struct {
	Alias             map[string]string
	Deps              map[string]string
	External          set.ReadOnlySet[string]
	Conditions        []string
	KeepNames         bool
	IgnoreAnnotations bool
	ExternalRequire   bool
}

func parseAliasArg(value string, maxBytes int) (map[string]string, error) {
	alias := map[string]string{}
	if len(value) > maxBytes {
		return nil, fmt.Errorf("alias query exceeds %d bytes", maxBytes)
	}
	validSpecifier := func(specifier string) bool {
		if len(specifier) > 512 || strings.ContainsAny(specifier, `\?#%&`) || strings.IndexFunc(specifier, unicode.IsControl) >= 0 {
			return false
		}
		pkgName, pkgVersion, subPath := splitEsmPath(specifier)
		if !npm.ValidatePackageName(pkgName) || len(pkgVersion) > 256 {
			return false
		}
		if subPath != "" {
			for segment := range strings.SplitSeq(subPath, "/") {
				if len(segment) > 214 || segment == "" || segment == "." || segment == ".." || strings.ContainsRune(segment, '=') {
					return false
				}
			}
		}
		return true
	}
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		name, to := utils.SplitByFirstByte(p, ':')
		name = strings.TrimSpace(name)
		to = strings.TrimSpace(to)
		if !validSpecifier(name) || !validSpecifier(to) {
			return nil, fmt.Errorf("invalid alias %q", p)
		}
		if _, ok := alias[name]; !ok && len(alias) >= maxBuildArgItems {
			return nil, fmt.Errorf("alias query exceeds %d entries", maxBuildArgItems)
		}
		alias[name] = to
	}
	return alias, nil
}

func parseDepsArg(value string, maxBytes int) (map[string]string, error) {
	deps := map[string]string{}
	if len(value) > maxBytes {
		return nil, fmt.Errorf("deps query exceeds %d bytes", maxBytes)
	}
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pkgName, pkgVersion, subPath := splitEsmPath(p)
		if !npm.ValidatePackageName(pkgName) || len(pkgVersion) > 256 || subPath != "" || strings.ContainsAny(pkgVersion, `\?#%&`) || strings.IndexFunc(pkgVersion, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("invalid dependency %q", p)
		}
		if _, ok := deps[pkgName]; !ok && len(deps) >= maxBuildArgItems {
			return nil, fmt.Errorf("deps query exceeds %d entries", maxBuildArgItems)
		}
		deps[pkgName] = pkgVersion
	}
	return deps, nil
}

func parseExternalArg(value string) (*set.Set[string], bool, error) {
	external := set.New[string]()
	if len(value) > maxBuildArgBytes {
		return nil, false, fmt.Errorf("external query exceeds %d bytes", maxBuildArgBytes)
	}
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "*" {
			return set.New[string](), true, nil
		}
		isNamespace := strings.HasPrefix(p, "@") && !strings.Contains(p[1:], "/") && npm.Naming.Match(p[1:])
		if len(p) > 214 || (!npm.ValidatePackageName(p) && !isNamespace && !(strings.HasPrefix(p, "node:") && nodeBuiltinModules[p[5:]])) {
			return nil, false, fmt.Errorf("invalid external %q", p)
		}
		if !external.Has(p) && external.Len() >= maxBuildArgItems {
			return nil, false, fmt.Errorf("external query exceeds %d entries", maxBuildArgItems)
		}
		external.Add(p)
	}
	return external, false, nil
}

func parseConditionsArg(value string) ([]string, error) {
	conditions := make([]string, 0)
	seen := set.New[string]()
	if len(value) > maxBuildArgBytes {
		return nil, fmt.Errorf("conditions query exceeds %d bytes", maxBuildArgBytes)
	}
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > 128 || !npm.Versioning.Match(p) {
			return nil, fmt.Errorf("invalid condition %q", p)
		}
		if !seen.Has(p) {
			if len(conditions) >= maxBuildArgItems {
				return nil, fmt.Errorf("conditions query exceeds %d entries", maxBuildArgItems)
			}
			seen.Add(p)
			conditions = append(conditions, p)
		}
	}
	return conditions, nil
}

func decodeBuildArgs(argsString string) (args BuildArgs, err error) {
	encoded := strings.TrimPrefix(argsString, "X-")
	if len(encoded) > base64.RawURLEncoding.EncodedLen(maxEncodedBuildArgBytes) {
		return args, fmt.Errorf("encoded build args exceed %d bytes", maxEncodedBuildArgBytes)
	}
	s, err := atobUrl(encoded)
	if err == nil {
		if len(s) > maxEncodedBuildArgBytes {
			return args, fmt.Errorf("encoded build args exceed %d bytes", maxEncodedBuildArgBytes)
		}
		args = BuildArgs{}
		seen := set.New[byte]()
		for p := range strings.SplitSeq(s, "\n") {
			if p == "" {
				continue
			}
			key := p[0]
			if key == 'C' {
				key = 'c'
			}
			if strings.ContainsRune("adec", rune(key)) {
				if seen.Has(key) {
					return args, fmt.Errorf("duplicate encoded build arg %q", p[:1])
				}
				seen.Add(key)
			}
			if p[0] == 'a' {
				args.Alias, err = parseAliasArg(p[1:], maxEncodedBuildArgBytes)
				if err != nil {
					return
				}
			} else if p[0] == 'd' {
				args.Deps, err = parseDepsArg(p[1:], maxEncodedBuildArgBytes)
				if err != nil {
					return
				}
			} else if p[0] == 'e' {
				var external *set.Set[string]
				var externalAll bool
				external, externalAll, err = parseExternalArg(p[1:])
				if err != nil {
					return
				}
				if externalAll {
					return args, errors.New("external all must use the path prefix")
				}
				args.External = *external.ReadOnly()
			} else if p[0] == 'c' || p[0] == 'C' {
				args.Conditions, err = parseConditionsArg(p[1:])
				if err != nil {
					return
				}
			} else {
				switch p {
				case "r":
					args.ExternalRequire = true
				case "k":
					args.KeepNames = true
				case "i":
					args.IgnoreAnnotations = true
				default:
					return args, fmt.Errorf("invalid encoded build arg %q", p)
				}
			}
		}
	}
	return
}

func encodeBuildArgs(args BuildArgs, isDts bool) string {
	lines := []string{}
	if len(args.Alias) > 0 {
		var ss sort.StringSlice
		for from, to := range args.Alias {
			ss = append(ss, fmt.Sprintf("%s:%s", from, to))
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("a%s", strings.Join(ss, ",")))
		}
	}
	if len(args.Deps) > 0 {
		var ss sort.StringSlice
		for name, version := range args.Deps {
			ss = append(ss, fmt.Sprintf("%s@%s", name, version))
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("d%s", strings.Join(ss, ",")))
		}
	}
	if args.External.Len() > 0 {
		var ss sort.StringSlice
		for _, name := range args.External.Values() {
			ss = append(ss, name)
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("e%s", strings.Join(ss, ",")))
		}
	}
	if len(args.Conditions) > 0 {
		lines = append(lines, fmt.Sprintf("C%s", strings.Join(args.Conditions, ",")))
	}
	if !isDts {
		if args.ExternalRequire {
			lines = append(lines, "r")
		}
		if args.KeepNames {
			lines = append(lines, "k")
		}
		if args.IgnoreAnnotations {
			lines = append(lines, "i")
		}
	}
	if len(lines) > 0 {
		return btoaUrl(strings.Join(lines, "\n"))
	}
	return ""
}

func (ctx *BuildContext) normalizeBuildArgs() error {
	if len(ctx.args.Alias) == 0 && len(ctx.args.Deps) == 0 && ctx.args.External.Len() == 0 {
		return nil
	}
	rawPath := ctx.Path()
	before := encodeBuildArgs(ctx.args, ctx.target == "types")
	if err := resolveBuildArgs(ctx.npmrc, ctx.wd, &ctx.args, ctx.esmPath); err != nil {
		return err
	}
	for name, version := range ctx.args.Deps {
		p, err := ctx.npmrc.getPackageInfoContext(ctx.Context(), name, version)
		if err != nil {
			return err
		}
		ctx.args.Deps[name] = strings.TrimPrefix(p.Version, "v")
	}
	for from, to := range ctx.args.Alias {
		name, version, subPath := splitEsmPath(to)
		if version == "" {
			if v, ok := ctx.args.Deps[name]; ok {
				version = v
			} else if ctx.pkgJson != nil {
				if v, ok := ctx.pkgJson.Dependencies[name]; ok {
					version = v
				} else if v, ok := ctx.pkgJson.PeerDependencies[name]; ok {
					version = v
				}
			}
			if version != "" {
				p, err := npm.ResolveDependencyVersion(version)
				if err != nil {
					return err
				}
				if p.Url != "" || p.Github || p.PkgPrNew {
					continue
				}
				if p.Name != "" {
					name = p.Name
					version = p.Version
				}
			}
		}
		p, err := ctx.npmrc.getPackageInfoContext(ctx.Context(), name, version)
		if err != nil {
			return err
		}
		to = p.Name + "@" + strings.TrimPrefix(p.Version, "v")
		if subPath != "" {
			to += "/" + subPath
		}
		ctx.args.Alias[from] = to
	}
	after := encodeBuildArgs(ctx.args, ctx.target == "types")
	if len(after) > base64.RawURLEncoding.EncodedLen(maxEncodedBuildArgBytes) {
		return fmt.Errorf("encoded build args exceed %d bytes", maxEncodedBuildArgBytes)
	}
	if before != after {
		if ctx.rawPath == "" {
			ctx.rawPath = rawPath
		}
		ctx.path = ""
	}
	return nil
}

// resolveBuildArgs resolves `alias`, `deps`, `external` of the build args
func resolveBuildArgs(npmrc *NpmRC, installDir string, args *BuildArgs, esm EsmPath) error {
	if len(args.Alias) > 0 || len(args.Deps) > 0 || args.External.Len() > 0 {
		subPathDeps := set.New(strings.Split(esm.SubPath, "/")...)
		// quick check if the alias, deps, external are all in dependencies of the package
		deps, ok, err := func() (deps *set.Set[string], ok bool, err error) {
			var p *npm.PackageJSON
			pkgJsonPath := path.Join(installDir, "node_modules", esm.PkgName, "package.json")
			if existsFile(pkgJsonPath) {
				var raw npm.PackageJSONRaw
				err = utils.ParseJSONFile(pkgJsonPath, &raw)
				if err == nil {
					p = raw.ToNpmPackage()
				}
			} else if esm.GhPrefix || esm.PrPrefix {
				p, err = npmrc.installPackage(esm.Package())
			} else {
				p, err = npmrc.getPackageInfo(esm.PkgName, esm.PkgVersion)
			}
			if err != nil {
				return
			}
			deps = set.New[string]()
			for name := range p.Dependencies {
				deps.Add(name)
			}
			for name := range p.PeerDependencies {
				deps.Add(name)
			}
			if len(args.Alias) > 0 {
				for from := range args.Alias {
					if !deps.Has(from) && !subPathDeps.Has(from) {
						return nil, false, nil
					}
				}
			}
			if len(args.Deps) > 0 {
				for name := range args.Deps {
					if !deps.Has(name) && !subPathDeps.Has(name) {
						return nil, false, nil
					}
				}
			}
			if args.External.Len() > 0 {
				for _, name := range args.External.Values() {
					if !deps.Has(name) && !subPathDeps.Has(name) {
						return nil, false, nil
					}
				}
			}
			return deps, true, nil
		}()
		if err != nil {
			return err
		}
		if !ok {
			deps = set.New[string]()
			err = walkDeps(npmrc, installDir, esm.Package(), deps)
			if err != nil {
				return err
			}
		}
		if len(args.Alias) > 0 {
			alias := map[string]string{}
			for from, to := range args.Alias {
				if deps.Has(from) || subPathDeps.Has(from) {
					alias[from] = to
				}
			}
			for from, to := range alias {
				pkgName := toPackageName(to)
				if pkgName == esm.PkgName {
					delete(alias, from)
				} else {
					deps.Add(pkgName)
				}
			}
			args.Alias = alias
		}
		if len(args.Deps) > 0 {
			depsArg := map[string]string{}
			for name, version := range args.Deps {
				// Some submodules import an undeclared package matching their name,
				// for example "htm/preact".
				if name != esm.PkgName && (deps.Has(name) || subPathDeps.Has(name)) {
					depsArg[name] = version
				}
			}
			args.Deps = depsArg
		}
		if args.External.Len() > 0 {
			external := make([]string, 0, args.External.Len())
			for _, name := range args.External.Values() {
				if strings.HasPrefix(name, "node:") {
					if nodeBuiltinModules[name[5:]] {
						external = append(external, name)
					}
					continue
				}
				// Check if this is a namespace pattern (e.g., @radix-ui)
				isNamespace := strings.HasPrefix(name, "@") && !strings.Contains(name[1:], "/")
				if isNamespace {
					matched := false
					for _, dep := range deps.Values() {
						if strings.HasPrefix(dep, name+"/") {
							matched = true
							break
						}
					}
					if matched {
						external = append(external, name)
					}
					continue
				}
				// if the subModule externalizes the package entry
				if name == esm.PkgName && esm.SubPath != "" {
					external = append(external, name)
					continue
				}
				if name != esm.PkgName && (deps.Has(name) || subPathDeps.Has(name)) {
					external = append(external, name)
				}
			}
			args.External = *set.NewReadOnly(external...)
		}
	}
	return nil
}

func walkDeps(npmrc *NpmRC, installDir string, pkg npm.Package, mark *set.Set[string]) (err error) {
	if mark.Has(pkg.Name) {
		return
	}
	mark.Add(pkg.Name)
	var p *npm.PackageJSON
	pkgJsonPath := path.Join(installDir, "node_modules", pkg.Name, "package.json")
	if existsFile(pkgJsonPath) {
		var raw npm.PackageJSONRaw
		err = utils.ParseJSONFile(pkgJsonPath, &raw)
		if err == nil {
			p = raw.ToNpmPackage()
		}
	} else if pkg.Github || pkg.PkgPrNew {
		p, err = npmrc.installPackage(pkg)
	} else {
		p, err = npmrc.getPackageInfo(pkg.Name, pkg.Version)
	}
	if err != nil {
		return
	}
	pkgDeps := map[string]string{}
	maps.Copy(pkgDeps, p.Dependencies)
	maps.Copy(pkgDeps, p.PeerDependencies)
	for name, version := range pkgDeps {
		depPkg := npm.Package{Name: name, Version: version}
		p, e := npm.ResolveDependencyVersion(version)
		if e == nil && p.Name != "" {
			depPkg = p
		}
		err := walkDeps(npmrc, installDir, depPkg, mark)
		if err != nil {
			return err
		}
	}
	return
}
