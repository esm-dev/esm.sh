const { spawn } = require('child_process')
const { readFileSync, writeFileSync } = require('fs')
const { resolve } = require('path')

const p = spawn('wasm-pack', ['build', '--target', 'web'], { cwd: __dirname })

p.stdout.on('data', data => process.stdout.write(data))
p.stderr.on('data', data => process.stderr.write(data))
p.on('close', code => {
  if (code === 0) {
    const jsFile = resolve(__dirname, './pkg/esm_compiler.js')
    const jsCode = readFileSync(jsFile, 'utf-8')
    writeFileSync(
      jsFile,
      jsCode.replace(`import * as __wbg_star0 from 'env';`, '')
        .replace(`imports['env'] = __wbg_star0;`, `imports['env'] = { now: () => Date.now() };`)
    )
  }
})
