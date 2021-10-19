const { resolve } = require('path')
const { serviceName, main } = require('./index')

if (serviceName !== 'cjsExports') {
  console.error('unexpected servce name:', serviceName)
  process.exit(1)
}

main({ cjsFile: resolve(__dirname, 'index.js') }).then(ret => {
  if (ret.exports.join(',') !== 'serviceName,main') {
    console.error('unexpected exports of index.js:', ret.exports)
    process.exit(1)
  }
  console.log('done')
})
