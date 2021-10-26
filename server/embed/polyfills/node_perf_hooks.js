// https://nodejs.org/api/perf_hooks.html

function notImplemented(name) {
  throw new Error(`[esm.sh] pref_hooks: '${name}' is not implemented`)
}

export function createHistogram() {
  notImplemented('createHistogram')
}

export function monitorEventLoopDelay() {
  notImplemented('monitorEventLoopDelay')
}

export default {
  createHistogram,
  monitorEventLoopDelay
}
