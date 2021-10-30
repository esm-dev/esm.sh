const { serviceName, main } = require('./index')

if (serviceName !== 'cjsExports') {
  console.error('unexpected servce name:', serviceName)
  process.exit(1)
}

main({ buildDir: __dirname, importPath: '.' }).then(ret => {
  if (ret.exports.join(',') !== 'serviceName,main') {
    console.error('unexpected exports of index.js:', ret.exports)
    process.exit(1)
  }
  console.log('Done')
})
