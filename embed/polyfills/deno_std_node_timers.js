/* deno mod bundle
 * entry: deno.land/std/node/timers.ts
 * version: 0.106.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild timers.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_timers.js
 */

// timers.ts
var setTimeout = globalThis.setTimeout
var clearTimeout = globalThis.clearTimeout
var setInterval = globalThis.setInterval
var clearInterval = globalThis.clearInterval
var setImmediate = (cb, ...args) => globalThis.setTimeout(cb, 0, ...args)
var clearImmediate = globalThis.clearTimeout
var timers_default = {
  setTimeout,
  clearTimeout,
  setInterval,
  clearInterval,
  setImmediate,
  clearImmediate
}
export { clearImmediate, clearInterval, clearTimeout, timers_default as default, setImmediate, setInterval, setTimeout }
