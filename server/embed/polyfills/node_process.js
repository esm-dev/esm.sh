/* esm.sh - Process polyfill for browser/Deno */

import { EventEmitter } from "./node_events.js";

function hrtime(time) {
  const milli = performance.now();
  const sec = Math.floor(milli / 1000);
  const nano = Math.floor(milli * 1000000 - sec * 1000000000);
  if (!time) {
    return [sec, nano];
  }
  const [prevSec, prevNano] = time;
  return [sec - prevSec, nano - prevNano];
}
hrtime.bigint = function () {
  const [sec, nano] = hrtime();
  return BigInt(sec) * 1_000_000_000n + BigInt(nano);
};

class Process extends EventEmitter {
  title = "browser";
  browser = true;
  env = {};
  argv = [];
  pid = 0;
  arch = "unknown";
  platform = "browser";
  version = "v18.12.1";
  versions = {
    node: "18.12.1",
    uv: "1.43.0",
    zlib: "1.2.11",
    brotli: "1.0.9",
    ares: "1.18.1",
    modules: "108",
    nghttp2: "1.47.0",
    napi: "8",
    llhttp: "6.0.10",
    openssl: "3.0.7+quic",
    cldr: "41.0",
    icu: "71.1",
    tz: "2022b",
    unicode: "14.0",
    ngtcp2: "0.8.1",
    nghttp3: "0.7.0",
  };
  emitWarning = () => {
    throw new Error("process.emitWarning is not supported");
  };
  binding = () => {
    throw new Error("process.binding is not supported");
  };
  cwd = () => {
    throw new Error("process.cwd is not supported");
  };
  chdir = (path) => {
    throw new Error("process.chdir is not supported");
  };
  umask = () => 0o22;
  nextTick = (func, ...args) => queueMicrotask(() => func(...args));
  hrtime = hrtime;
  constructor() {
    super();
  }
}

const process = new Process();

// partly copied from https://github.com/denoland/deno_std/tree/v0.177.0/node
if (typeof Deno !== "undefined") {
  process.name = "deno";
  process.pid = Deno.pid;
  process.cwd = () => Deno.cwd();
  process.chdir = (d) => Deno.chdir(d);
  process.arch = Deno.build.arch;
  process.platform = Deno.build.os;
  process.versions = { ...process.versions, ...Deno.version };

  process.env = new Proxy({}, {
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
  });

  // The first 2 items are placeholders.
  // They will be overwritten by the below Object.defineProperty calls.
  const argv = ["", "", ...Deno.args];
  Object.defineProperty(argv, "0", { get: Deno.execPath });
  Object.defineProperty(argv, "1", {
    get: () => {
      if (Deno.mainModule.startsWith("file:")) {
        return new URL(Deno.mainModule).pathname;
      } else {
        return join(Deno.cwd(), "$deno$node.js");
      }
    },
  });
  process.argv = argv;
}

export default process;
