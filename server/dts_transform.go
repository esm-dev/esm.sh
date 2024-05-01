package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/ije/gox/utils"
)

func (task *BuildTask) TransformDTS(dts string) (n int, err error) {
	buildArgsPrefix := encodeBuildArgsPrefix(task.Args, task.Pkg, true)
	marker := newStringSet()
	err = task.transformDTS(dts, buildArgsPrefix, marker)
	if err == nil {
		n = marker.Len()
	}
	return
}

func (task *BuildTask) transformDTS(dts string, aliasDepsPrefix string, marker *StringSet) (err error) {
	// don't transform repeatly
	if marker.Has(aliasDepsPrefix + dts) {
		return
	}
	marker.Add(aliasDepsPrefix + dts)

	var pkgInfo NpmPackageInfo
	pkgJsonPath := path.Join(task.wd, "node_modules", task.Pkg.Name, "package.json")
	err = parseJSONFile(pkgJsonPath, &pkgInfo)
	if err != nil {
		return
	}

	pkgName, version, subPath := splitPkgPath(utils.CleanPath(dts))
	pkgNameWithVersion := pkgName
	if version != "" {
		pkgNameWithVersion = pkgNameWithVersion + "@" + version
	}
	dir := "/" + task._ghPrefix()
	dtsPath := utils.CleanPath(strings.Join(append([]string{
		dir,
		pkgNameWithVersion,
		aliasDepsPrefix,
	}, strings.Split(subPath, "/")...), "/"))
	savePath := normalizeSavePath(path.Join("types", dtsPath))
	_, err = fs.Stat(savePath)
	if err != nil && err != storage.ErrNotFound {
		return
	}

	dtsFilePath := path.Join(task.wd, "node_modules", regexpFullVersionPath.ReplaceAllString(dts, "$1/"))
	dtsDir := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		return
	}
	defer dtsFile.Close()

	allDeclModules := newStringSet()
	pass1Buf := bytes.NewBuffer(nil)
	err = walkDts(dtsFile, pass1Buf, func(specifier string, kind string, position int) string {
		if kind == "declareModule" {
			allDeclModules.Add(specifier)
		}
		return specifier
	})
	if err != nil {
		return
	}

	internalDeclModules := newStringSet()
	for _, path := range allDeclModules.Values() {
		if pkgName == "@types/node" {
			if strings.HasPrefix(path, "node:") {
				continue
			}
		} else if _, ok := pkgInfo.Dependencies[path]; ok {
			continue
		} else if _, ok := pkgInfo.PeerDependencies[path]; ok {
			continue
		} else if path == pkgName || strings.HasPrefix(path, pkgName+"/") {
			continue
		}
		internalDeclModules.Add(path)
	}

	origin := cfg.CdnOrigin
	if origin == "" {
		port := cfg.Port
		if port == 0 {
			port = 8080
		}
		if port == 80 {
			origin = "http://localhost"
		} else {
			origin = fmt.Sprintf("http://localhost:%d", port)
		}
	}

	resolveDir := task.resolveDir
	buf := bytes.NewBuffer(nil)
	footer := bytes.NewBuffer(nil)
	imports := newStringSet()
	dtsBasePath := origin + cfg.CdnBasePath

	if pkgName == "@types/node" {
		fmt.Fprintf(buf, "/// <reference path=\"%s/node.ns.d.ts\" />\n", dtsBasePath)
	}

	err = walkDts(pass1Buf, buf, func(specifier string, kind string, position int) string {
		// resove `declare module "xxx" {}`
		if kind == "declareModule" {
			if strings.HasPrefix(specifier, "node:") && pkgName == "@types/node" {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, strings.TrimPrefix(specifier, "node:"))
			}
			if internalDeclModules.Has(specifier) {
				return specifier
			}
		}

		if task.Args.external.Has("*") && !strings.HasPrefix(pkgName, "@types/") && !isRelativeSpecifier(specifier) {
			return specifier
		}

		res := specifier

		// use `@types/xxx`
		switch res {
		case "estree", "estree-jsx", "unist", "react", "react-dom":
			res = fmt.Sprintf("@types/%s", res)
		}

		// fix some weird import paths
		if kind == "importCall" {
			if task.Pkg.Name == "@mdx-js/mdx" {
				if (strings.Contains(dts, "plugin/recma-document") || strings.Contains(dts, "plugin/recma-jsx-rewrite")) && res == "@types/estree" {
					res = "@types/estree-jsx"
				}
			}
			if strings.HasPrefix(dts, "remark-rehype") && res == "mdast-util-to-hast/lib" {
				res = "mdast-util-to-hast"
			}
		}

		// use `?alias`
		to, ok := task.Args.alias[res]
		if ok {
			res = to
		}

		if internalDeclModules.Has(res) || task.Args.external.Has(getPkgName(res)) {
			return res
		}

		if isRelativeSpecifier(res) {
			if res == "." {
				res = "./index.d.ts"
			}
			if res == ".." {
				res = "../index.d.ts"
			}
			// some types is using `.m?js` extname
			res = strings.TrimSuffix(res, ".mjs")
			res = strings.TrimSuffix(res, ".js")
			if !strings.HasSuffix(res, ".d.ts") && !strings.HasSuffix(res, ".d.mts") {
				if existsFile(path.Join(dtsDir, res+".d.ts")) {
					res = res + ".d.ts"
				} else if existsFile(path.Join(dtsDir, res+".d.mts")) {
					res = res + ".d.mts"
				} else if existsFile(path.Join(dtsDir, res, "index.d.ts")) {
					res = strings.TrimSuffix(res, "/") + "/index.d.ts"
				} else if existsFile(path.Join(dtsDir, res, "index.d.mts")) {
					res = strings.TrimSuffix(res, "/") + "/index.d.mts"
				} else {
					var p NpmPackageInfo
					packageJSONFile := path.Join(dtsDir, res, "package.json")
					if existsFile(packageJSONFile) && parseJSONFile(packageJSONFile, &p) == nil {
						if p.Types != "" {
							res = strings.TrimSuffix(res, "/") + utils.CleanPath(p.Types)
						} else if p.Typings != "" {
							res = strings.TrimSuffix(res, "/") + utils.CleanPath(p.Typings)
						}
					}
				}
			}
			if (strings.HasSuffix(dts, ".d.ts") || strings.HasSuffix(dts, ".d.mts")) && !strings.HasSuffix(dts, "~.d.ts") {
				imports.Add(res)
			}
		} else {
			if res == "node" {
				return fmt.Sprintf("%s/node.ns.d.ts", dtsBasePath)
			}
			if strings.HasPrefix(res, "node:") {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, strings.TrimPrefix(res, "node:"))
			}
			if _, ok := nodejsInternalModules[res]; ok {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, res)
			}

			depTypePkgName := getPkgName(res)
			maybeVersion := []string{"latest"}
			if v, ok := pkgInfo.Dependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			} else if v, ok := pkgInfo.PeerDependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			}

			var (
				info            NpmPackageInfo
				subpath         string
				fromPackageJSON bool
			)
			for _, version := range maybeVersion {
				var pkg Pkg
				pkg, _, _, err = validatePkgPath(res)
				if err != nil {
					break
				}
				subpath = toModuleBareName(pkg.SubPath, false)
				info, fromPackageJSON, err = getPackageInfo(resolveDir, pkg.Name, version)
				if err != nil || ((info.Types == "" && info.Typings == "") && !strings.HasPrefix(info.Name, "@types/")) {
					p, ok, e := getPackageInfo(resolveDir, toTypesPackageName(pkg.Name), version)
					if e == nil {
						info = p
						fromPackageJSON = ok
						err = nil
					}
				}
				if err == nil {
					break
				}
			}
			if err != nil {
				return res
			}

			// resolve types with `exports` and `typesVersions` contidions
			info = task.normalizeNpmPackage(info)

			// use version defined in `?deps`
			if pkg, ok := task.Args.deps.Get(depTypePkgName); ok {
				info.Version = pkg.Version
			}

			// copy dependent dts files in the node_modules directory in current build context
			if fromPackageJSON {
				typesPath := task.toTypesPath(resolveDir, info, "", "", subpath)
				if strings.HasSuffix(typesPath, ".d.ts") && !strings.HasSuffix(typesPath, "~.d.ts") {
					imports.Add(typesPath)
				}
				res = strings.TrimPrefix(typesPath, info.Name+"@"+info.Version+"/")
			} else {
				if subpath != "" {
					res = subpath
				} else if info.Types != "" {
					res = utils.CleanPath(info.Types)[1:]
				} else if info.Typings != "" {
					res = utils.CleanPath(info.Typings)[1:]
				} else {
					res = "index.d.ts"
				}
				if !strings.HasSuffix(res, ".d.ts") && !strings.HasSuffix(res, "/*") {
					res += "~.d.ts"
				}
			}
			pkgPath := info.Name + "@" + info.Version + "/" + encodeBuildArgsPrefix(task.Args, Pkg{Name: info.Name}, true)
			res = fmt.Sprintf("%s/%s%s", dtsBasePath, pkgPath, res)
		}

		if kind == "declareModule" && strings.HasSuffix(res, "/"+dts) {
			moduleName := pkgNameWithVersion
			if _, _, subPath := splitPkgPath(specifier); subPath != "" {
				moduleName = moduleName + "/" + subPath
			}
			aliasDeclareModule(footer, fmt.Sprintf("%s/%s", dtsBasePath, moduleName), res)
			aliasDeclareModule(footer, fmt.Sprintf("%s/%s?*", dtsBasePath, moduleName), res)
			aliasDeclareModule(footer, fmt.Sprintf("%s/%s", dtsBasePath, moduleName), res)
			aliasDeclareModule(footer, fmt.Sprintf("%s/%s?*", dtsBasePath, moduleName), res)
		}

		return res
	})
	if err != nil {
		return
	}

	// workaroud for `@types/node`
	if pkgName == "@types/node" {
		dtsData := buf.Bytes()
		dtsData = bytes.ReplaceAll(dtsData, []byte(" implements NodeJS.ReadableStream"), []byte{})
		dtsData = bytes.ReplaceAll(dtsData, []byte(" implements NodeJS.WritableStream"), []byte{})
		if strings.HasSuffix(savePath, "/buffer.d.ts") {
			dtsData = bytes.ReplaceAll(dtsData, []byte(" export { Buffer };"), []byte(" export const Buffer: Buffer;"))
		}
		if strings.HasSuffix(savePath, "/url.d.ts") || strings.HasSuffix(savePath, "/buffer.d.ts") {
			dtsData, err = removeGlobalBlock(dtsData)
			if err != nil {
				return
			}
		}
		buf = bytes.NewBuffer(dtsData)
	}

	// fix preact/compat types
	if pkgName == "preact" && strings.HasSuffix(savePath, "/compat/src/index.d.ts") {
		dtsData := buf.Bytes()
		if !bytes.Contains(dtsData, []byte("export type PropsWithChildren")) {
			dtsData = bytes.ReplaceAll(
				dtsData,
				[]byte("export import ComponentProps = preact.ComponentProps;"),
				[]byte("export import ComponentProps = preact.ComponentProps;\n\n// added by esm.sh\nexport type PropsWithChildren<P = unknown> = P & { children?: preact.ComponentChildren };"),
			)
			buf = bytes.NewBuffer(dtsData)
		}
	}

	if footer.Len() > 0 {
		buf.WriteString("\n// added by esm.sh\n")
		io.Copy(buf, footer)
	}

	_, err = fs.WriteFile(savePath, buf)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, importDts := range imports.Values() {
		if isRelativeSpecifier(importDts) {
			if strings.HasPrefix(importDts, "/") {
				pkg, subpath := utils.SplitByFirstByte(importDts, '/')
				if strings.HasPrefix(pkg, "@") {
					n, _ := utils.SplitByFirstByte(subpath, '/')
					pkg = fmt.Sprintf("%s/%s", pkg, n)
				}
				importDts = path.Join(pkg, importDts)
			} else {
				importDts = path.Join(path.Dir(dts), importDts)
			}
		}
		wg.Add(1)
		go func(importDts string) {
			err := task.transformDTS(importDts, aliasDepsPrefix, marker)
			if err != nil {
				errors = append(errors, err)
			}
			wg.Done()
		}(importDts)
	}
	wg.Wait()

	if len(errors) > 0 {
		err = errors[0]
	}

	return
}

