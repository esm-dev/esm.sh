const ns = require('.')

ns.parseCjsExports({ buildDir: __dirname, importPath: './index.js' }).then(ret => {
  const { exports } = ret
  if (exports.join(',') !== 'parseCjsExports') {
    console.error('unexpected exports of index.js:', exports)
    process.exit(1)
  }
  console.log('Done')
})
