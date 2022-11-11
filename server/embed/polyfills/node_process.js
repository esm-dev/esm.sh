/*
  (The MIT License)

  Copyright (c) 2020 Mathias Rasmussen <mathiasvr@gmail.com>

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

const deno = typeof Deno !== "undefined";

export default {
  title: deno ? "deno" : "browser",
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
  version: "v16.18.0",
  versions: {
    node: '16.18.0',
    v8: '9.4.146.26-node.22',
    uv: '1.43.0',
    zlib: '1.2.11',
    brotli: '1.0.9',
    ares: '1.18.1',
    modules: '93',
    nghttp2: '1.47.0',
    napi: '8',
    llhttp: '6.0.10',
    openssl: '1.1.1q+quic',
    cldr: '41.0',
    icu: '71.1',
    tz: '2022b',
    unicode: '14.0',
    ngtcp2: '0.8.1',
    nghttp3: '0.7.0',
    ...(deno ? Deno.version ?? {} : {})
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
  emitWarning: () => { throw new Error("process.emitWarning is not supported") },
  binding: () => { throw new Error("process.binding is not supported") },
  cwd: () => deno ? Deno.cwd?.() ?? "/" : "/",
  chdir: (path) => {
    if (deno) {
      Deno.chdir(path)
    } else {
      throw new Error("process.chdir is not supported")
    }
  },
  umask: () => deno ? Deno.umask ?? 0 : 0,
  nextTick: (func, ...args) => queueMicrotask(() => func(...args))
};
