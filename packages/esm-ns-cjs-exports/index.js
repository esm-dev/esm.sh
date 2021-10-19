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

function isObject(v) {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function verifyExports(exports) {
  return Array.from(new Set(exports.filter(name => identRegexp.test(name) && !reservedWords.has(name))))
}

async function getJSONKeys(jsonFile) {
  const content = fs.readFileSync(jsonFile).toString()
  const v = JSON.parse(content)
  if (isObject(v)) {
    return Object.keys(v)
  }
  return []
}

exports.serviceName = 'cjsExports'
exports.main = async input => {
  const { cjsFile, nodeEnv = 'production' } = input
  const exports = []

  const requires = [{ path: cjsFile, callMode: false }]
  while (requires.length > 0) {
    const p = requires.pop()
    const code = fs.readFileSync(p.path).toString()
    const results = parse(p.path, code, nodeEnv, p.callMode)
    exports.push(...results.exports)
    for (let reexport of results.reexports) {
      const callMode = reexport.endsWith('()')
      if (callMode) {
        reexport = reexport.slice(0, -2)
      }
      const path = await resolve(dirname(p.path), reexport)
      if (path.endsWith('.json')) {
        exports.push(...getJSONKeys(path))
      } else {
        requires.push({ path, callMode })
      }
    }
  }

  return {
    exports: verifyExports(exports)
  }
}