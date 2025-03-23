package server

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/ije/gox/set"
)

type Ref struct {
	entries   *set.Set[string]
	importers *set.Set[string]
}

func (ctx *BuildContext) analyzeSplitting() (err error) {
	exportNames := set.New[string]()

	for _, exportName := range ctx.pkgJson.Exports.keys {
		exportName := stripEntryModuleExt(exportName)
		if (exportName == "." || (strings.HasPrefix(exportName, "./") && !strings.ContainsRune(exportName, '*'))) && !endsWith(exportName, ".json", ".css", ".wasm", ".d.ts", ".d.mts", ".d.cts") {
			v := ctx.pkgJson.Exports.values[exportName]
			if s, ok := v.(string); ok {
				if endsWith(s, ".json", ".css", ".wasm", ".d.ts", ".d.mts", ".d.cts") {
					continue
				}
				if len(a) > 0 {
					n, e := strconv.Atoi(a[0])
					if e == nil && n <= len(a)-1 {
						ctx.splitting = set.NewReadOnly(a[1 : n+1]...)
						if DEBUG {
							ctx.logger.Debugf("build(%s): splitting.txt found with %d shared modules", ctx.esmPath.Specifier(), ctx.splitting.Len())
						}
						return true
					}
				}
				return false
			}

			// check if the splitting has been analyzed
			if readSplittingTxt() {
				return
			}

			// only one analyze process is allowed at the same time for the same package
			unlock := installMutex.Lock(splittingTxtPath)
			defer unlock()

			// skip analyze if the package has been analyzed by another request
			if readSplittingTxt() {
				return
			}

			defer func() {
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
				os.WriteFile(splittingTxtPath, buf[0:bufSize-1], 0644)
			}()

			refs := map[string]Ref{}
			for _, exportName := range exportNames.Values() {
				esm := ctx.esmPath
				esm.SubPath = exportName
				esm.SubModuleName = stripEntryModuleExt(exportName)
				b := &BuildContext{
					npmrc:       ctx.npmrc,
					logger:      ctx.logger,
					db:          ctx.db,
					storage:     ctx.storage,
					esmPath:     esm,
					args:        ctx.args,
					externalAll: ctx.externalAll,
					target:      ctx.target,
					dev:         ctx.dev,
					wd:          ctx.wd,
					pkgJson:     ctx.pkgJson,
				}
				_, includes, err := b.buildModule(true)
				if err != nil {
					return fmt.Errorf("failed to analyze %s: %v", esm.Specifier(), err)
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
					refBy := set.New[string]()
					bubble(modulePath, func(importer string) { refBy.Add(importer) }, nil)
					if refBy.Len() > 1 {
						splitting.Add(modulePath)
					}
				}
				ctx.splitting = splitting.ReadOnly()
				if DEBUG {
					ctx.logger.Debugf("build(%s): found %d shared modules from %d modules", ctx.esmPath.Specifier(), shared.Len(), len(refs))
				}
			}
		}
	}

	if exportNames.Len() > 1 {
		splittingTxtPath := path.Join(ctx.wd, "splitting.txt")
		readSplittingTxt := func() bool {
			f, err := os.Open(splittingTxtPath)
			if err != nil {
				return false
			}
			defer f.Close()

			var a []string
			var i int
			var r = bufio.NewReader(f)
			for {
				line, readErr := r.ReadString('\n')
				if readErr == nil || readErr == io.EOF {
					line = strings.TrimSpace(line)
					if line != "" {
						if a == nil {
							n, e := strconv.Atoi(line)
							if e != nil {
								break
							}
							a = make([]string, n+1)
						}
						a[i] = line
						i++
					}
				}
				if readErr != nil {
					break
				}
			}
			if len(a) > 0 {
				n, e := strconv.Atoi(a[0])
				if e == nil && n <= len(a)-1 {
					ctx.splitting = set.NewReadOnly(a[1 : n+1]...)
					if DEBUG {
						ctx.logger.Debugf("build(%s): splitting.txt found with %d shared modules", ctx.esm.Specifier(), ctx.splitting.Len())
					}
					return true
				}
			}
			return false
		}

		// check if the splitting has been analyzed
		if readSplittingTxt() {
			return
		}

		// only one analyze process is allowed at the same time for the same package
		unlock := installMutex.Lock(splittingTxtPath)
		defer unlock()

		// skip analyze if the package has been analyzed by another request
		if readSplittingTxt() {
			return
		}

		defer func() {
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
			os.WriteFile(splittingTxtPath, buf[0:bufSize-1], 0644)
		}()

		refs := map[string]Ref{}
		for _, exportName := range exportNames.Values() {
			esm := ctx.esm
			esm.SubPath = exportName
			esm.SubModuleName = stripEntryModuleExt(exportName)
			b := &BuildContext{
				npmrc:       ctx.npmrc,
				logger:      ctx.logger,
				db:          ctx.db,
				storage:     ctx.storage,
				esm:         esm,
				args:        ctx.args,
				externalAll: ctx.externalAll,
				target:      ctx.target,
				dev:         ctx.dev,
				wd:          ctx.wd,
				pkgJson:     ctx.pkgJson,
			}
			_, includes, err := b.buildModule(true)
			if err != nil {
				return fmt.Errorf("failed to analyze %s: %v", esm.Specifier(), err)
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
				refBy := set.New[string]()
				bubble(modulePath, func(importer string) { refBy.Add(importer) }, nil)
				if refBy.Len() > 1 {
					splitting.Add(modulePath)
				}
			}
			ctx.splitting = splitting.ReadOnly()
			if DEBUG {
				ctx.logger.Debugf("build(%s): found %d shared modules from %d modules", ctx.esm.Specifier(), shared.Len(), len(refs))
			}
		}
	}

	return
}
