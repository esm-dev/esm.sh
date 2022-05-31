/*
  (The MIT License)

  Copyright (c) 2013 Roman Shtylman <shtylman@gmail.com>

  Permission is hereby granted, free of charge, to any person obtaining
  a copy of this software and associated documentation files (the
  'Software'), to deal in the Software without restriction, including
  without limitation the rights to use, copy, modify, merge, publish,
  distribute, sublicense, and/or sell copies of the Software, and to
  permit persons to whom the Software is furnished to do so, subject to
  the following conditions:

  The above copyright notice and this permission notice shall be
  included in all copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
  EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
  MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
  IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
  CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
  TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
  SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

// cached from whatever global is present so that test runners that stub it
// don't break things. But we need to wrap it in a try catch in case it is
// wrapped in strict mode code which doesn't define any globals. It's inside a
// function because try/catches deoptimize in certain engines.

import { EventEmitter } from "./node_events.js"
const events = new EventEmitter()
events.setMaxListeners(1 << 10) // 1024

let cachedSetTimeout;
let cachedClearTimeout;

function defaultSetTimeout() {
  throw new Error('setTimeout has not been defined');
}

function defaultClearTimeout() {
  throw new Error('clearTimeout has not been defined');
}

(function () {
  try {
    if (typeof setTimeout === 'function') {
      cachedSetTimeout = setTimeout;
    } else {
      cachedSetTimeout = defaultSetTimeout;
    }
  } catch (e) {
    cachedSetTimeout = defaultSetTimeout;
  }
  try {
    if (typeof clearTimeout === 'function') {
      cachedClearTimeout = clearTimeout;
    } else {
      cachedClearTimeout = defaultClearTimeout;
    }
  } catch (e) {
    cachedClearTimeout = defaultClearTimeout;
  }
}())

function runTimeout(fn) {
  if (cachedSetTimeout === setTimeout) {
    //normal enviroments in sane situations
    return setTimeout(fn, 0);
  }
  // if setTimeout wasn't available but was latter defined
  if ((cachedSetTimeout === defaultSetTimeout || !cachedSetTimeout) && setTimeout) {
    cachedSetTimeout = setTimeout;
    return setTimeout(fn, 0);
  }
  try {
    // when when somebody has screwed with setTimeout but no I.E. maddness
    return cachedSetTimeout(fn, 0);
  } catch (e) {
    try {
      // When we are in I.E. but the script has been evaled so I.E. doesn't trust the global object when called normally
      return cachedSetTimeout.call(null, fn, 0);
    } catch (e) {
      // same as above but when it's a version of I.E. that must have the global object for 'this', hopfully our context correct otherwise it will throw a global error
      return cachedSetTimeout.call(this, fn, 0);
    }
  }
}

function runClearTimeout(marker) {
  if (cachedClearTimeout === clearTimeout) {
    //normal enviroments in sane situations
    return clearTimeout(marker);
  }
  // if clearTimeout wasn't available but was latter defined
  if ((cachedClearTimeout === defaultClearTimeout || !cachedClearTimeout) && clearTimeout) {
    cachedClearTimeout = clearTimeout;
    return clearTimeout(marker);
  }
  try {
    // when when somebody has screwed with setTimeout but no I.E. maddness
    return cachedClearTimeout(marker);
  } catch (e) {
    try {
      // When we are in I.E. but the script has been evaled so I.E. doesn't  trust the global object when called normally
      return cachedClearTimeout.call(null, marker);
    } catch (e) {
      // same as above but when it's a version of I.E. that must have the global object for 'this', hopfully our context correct otherwise it will throw a global error.
      // Some versions of I.E. have different rules for clearTimeout vs setTimeout
      return cachedClearTimeout.call(this, marker);
    }
  }
}

let queue = [];
let queueIndex = -1;
let currentQueue;
let draining = false;

function cleanUpNextTick() {
  if (!draining || !currentQueue) {
    return;
  }
  draining = false;
  if (currentQueue.length) {
    queue = currentQueue.concat(queue);
  } else {
    queueIndex = -1;
  }
  if (queue.length) {
    drainQueue();
  }
}

function drainQueue() {
  if (draining) {
    return;
  }
  let timeout = runTimeout(cleanUpNextTick);
  draining = true;

  let len = queue.length;
  while (len) {
    currentQueue = queue;
    queue = [];
    while (++queueIndex < len) {
      if (currentQueue) {
        currentQueue[queueIndex].run();
      }
    }
    queueIndex = -1;
    len = queue.length;
  }
  currentQueue = null;
  draining = false;
  runClearTimeout(timeout);
}

class Item {
  constructor(fn, array) {
    this.fn = fn;
    this.array = array;
  }
  run() {
    this.fn.apply(null, this.array);
  }
}

const deno = typeof Deno !== 'undefined';

export default {
  title: 'browser',
  browser: true,
  env: deno ? new Proxy({}, {
    get(_target, prop) {
      return Deno.env.get(String(prop));
    },
    ownKeys: () => Reflect.ownKeys(Deno.env.toObject()),
    getOwnPropertyDescriptor: (_target, name) => {
      const e = Deno.env.toObject();
      if (name in Deno.env.toObject()) {
        const o = { enumerable: true, configurable: true };
        if (typeof name === "string") {
          o.value = e[name];
        }
        return o;
      }
    },
    set(_target, prop, value) {
      Deno.env.set(String(prop), String(value));
      return value;
    },
  }) : {},
  argv: deno ? Deno.args ?? [] : [],
  pid: deno ? Deno.pid ?? 0 : 0,
  version: 'v16.14.0',
  versions: {
    node: '16.14.0',
    v8: '9.4.146.24-node.20',
    uv: '1.43.0',
    zlib: '1.2.11',
    brotli: '1.0.9',
    ares: '1.18.1',
    modules: '93',
    nghttp2: '1.45.1',
    napi: '8',
    llhttp: '6.0.4',
    openssl: '1.1.1m+quic',
    cldr: '40.0',
    icu: '70.1',
    tz: '2021a3',
    unicode: '14.0',
    ...(deno ? Deno.version ?? { deno: "1.0.0-denodeploy.beta-4" } : {})
  },
  on: (...args) => events.on(...args),
  addListener: (...args) => events.addListener(...args),
  once: (...args) => events.once(...args),
  off: (...args) => events.off(...args),
  removeListener: (...args) => events.removeListener(...args),
  removeAllListeners: (...args) => events.removeAllListeners(...args),
  emit: (...args) => events.emit(...args),
  prependListener: (...args) => events.prependListener(...args),
  prependOnceListener: (...args) => events.prependOnceListener(...args),
  listeners: () => [],
  emitWarning: () => { throw new Error('process.emitWarning is not supported') },
  binding: () => { throw new Error('process.binding is not supported') },
  cwd: () => deno ? Deno.cwd?.() ?? '/' : '/',
  chdir: (path) => {
    if (deno) {
      Deno.chdir(path)
    } else {
      throw new Error('process.chdir is not supported')
    }
  },
  umask: () => deno ? Deno.umask ?? 0 : 0,
  // arrow function don't have `arguments`
  nextTick: function (fn) {
    let args = new Array(arguments.length - 1);
    if (arguments.length > 1) {
      for (let i = 1; i < arguments.length; i++) {
        args[i - 1] = arguments[i];
      }
    }
    queue.push(new Item(fn, args));
    if (queue.length === 1 && !draining) {
      runTimeout(drainQueue);
    }
  },
};
