package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/ije/gox/utils"
)

func (task *BuildTask) TransformDTS(dts string) (n int, err error) {
	buildArgsPrefix := encodeBuildArgsPrefix(task.BuildArgs, task.Pkg, true)
	marker := newStringSet()
	err = task.transformDTS(dts, buildArgsPrefix, marker)
	if err == nil {
		n = marker.Len()
	}
	return
}

func (task *BuildTask) transformDTS(dts string, aliasDepsPrefix string, marker *stringSet) (err error) {
	// don't transform repeatly
	if marker.Has(aliasDepsPrefix + dts) {
		return
	}
	marker.Add(aliasDepsPrefix + dts)

	var pkgInfo NpmPackage
	pkgJsonPath := path.Join(task.wd, "node_modules", task.Pkg.Name, "package.json")
	err = utils.ParseJSONFile(pkgJsonPath, &pkgInfo)
	if err != nil {
		return
	}

	pkgNameWithVersion, submodule := splitPkgPath(utils.CleanPath(dts))
	pkgName, _ := utils.SplitByLastByte(pkgNameWithVersion, '@')
	if pkgName == "" {
		pkgName = pkgNameWithVersion
	}

	dir := fmt.Sprintf("/v%d", task.BuildVersion)
	if task.Pkg.FromGithub {
		dir += "/gh"
	}
	dtsPath := utils.CleanPath(strings.Join(append([]string{
		dir,
		pkgNameWithVersion,
		aliasDepsPrefix,
	}, strings.Split(submodule, "/")...), "/"))
	savePath := path.Join("types", getTypesRoot(task.CdnOrigin), dtsPath)
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
	err = walkDts(dtsFile, pass1Buf, func(name string, kind string, position int) string {
		if kind == "declareModule" {
			allDeclModules.Add(name)
		}
		return name
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

	wd := task.getRealWD()
	buf := bytes.NewBuffer(nil)
	imports := newStringSet()
	dtsBasePath := fmt.Sprintf("%s%s/v%d", task.CdnOrigin, cfg.BasePath, task.BuildVersion)
	if pkgName == "@types/node" {
		fmt.Fprintf(buf, "/// <reference path=\"%s/node.ns.d.ts\" />\n", dtsBasePath)
	}
	err = walkDts(pass1Buf, buf, func(importPath string, kind string, position int) string {
		// resove `declare module "xxx" {}`
		if kind == "declareModule" {
			if strings.HasPrefix(importPath, "node:") && pkgName == "@types/node" {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, strings.TrimPrefix(importPath, "node:"))
			}
			if internalDeclModules.Has(importPath) {
				return importPath
			}
		}

		if task.external.Has("*") && !strings.HasPrefix(pkgName, "@types/") && !isLocalSpecifier(importPath) {
			return importPath
		}

		// use `@types/xxx`
		switch importPath {
		case "estree", "estree-jsx", "unist", "react", "react-dom":
			importPath = fmt.Sprintf("@types/%s", importPath)
		}

		// fix some weird import paths
		if kind == "importCall" {
			if task.Pkg.Name == "@mdx-js/mdx" {
				if (strings.Contains(dts, "plugin/recma-document") || strings.Contains(dts, "plugin/recma-jsx-rewrite")) && importPath == "@types/estree" {
					importPath = "@types/estree-jsx"
				}
			}
			if strings.HasPrefix(dts, "remark-rehype") && importPath == "mdast-util-to-hast/lib" {
				importPath = "mdast-util-to-hast"
			}
		}

		// use `?alias`
		to, ok := task.alias[importPath]
		if ok {
			importPath = to
		}

		if internalDeclModules.Has(importPath) || task.external.Has(importPath) {
			return importPath
		}

		if isLocalSpecifier(importPath) {
			if importPath == "." {
				importPath = "./index.d.ts"
			}
			if importPath == ".." {
				importPath = "../index.d.ts"
			}
			// some types is using `.js` extname
			importPath = strings.TrimSuffix(importPath, ".js")
			if !strings.HasSuffix(importPath, ".d.ts") {
				if fileExists(path.Join(dtsDir, importPath, "index.d.ts")) {
					importPath = strings.TrimSuffix(importPath, "/") + "/index.d.ts"
				} else if fileExists(path.Join(dtsDir, importPath+".d.ts")) {
					importPath = importPath + ".d.ts"
				} else {
					var p NpmPackage
					packageJSONFile := path.Join(dtsDir, importPath, "package.json")
					if fileExists(packageJSONFile) && utils.ParseJSONFile(packageJSONFile, &p) == nil {
						if p.Types != "" {
							importPath = strings.TrimSuffix(importPath, "/") + utils.CleanPath(p.Types)
						} else if p.Typings != "" {
							importPath = strings.TrimSuffix(importPath, "/") + utils.CleanPath(p.Typings)
						}
					}
				}
			}
			if strings.HasSuffix(dts, ".d.ts") && !strings.HasSuffix(dts, "~.d.ts") {
				imports.Add(importPath)
			}
		} else {
			if importPath == "node" {
				return fmt.Sprintf("%s/node.ns.d.ts", dtsBasePath)
			}
			if strings.HasPrefix(importPath, "node:") {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, strings.TrimPrefix(importPath, "node:"))
			}
			if _, ok := builtInNodeModules[importPath]; ok {
				return fmt.Sprintf("%s/@types/node@%s/%s.d.ts", dtsBasePath, nodeTypesVersion, importPath)
			}

			depTypePkgName, _ := splitPkgPath(importPath)
			maybeVersion := []string{"latest"}
			if v, ok := pkgInfo.Dependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			} else if v, ok := pkgInfo.PeerDependencies[depTypePkgName]; ok {
				maybeVersion = []string{v, "latest"}
			}

			var (
				info            NpmPackage
				subpath         string
				fromPackageJSON bool
			)
			for _, version := range maybeVersion {
				var pkg Pkg
				pkg, _, err = validatePkgPath(importPath)
				if err != nil {
					break
				}
				subpath = pkg.Submodule
				info, fromPackageJSON, err = getPackageInfo(wd, pkg.Name, version)
				if err != nil || ((info.Types == "" && info.Typings == "") && !strings.HasPrefix(info.Name, "@types/")) {
					p, ok, e := getPackageInfo(wd, toTypesPackageName(pkg.Name), version)
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
				return importPath
			}

			// use types with `exports` and `typesVersions` contidions
			info = task.fixNpmPackage(info)

			// use version defined in `?deps`
			if pkg, ok := task.deps.Get(depTypePkgName); ok {
				info.Version = pkg.Version
			}

			// copy dependent dts files in the node_modules directory in current build context
			if fromPackageJSON {
				typesPath := task.toTypesPath(wd, info, "", "", subpath)
				if strings.HasSuffix(typesPath, ".d.ts") && !strings.HasSuffix(typesPath, "~.d.ts") {
					imports.Add(typesPath)
				}
				importPath = strings.TrimPrefix(typesPath, info.Name+"@"+info.Version+"/")
			} else {
				if subpath != "" {
					importPath = subpath
				} else if info.Types != "" {
					importPath = utils.CleanPath(info.Types)[1:]
				} else if info.Typings != "" {
					importPath = utils.CleanPath(info.Typings)[1:]
				} else {
					importPath = "index.d.ts"
				}
				if !strings.HasSuffix(importPath, ".d.ts") && !strings.HasSuffix(importPath, "/*") {
					importPath += "~.d.ts"
				}
			}
			bv := task.BuildVersion
			if stableBuild[info.Name] || stableBuild[strings.TrimPrefix(info.Name, "@types/")] {
				bv = STABLE_VERSION
			}
			pkgPath := info.Name + "@" + info.Version + "/" + encodeBuildArgsPrefix(task.BuildArgs, Pkg{Name: info.Name}, true)
			importPath = fmt.Sprintf("%s%s/v%d/%s%s", task.CdnOrigin, cfg.BasePath, bv, pkgPath, importPath)
		}
		return importPath
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

	_, err = fs.WriteFile(savePath, buf)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, importDts := range imports.Values() {
		if isLocalSpecifier(importDts) {
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

func (task *BuildTask) toTypesPath(wd string, p NpmPackage, version string, buildArgsPrefix string, subpath string) string {
	var types string
	if subpath != "" {
		t := &BuildTask{
			BuildArgs: task.BuildArgs,
			Pkg: Pkg{
				Name:      p.Name,
				Version:   p.Version,
				Submodule: subpath,
				Subpath:   subpath,
			},
			Target: task.Target,
			Dev:    false,
			wd:     wd,
		}
		_, p, _, e := t.analyze()
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

	if !endsWith(types, ".d.ts", ".d.mts") && !strings.HasSuffix(types, "/*") {
		pkgDir := path.Join(wd, "node_modules", p.Name)
		if fileExists(path.Join(pkgDir, types, "index.d.ts")) {
			types = types + "/index.d.ts"
		} else if fileExists(path.Join(pkgDir, types+".d.ts")) {
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
