package server

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/esm-dev/esm.sh/server/storage"
)

// transformDTS transforms a `.d.ts` file for deno/editor-lsp
func transformDTS(ctx *BuildContext, dts string, buildArgsPrefix string, marker *StringSet) (n int, err error) {
	isRoot := marker == nil
	if isRoot {
		marker = NewStringSet()
	}
	dtsPath := path.Join("/"+ctx.pkg.ghPrefix(), ctx.pkg.Fullname(), buildArgsPrefix, dts)
	if marker.Has(dtsPath) {
		// don't transform repeatly
		return
	}
	marker.Add(dtsPath)

	savePath := normalizeSavePath(ctx.zoneId, path.Join("types", dtsPath))
	// check if the dts file has been transformed
	_, err = fs.Stat(savePath)
	if err == nil || err != storage.ErrNotFound {
		return
	}

	dtsFilePath := path.Join(ctx.wd, "node_modules", ctx.pkg.Name, dts)
	dtsWD := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		// if the dts file does not exist, print a warning but continue to build
		if os.IsNotExist(err) {
			if isRoot {
				err = fmt.Errorf("types not found")
			} else {
				log.Warnf("dts file not found: %s", dtsFilePath)
				err = nil
			}
		}
		return
	}

	buffer := bytes.NewBuffer(nil)
	internalDeps := NewStringSet()
	referenceNodeTypes := false
	hasReferenceNodeTypes := false

	err = walkDts(dtsFile, buffer, func(specifier string, kind string, position int) (string, error) {
		if ctx.pkg.Name == "@types/node" {
			return specifier, nil
		}

		if isRelativeSpecifier(specifier) {
			if specifier == "." {
				specifier = "./"
			} else if specifier == ".." {
				specifier = "../"
			}
			specifier = strings.TrimSuffix(specifier, ".d")
			if !endsWith(specifier, ".d.ts", ".d.mts") {
				var p PackageJSON
				var hasTypes bool
				if parseJSONFile(path.Join(dtsWD, specifier, "package.json"), &p) == nil {
					dir := path.Join("/", path.Dir(dts))
					if p.Types != "" {
						specifier, _ = relPath(dir, "/"+path.Join(dir, specifier, p.Types))
						hasTypes = true
					} else if p.Typings != "" {
						specifier, _ = relPath(dir, "/"+path.Join(dir, specifier, p.Typings))
						hasTypes = true
					}
				}
				if !hasTypes {
					if existsFile(path.Join(dtsWD, specifier+".d.mts")) {
						specifier = specifier + ".d.mts"
					} else if existsFile(path.Join(dtsWD, specifier+".d.ts")) {
						specifier = specifier + ".d.ts"
					} else if existsFile(path.Join(dtsWD, specifier, "index.d.mts")) {
						specifier = strings.TrimSuffix(specifier, "/") + "/index.d.mts"
					} else if existsFile(path.Join(dtsWD, specifier, "index.d.ts")) {
						specifier = strings.TrimSuffix(specifier, "/") + "/index.d.ts"
					} else if endsWith(specifier, ".js", ".mjs", ".cjs") {
						specifier = stripModuleExt(specifier)
						if existsFile(path.Join(dtsWD, specifier+".d.mts")) {
							specifier = specifier + ".d.mts"
						} else if existsFile(path.Join(dtsWD, specifier+".d.ts")) {
							specifier = specifier + ".d.ts"
						}
					}
				}
			}

			if endsWith(specifier, ".d.ts", ".d.mts") {
				internalDeps.Add(specifier)
			} else {
				specifier += ".d.ts"
			}
			return specifier, nil
		}

		if (kind == "referenceTypes" || kind == "referencePath") && specifier == "node" {
			hasReferenceNodeTypes = true
			return fmt.Sprintf("{ESM_CDN_ORIGIN}/@types/node@%s/index.d.ts", nodeTypesVersion), nil
		}

		if specifier == "node" || strings.HasPrefix(specifier, "node:") || nodejsInternalModules[specifier] {
			referenceNodeTypes = true
			return specifier, nil
		}

		depPkgName, _, subPath, _ := splitPkgPath(specifier)
		specifier = depPkgName
		if subPath != "" {
			specifier += "/" + subPath
		}

		if depPkgName == ctx.pkg.Name {
			if strings.ContainsRune(subPath, '*') {
				return fmt.Sprintf(
					"{ESM_CDN_ORIGIN}/%s%s/%s%s",
					ctx.pkg.ghPrefix(),
					ctx.pkg.Fullname(),
					ctx.getBuildArgsPrefix(ctx.pkg, true),
					subPath,
				), nil
			} else {
				depPkg := Pkg{
					Name:      depPkgName,
					Version:   ctx.pkg.Version,
					SubPath:   subPath,
					SubModule: subPath,
				}
				entry := ctx.resolveEntry(depPkg)
				if entry.dts != "" {
					return fmt.Sprintf(
						"{ESM_CDN_ORIGIN}/%s%s/%s%s",
						ctx.pkg.ghPrefix(),
						ctx.pkg.Fullname(),
						ctx.getBuildArgsPrefix(ctx.pkg, true),
						strings.TrimPrefix(entry.dts, "./"),
					), nil
				}
			}
			return specifier, nil
		}

		// respect `?alias` query
		alias, ok := ctx.args.alias[depPkgName]
		if ok {
			aliasPkgName, _, aliasSubPath, _ := splitPkgPath(alias)
			depPkgName = aliasPkgName
			if aliasSubPath != "" {
				if subPath != "" {
					subPath = aliasSubPath + "/" + subPath
				} else {
					subPath = aliasSubPath
				}
			}
			specifier = depPkgName
			if subPath != "" {
				specifier += "/" + subPath
			}
		}

		// respect `?external` query
		if ctx.args.external.Has("*") || ctx.args.external.Has(depPkgName) {
			return specifier, nil
		}

		typesPkgName := toTypesPkgName(depPkgName)
		if _, ok := ctx.pkgJson.Dependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
		} else if _, ok := ctx.pkgJson.PeerDependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
		}

		_, p, _, err := ctx.lookupDep(depPkgName)
		if err != nil {
			return "", err
		}

		depPkg := Pkg{
			Name:      depPkgName,
			Version:   p.Version,
			SubPath:   subPath,
			SubModule: subPath,
		}
		args := BuildArgs{
			external: NewStringSet(),
			exports:  NewStringSet(),
		}
		b := NewBuildContext(ctx.zoneId, ctx.npmrc, depPkg, args, "types", BundleFalse, false, false)
		dts, err := b.LookupTypes()
		if err != nil {
			return "", err
		}

		if kind == "declareModule" {
			if dts != "" {
				return fmt.Sprintf("{ESM_CDN_ORIGIN}%s", dts), nil
			}
			return fmt.Sprintf("{ESM_CDN_ORIGIN}/%s", depPkg.String()), nil
		}

		if dts != "" {
			return fmt.Sprintf("{ESM_CDN_ORIGIN}%s", dts), nil
		}
		return fmt.Sprintf("{ESM_CDN_ORIGIN}%s", b.Path()), nil
	})
	if err != nil {
		return
	}
	dtsFile.Close()

	if referenceNodeTypes && !hasReferenceNodeTypes {
		refTag := []byte(fmt.Sprintf("/// <reference path=\"{ESM_CDN_ORIGIN}/@types/node@%s/index.d.ts\" />\n", nodeTypesVersion))
		buffer = bytes.NewBuffer(concatBytes(refTag, buffer.Bytes()))
	}

	_, err = fs.WriteFile(savePath, bytes.NewBuffer(ctx.rewriteDTS(dts, buffer.Bytes())))
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, s := range internalDeps.Values() {
		wg.Add(1)
		go func(s string) {
			j, err := transformDTS(ctx, "./"+path.Join(path.Dir(dts), s), buildArgsPrefix, marker)
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
