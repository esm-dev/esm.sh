package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ije/gox/utils"
)

type BuildArgs struct {
	alias             map[string]string
	conditions        *StringSet
	denoStdVersion    string
	deps              PkgSlice
	exports           *StringSet
	external          *StringSet
	ignoreAnnotations bool
	ignoreRequire     bool
	jsxRuntime        *Pkg
	keepNames         bool
}

func decodeBuildArgsPrefix(raw string) (args BuildArgs, err error) {
	s, err := atobUrl(strings.TrimPrefix(strings.TrimSuffix(raw, "/"), "X-"))
	if err == nil {
		args = BuildArgs{
			external:   newStringSet(),
			exports:    newStringSet(),
			conditions: newStringSet(),
		}
		for _, p := range strings.Split(s, "\n") {
			if strings.HasPrefix(p, "a/") {
				args.alias = map[string]string{}
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "a/"), "alias:"), ",") {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" {
						args.alias[name] = to
					}
				}
			} else if strings.HasPrefix(p, "d/") {
				for _, p := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "d/"), "deps:"), ",") {
					m, _, _, err := validatePkgPath(p)
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
			} else if strings.HasPrefix(p, "e/") {
				for _, name := range strings.Split(strings.TrimPrefix(p, "e/"), ",") {
					args.external.Add(name)
				}
			} else if strings.HasPrefix(p, "ts/") {
				for _, name := range strings.Split(strings.TrimPrefix(p, "ts/"), ",") {
					args.exports.Add(name)
				}
			} else if strings.HasPrefix(p, "c/") {
				for _, name := range strings.Split(strings.TrimPrefix(p, "c/"), ",") {
					args.conditions.Add(name)
				}
			} else if strings.HasPrefix(p, "dsv/") {
				args.denoStdVersion = strings.TrimPrefix(p, "dsv/")
			} else if strings.HasPrefix(p, "jsx/") {
				p, _, _, e := validatePkgPath(strings.TrimPrefix(p, "jsx/"))
				if e == nil {
					args.jsxRuntime = &p
				}
			} else {
				switch p {
				case "ir":
					args.ignoreRequire = true
				case "kn":
					args.keepNames = true
				case "ia":
					args.ignoreAnnotations = true
				}
			}
		}
	}
	return
}

func encodeBuildArgsPrefix(args BuildArgs, pkg Pkg, isDts bool) string {
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
			lines = append(lines, fmt.Sprintf("a/%s", strings.Join(ss, ",")))
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
			lines = append(lines, fmt.Sprintf("d/%s", strings.Join(ss, ",")))
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
			lines = append(lines, fmt.Sprintf("e/%s", strings.Join(ss, ",")))
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
				lines = append(lines, fmt.Sprintf("ts/%s", strings.Join(ss, ",")))
			}
		}
	}
	if args.conditions.Len() > 0 {
		var ss sort.StringSlice
		for _, name := range args.conditions.Values() {
			ss = append(ss, name)
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("c/%s", strings.Join(ss, ",")))
		}
	}
	if !isDts {
		if args.denoStdVersion != "" && args.denoStdVersion != denoStdVersion {
			lines = append(lines, fmt.Sprintf("dsv/%s", args.denoStdVersion))
		}
		if args.ignoreRequire {
			lines = append(lines, "ir")
		}
		if args.keepNames {
			lines = append(lines, "kn")
		}
		if args.ignoreAnnotations {
			lines = append(lines, "ia")
		}
	}
	if args.jsxRuntime != nil {
		lines = append(lines, fmt.Sprintf("jsx/%s", args.jsxRuntime.String()))
	}
	if len(lines) > 0 {
		return fmt.Sprintf("X-%s/", btoaUrl(strings.Join(lines, "\n")))
	}
	return ""
}

func fixBuildArgs(args *BuildArgs, pkg Pkg) {
	if len(args.alias) > 0 || len(args.deps) > 0 || args.external.Len() > 0 {
		depTree := newStringSet(walkDeps(newStringSet(), pkg)...)
		if len(args.alias) > 0 {
			alias := map[string]string{}
			for from, to := range args.alias {
				if depTree.Has(from) {
					alias[from] = to
				}
			}
			for _, to := range alias {
				pkgName, _, _ := splitPkgPath(to)
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
			external := newStringSet()
			for _, name := range args.external.Values() {
				if depTree.Has(name) {
					external.Add(name)
				}
			}
			args.external = external
		}
	}
}

func walkDeps(marker *StringSet, pkg Pkg) (deps []string) {
	if marker.Has(pkg.Name) {
		return nil
	}
	marker.Add(pkg.Name)
	p, _, err := getPackageInfo("", pkg.Name, pkg.Version)
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
			c <- walkDeps(marker, Pkg{Name: name, Version: version})
		}(ch, marker, name, version)
	}
	for range pkgDeps {
		deps = append(deps, <-ch...)
	}
	return
}
