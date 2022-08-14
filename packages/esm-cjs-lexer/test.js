const fs = require('fs')
const { parse } = require('./pkg/esm_cjs_lexer')

exports.foo = true
module.exports.bar = true

const code = fs.readFileSync("./test.js", "utf-8")
const results = parse("./test.js", code, "developments", false)

if (results.exports.join(',') !== 'foo,bar') {
  console.error('unexpected exports of index.js:', exports)
  process.exit(1)
}
console.log("done")
