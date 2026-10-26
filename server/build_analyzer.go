package server

import (
	"bufio"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/set"
)

type Ref struct {
	entries   *set.Set[string]
	importers *set.Set[string]
}

func (ctx *BuildContext) analyzeSplitting() {
	exportNames := set.New[string]()

	for _, exportName := range ctx.pkgJson.Exports.Keys() {
		exportName := stripEntryModuleExt(exportName)
		if (exportName == "." || (strings.HasPrefix(exportName, "./") && !strings.ContainsRune(exportName, '*'))) && !endsWith(exportName, ".json", ".css", ".wasm", ".d.ts", ".d.mts", ".d.cts") {
			v, _ := ctx.pkgJson.Exports.Get(exportName)
			if s, ok := v.(string); ok {
				if endsWith(s, ".json", ".css", ".wasm", ".d.ts", ".d.mts", ".d.cts") {
					continue
				}
			} else if obj, ok := v.(npm.JSONObject); ok {
				// ignore types only exports
				if keys := obj.Keys(); len(keys) == 1 && keys[0] == "types" {
					continue
				}
			}
			if exportName == "." {
				exportNames.Add("")
			} else if strings.HasPrefix(exportName, "./") {
				exportNames.Add(exportName[2:])
			}
		}
	}

	if exportNames.Len() > 1 {
		name := "splitting-v2-" + ctx.target
		if ctx.dev {
			name += "-dev"
		}
		if ctx.externalAll {
			name += "-x"
		}
		if a := encodeBuildArgs(ctx.args, false); a != "" {
			name += "-" + strconv.FormatUint(xxhash.Sum64([]byte(a)), 36)
		}
		splittingTxtPath := path.Join(ctx.wd, name+".txt")
		readSplittingTxt := func() bool {
			f, err := os.Open(splittingTxtPath)
			if err != nil {
				return false
			}
			defer f.Close()

			r := bufio.NewScanner(f)
			if !r.Scan() {
				return false
			}
			n, err := strconv.Atoi(strings.TrimSpace(r.Text()))
			if err != nil || n < 0 {
				return false
			}
			var modules []string
			for r.Scan() {
				if module := strings.TrimSpace(r.Text()); module != "" {
					if len(modules) == n {
						return false
					}
					modules = append(modules, module)
				}
			}
			if r.Err() != nil || len(modules) != n {
				return false
			}
			ctx.splitting = set.NewReadOnly(modules...)
			if DEBUG {
				ctx.logger.Debugf("build(%s): splitting.txt found with %d shared modules", ctx.esmPath.String(), ctx.splitting.Len())
			}
			return true
		}

		// check if the splitting has been analyzed
		if readSplittingTxt() {
			return
		}

		// only one analyze process is allowed at the same time for the same package
		ctx.waitLock(splittingTxtPath)
		unlock, err := lockInstall(ctx.Context(), splittingTxtPath)
		if err != nil {
			return
		}
		defer unlock()

		// skip analyze if the package has been analyzed by another request
		if readSplittingTxt() {
			return
		}

		partial := false
		defer func() {
			// [warning] a canceled or failed analysis is partial, caching it would duplicate shared modules
			if partial || ctx.checkCanceled() != nil {
				return
			}
			splitting := []string{}
			if ctx.splitting != nil {
				splitting = ctx.splitting.Values()
			}
			// write the splitting result to 'splitting.txt'
			sizeStr := strconv.FormatUint(uint64(len(splitting)), 10)
			bufSize := len(sizeStr) + 1
			for _, s := range splitting {
				bufSize += len(s) + 1
			}
			buf := make([]byte, bufSize)
			i := copy(buf, sizeStr)
			buf[i] = '\n'
			i++
			for _, s := range splitting {
				i += copy(buf[i:], s)
				buf[i] = '\n'
				i++
			}
			// readers don't take the lock, so write a temp file and rename it
			if os.WriteFile(splittingTxtPath+".tmp", buf[0:bufSize-1], 0644) == nil {
				os.Rename(splittingTxtPath+".tmp", splittingTxtPath)
			}
		}()

		refs := map[string]Ref{}
		for _, exportName := range exportNames.Values() {
			if ctx.yield() != nil {
				return
			}
			esmPath := ctx.esmPath
			esmPath.SubPath = exportName
			b := &BuildContext{
				npmrc:       ctx.npmrc,
				logger:      ctx.logger,
				metaDB:      ctx.metaDB,
				storage:     ctx.storage,
				esmPath:     esmPath,
				args:        ctx.args,
				externalAll: ctx.externalAll,
				target:      ctx.target,
				dev:         ctx.dev,
				wd:          ctx.wd,
				pkgJson:     ctx.pkgJson,
				ctx:         ctx.ctx,
			}
			_, includes, err := b.buildModule(true)
			if ctx.checkCanceled() != nil {
				return
			}
			if err != nil {
				// splitting is an optimization, so ignore an export that can't be analyzed,
				// like an asset esbuild has no loader for
				if err.Error() != "could not resolve build entry" {
					ctx.logger.Warnf("build(%s): failed to analyze %s: %v", ctx.esmPath.String(), esmPath.String(), err)
					// [warning] a partial analysis is not cached, so each build of the package would redo it,
					// the export build fails with a code error too, so the analysis is still complete
					if _, ok := err.(codeError); !ok {
						partial = true
					}
				}
				continue
			}
			for _, include := range includes {
				module, importer := include[0], include[1]
				ref, ok := refs[module]
				if !ok {
					ref = Ref{entries: set.New[string](), importers: set.New[string]()}
					refs[module] = ref
				}
				ref.importers.Add(importer)
				ref.entries.Add(exportName)
			}
		}
		shared := set.New[string]()
		for mod, ref := range refs {
			if ref.entries.Len() > 1 && ref.importers.Len() > 1 {
				shared.Add(mod)
			}
		}
		var bubble func(modulePath string, f func(string), mark *set.Set[string])
		bubble = func(modulePath string, f func(string), mark *set.Set[string]) {
			hasMark := mark != nil
			if !hasMark {
				mark = set.New[string]()
			} else if mark.Has(modulePath) {
				return
			}
			mark.Add(modulePath)
			ref, ok := refs[modulePath]
			if ok {
				if shared.Has(modulePath) && hasMark {
					f(modulePath)
					return
				}
				for _, importer := range ref.importers.Values() {
					bubble(importer, f, mark)
				}
			} else {
				// modulePath is an entry module
				f(modulePath)
			}
		}
		if shared.Len() > 0 {
			splitting := set.New[string]()
			for _, modulePath := range shared.Values() {
				if ctx.checkCanceled() != nil {
					return
				}
				refBy := set.New[string]()
				bubble(modulePath, func(importer string) { refBy.Add(importer) }, nil)
				if refBy.Len() > 1 {
					splitting.Add(modulePath)
				}
			}
			ctx.splitting = splitting.ReadOnly()
			if DEBUG {
				ctx.logger.Debugf("build(%s): found %d shared modules from %d modules", ctx.esmPath.String(), shared.Len(), len(refs))
			}
		}
	}
}
