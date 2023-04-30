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
	treeShaking       *stringSet
	denoStdVersion    string
	ignoreAnnotations bool
	ignoreRequire     bool
	keepNames         bool
}

func decodeBuildArgsPrefix(raw string) (args BuildArgs, err error) {
	s, err := atobUrl(strings.TrimPrefix(strings.TrimSuffix(raw, "/"), "X-"))
	if err == nil {
		args = BuildArgs{
			external:    newStringSet(),
			treeShaking: newStringSet(),
			conditions:  newStringSet(),
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
					args.treeShaking.Add(name)
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

func encodeBuildArgsPrefix(args BuildArgs, pkg Pkg, forTypes bool) string {
	lines := []string{}
	if !(stableBuild[pkg.Name] && pkg.Submodule == "") {
		if len(args.alias) > 0 {
			var ss sort.StringSlice
			for name, to := range args.alias {
				if name != pkg.Name {
					ss = append(ss, fmt.Sprintf("%s:%s", name, to))
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
				if pkg.Name == "react-dom" && p.Name == "react" {
					continue
				}
				if p.Name != pkg.Name {
					ss = append(ss, fmt.Sprintf("%s@%s", p.Name, p.Version))
				}
			}
			if len(ss) > 0 {
				ss.Sort()
				lines = append(lines, fmt.Sprintf("d/%s", strings.Join(ss, ",")))
			}
		}
		if args.external.Size() > 0 {
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
		if args.treeShaking.Size() > 0 {
			var ss sort.StringSlice
			for _, name := range args.treeShaking.Values() {
				ss = append(ss, name)
			}
			if len(ss) > 0 {
				ss.Sort()
				lines = append(lines, fmt.Sprintf("ts/%s", strings.Join(ss, ",")))
			}
		}
	}
	if args.conditions.Size() > 0 {
		var ss sort.StringSlice
		for _, name := range args.conditions.Values() {
			ss = append(ss, name)
		}
		if len(ss) > 0 {
			ss.Sort()
			lines = append(lines, fmt.Sprintf("c/%s", strings.Join(ss, ",")))
		}
	}
	if !forTypes {
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
