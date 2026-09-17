package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/utils"
)

type dtsFile struct {
	ready   chan struct{}
	statErr error
	visited bool
	done    chan struct{}
	err     error
}

type dtsTransform struct {
	ctx     context.Context
	storage storage.Storage
	files   map[string]*dtsFile
	slots   chan struct{}
	pending sync.WaitGroup
}

// stat prefetches storage metadata while the caller walks the declaration graph.
func (t *dtsTransform) stat(savePath string) *dtsFile {
	if file, ok := t.files[savePath]; ok {
		return file
	}
	file := &dtsFile{ready: make(chan struct{})}
	t.files[savePath] = file
	t.pending.Go(func() {
		defer close(file.ready)
		select {
		case t.slots <- struct{}{}:
			defer func() { <-t.slots }()
		case <-t.ctx.Done():
			file.statErr = t.ctx.Err()
			return
		}
		if file.statErr = t.ctx.Err(); file.statErr != nil {
			return
		}
		if s, ok := t.storage.(interface {
			StatContext(context.Context, string) (storage.Stat, error)
		}); ok {
			_, file.statErr = s.StatContext(t.ctx, savePath)
		} else {
			_, file.statErr = t.storage.Stat(savePath)
		}
	})
	return file
}

func (ctx *BuildContext) transformDTS(dts string) error {
	start := time.Now()
	n, err := transformDTS(ctx, dts, ctx.getBuildArgsPrefix(true), nil)
	if err != nil {
		return err
	}
	if DEBUG {
		ctx.logger.Debugf("transform dts '%s'(%d related dts files) in %v", dts, n, time.Since(start))
	}
	return nil
}

