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
	externalAll       bool
	external          *StringSet
	exports           *StringSet
	conditions        []string
	jsxRuntime        *ESM
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
				deps := map[string]string{}
				for _, p := range strings.Split(p[1:], ",") {
					pkgName, pkgVersion, _, _ := splitPkgPath(p)
					deps[pkgName] = pkgVersion
				}
				args.deps = deps
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
				p, _, _, _, e := praseESMPath(npmrc, p[1:])
				if e == nil {
					args.jsxRuntime = &p
				}
			} else {
				switch p {
				case "*":
					args.externalAll = true
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
	if args.externalAll {
		lines = append(lines, "*")
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

// normalizeBuildArgs removes invalid alias, deps, external from the build args
func normalizeBuildArgs(npmrc *NpmRC, installDir string, args *BuildArgs, url ESM) error {
	if len(args.alias) > 0 || len(args.deps) > 0 || args.external.Len() > 0 {
		depsSet := NewStringSet()
		err := walkDeps(npmrc, installDir, url, depsSet)
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
				pkgName, _, _, _ := splitPkgPath(to)
				if pkgName == url.PkgName {
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
				if url.PkgName == "react-dom" && name == "react" {
					// react version should be the same as react-dom
					continue
				}
				if depsSet.Has(name) {
					if name != url.PkgName {
						deps[name] = version
					}
				}
			}
			args.deps = deps
		}
		if args.external.Len() > 0 {
			external := NewStringSet()
			for _, name := range args.external.Values() {
				if strings.HasPrefix(name, "node:") || depsSet.Has(name) {
					if name != url.PkgName || url.SubPath != "" {
						external.Add(name)
					}
				}
			}
			args.external = external
		}
	}
	return nil
}

func walkDeps(npmrc *NpmRC, installDir string, url ESM, mark *StringSet) (err error) {
	if mark.Has(url.PkgName) {
		return
	}
	mark.Add(url.PkgName)
	var p PackageJSON
	pkgJsonPath := path.Join(installDir, "node_modules", ".pnpm", "node_modules", url.PkgName, "package.json")
	if !existsFile(pkgJsonPath) {
		pkgJsonPath = path.Join(installDir, "node_modules", url.PkgName, "package.json")
	}
	if existsFile(pkgJsonPath) {
		err = utils.ParseJSONFile(pkgJsonPath, &p)
	} else if regexpVersionStrict.MatchString(url.PkgVersion) || url.GhPrefix {
		p, err = npmrc.installPackage(url)
	} else {
		return nil
		// p, err = npmrc.getPackageInfo(module.PkgName, module.PkgVersion)
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
		if strings.HasPrefix(name, "@types/") || strings.HasPrefix(name, "@babel/") || strings.HasPrefix(name, "is-") {
			continue
		}
		err := walkDeps(npmrc, installDir, ESM{PkgName: name, PkgVersion: version}, mark)
		if err != nil {
			return err
		}
	}
	return
}
