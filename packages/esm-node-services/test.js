const ns = require('.')

ns.parseCjsExports({ buildDir: __dirname, pkgName: "tapable", importPath: './lib/index.js' }).then(ret => {
  const { exports } = ret
  if (exports.join(',') !== [
    '__esModule',
    'SyncHook',
    'SyncBailHook',
    'SyncWaterfallHook',
    'SyncLoopHook',
    'AsyncParallelHook',
    'AsyncParallelBailHook',
    'AsyncSeriesHook',
    'AsyncSeriesBailHook',
    'AsyncSeriesLoopHook',
    'AsyncSeriesWaterfallHook',
    'HookMap',
    'MultiHook',
  ].join(',')) {
    console.error('unexpected exports:', exports)
    process.exit(1)
  }
  console.log('Done')
})
