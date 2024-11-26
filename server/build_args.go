package server

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ije/gox/utils"
)

type BuildArgs struct {
	alias             map[string]string
	deps              map[string]string
	external          *StringSet
	conditions        []string
	keepNames         bool
	ignoreAnnotations bool
	externalRequire   bool
}

func decodeBuildArgs(argsString string) (args BuildArgs, err error) {
	s, err := atobUrl(argsString)
	if err == nil {
		args = BuildArgs{
			external: NewStringSet(),
		}
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
					pkgName, pkgVersion, _, _ := splitESMPath(p)
					deps[pkgName] = pkgVersion
				}
				args.deps = deps
			} else if strings.HasPrefix(p, "e") {
				for _, name := range strings.Split(p[1:], ",") {
					args.external.Add(name)
				}
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
func resolveBuildArgs(npmrc *NpmRC, installDir string, args *BuildArgs, esmPath ESMPath) error {
	if len(args.alias) > 0 || len(args.deps) > 0 || args.external.Len() > 0 {
		depsSet := NewStringSet()
		err := walkDeps(npmrc, installDir, esmPath, depsSet)
		if err != nil {
			return err
		}
		if len(args.alias) > 0 {
			alias := map[string]string{}
			for from, to := range args.alias {
				if depsSet.Has(from) {
					alias[from] = to
				}
			}
			for from, to := range alias {
				pkgName, _, _, _ := splitESMPath(to)
				if pkgName == esmPath.PkgName {
					delete(alias, from)
				} else {
					depsSet.Add(pkgName)
				}
			}
			args.alias = alias
		}
		if len(args.deps) > 0 {
			deps := map[string]string{}
			for name, version := range args.deps {
				// The version of package "react" should be the same as "react-dom"
				if esmPath.PkgName == "react-dom" && name == "react" {
					continue
				}
				if name != esmPath.PkgName && depsSet.Has(name) {
					deps[name] = version
					continue
				}
				// fix some edge cases
				// for example, the package "htm" doesn't declare 'preact' as a dependency explicitly
				// as a workaround, we check if the package name is in the subPath of the package
				if esmPath.SubBareName != "" && contains(strings.Split(esmPath.SubBareName, "/"), name) {
					deps[name] = version
				}
			}
			args.deps = deps
		}
		if args.external.Len() > 0 {
			external := NewStringSet()
			for _, name := range args.external.Values() {
				if strings.HasPrefix(name, "node:") {
					if nodeBuiltinModules[name[5:]] {
						external.Add(name)
					}
					continue
				}
				// if the subModule externalizes the package entry
				if name == esmPath.PkgName && esmPath.SubPath != "" {
					external.Add(name)
					continue
				}
				if name != esmPath.PkgName && depsSet.Has(name) {
					external.Add(name)
				}
			}
			args.external = external
		}
	}
	return nil
}

func walkDeps(npmrc *NpmRC, installDir string, esmPath ESMPath, mark *StringSet) (err error) {
	if mark.Has(esmPath.PkgName) {
		return
	}
	mark.Add(esmPath.PkgName)
	var p *PackageJSON
	pkgJsonPath := path.Join(installDir, "node_modules", ".pnpm", "node_modules", esmPath.PkgName, "package.json")
	if !existsFile(pkgJsonPath) {
		pkgJsonPath = path.Join(installDir, "node_modules", esmPath.PkgName, "package.json")
	}
	if existsFile(pkgJsonPath) {
		var raw PackageJSONRaw
		err = utils.ParseJSONFile(pkgJsonPath, &raw)
		if err == nil {
			p = raw.ToNpmPackage()
		}
	} else if regexpVersionStrict.MatchString(esmPath.PkgVersion) || esmPath.GhPrefix || esmPath.PrPrefix {
		p, err = npmrc.installPackage(esmPath)
	} else {
		return nil
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
		if strings.HasPrefix(name, "@types/") || strings.HasPrefix(name, "@babel/") {
			continue
		}
		err := walkDeps(npmrc, installDir, ESMPath{PkgName: name, PkgVersion: version}, mark)
		if err != nil {
			return err
		}
	}
	return
}
