package server

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/ije/gox/utils"
)

// transformDTS transforms a `.d.ts` file for deno/editor-lsp
func transformDTS(ctx *BuildContext, dts string, buildArgsPrefix string, marker *StringSet) (n int, err error) {
	isEntry := marker == nil
	if isEntry {
		marker = NewStringSet()
	}
	dtsPath := path.Join("/"+ctx.esmPath.PackageName(), buildArgsPrefix, dts)
	if marker.Has(dtsPath) {
		// don't transform repeatly
		return
	}
	marker.Add(dtsPath)

	savePath := normalizeSavePath(ctx.zoneId, path.Join("types", dtsPath))
	// check if the dts file has been transformed
	_, err = buildStorage.Stat(savePath)
	if err == nil || err != storage.ErrNotFound {
		return
	}

	dtsFilePath := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, dts)
	dtsWD := path.Dir(dtsFilePath)
	dtsFile, err := os.Open(dtsFilePath)
	if err != nil {
		// if the dts file does not exist, print a warning but continue to build
		if os.IsNotExist(err) {
			if isEntry {
				err = fmt.Errorf("types not found")
			} else {
				log.Warnf("dts not found: %s", dtsFilePath)
				err = nil
			}
		}
		return
	}

	buffer := bytes.NewBuffer(nil)
	internalDts := NewStringSet()
	withNodeBuiltinModule := false
	hasReferenceTypesNode := false

	err = parseDts(dtsFile, buffer, func(specifier string, kind TsImportKind, position int) (string, error) {
		if ctx.esmPath.PkgName == "@types/node" {
			return specifier, nil
		}

		// normalize specifier
		specifier = normalizeImportSpecifier(specifier)

		if isRelPathSpecifier(specifier) {
			specifier = strings.TrimSuffix(specifier, ".d")
			if !endsWith(specifier, ".d.ts", ".d.mts") {
				var p PackageJSONRaw
				var hasTypes bool
				if utils.ParseJSONFile(path.Join(dtsWD, specifier, "package.json"), &p) == nil {
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
				internalDts.Add(specifier)
			} else {
				specifier += ".d.ts"
			}
			return specifier, nil
		}

		if (kind == TsReferencePath || kind == TsReferenceTypes) && specifier == "node" {
			hasReferenceTypesNode = true
			return fmt.Sprintf("{ESM_CDN_ORIGIN}/@types/node@%s/index.d.ts", nodeTypesVersion), nil
		}

		if specifier == "node" || isNodeBuiltInModule(specifier) {
			withNodeBuiltinModule = true
			return specifier, nil
		}

		depPkgName, _, subPath, _ := splitESMPath(specifier)
		specifier = depPkgName
		if subPath != "" {
			specifier += "/" + subPath
		}

		if depPkgName == ctx.esmPath.PkgName {
			if strings.ContainsRune(subPath, '*') {
				return fmt.Sprintf(
					"{ESM_CDN_ORIGIN}/%s/%s%s",
					ctx.esmPath.PackageName(),
					ctx.getBuildArgsPrefix(true),
					subPath,
				), nil
			} else {
				entry := ctx.resolveEntry(ESMPath{
					PkgName:     depPkgName,
					PkgVersion:  ctx.esmPath.PkgVersion,
					SubPath:     subPath,
					SubBareName: subPath,
				})
				if entry.dts != "" {
					return fmt.Sprintf(
						"{ESM_CDN_ORIGIN}/%s/%s%s",
						ctx.esmPath.PackageName(),
						ctx.getBuildArgsPrefix(true),
						strings.TrimPrefix(entry.dts, "./"),
					), nil
				}
			}
			return specifier, nil
		}

		// respect `?alias` query
		alias, ok := ctx.args.alias[depPkgName]
		if ok {
			aliasPkgName, _, aliasSubPath, _ := splitESMPath(alias)
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
		if ctx.args.externalAll || ctx.args.external.Has(depPkgName) {
			return specifier, nil
		}

		typesPkgName := toTypesPackageName(depPkgName)
		if _, ok := ctx.packageJson.Dependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
		} else if _, ok := ctx.packageJson.PeerDependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
		}

		_, p, err := ctx.lookupDep(depPkgName, true)
		if err != nil {
			if kind == TsDeclareModule && strings.HasSuffix(err.Error(), " not found") {
				return specifier, nil
			}
			return "", err
		}

		dtsModule := ESMPath{
			PkgName:     p.Name,
			PkgVersion:  p.Version,
			SubPath:     subPath,
			SubBareName: subPath,
		}
		args := BuildArgs{
			external: NewStringSet(),
			exports:  NewStringSet(),
		}
		b := NewBuildContext(ctx.zoneId, ctx.npmrc, dtsModule, args, "types", false, BundleFalse, false)
		err = b.install()
		if err != nil {
			return "", err
		}

		dtsPath, err := b.resloveDTS(b.resolveEntry(dtsModule))
		if err != nil {
			return "", err
		}

		if dtsPath != "" {
			return fmt.Sprintf("{ESM_CDN_ORIGIN}%s", dtsPath), nil
		}

		if kind == TsDeclareModule {
			return fmt.Sprintf("{ESM_CDN_ORIGIN}/%s", dtsModule.String()), nil
		}

		return fmt.Sprintf("{ESM_CDN_ORIGIN}%s", b.Path()), nil
	})
	if err != nil {
		return
	}
	dtsFile.Close()

	if withNodeBuiltinModule && !hasReferenceTypesNode {
		ref := []byte(fmt.Sprintf("/// <reference path=\"{ESM_CDN_ORIGIN}/@types/node@%s/index.d.ts\" />\n", nodeTypesVersion))
		buffer = bytes.NewBuffer(concatBytes(ref, buffer.Bytes()))
	}

	err = buildStorage.Put(savePath, bytes.NewBuffer(ctx.rewriteDTS(dts, buffer.Bytes())))
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var errors []error
	for _, s := range internalDts.Values() {
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
