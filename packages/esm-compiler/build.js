const { spawn } = require('child_process')
const { readFileSync, writeFileSync } = require('fs')
const { resolve } = require('path')

const ls = spawn('wasm-pack', ['build', '--target', 'web'], { cwd: __dirname })

ls.stdout.on('data', data => process.stdout.write(data))
ls.stderr.on('data', data => process.stderr.write(data))
ls.on('close', code => {
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
