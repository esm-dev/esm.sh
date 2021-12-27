const fs = require('fs')
const { dirname } = require('path')
const { promisify } = require('util')
const { parse } = require('cjs-esm-exports')
const enhancedResolve = require('enhanced-resolve')

const identRegexp = /^[a-zA-Z_\$][a-zA-Z0-9_\$]*$/
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
const builtInNodeModules = new Set([
  "assert",
  "async_hooks",
  "child_process",
  "cluster",
  "buffer",
  "console",
  "constants",
  "crypto",
  "dgram",
  "dns",
  "domain",
  "events",
  "fs",
  "fs/promises",
  "http",
  "http2",
  "https",
  "inspector",
  "module",
  "net",
  "os",
  "path",
  "path/posix",
  "path/win32",
  "perf_hooks",
  "process",
  "punycode",
  "querystring",
  "readline",
  "repl",
  "stream",
  "stream/promises",
  "stream/web",
  "_stream_duplex",
  "_stream_passthrough",
  "_stream_readable",
  "_stream_transform",
  "_stream_writable",
  "string_decoder",
  "sys",
  "timers",
  "tls",
  "trace_events",
  "tty",
  "url",
  "util",
  "v8",
  "vm",
  "worker_threads",
  "zlib",
])
const requireModeAllowList = [
  'typescript',
  'he',
  'safe-buffer'
]

function isObject(v) {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function getJSONKeys(jsonFile) {
  const content = fs.readFileSync(jsonFile).toString()
  const v = JSON.parse(content)
  if (isObject(v)) {
    return Object.keys(v)
  }
  return []
}

function verifyExports(exports) {
  return Array.from(new Set(exports.filter(name => identRegexp.test(name) && !reservedWords.has(name))))
}

exports.parseCjsExports = async input => {
  const { buildDir, importPath, nodeEnv = 'production' } = input
  const entry = await resolve(buildDir, importPath)
  const exports = []

  /* workaround for edge cases that can't be parsed by cjsLexer correctly */
  for (const name of requireModeAllowList) {
    if (importPath === name || importPath.startsWith(name + '/')) {
      process.env.NODE_ENV = nodeEnv
      const mod = require(entry)
      if (isObject(mod) || typeof mod === 'function') {
        for (const key of Object.keys(mod)) {
          if (typeof key === 'string' && key !== '') {
            exports.push(key)
          }
        }
      }
      return { exports }
    }
  }

  if (entry.endsWith('.json')) {
    return {
      exports: verifyExports(getJSONKeys(entry))
    }
  }

  if (!entry.endsWith('.js') && !entry.endsWith('.cjs')) {
    return { exports }
  }

  const requires = [{ path: entry, callMode: false }]
  while (requires.length > 0) {
    try {
      const req = requires.pop()
      const code = fs.readFileSync(req.path).toString()
      const results = parse(req.path, code, nodeEnv, req.callMode)
      exports.push(...results.exports)
      for (let reexport of results.reexports) {
        const callMode = reexport.endsWith('()')
        if (callMode) {
          reexport = reexport.slice(0, -2)
        }
        if (builtInNodeModules.has(reexport)) {
          const mod = require(reexport)
          exports.push(...Object.keys(mod))
        } else {
          const path = await resolve(dirname(req.path), reexport)
          if (path.endsWith('.json')) {
            exports.push(...getJSONKeys(path))
          } else {
            requires.push({ path, callMode })
          }
        }
      }
    } catch (err) {
      return Promise.reject(err)
    }
  }

  return {
    exports: verifyExports(exports)
  }
}