// transformDTS transforms a `.d.ts` file for deno/editor-lsp
func transformDTS(ctx *BuildContext, dts string, buildArgsPrefix string, transform *dtsTransform) (n int, err error) {
	entry := transform == nil
	if entry {
		buildCtx, cancel := context.WithCancel(ctx.Context())
		defer cancel()
		transform = &dtsTransform{ctx: buildCtx, storage: ctx.storage, files: map[string]*dtsFile{}, slots: make(chan struct{}, 16)}
		defer func() {
			if err != nil {
				cancel()
			}
			transform.pending.Wait()
		}()
	}
	if err = transform.ctx.Err(); err != nil {
		return
	}

	savePath := normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), buildArgsPrefix, dts))
	file := transform.stat(savePath)
	if file.visited {
		return
	}
	file.visited = true
	select {
	case <-file.ready:
		err = file.statErr
	case <-transform.ctx.Done():
		err = transform.ctx.Err()
	}
	if canceled := transform.ctx.Err(); canceled != nil {
		err = canceled
	}
	if err == nil || err != storage.ErrNotFound {
		return
	}

	dtsFilename := path.Join(ctx.wd, "node_modules", ctx.esmPath.PkgName, dts)
	dtsContent, err := os.Open(dtsFilename)
	if err != nil {
		// if the dts file does not exist, print a warning but continue to build
		if os.IsNotExist(err) {
			if entry {
				err = fmt.Errorf("types not found")
			} else {
				err = nil
			}
		}
		return
	}
	buffer := &bytes.Buffer{}

	deps := map[string]*dtsFile{}
	type resolutionKey struct {
		specifier string
		kind      TsImportKind
	}
	type resolutionValue struct {
		specifier string
		err       error
	}
	resolutions := map[resolutionKey]resolutionValue{}

	err = parseDts(dtsContent, buffer, func(specifier string, kind TsImportKind, position int) (resolvedSpecifier string, resolveErr error) {
		if err := transform.ctx.Err(); err != nil {
			return "", err
		}
		key := resolutionKey{specifier, kind}
		if cached, ok := resolutions[key]; ok {
			return cached.specifier, cached.err
		}
		defer func() {
			resolutions[key] = resolutionValue{resolvedSpecifier, resolveErr}
		}()

		if ctx.esmPath.PkgName == "@types/node" {
			if strings.HasPrefix(specifier, "node:") || nodeBuiltinModules[specifier] || isRelPathSpecifier(specifier) {
				return specifier, nil
			}
		}
		if isHttpSpecifier(specifier) {
			return specifier, nil
		}

		// normalize specifier
		rawSpecifier := specifier
		specifier = normalizeImportSpecifier(specifier)

		if isRelPathSpecifier(specifier) {
			dtsDir := path.Dir(dtsFilename)
			specifier = strings.TrimSuffix(specifier, ".d")
			if !endsWith(specifier, ".d.ts", ".d.mts", ".d.cts") {
				var p npm.PackageJSONRaw
				var isSubmodule bool
				if utils.ParseJSONFile(path.Join(dtsDir, specifier, "package.json"), &p) == nil {
					dir := path.Join("/", path.Dir(dts))
					if types := p.Types.String(); types != "" {
						specifier, _ = relPath(dir, "/"+path.Join(dir, specifier, types))
						isSubmodule = true
					} else if typings := p.Typings.String(); typings != "" {
						specifier, _ = relPath(dir, "/"+path.Join(dir, specifier, typings))
						isSubmodule = true
					}
				}
				if !isSubmodule {
					if existsFile(path.Join(dtsDir, specifier+".d.mts")) {
						specifier = specifier + ".d.mts"
					} else if existsFile(path.Join(dtsDir, specifier+".d.ts")) {
						specifier = specifier + ".d.ts"
					} else if existsFile(path.Join(dtsDir, specifier+".d.cts")) {
						specifier = specifier + ".d.cts"
					} else if endsWith(specifier, ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts") {
						specifier = stripModuleExt(specifier)
						if existsFile(path.Join(dtsDir, specifier+".d.mts")) {
							specifier = specifier + ".d.mts"
						} else if existsFile(path.Join(dtsDir, specifier+".d.ts")) {
							specifier = specifier + ".d.ts"
						} else if existsFile(path.Join(dtsDir, specifier+".d.cts")) {
							specifier = specifier + ".d.cts"
						}
					} else if existsFile(path.Join(dtsDir, specifier, "index.d.mts")) {
						specifier = strings.TrimSuffix(specifier, "/") + "/index.d.mts"
					} else if existsFile(path.Join(dtsDir, specifier, "index.d.ts")) {
						specifier = strings.TrimSuffix(specifier, "/") + "/index.d.ts"
					} else if existsFile(path.Join(dtsDir, specifier, "index.d.cts")) {
						specifier = strings.TrimSuffix(specifier, "/") + "/index.d.cts"
					}
				}
			}

			if endsWith(specifier, ".d.ts", ".d.mts", ".d.cts") {
				deps[specifier] = transform.stat(normalizeSavePath(path.Join("types", "/"+ctx.esmPath.PackageId(), buildArgsPrefix, path.Dir(dts), specifier)))
			} else {
				specifier += ".d.ts"
			}
			return specifier, nil
		}

		if kind == TsReferenceTypes && specifier == "node" {
			// return empty string to ignore the reference types 'node'
			return "", nil
		}

		if specifier == "node" || isNodeBuiltinSpecifier(specifier) {
			return specifier, nil
		}

		depPkgName, depPkgVersion, subPath := splitEsmPath(specifier)
		specifier = depPkgName
		if depPkgVersion != "" {
			specifier += "@" + depPkgVersion
		}
		if len(subPath) > 0 {
			specifier += "/" + subPath
		}

		if depPkgName == ctx.esmPath.PkgName && (depPkgVersion == "" || depPkgVersion == ctx.esmPath.PkgVersion) {
			if strings.ContainsRune(subPath, '*') {
				return fmt.Sprintf(
					"{ESM_CDN_ORIGIN}/%s/%s%s",
					ctx.esmPath.PackageId(),
					ctx.getBuildArgsPrefix(true),
					subPath,
				), nil
			} else {
				entry := ctx.resolveEntry(EsmPath{
					PkgName:    depPkgName,
					PkgVersion: ctx.esmPath.PkgVersion,
					SubPath:    stripEntryModuleExt(subPath),
				})
				if entry.types != "" {
					return fmt.Sprintf(
						"{ESM_CDN_ORIGIN}/%s/%s%s",
						ctx.esmPath.PackageId(),
						ctx.getBuildArgsPrefix(true),
						strings.TrimPrefix(entry.types, "./"),
					), nil
				}
			}
			// virtual module
			return "https://esm.sh/" + specifier, nil
		}

		// respect `?alias` query
		alias, ok := ctx.args.Alias[depPkgName]
		aliased := ok
		if ok {
			aliasPkgName, aliasPkgVersion, aliasSubPath := splitEsmPath(alias)
			depPkgName = aliasPkgName
			depPkgVersion = aliasPkgVersion
			if len(aliasSubPath) > 0 {
				if len(subPath) > 0 {
					subPath = aliasSubPath + "/" + subPath
				} else {
					subPath = aliasSubPath
				}
			}
			specifier = depPkgName
			if depPkgVersion != "" {
				specifier += "@" + depPkgVersion
			}
			if len(subPath) > 0 {
				specifier += "/" + subPath
			}
		}

		// respect `?external` query
		if ctx.externalAll || ctx.args.External.Has(depPkgName) || isPackageInExternalNamespace(depPkgName, ctx.args.External) {
			if !aliased && strings.HasPrefix(rawSpecifier, "npm:") {
				return rawSpecifier, nil
			}
			return specifier, nil
		}

		typesPkgName := npm.ToTypesPackageName(depPkgName)
		if _, ok := ctx.pkgJson.Dependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
			depPkgVersion = ""
		} else if _, ok := ctx.pkgJson.PeerDependencies[typesPkgName]; ok {
			depPkgName = typesPkgName
			depPkgVersion = ""
		}

		var p *npm.PackageJSON
		var err error
		if depPkgVersion != "" {
			p, err = ctx.npmrc.getPackageInfoContext(ctx.Context(), depPkgName, depPkgVersion)
		} else {
			_, p, err = ctx.resolveDependency(depPkgName, true)
		}
		if err != nil {
			if kind == TsDeclareModule && strings.HasSuffix(err.Error(), " not found") {
				return specifier, nil
			}
			return "", err
		}

		dtsModule := EsmPath{
			PkgName:    p.Name,
			PkgVersion: p.Version,
			SubPath:    stripEntryModuleExt(subPath),
		}
		args := BuildArgs{
			Alias:      ctx.args.Alias,
			Deps:       ctx.args.Deps,
			External:   ctx.args.External,
			Conditions: ctx.args.Conditions,
		}
		b := &BuildContext{
			npmrc:   ctx.npmrc,
			logger:  ctx.logger,
			esmPath: dtsModule,
			args:    args,
			target:  "types",
			ctx:     ctx.ctx,
		}
		err = b.install()
		if err != nil {
			return "", err
		}
		err = resolveBuildArgs(ctx.npmrc, b.wd, &b.args, dtsModule)
		if err != nil {
			return "", err
		}

		dtsPath, err := b.resolveDTS(b.resolveEntry(dtsModule))
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
	dtsContent.Close()
	if err != nil {
		return
	}

	var dependencies []*dtsFile
	for s, dependency := range deps {
		var j int
		j, err = transformDTS(ctx, "./"+path.Join(path.Dir(dts), s), buildArgsPrefix, transform)
		if err != nil {
			return
		}
		n += j
		// Ancestors in a cycle have no upload yet, matching the depth-first walk.
		if dependency.done != nil {
			dependencies = append(dependencies, dependency)
		}
	}

	file.done = make(chan struct{})
	transform.pending.Go(func() {
		defer close(file.done)
		for _, dependency := range dependencies {
			select {
			case <-dependency.done:
				if dependency.err != nil {
					file.err = dependency.err
					return
				}
			case <-transform.ctx.Done():
				file.err = transform.ctx.Err()
				return
			}
		}
		select {
		case transform.slots <- struct{}{}:
			defer func() { <-transform.slots }()
		case <-transform.ctx.Done():
			file.err = transform.ctx.Err()
			return
		}
		if file.err = transform.ctx.Err(); file.err != nil {
			return
		}
		content := ctx.rewriteDTS(dts, buffer)
		if s, ok := ctx.storage.(interface {
			PutContext(context.Context, string, io.Reader) error
		}); ok {
			file.err = s.PutContext(transform.ctx, savePath, content)
		} else {
			file.err = ctx.storage.Put(savePath, content)
		}
		if file.err == nil {
			file.err = transform.ctx.Err()
		}
	})
	if entry {
		select {
		case <-file.done:
			err = file.err
		case <-transform.ctx.Done():
			err = transform.ctx.Err()
		}
	} else {
		n++
	}
	return
}
