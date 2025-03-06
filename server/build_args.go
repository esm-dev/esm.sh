package server

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
)

type BuildArgs struct {
	alias             map[string]string
	deps              map[string]string
	external          set.ReadOnlySet[string]
	conditions        []string
	keepNames         bool
	ignoreAnnotations bool
	externalRequire   bool
}

func decodeBuildArgs(argsString string) (args BuildArgs, err error) {
	s, err := atobUrl(argsString)
	if err == nil {
		args = BuildArgs{}
		for _, p := range strings.Split(s, "\n") {
			if strings.HasPrefix(p, "a") {
				args.alias = map[string]string{}
				for _, p := range strings.Split(p[1:], ",") {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" {
						args.alias[name] = to
					}
				}
			} else if strings.HasPrefix(p, "d") {
				deps := map[string]string{}
				for _, p := range strings.Split(p[1:], ",") {
					pkgName, pkgVersion, _, _ := splitEsmPath(p)
					deps[pkgName] = pkgVersion
				}
				args.deps = deps
			} else if strings.HasPrefix(p, "e") {
				args.external = *set.NewReadOnly(strings.Split(p[1:], ",")...)
			} else if strings.HasPrefix(p, "c") {
				args.conditions = append(args.conditions, strings.Split(p[1:], ",")...)
			} else {
				switch p {
				case "r":
					args.externalRequire = true
				case "k":
					args.keepNames = true
				case "i":
					args.ignoreAnnotations = true

				}
			}
		}
	}
	return
}

func encodeBuildArgs(args BuildArgs, isDts bool) string {
	lines := []string{}
	if len(args.alias) > 0 {
		var ss sort.StringSlice
		for from, to := range args.alias {
			ss = append(ss, fmt.Sprintf("%s:%s", from, to))
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("a%s", strings.Join(ss, ",")))
		}
	}
	if len(args.deps) > 0 {
		var ss sort.StringSlice
		for name, version := range args.deps {
			ss = append(ss, fmt.Sprintf("%s@%s", name, version))
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("d%s", strings.Join(ss, ",")))
		}
	}
	if args.external.Len() > 0 {
		var ss sort.StringSlice
		for _, name := range args.external.Values() {
			ss = append(ss, name)
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("e%s", strings.Join(ss, ",")))
		}
	}
	if len(args.conditions) > 0 {
		var ss sort.StringSlice
		for _, name := range args.conditions {
			ss = append(ss, name)
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("c%s", strings.Join(ss, ",")))
		}
	}
	if !isDts {
		if args.externalRequire {
			lines = append(lines, "r")
		}
		if args.keepNames {
			lines = append(lines, "k")
		}
		if args.ignoreAnnotations {
			lines = append(lines, "i")
		}
	}
	if len(lines) > 0 {
		return btoaUrl(strings.Join(lines, "\n"))
	}
	return ""
}

// resolveBuildArgs resolves `alias`, `deps`, `external` of the build args
func resolveBuildArgs(npmrc *NpmRC, installDir string, args *BuildArgs, esm EsmPath) error {
	if len(args.alias) > 0 || len(args.deps) > 0 || args.external.Len() > 0 {
		// quick check if the alias, deps, external are all in dependencies of the package
		deps, ok, err := func() (deps *set.Set[string], ok bool, err error) {
			var p *PackageJSON
			pkgJsonPath := path.Join(installDir, "node_modules", esm.PkgName, "package.json")
			if existsFile(pkgJsonPath) {
				var raw PackageJSONRaw
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
			if len(args.alias) > 0 {
				for from := range args.alias {
					if !deps.Has(from) {
						return nil, false, nil
					}
				}
			}
			if len(args.deps) > 0 {
				for name := range args.deps {
					if !deps.Has(name) {
						return nil, false, nil
					}
				}
			}
			if args.external.Len() > 0 {
				for _, name := range args.external.Values() {
					if !deps.Has(name) {
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
		if len(args.alias) > 0 {
			alias := map[string]string{}
			for from, to := range args.alias {
				if deps.Has(from) {
					alias[from] = to
				}
			}
			for from, to := range alias {
				pkgName, _, _, _ := splitEsmPath(to)
				if pkgName == esm.PkgName {
					delete(alias, from)
				} else {
					deps.Add(pkgName)
				}
			}
			args.alias = alias
		}
		if len(args.deps) > 0 {
			depsArg := map[string]string{}
			for name, version := range args.deps {
				if name != esm.PkgName && deps.Has(name) {
					depsArg[name] = version
					continue
				}
				// fix some edge cases
				// for example, the package "htm" doesn't declare 'preact' as a dependency explicitly
				// as a workaround, we check if the package name is in the subPath of the package
				if esm.SubModuleName != "" && slices.Contains(strings.Split(esm.SubModuleName, "/"), name) {
					depsArg[name] = version
				}
			}
			args.deps = depsArg
		}
		if args.external.Len() > 0 {
			external := make([]string, 0, args.external.Len())
			for _, name := range args.external.Values() {
				if strings.HasPrefix(name, "node:") {
					if nodeBuiltinModules[name[5:]] {
						external = append(external, name)
					}
					continue
				}
				// if the subModule externalizes the package entry
				if name == esm.PkgName && esm.SubPath != "" {
					external = append(external, name)
					continue
				}
				if name != esm.PkgName && deps.Has(name) {
					external = append(external, name)
				}
			}
			args.external = *set.NewReadOnly(external...)
		}
	}
	return nil
}

func walkDeps(npmrc *NpmRC, installDir string, pkg Package, mark *set.Set[string]) (err error) {
	if mark.Has(pkg.Name) {
		return
	}
	mark.Add(pkg.Name)
	var p *PackageJSON
	pkgJsonPath := path.Join(installDir, "node_modules", pkg.Name, "package.json")
	if existsFile(pkgJsonPath) {
		var raw PackageJSONRaw
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
	for name, version := range p.Dependencies {
		pkgDeps[name] = version
	}
	for name, version := range p.PeerDependencies {
		pkgDeps[name] = version
	}
	for name, version := range pkgDeps {
		depPkg := Package{Name: name, Version: version}
		p, e := resolveDependencyVersion(version)
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