// to remove `global { ... }`
func removeGlobalBlock(input []byte) (output []byte, err error) {
	start := bytes.Index(input, []byte("global {"))
	if start == -1 {
		return input, nil
	}
	dep := 1
	for i := start + 8; i < len(input); i++ {
		c := input[i]
		if c == '{' {
			dep++
		} else if c == '}' {
			dep--
		}
		if dep == 0 {
			return bytes.Join([][]byte{input[:start], input[i+1:]}, nil), nil
		}
	}
	return nil, errors.New("removeGlobalBlock: global block not end")
}

func (task *BuildTask) toTypesPath(wd string, p NpmPackageInfo, version string, buildArgsPrefix string, subpath string) string {
	var types string
	if subpath != "" {
		t := &BuildTask{
			Args: task.Args,
			Pkg: Pkg{
				Name:      p.Name,
				Version:   p.Version,
				SubModule: subpath,
				SubPath:   subpath,
			},
			Target: task.Target,
			Dev:    false,
			wd:     wd,
		}
		_, p, _, e := t.analyze(false)
		if e == nil {
			types = p.Types
		}
		if types == "" {
			types = subpath
		}
	} else if p.Types != "" {
		types = p.Types
	} else if p.Typings != "" {
		types = p.Typings
	} else if strings.HasPrefix(p.Name, "@types/") {
		if strings.HasSuffix(p.Main, ".d.ts") {
			types = p.Main
		} else {
			types = "index.d.ts"
		}
	} else {
		types = "index.d.ts"
	}

	if endsWith(types, ".d") {
		pkgDir := path.Join(wd, "node_modules", p.Name)
		if existsFile(path.Join(pkgDir, types+".ts")) {
			types = types + ".ts"
		} else if existsFile(path.Join(pkgDir, types+".mts")) {
			types = types + ".mts"
		}
	}

	if !endsWith(types, ".d.ts", ".d.mts") && !strings.HasSuffix(types, "/*") {
		pkgDir := path.Join(wd, "node_modules", p.Name)
		if existsFile(path.Join(pkgDir, types, "index.d.ts")) {
			types = types + "/index.d.ts"
		} else if existsFile(path.Join(pkgDir, types+".d.ts")) {
			types = types + ".d.ts"
		} else {
			types = types + "~.d.ts" // dynamic
		}
	}

	if version == "" {
		version = p.Version
	}

	return fmt.Sprintf("%s@%s/%s%s", p.Name, version, buildArgsPrefix, utils.CleanPath(types)[1:])
}

func aliasDeclareModule(wr io.Writer, aliasName string, moduleName string) {
	fmt.Fprintf(wr, "declare module \"%s\" {\n", aliasName)
	fmt.Fprintf(wr, "  export * from \"%s\";\n", moduleName)
	fmt.Fprintf(wr, "}\n")
}
