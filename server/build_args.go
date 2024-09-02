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
	conditions        *stringSet
	external          *stringSet
	exports           *stringSet
	denoStdVersion    string
	ignoreAnnotations bool
	ignoreRequire     bool
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
					m, _, err := validatePkgPath(p)
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
	pkgDeps := newStringSet("_")
	if len(args.alias)+len(args.deps)+args.external.Len() > 0 && cfg != nil {
		info, _, err := getPackageInfo("", pkg.Name, pkg.Version)
		if err == nil {
			pkgDeps.Reset()
			for name := range info.Dependencies {
				pkgDeps.Add(name)
			}
			for name := range info.PeerDependencies {
				pkgDeps.Add(name)
			}
			if pkg.SubPath != "" {
				pkgDeps.Add(pkg.Name)
			}
		}
	}
	if len(args.alias) > 0 && pkgDeps.Len() > 0 {
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
	if len(args.deps) > 0 && pkgDeps.Len() > 0 {
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
	if args.external.Len() > 0 && (args.external.Has("*") || pkgDeps.Len() > 0) {
		var ss sort.StringSlice
		for _, name := range args.external.Values() {
			if name != pkg.Name || pkg.SubPath != "" {
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
	if len(lines) > 0 {
		return fmt.Sprintf("X-%s/", btoaUrl(strings.Join(lines, "\n")))
	}
	return ""
}
