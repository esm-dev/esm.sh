package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

var cjsLexerServerPort = uint16(8088)
var cjsModuleLexerVersion = "1.2.2"

type cjsModuleLexerResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string, nodeEnv string) (ret cjsModuleLexerResult, err error) {
	url := fmt.Sprintf("http://0.0.0.0:%d", cjsLexerServerPort)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	req.Header.Add("build-dir", buildDir)
	req.Header.Add("import-path", importPath)
	req.Header.Add("node-env", nodeEnv)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = errors.New(resp.Status)
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&ret)
	return
}

/** use a cjs-module-lexer http server instead of child process */
func startCJSLexerServer(pidFile string, isDev bool) (err error) {
	wd := path.Join(os.TempDir(), fmt.Sprintf("esmd-%d-cjs-module-lexer-%s", VERSION, cjsModuleLexerVersion))
	ensureDir(wd)

	// install cjs-module-lexer
	cmd := exec.Command("yarn", "add", fmt.Sprintf("cjs-module-lexer@%s", cjsModuleLexerVersion), "enhanced-resolve")
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("install cjs-module-lexer: %s", string(output))
		return
	}

	errBuf := bytes.NewBuffer(nil)
	jsBuf := bytes.NewBufferString(fmt.Sprintf(`
		const fs = require('fs')
		const { dirname, join } = require('path')
		const http = require('http')
		const { promisify } = require('util')
		const cjsLexer = require('cjs-module-lexer')
		const enhancedResolve = require('enhanced-resolve')

		const identRegexp = /^[a-zA-Z_\$][a-zA-Z0-9_\$]+$/
		const resolve = promisify(enhancedResolve.create({
			mainFields: ['browser', 'module', 'main']
		}))
		const reservedWords = new Set([
			'abstract', 'arguments', 'await', 'boolean',
			'break', 'byte', 'case', 'catch',
			'char', 'class', 'const', 'continue',
			'debugger', 'default', 'delete', 'do',
			'double', 'else', 'enum', 'eval',
			'export', 'extends', 'false', 'final',
			'finally', 'float', 'for', 'function',
			'goto', 'if', 'implements', 'import',
			'in', 'instanceof', 'int', 'interface',
			'let', 'long', 'native', 'new',
			'null', 'package', 'private', 'protected',
			'public', 'return', 'short', 'static',
			'super', 'switch', 'synchronized', 'this',
			'throw', 'throws', 'transient', 'true',
			'try', 'typeof', 'var', 'void',
			'volatile', 'while', 'with', 'yield',
			'__esModule'
		])

		let cjsLexerReady = false

		function isObject(v) {
			return typeof v === 'object' && v !== null && !Array.isArray(v)
		}

		function verifyExports(exports) {
			return Array.from(new Set(exports.filter(name => identRegexp.test(name) && !reservedWords.has(name))))
		}

		async function getExports (buildDir, importPath, nodeEnv = 'production') {
			process.env.NODE_ENV = nodeEnv

			if (!cjsLexerReady) {
				await cjsLexer.init()
				cjsLexerReady = true
			}

			const entry = await resolve(buildDir, importPath)
			const exports = []

			/* handle entry ends with '.json' */
			if (entry.endsWith('.json')) {
				try {
					const content = fs.readFileSync(entry).toString()
					const mod = JSON.parse(content)
					if (isObject(mod)) {
						exports.push(...Object.keys(mod))
					}
					return { 
						exports: verifyExports(exports) 
					}
				} catch(e) {
					return { error: e.message }
				}
			}

			/* the below code was stolen from https://github.com/evanw/esbuild/issues/442#issuecomment-739340295 */
			try {
				const paths = []
				paths.push(entry)
				while (paths.length > 0) {
					const currentPath = paths.pop()
					const code = fs.readFileSync(currentPath).toString()
					const results = cjsLexer.parse(code)
					exports.push(...results.exports)
					for (const reexport of results.reexports) {
						if (!reexport.endsWith('.json')) {
							paths.push(await resolve(dirname(currentPath), reexport))
						}
					}
				}
			} catch(e) {
				return { error: e.message }
			}

			/* the workaround when the cjsLexer didn't get any exports */
			if (exports.length === 0) {
				try {
					const entry = await resolve(buildDir, importPath)
					const mod = require(entry) 
					if (isObject(mod) || typeof mod === 'function') {
						for (const key of Object.keys(mod)) {
							if (typeof key === 'string' && key !== '') {
								exports.push(key)
							}
						}
					}
				} catch(e) {
					return { error: e.message }
				}
			}

			return { 
				exports: verifyExports(exports)
			}
		}

		const server = http.createServer(async function (req, resp) {
			const buildDir = req.headers['build-dir']
			const importPath = req.headers['import-path']
			const nodeEnv = req.headers['node-env']
			if (!buildDir || !importPath) {
				resp.write('Bad request')
				resp.end()
				return
			}
			try {
				const ret = await getExports(buildDir, importPath, nodeEnv)
				resp.write(JSON.stringify(ret))
			} catch(e) {
				resp.write(JSON.stringify({ error: e.message }))
			}
			resp.end()
		})

		server.on('error', (e) => {
			if (e.code === 'EADDRINUSE') {
				console.error('EADDRINUSE')
				process.exit(1)
			}
		})

		server.listen(%d, () => {
			if (process.env.NODE_ENV === 'development') {
				console.log('[debug] cjs lexer server ready on http://localhost:%d')
			}
		})
	`, cjsLexerServerPort, cjsLexerServerPort))

	// kill previous node process if exists
	if data, err := ioutil.ReadFile(pidFile); err == nil {
		if i, err := strconv.Atoi(string(data)); err == nil {
			if p, err := os.FindProcess(i); err == nil {
				p.Kill()
			}
		}
	}

	cmd = exec.Command("node")
	cmd.Stdin = jsBuf
	cmd.Dir = wd
	cmd.Stderr = errBuf
	env := "production"
	if isDev {
		env = "development"
		cmd.Stdout = os.Stdout
	}
	cmd.Env = append(os.Environ(), fmt.Sprintf(`NODE_ENV=%s`, env))

	err = cmd.Start()
	if err != nil {
		return
	}

	// store node process pid
	ioutil.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	// wait the process to exit
	cmd.Wait()

	if errBuf.Len() > 0 {
		err = errors.New(strings.TrimSpace(errBuf.String()))
	}
	return
}
