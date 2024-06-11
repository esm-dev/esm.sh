package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ije/gox/utils"
)

type BuildArgs struct {
	alias             map[string]string
	deps              PkgSlice
	external          *StringSet
	exports           *StringSet
	conditions        []string
	jsxRuntime        *Pkg
	keepNames         bool
	ignoreAnnotations bool
	externalRequire   bool
}

func decodeBuildArgs(npmrc *NpmRC, argsString string) (args BuildArgs, err error) {
	s, err := atobUrl(argsString)
	if err == nil {
		args = BuildArgs{
			external: NewStringSet(),
			exports:  NewStringSet(),
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
				for _, p := range strings.Split(p[1:], ",") {
					m, _, _, _, err := validatePkgPath(npmrc, p)
					if err != nil {
						if strings.HasSuffix(err.Error(), "not found") {
							continue
						}
						return args, err
					}
					if !args.deps.Has(m.Name) {
						args.deps = append(args.deps, m)
					}
				}
			} else if strings.HasPrefix(p, "e") {
				for _, name := range strings.Split(p[1:], ",") {
					args.external.Add(name)
				}
			} else if strings.HasPrefix(p, "s") {
				for _, name := range strings.Split(p[1:], ",") {
					args.exports.Add(name)
				}
			} else if strings.HasPrefix(p, "c") {
				args.conditions = append(args.conditions, strings.Split(p[1:], ",")...)
			} else if strings.HasPrefix(p, "x") {
				p, _, _, _, e := validatePkgPath(npmrc, p[1:])
				if e == nil {
					args.jsxRuntime = &p
				}
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

func encodeBuildArgs(args BuildArgs, pkg Pkg, isDts bool) string {
	lines := []string{}
	if len(args.alias) > 0 {
		var ss sort.StringSlice
		for from, to := range args.alias {
			if from != pkg.Name {
				ss = append(ss, fmt.Sprintf("%s:%s", from, to))
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("a%s", strings.Join(ss, ",")))
		}
	}
	if len(args.deps) > 0 {
		var ss sort.StringSlice
		for _, p := range args.deps {
			if p.Name != pkg.Name {
				ss = append(ss, fmt.Sprintf("%s@%s", p.Name, p.Version))
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("d%s", strings.Join(ss, ",")))
		}
	}
	if args.external.Len() > 0 {
		var ss sort.StringSlice
		for _, name := range args.external.Values() {
			if name != pkg.Name {
				ss = append(ss, name)
			}
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("e%s", strings.Join(ss, ",")))
		}
	}
	if !isDts {
		if args.exports.Len() > 0 {
			var ss sort.StringSlice
			for _, name := range args.exports.Values() {
				ss = append(ss, name)
			}
			if len(ss) > 0 {
				ss.Sort()
				lines = append(lines, fmt.Sprintf("s%s", strings.Join(ss, ",")))
			}
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
	if args.jsxRuntime != nil {
		lines = append(lines, fmt.Sprintf("x%s", args.jsxRuntime.String()))
	}
	if len(lines) > 0 {
		return btoaUrl(strings.Join(lines, "\n"))
	}
	return ""
}

func fixBuildArgs(npmrc *NpmRC, args *BuildArgs, pkg Pkg) {
	if len(args.alias) > 0 || len(args.deps) > 0 || args.external.Len() > 0 {
		depTree := NewStringSet(walkDeps(npmrc, NewStringSet(), pkg)...)
		if len(args.alias) > 0 {
			alias := map[string]string{}
			for from, to := range args.alias {
				if depTree.Has(from) {
					alias[from] = to
				}
			}
			for _, to := range alias {
				pkgName, _, _, _ := splitPkgPath(to)
				depTree.Add(pkgName)
			}
			args.alias = alias
		}
		if len(args.deps) > 0 {
			var deps PkgSlice
			for _, p := range args.deps {
				if depTree.Has(p.Name) {
					deps = append(deps, p)
				}
			}
			args.deps = deps
		}
		if args.external.Len() > 0 {
			external := NewStringSet()
			for _, name := range args.external.Values() {
				if depTree.Has(name) {
					external.Add(name)
				}
			}
			args.external = external
		}
	}
}

func walkDeps(npmrc *NpmRC, marker *StringSet, pkg Pkg) (deps []string) {
	if marker.Has(pkg.Name) {
		return nil
	}
	marker.Add(pkg.Name)
	p, err := npmrc.getPackageInfo(pkg.Name, pkg.Version)
	if err != nil {
		return nil
	}
	pkgDeps := map[string]string{}
	for name, version := range p.Dependencies {
		pkgDeps[name] = version
	}
	for name, version := range p.PeerDependencies {
		pkgDeps[name] = version
	}
	ch := make(chan []string, len(pkgDeps))
	for name, version := range pkgDeps {
		deps = append(deps, name)
		go func(c chan []string, marker *StringSet, name, version string) {
			c <- walkDeps(npmrc, marker, Pkg{Name: name, Version: version})
		}(ch, marker, name, version)
	}
	for range pkgDeps {
		deps = append(deps, <-ch...)
	}
	return
}
