package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
)

const cjsModuleLexerVersion = "1.2.2"

type cjsModuleLexerResult struct {
	Exports []string `json:"exports"`
	Error   string   `json:"error"`
}

func parseCJSModuleExports(buildDir string, importPath string) (ret cjsModuleLexerResult, err error) {
	url := fmt.Sprintf("http://0.0.0.0:%d", config.cjsLexerServerPort)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	req.Header.Add("build-dir", buildDir)
	req.Header.Add("import-path", importPath)
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

/* use a cjs-module-lexer http server instead of child process */
func startCJSLexerServer(port uint16, isDev bool) (err error) {
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

	const resolve = promisify(enhancedResolve.create({
		mainFields: ['main']
	}))
	const identRegexp = /^[a-zA-Z_\$][a-zA-Z0-9_\$]+$/
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

	async function getExports (buildDir, importPath) {
		if (!cjsLexerReady) {
			await cjsLexer.init()
			cjsLexerReady = true
		}

		const exports = []
		const paths = []

		try {
			// the below code was stolen from https://github.com/evanw/esbuild/issues/442#issuecomment-739340295
			const jsFile = await resolve(buildDir, importPath)
			if (!jsFile.endsWith('.json')) {
				paths.push(jsFile) 
			}
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

		try {
			if (!jsFile.endsWith('.json')) {
				const mod = require(jsFile)
				if (typeof mod === 'object' && mod !== null && !Array.isArray(mod)) {
					for (const key of Object.keys(mod)) {
						if (typeof key === 'string' && key !== '' && !exports.includes(key)) {
						exports.push(key)
						}
					}
				}
			}
		} catch(e) {}
		
		return { 
			exports: Array.from(new Set(exports.filter(name => identRegexp.test(name) && !reservedWords.has(name))))
		}
	}

	const server = http.createServer(function (req, resp) {
		const buildDir = req.headers['build-dir']
		const importPath = req.headers['import-path']
		if (!buildDir || !importPath) {
			resp.write('Bad request')
			resp.end()
			return
		}
		getExports(buildDir, importPath).then(ret => {
			resp.write(JSON.stringify(ret))
			resp.end()
		})
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
`, port, port))

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

	cmd.Wait()

	if errBuf.Len() > 0 {
		err = errors.New(strings.TrimSpace(errBuf.String()))
	}
	return
}
