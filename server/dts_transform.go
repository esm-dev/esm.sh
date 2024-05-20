package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/ije/gox/utils"
)

// transformDTS transforms a `.d.ts` file for deno/editor-lsp
func transformDTS(ctx *BuildContext, dts string, buildArgsPrefix string, marker *StringSet) (n int, err error) {
	root := marker == nil
	if root {
		marker = NewStringSet()
	}
	// don't transform repeatly
	if key := buildArgsPrefix + dts; marker.Has(key) {
		return
	} else {
		marker.Add(buildArgsPrefix + dts)
	}

	pkgName, version, subPath, _ := splitPkgPath(dts)
	pkgNameWithVersion := pkgName
	if version != "" {
		pkgNameWithVersion = pkgNameWithVersion + "@" + version
	}
	dtsPath := path.Join("/"+ctx.pkg.ghPrefix(), pkgNameWithVersion, buildArgsPrefix, subPath)
	savePath := normalizeSavePath(ctx.zoneId, path.Join("types", dtsPath))

	// check if the dts file has been transformed
	_, err = fs.Stat(savePath)
	if err == nil || err != storage.ErrNotFound {
		return
	}

	dtsFilePath := path.Join(ctx.wd, "node_modules", pkgName, subPath)
	dtsWD := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		// if the dts file does not exist, print a warning but continue to build
		if os.IsNotExist(err) {
			if root {
				err = fmt.Errorf("types not found")
			} else {
				log.Warnf("dts file not found: %s", dtsFilePath)
				err = nil
			}
		}
		return
	}
	defer dtsFile.Close()

	buffer := bytes.NewBuffer(nil)
	selfDeps := NewStringSet()
	varyOrigin := false

	if pkgName == "@types/node" {
		relPath, _ := filepath.Rel(path.Dir("/"+dtsPath), "/node.ns.d.ts")
		fmt.Fprintf(buffer, "/// <reference path=\"%s\" />\n", relPath)
	}

	err = walkDts(dtsFile, buffer, func(specifier string, kind string, position int) (string, error) {
		if kind == "declareModule" {
			// resove `declare module "node:*" {}`
			if pkgName == "@types/node" {
				if strings.HasPrefix(specifier, "node:") {
					varyOrigin = true
					return fmt.Sprintf("__ESM_CDN_ORIGIN__/@types/node@%s/%s.d.ts", nodeTypesVersion, strings.TrimPrefix(specifier, "node:")), nil
				}
				return specifier, nil
			}
		}

		if isRelativeSpecifier(specifier) {
			if specifier == "." {
				specifier = "./"
			} else if specifier == ".." {
				specifier = "../"
			}
			if !endsWith(specifier, ".d.ts", ".d.mts") {
				var p PackageJSON
				specifier = stripModuleExt(specifier)
				if parseJSONFile(path.Join(dtsWD, specifier, "package.json"), &p) == nil {
					if p.Types != "" {
						specifier = strings.TrimSuffix(specifier, "/") + utils.CleanPath(p.Types)
					} else if p.Typings != "" {
						specifier = strings.TrimSuffix(specifier, "/") + utils.CleanPath(p.Typings)
					}
				} else if !strings.HasSuffix(specifier, "/") && existsFile(path.Join(dtsWD, specifier+".d.mts")) {
					specifier = specifier + ".d.mts"
				} else if !strings.HasSuffix(specifier, "/") && existsFile(path.Join(dtsWD, specifier+".d.ts")) {
					specifier = specifier + ".d.ts"
				} else if existsFile(path.Join(dtsWD, specifier, "index.d.mts")) {
					specifier = strings.TrimSuffix(specifier, "/") + "/index.d.mts"
				} else if existsFile(path.Join(dtsWD, specifier, "index.d.ts")) {
					specifier = strings.TrimSuffix(specifier, "/") + "/index.d.ts"
				}
			}
			selfDeps.Add(specifier)
			return specifier, nil
		}

		if specifier == "node" {
			relPath, _ := filepath.Rel(path.Dir("/"+dtsPath), "/node.ns.d.ts")
			return relPath, nil
		}
		if strings.HasPrefix(specifier, "node:") {
			relPath, _ := filepath.Rel(path.Dir("/"+dtsPath), fmt.Sprintf("/@types/node@%s/%s.d.ts", nodeTypesVersion, strings.TrimPrefix(specifier, "node:")))
			return relPath, nil
		}
		if _, ok := nodejsInternalModules[strings.Split(specifier, "/")[0]]; ok {
			if pkgName == "@types/node" {
				return specifier, nil
			}
			relPath, _ := filepath.Rel(path.Dir("/"+dtsPath), fmt.Sprintf("/@types/node@%s/%s.d.ts", nodeTypesVersion, specifier))
			return relPath, nil
		}

		depPkgName, _, subPath, _ := splitPkgPath(utils.CleanPath(specifier))
		specifier = depPkgName
		if subPath != "" {
			specifier += "/" + subPath
		}

		fmt.Println(specifier)

		// respect `?alias` query
		alias, ok := ctx.args.alias[depPkgName]
		if ok {
			depPkgName = alias
			specifier = fmt.Sprintf("%s/%s", depPkgName, subPath)
		}

		// respect `?external` query
		if ctx.args.external.Has("*") || ctx.args.external.Has(depPkgName) {
			return specifier, nil
		}

		if _, ok := ctx.pkgJson.Dependencies["@types/"+depPkgName]; ok {
			depPkgName = "@types/" + depPkgName
			specifier = fmt.Sprintf("%s/%s", depPkgName, subPath)
		} else if _, ok := ctx.pkgJson.Dependencies["@types/"+depPkgName]; ok {
			depPkgName = "@types/" + depPkgName
			specifier = fmt.Sprintf("%s/%s", depPkgName, subPath)
		}

		_, p, _, err := ctx.lookupDep(depPkgName)
		if (err == nil && p.Types == "" && p.Typings == "") || (err != nil && strings.HasSuffix(err.Error(), "not found") && !strings.HasSuffix(depPkgName, "@types/")) {
			depPkgName = toTypesPackageName("@types/" + depPkgName)
			specifier = fmt.Sprintf("%s/%s", depPkgName, subPath)
			_, p, _, err = ctx.lookupDep(depPkgName)
		}
		if err != nil {
			if strings.HasSuffix(err.Error(), "not found") {
				return specifier, nil
			}
			return "", err
		}

		depPkg := Pkg{
			Name:      depPkgName,
			Version:   p.Version,
			SubPath:   subPath,
			SubModule: subPath,
		}

		if kind == "declareModule" && strings.Contains(subPath, "*") {
			varyOrigin = true
			return fmt.Sprintf("__ESM_CDN_ORIGIN__/%s", depPkg.String()), nil
		}

		buildCtx := NewBuildContext(ctx.zoneId, ctx.npmrc, depPkg, ctx.args, "types", BundleFalse, false, false)
		ret, err := buildCtx.Build()
		if err != nil {
			return "", err
		}

		if kind == "declareModule" {
			varyOrigin = true
			return fmt.Sprintf("__ESM_CDN_ORIGIN__%s", ret.Dts), nil
		}

		if ret.Dts != "" {
			relPath, _ := filepath.Rel(path.Dir("/"+dtsPath), ret.Dts)
			return relPath, nil
		}
		return specifier, nil
	})
	if err != nil {
		return
	}

	// workaround for `@types/node`
	if pkgName == "@types/node" {
		dtsData := buffer.Bytes()
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
		buffer = bytes.NewBuffer(dtsData)
	}

	// fix preact/compat types
	if pkgName == "preact" && strings.HasSuffix(savePath, "/compat/src/index.d.ts") {
		dtsData := buffer.Bytes()
		if !bytes.Contains(dtsData, []byte("export type PropsWithChildren")) {
			dtsData = bytes.ReplaceAll(
				dtsData,
				[]byte("export import ComponentProps = preact.ComponentProps;"),
				[]byte("export import ComponentProps = preact.ComponentProps;\n\n// added by esm.sh\nexport type PropsWithChildren<P = unknown> = P & { children?: preact.ComponentChildren };"),
			)
			buffer = bytes.NewBuffer(dtsData)
		}
	}

	if varyOrigin {
		_, err = fs.WriteFile(savePath+".vo", bytes.NewBuffer(nil))
		if err != nil {
			return
		}
	}

	_, err = fs.WriteFile(savePath, buffer)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, s := range selfDeps.Values() {
		wg.Add(1)
		go func(s string) {
			j, err := transformDTS(ctx, path.Join(path.Dir(dts), s), buildArgsPrefix, marker)
			if err != nil {
				errors = append(errors, err)
			}
			n += j
			wg.Done()
		}(s)
	}
	wg.Wait()

	if len(errors) > 0 {
		err = errors[0]
	}
	return
}

// removes `global { ... }` block from dts
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
