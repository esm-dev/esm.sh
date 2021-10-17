package server

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const nodeServicesApp = `
	const fs = require('fs')
	const { dirname, join } = require('path')
	const http = require('http')
	const { promisify } = require('util')
	const { parseCjsExportsSync } = require('cjs-esm-exports')
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

	function isObject(v) {
		return typeof v === 'object' && v !== null && !Array.isArray(v)
	}

	function verifyExports(exports) {
		return Array.from(new Set(exports.filter(name => identRegexp.test(name) && !reservedWords.has(name))))
	}

	async function getExports (buildDir, importPath, nodeEnv = 'production') {
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
				const results = parseCjsExportsSync(currentPath, code, nodeEnv)
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
`

func startNodeServices(pidFile string) (err error) {
	wd := path.Join(os.TempDir(), fmt.Sprintf("esmd-%d-node-services", VERSION))
	ensureDir(wd)

	// install deps
	cmd := exec.Command("yarn", "add", "cjs-esm-expors", "enhanced-resolve")
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("install deps: %s", string(output))
		return
	}

	err = ioutil.WriteFile(path.Join(wd, "index.js"), []byte(nodeServicesApp), 0644)
	if err != nil {
		return
	}

	// kill previous node process if exists
	if data, err := ioutil.ReadFile(pidFile); err == nil {
		if i, err := strconv.Atoi(string(data)); err == nil {
			if p, err := os.FindProcess(i); err == nil {
				p.Kill()
			}
		}
	}

	errBuf := bytes.NewBuffer(nil)
	cmd = exec.Command("node", "index.js")
	cmd.Dir = wd
	cmd.Stderr = errBuf
	cmd.Stdout = os.Stdout

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
