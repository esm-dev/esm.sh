// timers.ts
var setTimeout = globalThis.setTimeout;
var clearTimeout = globalThis.clearTimeout;
var setInterval = globalThis.setInterval;
var clearInterval = globalThis.clearInterval;
var setImmediate = (cb, ...args) => globalThis.setTimeout(cb, 0, ...args);
var clearImmediate = globalThis.clearTimeout;
var timers_default = {
  setTimeout,
  clearTimeout,
  setInterval,
  clearInterval,
  setImmediate,
  clearImmediate
};
export {
  clearImmediate,
  clearInterval,
  clearTimeout,
  timers_default as default,
  setImmediate,
  setInterval,
  setTimeout
};
