package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/esbuild-internal/test"
	"github.com/ije/gox/utils"
)

var cjsModuleLexerAppDir string

func parseCJSModuleExports(buildDir string, importPath string) (exports []string, err error) {
	if cjsModuleLexerAppDir == "" {
		cjsModuleLexerAppDir = path.Join(os.TempDir(), "esmd-cjs-module-lexer")
		ensureDir(cjsModuleLexerAppDir)
		cmd := exec.Command("yarn", "add", "cjs-module-lexer", "enhanced-resolve")
		cmd.Dir = cjsModuleLexerAppDir
		var output []byte
		output, err = cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("yarn: %s", string(output))
			return
		}
	}

	start := time.Now()
	buf := bytes.NewBuffer(nil)
	buf.WriteString(fmt.Sprintf(`
		const fs = require('fs')
		const { dirname, join } = require('path')
		const { promisify } = require('util')
		const moduleLexer = require('cjs-module-lexer')
		const enhancedResolve = require('enhanced-resolve')

		const resolve = promisify(enhancedResolve.create({
			mainFields: ['browser', 'module', 'main']
		}))

		// the function 'getExports' is copied from https://github.com/evanw/esbuild/issues/442#issuecomment-739340295
		async function getExports () {
			await moduleLexer.init()

			const exports = []
			const paths = []

			try {
				paths.push(await resolve('%s', '%s')) 
				while (paths.length > 0) {
					const currentPath = paths.pop()
					const code = fs.readFileSync(currentPath).toString()
					const results = moduleLexer.parse(code)
					exports.push(...results.exports)
					for (const reexport of results.reexports) {
						paths.push(await resolve(dirname(currentPath), reexport))
					}
				}
				return exports
			} catch(e) {
				return []
			}
		}

		getExports().then(exports => {
			const saveDir = join('%s', '%s')
			if (!fs.existsSync(saveDir)){
				fs.mkdirSync(saveDir, {recursive: true});
			}
			fs.writeFileSync(join(saveDir, '__exports.json'), JSON.stringify(exports))
			process.exit(0)
		})
	`, buildDir, importPath, buildDir, importPath))

	cmd := exec.Command("node")
	cmd.Stdin = buf
	cmd.Dir = cjsModuleLexerAppDir
	output, e := cmd.CombinedOutput()
	if e != nil {
		err = fmt.Errorf("nodejs: %s", string(output))
		return
	}

	err = utils.ParseJSONFile(path.Join(buildDir, importPath, "__exports.json"), &exports)
	if err != nil {
		return
	}

	log.Debug("run cjs-module-lexer in", time.Now().Sub(start))
	return
}

func parseESModuleExports(nmDir string, filepath string) (exports []string, esm bool, err error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return
	}
	log := logger.NewDeferLog()
	ast, pass := js_parser.Parse(log, test.SourceForTest(string(data)), js_parser.Options{})
	if pass {
		esm = ast.ExportsKind == js_ast.ExportsESM
		if esm {
			for _, i := range ast.ExportStarImportRecords {
				src := ast.ImportRecords[i].Path.Text
				if strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") {
					fp := path.Join(path.Dir(filepath), ensureExt(src, ".js"))
					a, ok, e := parseESModuleExports(nmDir, fp)
					if e != nil {
						err = e
						return
					}
					if ok {
						for _, name := range a {
							if name != "default" {
								exports = append(exports, name)
							}
						}
					}
				} else {
					pkgFile := path.Join(nmDir, src, "package.json")
					if fileExists(pkgFile) {
						var p NpmPackage
						err = utils.ParseJSONFile(pkgFile, &p)
						if err != nil {
							return
						}
						if p.Module != "" {
							fp := path.Join(nmDir, src, ensureExt(p.Module, ".js"))
							a, ok, e := parseESModuleExports(nmDir, fp)
							if e != nil {
								err = e
								return
							}
							if ok {
								for _, name := range a {
									if name != "default" {
										exports = append(exports, name)
									}
								}
							}
						}
					}
				}
			}
			for name := range ast.NamedExports {
				exports = append(exports, name)
			}
		}
	}
	return
}
