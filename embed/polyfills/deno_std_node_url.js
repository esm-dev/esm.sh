/* deno mod bundle
 * entry: deno.land/std/node/url.ts
 * version: 0.110.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild url.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_url.js
 */

var __defProp = Object.defineProperty;
var __markAsModule = (target) =>
  __defProp(target, "__esModule", { value: true });
var __export = (target, all) => {
  __markAsModule(target);
  for (var name in all) {
    __defProp(target, name, { get: all[name], enumerable: true });
  }
};

// ../fmt/colors.ts
var { Deno: Deno2 } = globalThis;
var noColor = typeof Deno2?.noColor === "boolean" ? Deno2.noColor : true;
var ANSI_PATTERN = new RegExp(
  [
    "[\\u001B\\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?\\u0007)",
    "(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))",
  ].join("|"),
  "g",
);

// ../testing/_diff.ts
var DiffType;
(function (DiffType2) {
  DiffType2["removed"] = "removed";
  DiffType2["common"] = "common";
  DiffType2["added"] = "added";
})(DiffType || (DiffType = {}));

// ../testing/asserts.ts
var AssertionError = class extends Error {
  constructor(message) {
    super(message);
    this.name = "AssertionError";
  }
};
function unreachable() {
  throw new AssertionError("unreachable");
}

// ../_util/os.ts
var osType = (() => {
  const { Deno: Deno3 } = globalThis;
  if (typeof Deno3?.build?.os === "string") {
    return Deno3.build.os;
  }
  const { navigator } = globalThis;
  if (navigator?.appVersion?.includes?.("Win") ?? false) {
    return "windows";
  }
  return "linux";
})();
var isWindows = osType === "windows";

// _util/_util_promisify.ts
var kCustomPromisifiedSymbol = Symbol.for("nodejs.util.promisify.custom");
var kCustomPromisifyArgsSymbol = Symbol.for("nodejs.util.promisify.customArgs");
var NodeInvalidArgTypeError = class extends TypeError {
  constructor(argumentName, type, received) {
    super(
      `The "${argumentName}" argument must be of type ${type}. Received ${typeof received}`,
    );
    this.code = "ERR_INVALID_ARG_TYPE";
  }
};
function promisify(original) {
  if (typeof original !== "function") {
    throw new NodeInvalidArgTypeError("original", "Function", original);
  }
  if (original[kCustomPromisifiedSymbol]) {
    const fn2 = original[kCustomPromisifiedSymbol];
    if (typeof fn2 !== "function") {
      throw new NodeInvalidArgTypeError(
        "util.promisify.custom",
        "Function",
        fn2,
      );
    }
    return Object.defineProperty(fn2, kCustomPromisifiedSymbol, {
      value: fn2,
      enumerable: false,
      writable: false,
      configurable: true,
    });
  }
  const argumentNames = original[kCustomPromisifyArgsSymbol];
  function fn(...args) {
    return new Promise((resolve4, reject) => {
      original.call(this, ...args, (err, ...values) => {
        if (err) {
          return reject(err);
        }
        if (argumentNames !== void 0 && values.length > 1) {
          const obj = {};
          for (let i = 0; i < argumentNames.length; i++) {
            obj[argumentNames[i]] = values[i];
          }
          resolve4(obj);
        } else {
          resolve4(values[0]);
        }
      });
    });
  }
  Object.setPrototypeOf(fn, Object.getPrototypeOf(original));
  Object.defineProperty(fn, kCustomPromisifiedSymbol, {
    value: fn,
    enumerable: false,
    writable: false,
    configurable: true,
  });
  return Object.defineProperties(
    fn,
    Object.getOwnPropertyDescriptors(original),
  );
}
promisify.custom = kCustomPromisifiedSymbol;

// ../async/deferred.ts
function deferred() {
  let methods;
  let state = "pending";
  const promise = new Promise((resolve4, reject) => {
    methods = {
      async resolve(value) {
        await value;
        state = "fulfilled";
        resolve4(value);
      },
      reject(reason) {
        state = "rejected";
        reject(reason);
      },
    };
  });
  Object.defineProperty(promise, "state", { get: () => state });
  return Object.assign(promise, methods);
}

// ../async/mux_async_iterator.ts
var MuxAsyncIterator = class {
  constructor() {
    this.iteratorCount = 0;
    this.yields = [];
    this.throws = [];
    this.signal = deferred();
  }
  add(iterable) {
    ++this.iteratorCount;
    this.callIteratorNext(iterable[Symbol.asyncIterator]());
  }
  async callIteratorNext(iterator) {
    try {
      const { value, done } = await iterator.next();
      if (done) {
        --this.iteratorCount;
      } else {
        this.yields.push({ iterator, value });
      }
    } catch (e) {
      this.throws.push(e);
    }
    this.signal.resolve();
  }
  async *iterate() {
    while (this.iteratorCount > 0) {
      await this.signal;
      for (let i = 0; i < this.yields.length; i++) {
        const { iterator, value } = this.yields[i];
        yield value;
        this.callIteratorNext(iterator);
      }
      if (this.throws.length) {
        for (const e of this.throws) {
          throw e;
        }
        this.throws.length = 0;
      }
      this.yields.length = 0;
      this.signal = deferred();
    }
  }
  [Symbol.asyncIterator]() {
    return this.iterate();
  }
};

// ../async/tee.ts
var noop = () => {};
var AsyncIterableClone = class {
  constructor() {
    this.resolveCurrent = noop;
    this.consume = noop;
    this.currentPromise = new Promise((resolve4) => {
      this.resolveCurrent = resolve4;
    });
    this.consumed = new Promise((resolve4) => {
      this.consume = resolve4;
    });
  }
  reset() {
    this.currentPromise = new Promise((resolve4) => {
      this.resolveCurrent = resolve4;
    });
    this.consumed = new Promise((resolve4) => {
      this.consume = resolve4;
    });
  }
  async next() {
    const res = await this.currentPromise;
    this.consume();
    this.reset();
    return res;
  }
  async push(res) {
    this.resolveCurrent(res);
    await this.consumed;
  }
  [Symbol.asyncIterator]() {
    return this;
  }
};

// ../_util/assert.ts
var DenoStdInternalError = class extends Error {
  constructor(message) {
    super(message);
    this.name = "DenoStdInternalError";
  }
};
function assert(expr, msg = "") {
  if (!expr) {
    throw new DenoStdInternalError(msg);
  }
}

// ../bytes/mod.ts
function copy(src, dst, off = 0) {
  off = Math.max(0, Math.min(off, dst.byteLength));
  const dstBytesAvailable = dst.byteLength - off;
  if (src.byteLength > dstBytesAvailable) {
    src = src.subarray(0, dstBytesAvailable);
  }
  dst.set(src, off);
  return src.byteLength;
}

// ../io/buffer.ts
var MIN_READ = 32 * 1024;
var MAX_SIZE = 2 ** 32 - 2;
var Buffer2 = class {
  #buf;
  #off = 0;
  constructor(ab) {
    this.#buf = ab === void 0 ? new Uint8Array(0) : new Uint8Array(ab);
  }
  bytes(options = { copy: true }) {
    if (options.copy === false) return this.#buf.subarray(this.#off);
    return this.#buf.slice(this.#off);
  }
  empty() {
    return this.#buf.byteLength <= this.#off;
  }
  get length() {
    return this.#buf.byteLength - this.#off;
  }
  get capacity() {
    return this.#buf.buffer.byteLength;
  }
  truncate(n) {
    if (n === 0) {
      this.reset();
      return;
    }
    if (n < 0 || n > this.length) {
      throw Error("bytes.Buffer: truncation out of range");
    }
    this.#reslice(this.#off + n);
  }
  reset() {
    this.#reslice(0);
    this.#off = 0;
  }
  #tryGrowByReslice(n) {
    const l = this.#buf.byteLength;
    if (n <= this.capacity - l) {
      this.#reslice(l + n);
      return l;
    }
    return -1;
  }
  #reslice(len) {
    assert(len <= this.#buf.buffer.byteLength);
    this.#buf = new Uint8Array(this.#buf.buffer, 0, len);
  }
  readSync(p) {
    if (this.empty()) {
      this.reset();
      if (p.byteLength === 0) {
        return 0;
      }
      return null;
    }
    const nread = copy(this.#buf.subarray(this.#off), p);
    this.#off += nread;
    return nread;
  }
  read(p) {
    const rr = this.readSync(p);
    return Promise.resolve(rr);
  }
  writeSync(p) {
    const m = this.#grow(p.byteLength);
    return copy(p, this.#buf, m);
  }
  write(p) {
    const n = this.writeSync(p);
    return Promise.resolve(n);
  }
  #grow(n) {
    const m = this.length;
    if (m === 0 && this.#off !== 0) {
      this.reset();
    }
    const i = this.#tryGrowByReslice(n);
    if (i >= 0) {
      return i;
    }
    const c = this.capacity;
    if (n <= Math.floor(c / 2) - m) {
      copy(this.#buf.subarray(this.#off), this.#buf);
    } else if (c + n > MAX_SIZE) {
      throw new Error("The buffer cannot be grown beyond the maximum size.");
    } else {
      const buf = new Uint8Array(Math.min(2 * c + n, MAX_SIZE));
      copy(this.#buf.subarray(this.#off), buf);
      this.#buf = buf;
    }
    this.#off = 0;
    this.#reslice(Math.min(m + n, MAX_SIZE));
    return m;
  }
  grow(n) {
    if (n < 0) {
      throw Error("Buffer.grow: negative count");
    }
    const m = this.#grow(n);
    this.#reslice(m);
  }
  async readFrom(r) {
    let n = 0;
    const tmp = new Uint8Array(MIN_READ);
    while (true) {
      const shouldGrow = this.capacity - this.length < MIN_READ;
      const buf = shouldGrow
        ? tmp
        : new Uint8Array(this.#buf.buffer, this.length);
      const nread = await r.read(buf);
      if (nread === null) {
        return n;
      }
      if (shouldGrow) this.writeSync(buf.subarray(0, nread));
      else this.#reslice(this.length + nread);
      n += nread;
    }
  }
  readFromSync(r) {
    let n = 0;
    const tmp = new Uint8Array(MIN_READ);
    while (true) {
      const shouldGrow = this.capacity - this.length < MIN_READ;
      const buf = shouldGrow
        ? tmp
        : new Uint8Array(this.#buf.buffer, this.length);
      const nread = r.readSync(buf);
      if (nread === null) {
        return n;
      }
      if (shouldGrow) this.writeSync(buf.subarray(0, nread));
      else this.#reslice(this.length + nread);
      n += nread;
    }
  }
};
var CR = "\r".charCodeAt(0);
var LF = "\n".charCodeAt(0);

// ../io/streams.ts
var DEFAULT_BUFFER_SIZE = 32 * 1024;

// util.ts
var NumberIsSafeInteger = Number.isSafeInteger;
var DEFAULT_INSPECT_OPTIONS = {
  showHidden: false,
  depth: 2,
  colors: false,
  customInspect: true,
  showProxy: false,
  maxArrayLength: 100,
  maxStringLength: Infinity,
  breakLength: 80,
  compact: 3,
  sorted: false,
  getters: false,
};
inspect.defaultOptions = DEFAULT_INSPECT_OPTIONS;
inspect.custom = Symbol.for("nodejs.util.inspect.custom");
function inspect(object, ...opts) {
  if (typeof object === "string" && !object.includes("'")) {
    return `'${object}'`;
  }
  opts = { ...DEFAULT_INSPECT_OPTIONS, ...opts };
  return Deno.inspect(object, {
    depth: opts.depth,
    iterableLimit: opts.maxArrayLength,
    compact: !!opts.compact,
    sorted: !!opts.sorted,
    showProxy: !!opts.showProxy,
  });
}

// _errors.ts
var classRegExp = /^([A-Z][a-z0-9]*)+$/;
var kTypes = [
  "string",
  "function",
  "number",
  "object",
  "Function",
  "Object",
  "boolean",
  "bigint",
  "symbol",
];
var NodeErrorAbstraction = class extends Error {
  constructor(name, code, message) {
    super(message);
    this.code = code;
    this.name = name;
    this.stack = this.stack && `${name} [${this.code}]${this.stack.slice(20)}`;
  }
  toString() {
    return `${this.name} [${this.code}]: ${this.message}`;
  }
};
var NodeTypeError = class extends NodeErrorAbstraction {
  constructor(code, message) {
    super(TypeError.prototype.name, code, message);
    Object.setPrototypeOf(this, TypeError.prototype);
  }
};
var ERR_INVALID_ARG_TYPE = class extends NodeTypeError {
  constructor(name, expected, actual) {
    expected = Array.isArray(expected) ? expected : [expected];
    let msg = "The ";
    if (name.endsWith(" argument")) {
      msg += `${name} `;
    } else {
      const type = name.includes(".") ? "property" : "argument";
      msg += `"${name}" ${type} `;
    }
    msg += "must be ";
    const types = [];
    const instances = [];
    const other = [];
    for (const value of expected) {
      if (kTypes.includes(value)) {
        types.push(value.toLocaleLowerCase());
      } else if (classRegExp.test(value)) {
        instances.push(value);
      } else {
        other.push(value);
      }
    }
    if (instances.length > 0) {
      const pos = types.indexOf("object");
      if (pos !== -1) {
        types.splice(pos, 1);
        instances.push("Object");
      }
    }
    if (types.length > 0) {
      if (types.length > 2) {
        const last = types.pop();
        msg += `one of type ${types.join(", ")}, or ${last}`;
      } else if (types.length === 2) {
        msg += `one of type ${types[0]} or ${types[1]}`;
      } else {
        msg += `of type ${types[0]}`;
      }
      if (instances.length > 0 || other.length > 0) {
        msg += " or ";
      }
    }
    if (instances.length > 0) {
      if (instances.length > 2) {
        const last = instances.pop();
        msg += `an instance of ${instances.join(", ")}, or ${last}`;
      } else {
        msg += `an instance of ${instances[0]}`;
        if (instances.length === 2) {
          msg += ` or ${instances[1]}`;
        }
      }
      if (other.length > 0) {
        msg += " or ";
      }
    }
    if (other.length > 0) {
      if (other.length > 2) {
        const last = other.pop();
        msg += `one of ${other.join(", ")}, or ${last}`;
      } else if (other.length === 2) {
        msg += `one of ${other[0]} or ${other[1]}`;
      } else {
        if (other[0].toLowerCase() !== other[0]) {
          msg += "an ";
        }
        msg += `${other[0]}`;
      }
    }
    super("ERR_INVALID_ARG_TYPE", `${msg}.${invalidArgTypeHelper(actual)}`);
  }
};
var ERR_INVALID_ARG_VALUE = class extends NodeTypeError {
  constructor(name, value, reason) {
    super(
      "ERR_INVALID_ARG_VALUE",
      `The argument '${name}' ${reason}. Received ${inspect(value)}`,
    );
  }
};
function invalidArgTypeHelper(input) {
  if (input == null) {
    return ` Received ${input}`;
  }
  if (typeof input === "function" && input.name) {
    return ` Received function ${input.name}`;
  }
  if (typeof input === "object") {
    if (input.constructor && input.constructor.name) {
      return ` Received an instance of ${input.constructor.name}`;
    }
    return ` Received ${inspect(input, { depth: -1 })}`;
  }
  let inspected = inspect(input, { colors: false });
  if (inspected.length > 25) {
    inspected = `${inspected.slice(0, 25)}...`;
  }
  return ` Received type ${typeof input} (${inspected})`;
}
var windows = [
  [-4093, ["E2BIG", "argument list too long"]],
  [-4092, ["EACCES", "permission denied"]],
  [-4091, ["EADDRINUSE", "address already in use"]],
  [-4090, ["EADDRNOTAVAIL", "address not available"]],
  [-4089, ["EAFNOSUPPORT", "address family not supported"]],
  [-4088, ["EAGAIN", "resource temporarily unavailable"]],
  [-3e3, ["EAI_ADDRFAMILY", "address family not supported"]],
  [-3001, ["EAI_AGAIN", "temporary failure"]],
  [-3002, ["EAI_BADFLAGS", "bad ai_flags value"]],
  [-3013, ["EAI_BADHINTS", "invalid value for hints"]],
  [-3003, ["EAI_CANCELED", "request canceled"]],
  [-3004, ["EAI_FAIL", "permanent failure"]],
  [-3005, ["EAI_FAMILY", "ai_family not supported"]],
  [-3006, ["EAI_MEMORY", "out of memory"]],
  [-3007, ["EAI_NODATA", "no address"]],
  [-3008, ["EAI_NONAME", "unknown node or service"]],
  [-3009, ["EAI_OVERFLOW", "argument buffer overflow"]],
  [-3014, ["EAI_PROTOCOL", "resolved protocol is unknown"]],
  [-3010, ["EAI_SERVICE", "service not available for socket type"]],
  [-3011, ["EAI_SOCKTYPE", "socket type not supported"]],
  [-4084, ["EALREADY", "connection already in progress"]],
  [-4083, ["EBADF", "bad file descriptor"]],
  [-4082, ["EBUSY", "resource busy or locked"]],
  [-4081, ["ECANCELED", "operation canceled"]],
  [-4080, ["ECHARSET", "invalid Unicode character"]],
  [-4079, ["ECONNABORTED", "software caused connection abort"]],
  [-4078, ["ECONNREFUSED", "connection refused"]],
  [-4077, ["ECONNRESET", "connection reset by peer"]],
  [-4076, ["EDESTADDRREQ", "destination address required"]],
  [-4075, ["EEXIST", "file already exists"]],
  [-4074, ["EFAULT", "bad address in system call argument"]],
  [-4036, ["EFBIG", "file too large"]],
  [-4073, ["EHOSTUNREACH", "host is unreachable"]],
  [-4072, ["EINTR", "interrupted system call"]],
  [-4071, ["EINVAL", "invalid argument"]],
  [-4070, ["EIO", "i/o error"]],
  [-4069, ["EISCONN", "socket is already connected"]],
  [-4068, ["EISDIR", "illegal operation on a directory"]],
  [-4067, ["ELOOP", "too many symbolic links encountered"]],
  [-4066, ["EMFILE", "too many open files"]],
  [-4065, ["EMSGSIZE", "message too long"]],
  [-4064, ["ENAMETOOLONG", "name too long"]],
  [-4063, ["ENETDOWN", "network is down"]],
  [-4062, ["ENETUNREACH", "network is unreachable"]],
  [-4061, ["ENFILE", "file table overflow"]],
  [-4060, ["ENOBUFS", "no buffer space available"]],
  [-4059, ["ENODEV", "no such device"]],
  [-4058, ["ENOENT", "no such file or directory"]],
  [-4057, ["ENOMEM", "not enough memory"]],
  [-4056, ["ENONET", "machine is not on the network"]],
  [-4035, ["ENOPROTOOPT", "protocol not available"]],
  [-4055, ["ENOSPC", "no space left on device"]],
  [-4054, ["ENOSYS", "function not implemented"]],
  [-4053, ["ENOTCONN", "socket is not connected"]],
  [-4052, ["ENOTDIR", "not a directory"]],
  [-4051, ["ENOTEMPTY", "directory not empty"]],
  [-4050, ["ENOTSOCK", "socket operation on non-socket"]],
  [-4049, ["ENOTSUP", "operation not supported on socket"]],
  [-4048, ["EPERM", "operation not permitted"]],
  [-4047, ["EPIPE", "broken pipe"]],
  [-4046, ["EPROTO", "protocol error"]],
  [-4045, ["EPROTONOSUPPORT", "protocol not supported"]],
  [-4044, ["EPROTOTYPE", "protocol wrong type for socket"]],
  [-4034, ["ERANGE", "result too large"]],
  [-4043, ["EROFS", "read-only file system"]],
  [-4042, ["ESHUTDOWN", "cannot send after transport endpoint shutdown"]],
  [-4041, ["ESPIPE", "invalid seek"]],
  [-4040, ["ESRCH", "no such process"]],
  [-4039, ["ETIMEDOUT", "connection timed out"]],
  [-4038, ["ETXTBSY", "text file is busy"]],
  [-4037, ["EXDEV", "cross-device link not permitted"]],
  [-4094, ["UNKNOWN", "unknown error"]],
  [-4095, ["EOF", "end of file"]],
  [-4033, ["ENXIO", "no such device or address"]],
  [-4032, ["EMLINK", "too many links"]],
  [-4031, ["EHOSTDOWN", "host is down"]],
  [-4030, ["EREMOTEIO", "remote I/O error"]],
  [-4029, ["ENOTTY", "inappropriate ioctl for device"]],
  [-4028, ["EFTYPE", "inappropriate file type or format"]],
  [-4027, ["EILSEQ", "illegal byte sequence"]],
];
var darwin = [
  [-7, ["E2BIG", "argument list too long"]],
  [-13, ["EACCES", "permission denied"]],
  [-48, ["EADDRINUSE", "address already in use"]],
  [-49, ["EADDRNOTAVAIL", "address not available"]],
  [-47, ["EAFNOSUPPORT", "address family not supported"]],
  [-35, ["EAGAIN", "resource temporarily unavailable"]],
  [-3e3, ["EAI_ADDRFAMILY", "address family not supported"]],
  [-3001, ["EAI_AGAIN", "temporary failure"]],
  [-3002, ["EAI_BADFLAGS", "bad ai_flags value"]],
  [-3013, ["EAI_BADHINTS", "invalid value for hints"]],
  [-3003, ["EAI_CANCELED", "request canceled"]],
  [-3004, ["EAI_FAIL", "permanent failure"]],
  [-3005, ["EAI_FAMILY", "ai_family not supported"]],
  [-3006, ["EAI_MEMORY", "out of memory"]],
  [-3007, ["EAI_NODATA", "no address"]],
  [-3008, ["EAI_NONAME", "unknown node or service"]],
  [-3009, ["EAI_OVERFLOW", "argument buffer overflow"]],
  [-3014, ["EAI_PROTOCOL", "resolved protocol is unknown"]],
  [-3010, ["EAI_SERVICE", "service not available for socket type"]],
  [-3011, ["EAI_SOCKTYPE", "socket type not supported"]],
  [-37, ["EALREADY", "connection already in progress"]],
  [-9, ["EBADF", "bad file descriptor"]],
  [-16, ["EBUSY", "resource busy or locked"]],
  [-89, ["ECANCELED", "operation canceled"]],
  [-4080, ["ECHARSET", "invalid Unicode character"]],
  [-53, ["ECONNABORTED", "software caused connection abort"]],
  [-61, ["ECONNREFUSED", "connection refused"]],
  [-54, ["ECONNRESET", "connection reset by peer"]],
  [-39, ["EDESTADDRREQ", "destination address required"]],
  [-17, ["EEXIST", "file already exists"]],
  [-14, ["EFAULT", "bad address in system call argument"]],
  [-27, ["EFBIG", "file too large"]],
  [-65, ["EHOSTUNREACH", "host is unreachable"]],
  [-4, ["EINTR", "interrupted system call"]],
  [-22, ["EINVAL", "invalid argument"]],
  [-5, ["EIO", "i/o error"]],
  [-56, ["EISCONN", "socket is already connected"]],
  [-21, ["EISDIR", "illegal operation on a directory"]],
  [-62, ["ELOOP", "too many symbolic links encountered"]],
  [-24, ["EMFILE", "too many open files"]],
  [-40, ["EMSGSIZE", "message too long"]],
  [-63, ["ENAMETOOLONG", "name too long"]],
  [-50, ["ENETDOWN", "network is down"]],
  [-51, ["ENETUNREACH", "network is unreachable"]],
  [-23, ["ENFILE", "file table overflow"]],
  [-55, ["ENOBUFS", "no buffer space available"]],
  [-19, ["ENODEV", "no such device"]],
  [-2, ["ENOENT", "no such file or directory"]],
  [-12, ["ENOMEM", "not enough memory"]],
  [-4056, ["ENONET", "machine is not on the network"]],
  [-42, ["ENOPROTOOPT", "protocol not available"]],
  [-28, ["ENOSPC", "no space left on device"]],
  [-78, ["ENOSYS", "function not implemented"]],
  [-57, ["ENOTCONN", "socket is not connected"]],
  [-20, ["ENOTDIR", "not a directory"]],
  [-66, ["ENOTEMPTY", "directory not empty"]],
  [-38, ["ENOTSOCK", "socket operation on non-socket"]],
  [-45, ["ENOTSUP", "operation not supported on socket"]],
  [-1, ["EPERM", "operation not permitted"]],
  [-32, ["EPIPE", "broken pipe"]],
  [-100, ["EPROTO", "protocol error"]],
  [-43, ["EPROTONOSUPPORT", "protocol not supported"]],
  [-41, ["EPROTOTYPE", "protocol wrong type for socket"]],
  [-34, ["ERANGE", "result too large"]],
  [-30, ["EROFS", "read-only file system"]],
  [-58, ["ESHUTDOWN", "cannot send after transport endpoint shutdown"]],
  [-29, ["ESPIPE", "invalid seek"]],
  [-3, ["ESRCH", "no such process"]],
  [-60, ["ETIMEDOUT", "connection timed out"]],
  [-26, ["ETXTBSY", "text file is busy"]],
  [-18, ["EXDEV", "cross-device link not permitted"]],
  [-4094, ["UNKNOWN", "unknown error"]],
  [-4095, ["EOF", "end of file"]],
  [-6, ["ENXIO", "no such device or address"]],
  [-31, ["EMLINK", "too many links"]],
  [-64, ["EHOSTDOWN", "host is down"]],
  [-4030, ["EREMOTEIO", "remote I/O error"]],
  [-25, ["ENOTTY", "inappropriate ioctl for device"]],
  [-79, ["EFTYPE", "inappropriate file type or format"]],
  [-92, ["EILSEQ", "illegal byte sequence"]],
];
var linux = [
  [-7, ["E2BIG", "argument list too long"]],
  [-13, ["EACCES", "permission denied"]],
  [-98, ["EADDRINUSE", "address already in use"]],
  [-99, ["EADDRNOTAVAIL", "address not available"]],
  [-97, ["EAFNOSUPPORT", "address family not supported"]],
  [-11, ["EAGAIN", "resource temporarily unavailable"]],
  [-3e3, ["EAI_ADDRFAMILY", "address family not supported"]],
  [-3001, ["EAI_AGAIN", "temporary failure"]],
  [-3002, ["EAI_BADFLAGS", "bad ai_flags value"]],
  [-3013, ["EAI_BADHINTS", "invalid value for hints"]],
  [-3003, ["EAI_CANCELED", "request canceled"]],
  [-3004, ["EAI_FAIL", "permanent failure"]],
  [-3005, ["EAI_FAMILY", "ai_family not supported"]],
  [-3006, ["EAI_MEMORY", "out of memory"]],
  [-3007, ["EAI_NODATA", "no address"]],
  [-3008, ["EAI_NONAME", "unknown node or service"]],
  [-3009, ["EAI_OVERFLOW", "argument buffer overflow"]],
  [-3014, ["EAI_PROTOCOL", "resolved protocol is unknown"]],
  [-3010, ["EAI_SERVICE", "service not available for socket type"]],
  [-3011, ["EAI_SOCKTYPE", "socket type not supported"]],
  [-114, ["EALREADY", "connection already in progress"]],
  [-9, ["EBADF", "bad file descriptor"]],
  [-16, ["EBUSY", "resource busy or locked"]],
  [-125, ["ECANCELED", "operation canceled"]],
  [-4080, ["ECHARSET", "invalid Unicode character"]],
  [-103, ["ECONNABORTED", "software caused connection abort"]],
  [-111, ["ECONNREFUSED", "connection refused"]],
  [-104, ["ECONNRESET", "connection reset by peer"]],
  [-89, ["EDESTADDRREQ", "destination address required"]],
  [-17, ["EEXIST", "file already exists"]],
  [-14, ["EFAULT", "bad address in system call argument"]],
  [-27, ["EFBIG", "file too large"]],
  [-113, ["EHOSTUNREACH", "host is unreachable"]],
  [-4, ["EINTR", "interrupted system call"]],
  [-22, ["EINVAL", "invalid argument"]],
  [-5, ["EIO", "i/o error"]],
  [-106, ["EISCONN", "socket is already connected"]],
  [-21, ["EISDIR", "illegal operation on a directory"]],
  [-40, ["ELOOP", "too many symbolic links encountered"]],
  [-24, ["EMFILE", "too many open files"]],
  [-90, ["EMSGSIZE", "message too long"]],
  [-36, ["ENAMETOOLONG", "name too long"]],
  [-100, ["ENETDOWN", "network is down"]],
  [-101, ["ENETUNREACH", "network is unreachable"]],
  [-23, ["ENFILE", "file table overflow"]],
  [-105, ["ENOBUFS", "no buffer space available"]],
  [-19, ["ENODEV", "no such device"]],
  [-2, ["ENOENT", "no such file or directory"]],
  [-12, ["ENOMEM", "not enough memory"]],
  [-64, ["ENONET", "machine is not on the network"]],
  [-92, ["ENOPROTOOPT", "protocol not available"]],
  [-28, ["ENOSPC", "no space left on device"]],
  [-38, ["ENOSYS", "function not implemented"]],
  [-107, ["ENOTCONN", "socket is not connected"]],
  [-20, ["ENOTDIR", "not a directory"]],
  [-39, ["ENOTEMPTY", "directory not empty"]],
  [-88, ["ENOTSOCK", "socket operation on non-socket"]],
  [-95, ["ENOTSUP", "operation not supported on socket"]],
  [-1, ["EPERM", "operation not permitted"]],
  [-32, ["EPIPE", "broken pipe"]],
  [-71, ["EPROTO", "protocol error"]],
  [-93, ["EPROTONOSUPPORT", "protocol not supported"]],
  [-91, ["EPROTOTYPE", "protocol wrong type for socket"]],
  [-34, ["ERANGE", "result too large"]],
  [-30, ["EROFS", "read-only file system"]],
  [-108, ["ESHUTDOWN", "cannot send after transport endpoint shutdown"]],
  [-29, ["ESPIPE", "invalid seek"]],
  [-3, ["ESRCH", "no such process"]],
  [-110, ["ETIMEDOUT", "connection timed out"]],
  [-26, ["ETXTBSY", "text file is busy"]],
  [-18, ["EXDEV", "cross-device link not permitted"]],
  [-4094, ["UNKNOWN", "unknown error"]],
  [-4095, ["EOF", "end of file"]],
  [-6, ["ENXIO", "no such device or address"]],
  [-31, ["EMLINK", "too many links"]],
  [-112, ["EHOSTDOWN", "host is down"]],
  [-121, ["EREMOTEIO", "remote I/O error"]],
  [-25, ["ENOTTY", "inappropriate ioctl for device"]],
  [-4028, ["EFTYPE", "inappropriate file type or format"]],
  [-84, ["EILSEQ", "illegal byte sequence"]],
];
var errorMap = new Map(
  osType === "windows"
    ? windows
    : osType === "darwin"
    ? darwin
    : osType === "linux"
    ? linux
    : unreachable(),
);
var ERR_INVALID_FILE_URL_HOST = class extends NodeTypeError {
  constructor(x) {
    super(
      "ERR_INVALID_FILE_URL_HOST",
      `File URL host must be "localhost" or empty on ${x}`,
    );
  }
};
var ERR_INVALID_FILE_URL_PATH = class extends NodeTypeError {
  constructor(x) {
    super("ERR_INVALID_FILE_URL_PATH", `File URL path ${x}`);
  }
};
var ERR_INVALID_URL_SCHEME = class extends NodeTypeError {
  constructor(expected) {
    expected = Array.isArray(expected) ? expected : [expected];
    const res = expected.length === 2
      ? `one of scheme ${expected[0]} or ${expected[1]}`
      : `of scheme ${expected[0]}`;
    super("ERR_INVALID_URL_SCHEME", `The URL must be ${res}`);
  }
};

// ../path/_constants.ts
var CHAR_UPPERCASE_A = 65;
var CHAR_LOWERCASE_A = 97;
var CHAR_UPPERCASE_Z = 90;
var CHAR_LOWERCASE_Z = 122;
var CHAR_DOT = 46;
var CHAR_FORWARD_SLASH = 47;
var CHAR_BACKWARD_SLASH = 92;
var CHAR_COLON = 58;
var CHAR_QUESTION_MARK = 63;

// ../path/mod.ts
var mod_exports = {};
__export(mod_exports, {
  SEP: () => SEP,
  SEP_PATTERN: () => SEP_PATTERN,
  basename: () => basename3,
  common: () => common,
  delimiter: () => delimiter3,
  dirname: () => dirname3,
  extname: () => extname3,
  format: () => format3,
  fromFileUrl: () => fromFileUrl3,
  globToRegExp: () => globToRegExp,
  isAbsolute: () => isAbsolute3,
  isGlob: () => isGlob,
  join: () => join4,
  joinGlobs: () => joinGlobs,
  normalize: () => normalize4,
  normalizeGlob: () => normalizeGlob,
  parse: () => parse3,
  posix: () => posix,
  relative: () => relative3,
  resolve: () => resolve3,
  sep: () => sep3,
  toFileUrl: () => toFileUrl3,
  toNamespacedPath: () => toNamespacedPath3,
  win32: () => win32,
});

// ../path/win32.ts
var win32_exports = {};
__export(win32_exports, {
  basename: () => basename,
  delimiter: () => delimiter,
  dirname: () => dirname,
  extname: () => extname,
  format: () => format,
  fromFileUrl: () => fromFileUrl,
  isAbsolute: () => isAbsolute,
  join: () => join,
  normalize: () => normalize,
  parse: () => parse,
  relative: () => relative,
  resolve: () => resolve,
  sep: () => sep,
  toFileUrl: () => toFileUrl,
  toNamespacedPath: () => toNamespacedPath,
});

// ../path/_util.ts
function assertPath(path4) {
  if (typeof path4 !== "string") {
    throw new TypeError(
      `Path must be a string. Received ${JSON.stringify(path4)}`,
    );
  }
}
function isPosixPathSeparator(code) {
  return code === CHAR_FORWARD_SLASH;
}
function isPathSeparator(code) {
  return isPosixPathSeparator(code) || code === CHAR_BACKWARD_SLASH;
}
function isWindowsDeviceRoot(code) {
  return (
    (code >= CHAR_LOWERCASE_A && code <= CHAR_LOWERCASE_Z) ||
    (code >= CHAR_UPPERCASE_A && code <= CHAR_UPPERCASE_Z)
  );
}
function normalizeString(path4, allowAboveRoot, separator, isPathSeparator2) {
  let res = "";
  let lastSegmentLength = 0;
  let lastSlash = -1;
  let dots = 0;
  let code;
  for (let i = 0, len = path4.length; i <= len; ++i) {
    if (i < len) code = path4.charCodeAt(i);
    else if (isPathSeparator2(code)) break;
    else code = CHAR_FORWARD_SLASH;
    if (isPathSeparator2(code)) {
      if (lastSlash === i - 1 || dots === 1) {
      } else if (lastSlash !== i - 1 && dots === 2) {
        if (
          res.length < 2 ||
          lastSegmentLength !== 2 ||
          res.charCodeAt(res.length - 1) !== CHAR_DOT ||
          res.charCodeAt(res.length - 2) !== CHAR_DOT
        ) {
          if (res.length > 2) {
            const lastSlashIndex = res.lastIndexOf(separator);
            if (lastSlashIndex === -1) {
              res = "";
              lastSegmentLength = 0;
            } else {
              res = res.slice(0, lastSlashIndex);
              lastSegmentLength = res.length - 1 - res.lastIndexOf(separator);
            }
            lastSlash = i;
            dots = 0;
            continue;
          } else if (res.length === 2 || res.length === 1) {
            res = "";
            lastSegmentLength = 0;
            lastSlash = i;
            dots = 0;
            continue;
          }
        }
        if (allowAboveRoot) {
          if (res.length > 0) res += `${separator}..`;
          else res = "..";
          lastSegmentLength = 2;
        }
      } else {
        if (res.length > 0) res += separator + path4.slice(lastSlash + 1, i);
        else res = path4.slice(lastSlash + 1, i);
        lastSegmentLength = i - lastSlash - 1;
      }
      lastSlash = i;
      dots = 0;
    } else if (code === CHAR_DOT && dots !== -1) {
      ++dots;
    } else {
      dots = -1;
    }
  }
  return res;
}
function _format(sep4, pathObject) {
  const dir = pathObject.dir || pathObject.root;
  const base = pathObject.base ||
    (pathObject.name || "") + (pathObject.ext || "");
  if (!dir) return base;
  if (dir === pathObject.root) return dir + base;
  return dir + sep4 + base;
}
var WHITESPACE_ENCODINGS = {
  "	": "%09",
  "\n": "%0A",
  "\v": "%0B",
  "\f": "%0C",
  "\r": "%0D",
  " ": "%20",
};
function encodeWhitespace(string) {
  return string.replaceAll(/[\s]/g, (c) => {
    return WHITESPACE_ENCODINGS[c] ?? c;
  });
}

// ../path/win32.ts
var sep = "\\";
var delimiter = ";";
function resolve(...pathSegments) {
  let resolvedDevice = "";
  let resolvedTail = "";
  let resolvedAbsolute = false;
  for (let i = pathSegments.length - 1; i >= -1; i--) {
    let path4;
    const { Deno: Deno3 } = globalThis;
    if (i >= 0) {
      path4 = pathSegments[i];
    } else if (!resolvedDevice) {
      if (typeof Deno3?.cwd !== "function") {
        throw new TypeError("Resolved a drive-letter-less path without a CWD.");
      }
      path4 = Deno3.cwd();
    } else {
      if (
        typeof Deno3?.env?.get !== "function" ||
        typeof Deno3?.cwd !== "function"
      ) {
        throw new TypeError("Resolved a relative path without a CWD.");
      }
      path4 = Deno3.cwd();
      if (
        path4 === void 0 ||
        path4.slice(0, 3).toLowerCase() !== `${resolvedDevice.toLowerCase()}\\`
      ) {
        path4 = `${resolvedDevice}\\`;
      }
    }
    assertPath(path4);
    const len = path4.length;
    if (len === 0) continue;
    let rootEnd = 0;
    let device = "";
    let isAbsolute4 = false;
    const code = path4.charCodeAt(0);
    if (len > 1) {
      if (isPathSeparator(code)) {
        isAbsolute4 = true;
        if (isPathSeparator(path4.charCodeAt(1))) {
          let j = 2;
          let last = j;
          for (; j < len; ++j) {
            if (isPathSeparator(path4.charCodeAt(j))) break;
          }
          if (j < len && j !== last) {
            const firstPart = path4.slice(last, j);
            last = j;
            for (; j < len; ++j) {
              if (!isPathSeparator(path4.charCodeAt(j))) break;
            }
            if (j < len && j !== last) {
              last = j;
              for (; j < len; ++j) {
                if (isPathSeparator(path4.charCodeAt(j))) break;
              }
              if (j === len) {
                device = `\\\\${firstPart}\\${path4.slice(last)}`;
                rootEnd = j;
              } else if (j !== last) {
                device = `\\\\${firstPart}\\${path4.slice(last, j)}`;
                rootEnd = j;
              }
            }
          }
        } else {
          rootEnd = 1;
        }
      } else if (isWindowsDeviceRoot(code)) {
        if (path4.charCodeAt(1) === CHAR_COLON) {
          device = path4.slice(0, 2);
          rootEnd = 2;
          if (len > 2) {
            if (isPathSeparator(path4.charCodeAt(2))) {
              isAbsolute4 = true;
              rootEnd = 3;
            }
          }
        }
      }
    } else if (isPathSeparator(code)) {
      rootEnd = 1;
      isAbsolute4 = true;
    }
    if (
      device.length > 0 && resolvedDevice.length > 0 &&
      device.toLowerCase() !== resolvedDevice.toLowerCase()
    ) {
      continue;
    }
    if (resolvedDevice.length === 0 && device.length > 0) {
      resolvedDevice = device;
    }
    if (!resolvedAbsolute) {
      resolvedTail = `${path4.slice(rootEnd)}\\${resolvedTail}`;
      resolvedAbsolute = isAbsolute4;
    }
    if (resolvedAbsolute && resolvedDevice.length > 0) break;
  }
  resolvedTail = normalizeString(
    resolvedTail,
    !resolvedAbsolute,
    "\\",
    isPathSeparator,
  );
  return resolvedDevice + (resolvedAbsolute ? "\\" : "") + resolvedTail || ".";
}
function normalize(path4) {
  assertPath(path4);
  const len = path4.length;
  if (len === 0) return ".";
  let rootEnd = 0;
  let device;
  let isAbsolute4 = false;
  const code = path4.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      isAbsolute4 = true;
      if (isPathSeparator(path4.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path4.charCodeAt(j))) break;
        }
        if (j < len && j !== last) {
          const firstPart = path4.slice(last, j);
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path4.charCodeAt(j))) break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path4.charCodeAt(j))) break;
            }
            if (j === len) {
              return `\\\\${firstPart}\\${path4.slice(last)}\\`;
            } else if (j !== last) {
              device = `\\\\${firstPart}\\${path4.slice(last, j)}`;
              rootEnd = j;
            }
          }
        }
      } else {
        rootEnd = 1;
      }
    } else if (isWindowsDeviceRoot(code)) {
      if (path4.charCodeAt(1) === CHAR_COLON) {
        device = path4.slice(0, 2);
        rootEnd = 2;
        if (len > 2) {
          if (isPathSeparator(path4.charCodeAt(2))) {
            isAbsolute4 = true;
            rootEnd = 3;
          }
        }
      }
    }
  } else if (isPathSeparator(code)) {
    return "\\";
  }
  let tail;
  if (rootEnd < len) {
    tail = normalizeString(
      path4.slice(rootEnd),
      !isAbsolute4,
      "\\",
      isPathSeparator,
    );
  } else {
    tail = "";
  }
  if (tail.length === 0 && !isAbsolute4) tail = ".";
  if (tail.length > 0 && isPathSeparator(path4.charCodeAt(len - 1))) {
    tail += "\\";
  }
  if (device === void 0) {
    if (isAbsolute4) {
      if (tail.length > 0) return `\\${tail}`;
      else return "\\";
    } else if (tail.length > 0) {
      return tail;
    } else {
      return "";
    }
  } else if (isAbsolute4) {
    if (tail.length > 0) return `${device}\\${tail}`;
    else return `${device}\\`;
  } else if (tail.length > 0) {
    return device + tail;
  } else {
    return device;
  }
}
function isAbsolute(path4) {
  assertPath(path4);
  const len = path4.length;
  if (len === 0) return false;
  const code = path4.charCodeAt(0);
  if (isPathSeparator(code)) {
    return true;
  } else if (isWindowsDeviceRoot(code)) {
    if (len > 2 && path4.charCodeAt(1) === CHAR_COLON) {
      if (isPathSeparator(path4.charCodeAt(2))) return true;
    }
  }
  return false;
}
function join(...paths) {
  const pathsCount = paths.length;
  if (pathsCount === 0) return ".";
  let joined;
  let firstPart = null;
  for (let i = 0; i < pathsCount; ++i) {
    const path4 = paths[i];
    assertPath(path4);
    if (path4.length > 0) {
      if (joined === void 0) joined = firstPart = path4;
      else joined += `\\${path4}`;
    }
  }
  if (joined === void 0) return ".";
  let needsReplace = true;
  let slashCount = 0;
  assert(firstPart != null);
  if (isPathSeparator(firstPart.charCodeAt(0))) {
    ++slashCount;
    const firstLen = firstPart.length;
    if (firstLen > 1) {
      if (isPathSeparator(firstPart.charCodeAt(1))) {
        ++slashCount;
        if (firstLen > 2) {
          if (isPathSeparator(firstPart.charCodeAt(2))) ++slashCount;
          else {
            needsReplace = false;
          }
        }
      }
    }
  }
  if (needsReplace) {
    for (; slashCount < joined.length; ++slashCount) {
      if (!isPathSeparator(joined.charCodeAt(slashCount))) break;
    }
    if (slashCount >= 2) joined = `\\${joined.slice(slashCount)}`;
  }
  return normalize(joined);
}
function relative(from, to) {
  assertPath(from);
  assertPath(to);
  if (from === to) return "";
  const fromOrig = resolve(from);
  const toOrig = resolve(to);
  if (fromOrig === toOrig) return "";
  from = fromOrig.toLowerCase();
  to = toOrig.toLowerCase();
  if (from === to) return "";
  let fromStart = 0;
  let fromEnd = from.length;
  for (; fromStart < fromEnd; ++fromStart) {
    if (from.charCodeAt(fromStart) !== CHAR_BACKWARD_SLASH) break;
  }
  for (; fromEnd - 1 > fromStart; --fromEnd) {
    if (from.charCodeAt(fromEnd - 1) !== CHAR_BACKWARD_SLASH) break;
  }
  const fromLen = fromEnd - fromStart;
  let toStart = 0;
  let toEnd = to.length;
  for (; toStart < toEnd; ++toStart) {
    if (to.charCodeAt(toStart) !== CHAR_BACKWARD_SLASH) break;
  }
  for (; toEnd - 1 > toStart; --toEnd) {
    if (to.charCodeAt(toEnd - 1) !== CHAR_BACKWARD_SLASH) break;
  }
  const toLen = toEnd - toStart;
  const length = fromLen < toLen ? fromLen : toLen;
  let lastCommonSep = -1;
  let i = 0;
  for (; i <= length; ++i) {
    if (i === length) {
      if (toLen > length) {
        if (to.charCodeAt(toStart + i) === CHAR_BACKWARD_SLASH) {
          return toOrig.slice(toStart + i + 1);
        } else if (i === 2) {
          return toOrig.slice(toStart + i);
        }
      }
      if (fromLen > length) {
        if (from.charCodeAt(fromStart + i) === CHAR_BACKWARD_SLASH) {
          lastCommonSep = i;
        } else if (i === 2) {
          lastCommonSep = 3;
        }
      }
      break;
    }
    const fromCode = from.charCodeAt(fromStart + i);
    const toCode = to.charCodeAt(toStart + i);
    if (fromCode !== toCode) break;
    else if (fromCode === CHAR_BACKWARD_SLASH) lastCommonSep = i;
  }
  if (i !== length && lastCommonSep === -1) {
    return toOrig;
  }
  let out = "";
  if (lastCommonSep === -1) lastCommonSep = 0;
  for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
    if (i === fromEnd || from.charCodeAt(i) === CHAR_BACKWARD_SLASH) {
      if (out.length === 0) out += "..";
      else out += "\\..";
    }
  }
  if (out.length > 0) {
    return out + toOrig.slice(toStart + lastCommonSep, toEnd);
  } else {
    toStart += lastCommonSep;
    if (toOrig.charCodeAt(toStart) === CHAR_BACKWARD_SLASH) ++toStart;
    return toOrig.slice(toStart, toEnd);
  }
}
function toNamespacedPath(path4) {
  if (typeof path4 !== "string") return path4;
  if (path4.length === 0) return "";
  const resolvedPath = resolve(path4);
  if (resolvedPath.length >= 3) {
    if (resolvedPath.charCodeAt(0) === CHAR_BACKWARD_SLASH) {
      if (resolvedPath.charCodeAt(1) === CHAR_BACKWARD_SLASH) {
        const code = resolvedPath.charCodeAt(2);
        if (code !== CHAR_QUESTION_MARK && code !== CHAR_DOT) {
          return `\\\\?\\UNC\\${resolvedPath.slice(2)}`;
        }
      }
    } else if (isWindowsDeviceRoot(resolvedPath.charCodeAt(0))) {
      if (
        resolvedPath.charCodeAt(1) === CHAR_COLON &&
        resolvedPath.charCodeAt(2) === CHAR_BACKWARD_SLASH
      ) {
        return `\\\\?\\${resolvedPath}`;
      }
    }
  }
  return path4;
}
function dirname(path4) {
  assertPath(path4);
  const len = path4.length;
  if (len === 0) return ".";
  let rootEnd = -1;
  let end = -1;
  let matchedSlash = true;
  let offset = 0;
  const code = path4.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      rootEnd = offset = 1;
      if (isPathSeparator(path4.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path4.charCodeAt(j))) break;
        }
        if (j < len && j !== last) {
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path4.charCodeAt(j))) break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path4.charCodeAt(j))) break;
            }
            if (j === len) {
              return path4;
            }
            if (j !== last) {
              rootEnd = offset = j + 1;
            }
          }
        }
      }
    } else if (isWindowsDeviceRoot(code)) {
      if (path4.charCodeAt(1) === CHAR_COLON) {
        rootEnd = offset = 2;
        if (len > 2) {
          if (isPathSeparator(path4.charCodeAt(2))) rootEnd = offset = 3;
        }
      }
    }
  } else if (isPathSeparator(code)) {
    return path4;
  }
  for (let i = len - 1; i >= offset; --i) {
    if (isPathSeparator(path4.charCodeAt(i))) {
      if (!matchedSlash) {
        end = i;
        break;
      }
    } else {
      matchedSlash = false;
    }
  }
  if (end === -1) {
    if (rootEnd === -1) return ".";
    else end = rootEnd;
  }
  return path4.slice(0, end);
}
function basename(path4, ext = "") {
  if (ext !== void 0 && typeof ext !== "string") {
    throw new TypeError('"ext" argument must be a string');
  }
  assertPath(path4);
  let start = 0;
  let end = -1;
  let matchedSlash = true;
  let i;
  if (path4.length >= 2) {
    const drive = path4.charCodeAt(0);
    if (isWindowsDeviceRoot(drive)) {
      if (path4.charCodeAt(1) === CHAR_COLON) start = 2;
    }
  }
  if (ext !== void 0 && ext.length > 0 && ext.length <= path4.length) {
    if (ext.length === path4.length && ext === path4) return "";
    let extIdx = ext.length - 1;
    let firstNonSlashEnd = -1;
    for (i = path4.length - 1; i >= start; --i) {
      const code = path4.charCodeAt(i);
      if (isPathSeparator(code)) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else {
        if (firstNonSlashEnd === -1) {
          matchedSlash = false;
          firstNonSlashEnd = i + 1;
        }
        if (extIdx >= 0) {
          if (code === ext.charCodeAt(extIdx)) {
            if (--extIdx === -1) {
              end = i;
            }
          } else {
            extIdx = -1;
            end = firstNonSlashEnd;
          }
        }
      }
    }
    if (start === end) end = firstNonSlashEnd;
    else if (end === -1) end = path4.length;
    return path4.slice(start, end);
  } else {
    for (i = path4.length - 1; i >= start; --i) {
      if (isPathSeparator(path4.charCodeAt(i))) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
    }
    if (end === -1) return "";
    return path4.slice(start, end);
  }
}
function extname(path4) {
  assertPath(path4);
  let start = 0;
  let startDot = -1;
  let startPart = 0;
  let end = -1;
  let matchedSlash = true;
  let preDotState = 0;
  if (
    path4.length >= 2 && path4.charCodeAt(1) === CHAR_COLON &&
    isWindowsDeviceRoot(path4.charCodeAt(0))
  ) {
    start = startPart = 2;
  }
  for (let i = path4.length - 1; i >= start; --i) {
    const code = path4.charCodeAt(i);
    if (isPathSeparator(code)) {
      if (!matchedSlash) {
        startPart = i + 1;
        break;
      }
      continue;
    }
    if (end === -1) {
      matchedSlash = false;
      end = i + 1;
    }
    if (code === CHAR_DOT) {
      if (startDot === -1) startDot = i;
      else if (preDotState !== 1) preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (
    startDot === -1 ||
    end === -1 ||
    preDotState === 0 ||
    (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)
  ) {
    return "";
  }
  return path4.slice(startDot, end);
}
function format(pathObject) {
  if (pathObject === null || typeof pathObject !== "object") {
    throw new TypeError(
      `The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`,
    );
  }
  return _format("\\", pathObject);
}
function parse(path4) {
  assertPath(path4);
  const ret = { root: "", dir: "", base: "", ext: "", name: "" };
  const len = path4.length;
  if (len === 0) return ret;
  let rootEnd = 0;
  let code = path4.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      rootEnd = 1;
      if (isPathSeparator(path4.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path4.charCodeAt(j))) break;
        }
        if (j < len && j !== last) {
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path4.charCodeAt(j))) break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path4.charCodeAt(j))) break;
            }
            if (j === len) {
              rootEnd = j;
            } else if (j !== last) {
              rootEnd = j + 1;
            }
          }
        }
      }
    } else if (isWindowsDeviceRoot(code)) {
      if (path4.charCodeAt(1) === CHAR_COLON) {
        rootEnd = 2;
        if (len > 2) {
          if (isPathSeparator(path4.charCodeAt(2))) {
            if (len === 3) {
              ret.root = ret.dir = path4;
              return ret;
            }
            rootEnd = 3;
          }
        } else {
          ret.root = ret.dir = path4;
          return ret;
        }
      }
    }
  } else if (isPathSeparator(code)) {
    ret.root = ret.dir = path4;
    return ret;
  }
  if (rootEnd > 0) ret.root = path4.slice(0, rootEnd);
  let startDot = -1;
  let startPart = rootEnd;
  let end = -1;
  let matchedSlash = true;
  let i = path4.length - 1;
  let preDotState = 0;
  for (; i >= rootEnd; --i) {
    code = path4.charCodeAt(i);
    if (isPathSeparator(code)) {
      if (!matchedSlash) {
        startPart = i + 1;
        break;
      }
      continue;
    }
    if (end === -1) {
      matchedSlash = false;
      end = i + 1;
    }
    if (code === CHAR_DOT) {
      if (startDot === -1) startDot = i;
      else if (preDotState !== 1) preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (
    startDot === -1 ||
    end === -1 ||
    preDotState === 0 ||
    (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)
  ) {
    if (end !== -1) {
      ret.base = ret.name = path4.slice(startPart, end);
    }
  } else {
    ret.name = path4.slice(startPart, startDot);
    ret.base = path4.slice(startPart, end);
    ret.ext = path4.slice(startDot, end);
  }
  if (startPart > 0 && startPart !== rootEnd) {
    ret.dir = path4.slice(0, startPart - 1);
  } else ret.dir = ret.root;
  return ret;
}
function fromFileUrl(url) {
  url = url instanceof URL ? url : new URL(url);
  if (url.protocol != "file:") {
    throw new TypeError("Must be a file URL.");
  }
  let path4 = decodeURIComponent(
    url.pathname.replace(/\//g, "\\").replace(/%(?![0-9A-Fa-f]{2})/g, "%25"),
  ).replace(
    /^\\*([A-Za-z]:)(\\|$)/,
    "$1\\",
  );
  if (url.hostname != "") {
    path4 = `\\\\${url.hostname}${path4}`;
  }
  return path4;
}
function toFileUrl(path4) {
  if (!isAbsolute(path4)) {
    throw new TypeError("Must be an absolute path.");
  }
  const [, hostname, pathname] = path4.match(
    /^(?:[/\\]{2}([^/\\]+)(?=[/\\](?:[^/\\]|$)))?(.*)/,
  );
  const url = new URL("file:///");
  url.pathname = encodeWhitespace(pathname.replace(/%/g, "%25"));
  if (hostname != null && hostname != "localhost") {
    url.hostname = hostname;
    if (!url.hostname) {
      throw new TypeError("Invalid hostname.");
    }
  }
  return url;
}

// ../path/posix.ts
var posix_exports = {};
__export(posix_exports, {
  basename: () => basename2,
  delimiter: () => delimiter2,
  dirname: () => dirname2,
  extname: () => extname2,
  format: () => format2,
  fromFileUrl: () => fromFileUrl2,
  isAbsolute: () => isAbsolute2,
  join: () => join2,
  normalize: () => normalize2,
  parse: () => parse2,
  relative: () => relative2,
  resolve: () => resolve2,
  sep: () => sep2,
  toFileUrl: () => toFileUrl2,
  toNamespacedPath: () => toNamespacedPath2,
});
var sep2 = "/";
var delimiter2 = ":";
function resolve2(...pathSegments) {
  let resolvedPath = "";
  let resolvedAbsolute = false;
  for (let i = pathSegments.length - 1; i >= -1 && !resolvedAbsolute; i--) {
    let path4;
    if (i >= 0) path4 = pathSegments[i];
    else {
      const { Deno: Deno3 } = globalThis;
      if (typeof Deno3?.cwd !== "function") {
        throw new TypeError("Resolved a relative path without a CWD.");
      }
      path4 = Deno3.cwd();
    }
    assertPath(path4);
    if (path4.length === 0) {
      continue;
    }
    resolvedPath = `${path4}/${resolvedPath}`;
    resolvedAbsolute = path4.charCodeAt(0) === CHAR_FORWARD_SLASH;
  }
  resolvedPath = normalizeString(
    resolvedPath,
    !resolvedAbsolute,
    "/",
    isPosixPathSeparator,
  );
  if (resolvedAbsolute) {
    if (resolvedPath.length > 0) return `/${resolvedPath}`;
    else return "/";
  } else if (resolvedPath.length > 0) return resolvedPath;
  else return ".";
}
function normalize2(path4) {
  assertPath(path4);
  if (path4.length === 0) return ".";
  const isAbsolute4 = path4.charCodeAt(0) === CHAR_FORWARD_SLASH;
  const trailingSeparator =
    path4.charCodeAt(path4.length - 1) === CHAR_FORWARD_SLASH;
  path4 = normalizeString(path4, !isAbsolute4, "/", isPosixPathSeparator);
  if (path4.length === 0 && !isAbsolute4) path4 = ".";
  if (path4.length > 0 && trailingSeparator) path4 += "/";
  if (isAbsolute4) return `/${path4}`;
  return path4;
}
function isAbsolute2(path4) {
  assertPath(path4);
  return path4.length > 0 && path4.charCodeAt(0) === CHAR_FORWARD_SLASH;
}
function join2(...paths) {
  if (paths.length === 0) return ".";
  let joined;
  for (let i = 0, len = paths.length; i < len; ++i) {
    const path4 = paths[i];
    assertPath(path4);
    if (path4.length > 0) {
      if (!joined) joined = path4;
      else joined += `/${path4}`;
    }
  }
  if (!joined) return ".";
  return normalize2(joined);
}
function relative2(from, to) {
  assertPath(from);
  assertPath(to);
  if (from === to) return "";
  from = resolve2(from);
  to = resolve2(to);
  if (from === to) return "";
  let fromStart = 1;
  const fromEnd = from.length;
  for (; fromStart < fromEnd; ++fromStart) {
    if (from.charCodeAt(fromStart) !== CHAR_FORWARD_SLASH) break;
  }
  const fromLen = fromEnd - fromStart;
  let toStart = 1;
  const toEnd = to.length;
  for (; toStart < toEnd; ++toStart) {
    if (to.charCodeAt(toStart) !== CHAR_FORWARD_SLASH) break;
  }
  const toLen = toEnd - toStart;
  const length = fromLen < toLen ? fromLen : toLen;
  let lastCommonSep = -1;
  let i = 0;
  for (; i <= length; ++i) {
    if (i === length) {
      if (toLen > length) {
        if (to.charCodeAt(toStart + i) === CHAR_FORWARD_SLASH) {
          return to.slice(toStart + i + 1);
        } else if (i === 0) {
          return to.slice(toStart + i);
        }
      } else if (fromLen > length) {
        if (from.charCodeAt(fromStart + i) === CHAR_FORWARD_SLASH) {
          lastCommonSep = i;
        } else if (i === 0) {
          lastCommonSep = 0;
        }
      }
      break;
    }
    const fromCode = from.charCodeAt(fromStart + i);
    const toCode = to.charCodeAt(toStart + i);
    if (fromCode !== toCode) break;
    else if (fromCode === CHAR_FORWARD_SLASH) lastCommonSep = i;
  }
  let out = "";
  for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
    if (i === fromEnd || from.charCodeAt(i) === CHAR_FORWARD_SLASH) {
      if (out.length === 0) out += "..";
      else out += "/..";
    }
  }
  if (out.length > 0) return out + to.slice(toStart + lastCommonSep);
  else {
    toStart += lastCommonSep;
    if (to.charCodeAt(toStart) === CHAR_FORWARD_SLASH) ++toStart;
    return to.slice(toStart);
  }
}
function toNamespacedPath2(path4) {
  return path4;
}
function dirname2(path4) {
  assertPath(path4);
  if (path4.length === 0) return ".";
  const hasRoot = path4.charCodeAt(0) === CHAR_FORWARD_SLASH;
  let end = -1;
  let matchedSlash = true;
  for (let i = path4.length - 1; i >= 1; --i) {
    if (path4.charCodeAt(i) === CHAR_FORWARD_SLASH) {
      if (!matchedSlash) {
        end = i;
        break;
      }
    } else {
      matchedSlash = false;
    }
  }
  if (end === -1) return hasRoot ? "/" : ".";
  if (hasRoot && end === 1) return "//";
  return path4.slice(0, end);
}
function basename2(path4, ext = "") {
  if (ext !== void 0 && typeof ext !== "string") {
    throw new TypeError('"ext" argument must be a string');
  }
  assertPath(path4);
  let start = 0;
  let end = -1;
  let matchedSlash = true;
  let i;
  if (ext !== void 0 && ext.length > 0 && ext.length <= path4.length) {
    if (ext.length === path4.length && ext === path4) return "";
    let extIdx = ext.length - 1;
    let firstNonSlashEnd = -1;
    for (i = path4.length - 1; i >= 0; --i) {
      const code = path4.charCodeAt(i);
      if (code === CHAR_FORWARD_SLASH) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else {
        if (firstNonSlashEnd === -1) {
          matchedSlash = false;
          firstNonSlashEnd = i + 1;
        }
        if (extIdx >= 0) {
          if (code === ext.charCodeAt(extIdx)) {
            if (--extIdx === -1) {
              end = i;
            }
          } else {
            extIdx = -1;
            end = firstNonSlashEnd;
          }
        }
      }
    }
    if (start === end) end = firstNonSlashEnd;
    else if (end === -1) end = path4.length;
    return path4.slice(start, end);
  } else {
    for (i = path4.length - 1; i >= 0; --i) {
      if (path4.charCodeAt(i) === CHAR_FORWARD_SLASH) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
    }
    if (end === -1) return "";
    return path4.slice(start, end);
  }
}
function extname2(path4) {
  assertPath(path4);
  let startDot = -1;
  let startPart = 0;
  let end = -1;
  let matchedSlash = true;
  let preDotState = 0;
  for (let i = path4.length - 1; i >= 0; --i) {
    const code = path4.charCodeAt(i);
    if (code === CHAR_FORWARD_SLASH) {
      if (!matchedSlash) {
        startPart = i + 1;
        break;
      }
      continue;
    }
    if (end === -1) {
      matchedSlash = false;
      end = i + 1;
    }
    if (code === CHAR_DOT) {
      if (startDot === -1) startDot = i;
      else if (preDotState !== 1) preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (
    startDot === -1 ||
    end === -1 ||
    preDotState === 0 ||
    (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)
  ) {
    return "";
  }
  return path4.slice(startDot, end);
}
function format2(pathObject) {
  if (pathObject === null || typeof pathObject !== "object") {
    throw new TypeError(
      `The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`,
    );
  }
  return _format("/", pathObject);
}
function parse2(path4) {
  assertPath(path4);
  const ret = { root: "", dir: "", base: "", ext: "", name: "" };
  if (path4.length === 0) return ret;
  const isAbsolute4 = path4.charCodeAt(0) === CHAR_FORWARD_SLASH;
  let start;
  if (isAbsolute4) {
    ret.root = "/";
    start = 1;
  } else {
    start = 0;
  }
  let startDot = -1;
  let startPart = 0;
  let end = -1;
  let matchedSlash = true;
  let i = path4.length - 1;
  let preDotState = 0;
  for (; i >= start; --i) {
    const code = path4.charCodeAt(i);
    if (code === CHAR_FORWARD_SLASH) {
      if (!matchedSlash) {
        startPart = i + 1;
        break;
      }
      continue;
    }
    if (end === -1) {
      matchedSlash = false;
      end = i + 1;
    }
    if (code === CHAR_DOT) {
      if (startDot === -1) startDot = i;
      else if (preDotState !== 1) preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (
    startDot === -1 ||
    end === -1 ||
    preDotState === 0 ||
    (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)
  ) {
    if (end !== -1) {
      if (startPart === 0 && isAbsolute4) {
        ret.base = ret.name = path4.slice(1, end);
      } else {
        ret.base = ret.name = path4.slice(startPart, end);
      }
    }
  } else {
    if (startPart === 0 && isAbsolute4) {
      ret.name = path4.slice(1, startDot);
      ret.base = path4.slice(1, end);
    } else {
      ret.name = path4.slice(startPart, startDot);
      ret.base = path4.slice(startPart, end);
    }
    ret.ext = path4.slice(startDot, end);
  }
  if (startPart > 0) ret.dir = path4.slice(0, startPart - 1);
  else if (isAbsolute4) ret.dir = "/";
  return ret;
}
function fromFileUrl2(url) {
  url = url instanceof URL ? url : new URL(url);
  if (url.protocol != "file:") {
    throw new TypeError("Must be a file URL.");
  }
  return decodeURIComponent(
    url.pathname.replace(/%(?![0-9A-Fa-f]{2})/g, "%25"),
  );
}
function toFileUrl2(path4) {
  if (!isAbsolute2(path4)) {
    throw new TypeError("Must be an absolute path.");
  }
  const url = new URL("file:///");
  url.pathname = encodeWhitespace(
    path4.replace(/%/g, "%25").replace(/\\/g, "%5C"),
  );
  return url;
}

// ../path/separator.ts
var SEP = isWindows ? "\\" : "/";
var SEP_PATTERN = isWindows ? /[\\/]+/ : /\/+/;

// ../path/common.ts
function common(paths, sep4 = SEP) {
  const [first = "", ...remaining] = paths;
  if (first === "" || remaining.length === 0) {
    return first.substring(0, first.lastIndexOf(sep4) + 1);
  }
  const parts = first.split(sep4);
  let endOfPrefix = parts.length;
  for (const path4 of remaining) {
    const compare = path4.split(sep4);
    for (let i = 0; i < endOfPrefix; i++) {
      if (compare[i] !== parts[i]) {
        endOfPrefix = i;
      }
    }
    if (endOfPrefix === 0) {
      return "";
    }
  }
  const prefix = parts.slice(0, endOfPrefix).join(sep4);
  return prefix.endsWith(sep4) ? prefix : `${prefix}${sep4}`;
}

// ../path/glob.ts
var path = isWindows ? win32_exports : posix_exports;
var { join: join3, normalize: normalize3 } = path;
var regExpEscapeChars = [
  "!",
  "$",
  "(",
  ")",
  "*",
  "+",
  ".",
  "=",
  "?",
  "[",
  "\\",
  "^",
  "{",
  "|",
];
var rangeEscapeChars = ["-", "\\", "]"];
function globToRegExp(
  glob,
  {
    extended = true,
    globstar: globstarOption = true,
    os = osType,
    caseInsensitive = false,
  } = {},
) {
  if (glob == "") {
    return /(?!)/;
  }
  const sep4 = os == "windows" ? "(?:\\\\|/)+" : "/+";
  const sepMaybe = os == "windows" ? "(?:\\\\|/)*" : "/*";
  const seps = os == "windows" ? ["\\", "/"] : ["/"];
  const globstar = os == "windows"
    ? "(?:[^\\\\/]*(?:\\\\|/|$)+)*"
    : "(?:[^/]*(?:/|$)+)*";
  const wildcard = os == "windows" ? "[^\\\\/]*" : "[^/]*";
  const escapePrefix = os == "windows" ? "`" : "\\";
  let newLength = glob.length;
  for (; newLength > 1 && seps.includes(glob[newLength - 1]); newLength--);
  glob = glob.slice(0, newLength);
  let regExpString = "";
  for (let j = 0; j < glob.length;) {
    let segment = "";
    const groupStack = [];
    let inRange = false;
    let inEscape = false;
    let endsWithSep = false;
    let i = j;
    for (; i < glob.length && !seps.includes(glob[i]); i++) {
      if (inEscape) {
        inEscape = false;
        const escapeChars = inRange ? rangeEscapeChars : regExpEscapeChars;
        segment += escapeChars.includes(glob[i]) ? `\\${glob[i]}` : glob[i];
        continue;
      }
      if (glob[i] == escapePrefix) {
        inEscape = true;
        continue;
      }
      if (glob[i] == "[") {
        if (!inRange) {
          inRange = true;
          segment += "[";
          if (glob[i + 1] == "!") {
            i++;
            segment += "^";
          } else if (glob[i + 1] == "^") {
            i++;
            segment += "\\^";
          }
          continue;
        } else if (glob[i + 1] == ":") {
          let k = i + 1;
          let value = "";
          while (glob[k + 1] != null && glob[k + 1] != ":") {
            value += glob[k + 1];
            k++;
          }
          if (glob[k + 1] == ":" && glob[k + 2] == "]") {
            i = k + 2;
            if (value == "alnum") segment += "\\dA-Za-z";
            else if (value == "alpha") segment += "A-Za-z";
            else if (value == "ascii") segment += "\0-\x7F";
            else if (value == "blank") segment += "	 ";
            else if (value == "cntrl") segment += "\0-\x7F";
            else if (value == "digit") segment += "\\d";
            else if (value == "graph") segment += "!-~";
            else if (value == "lower") segment += "a-z";
            else if (value == "print") segment += " -~";
            else if (value == "punct") {
              segment += `!"#$%&'()*+,\\-./:;<=>?@[\\\\\\]^_\u2018{|}~`;
            } else if (value == "space") segment += "\\s\v";
            else if (value == "upper") segment += "A-Z";
            else if (value == "word") segment += "\\w";
            else if (value == "xdigit") segment += "\\dA-Fa-f";
            continue;
          }
        }
      }
      if (glob[i] == "]" && inRange) {
        inRange = false;
        segment += "]";
        continue;
      }
      if (inRange) {
        if (glob[i] == "\\") {
          segment += `\\\\`;
        } else {
          segment += glob[i];
        }
        continue;
      }
      if (
        glob[i] == ")" && groupStack.length > 0 &&
        groupStack[groupStack.length - 1] != "BRACE"
      ) {
        segment += ")";
        const type = groupStack.pop();
        if (type == "!") {
          segment += wildcard;
        } else if (type != "@") {
          segment += type;
        }
        continue;
      }
      if (
        glob[i] == "|" && groupStack.length > 0 &&
        groupStack[groupStack.length - 1] != "BRACE"
      ) {
        segment += "|";
        continue;
      }
      if (glob[i] == "+" && extended && glob[i + 1] == "(") {
        i++;
        groupStack.push("+");
        segment += "(?:";
        continue;
      }
      if (glob[i] == "@" && extended && glob[i + 1] == "(") {
        i++;
        groupStack.push("@");
        segment += "(?:";
        continue;
      }
      if (glob[i] == "?") {
        if (extended && glob[i + 1] == "(") {
          i++;
          groupStack.push("?");
          segment += "(?:";
        } else {
          segment += ".";
        }
        continue;
      }
      if (glob[i] == "!" && extended && glob[i + 1] == "(") {
        i++;
        groupStack.push("!");
        segment += "(?!";
        continue;
      }
      if (glob[i] == "{") {
        groupStack.push("BRACE");
        segment += "(?:";
        continue;
      }
      if (glob[i] == "}" && groupStack[groupStack.length - 1] == "BRACE") {
        groupStack.pop();
        segment += ")";
        continue;
      }
      if (glob[i] == "," && groupStack[groupStack.length - 1] == "BRACE") {
        segment += "|";
        continue;
      }
      if (glob[i] == "*") {
        if (extended && glob[i + 1] == "(") {
          i++;
          groupStack.push("*");
          segment += "(?:";
        } else {
          const prevChar = glob[i - 1];
          let numStars = 1;
          while (glob[i + 1] == "*") {
            i++;
            numStars++;
          }
          const nextChar = glob[i + 1];
          if (
            globstarOption &&
            numStars == 2 &&
            [...seps, void 0].includes(prevChar) &&
            [...seps, void 0].includes(nextChar)
          ) {
            segment += globstar;
            endsWithSep = true;
          } else {
            segment += wildcard;
          }
        }
        continue;
      }
      segment += regExpEscapeChars.includes(glob[i]) ? `\\${glob[i]}` : glob[i];
    }
    if (groupStack.length > 0 || inRange || inEscape) {
      segment = "";
      for (const c of glob.slice(j, i)) {
        segment += regExpEscapeChars.includes(c) ? `\\${c}` : c;
        endsWithSep = false;
      }
    }
    regExpString += segment;
    if (!endsWithSep) {
      regExpString += i < glob.length ? sep4 : sepMaybe;
      endsWithSep = true;
    }
    while (seps.includes(glob[i])) i++;
    if (!(i > j)) {
      throw new Error("Assertion failure: i > j (potential infinite loop)");
    }
    j = i;
  }
  regExpString = `^${regExpString}$`;
  return new RegExp(regExpString, caseInsensitive ? "i" : "");
}
function isGlob(str) {
  const chars = { "{": "}", "(": ")", "[": "]" };
  const regex =
    /\\(.)|(^!|\*|\?|[\].+)]\?|\[[^\\\]]+\]|\{[^\\}]+\}|\(\?[:!=][^\\)]+\)|\([^|]+\|[^\\)]+\))/;
  if (str === "") {
    return false;
  }
  let match;
  while ((match = regex.exec(str))) {
    if (match[2]) return true;
    let idx = match.index + match[0].length;
    const open = match[1];
    const close = open ? chars[open] : null;
    if (open && close) {
      const n = str.indexOf(close, idx);
      if (n !== -1) {
        idx = n + 1;
      }
    }
    str = str.slice(idx);
  }
  return false;
}
function normalizeGlob(glob, { globstar = false } = {}) {
  if (glob.match(/\0/g)) {
    throw new Error(`Glob contains invalid characters: "${glob}"`);
  }
  if (!globstar) {
    return normalize3(glob);
  }
  const s = SEP_PATTERN.source;
  const badParentPattern = new RegExp(
    `(?<=(${s}|^)\\*\\*${s})\\.\\.(?=${s}|$)`,
    "g",
  );
  return normalize3(glob.replace(badParentPattern, "\0")).replace(/\0/g, "..");
}
function joinGlobs(globs, { extended = false, globstar = false } = {}) {
  if (!globstar || globs.length == 0) {
    return join3(...globs);
  }
  if (globs.length === 0) return ".";
  let joined;
  for (const glob of globs) {
    const path4 = glob;
    if (path4.length > 0) {
      if (!joined) joined = path4;
      else joined += `${SEP}${path4}`;
    }
  }
  if (!joined) return ".";
  return normalizeGlob(joined, { extended, globstar });
}

// ../path/mod.ts
var path2 = isWindows ? win32_exports : posix_exports;
var win32 = win32_exports;
var posix = posix_exports;
var {
  basename: basename3,
  delimiter: delimiter3,
  dirname: dirname3,
  extname: extname3,
  format: format3,
  fromFileUrl: fromFileUrl3,
  isAbsolute: isAbsolute3,
  join: join4,
  normalize: normalize4,
  parse: parse3,
  relative: relative3,
  resolve: resolve3,
  sep: sep3,
  toFileUrl: toFileUrl3,
  toNamespacedPath: toNamespacedPath3,
} = path2;

// path.ts
var path_default = { ...mod_exports };

// url.ts
var forwardSlashRegEx = /\//g;
var percentRegEx = /%/g;
var backslashRegEx = /\\/g;
var newlineRegEx = /\n/g;
var carriageReturnRegEx = /\r/g;
var tabRegEx = /\t/g;
var _url = URL;
function fileURLToPath(path4) {
  if (typeof path4 === "string") path4 = new URL(path4);
  else if (!(path4 instanceof URL)) {
    throw new ERR_INVALID_ARG_TYPE("path", ["string", "URL"], path4);
  }
  if (path4.protocol !== "file:") {
    throw new ERR_INVALID_URL_SCHEME("file");
  }
  return isWindows ? getPathFromURLWin(path4) : getPathFromURLPosix(path4);
}
function getPathFromURLWin(url) {
  const hostname = url.hostname;
  let pathname = url.pathname;
  for (let n = 0; n < pathname.length; n++) {
    if (pathname[n] === "%") {
      const third = pathname.codePointAt(n + 2) | 32;
      if (
        (pathname[n + 1] === "2" && third === 102) ||
        (pathname[n + 1] === "5" && third === 99)
      ) {
        throw new ERR_INVALID_FILE_URL_PATH(
          "must not include encoded \\ or / characters",
        );
      }
    }
  }
  pathname = pathname.replace(forwardSlashRegEx, "\\");
  pathname = decodeURIComponent(pathname);
  if (hostname !== "") {
    return `\\\\${hostname}${pathname}`;
  } else {
    const letter = pathname.codePointAt(1) | 32;
    const sep4 = pathname[2];
    if (
      letter < CHAR_LOWERCASE_A || letter > CHAR_LOWERCASE_Z || sep4 !== ":"
    ) {
      throw new ERR_INVALID_FILE_URL_PATH("must be absolute");
    }
    return pathname.slice(1);
  }
}
function getPathFromURLPosix(url) {
  if (url.hostname !== "") {
    throw new ERR_INVALID_FILE_URL_HOST(osType);
  }
  const pathname = url.pathname;
  for (let n = 0; n < pathname.length; n++) {
    if (pathname[n] === "%") {
      const third = pathname.codePointAt(n + 2) | 32;
      if (pathname[n + 1] === "2" && third === 102) {
        throw new ERR_INVALID_FILE_URL_PATH(
          "must not include encoded / characters",
        );
      }
    }
  }
  return decodeURIComponent(pathname);
}
function encodePathChars(filepath) {
  if (filepath.includes("%")) {
    filepath = filepath.replace(percentRegEx, "%25");
  }
  if (!isWindows && filepath.includes("\\")) {
    filepath = filepath.replace(backslashRegEx, "%5C");
  }
  if (filepath.includes("\n")) {
    filepath = filepath.replace(newlineRegEx, "%0A");
  }
  if (filepath.includes("\r")) {
    filepath = filepath.replace(carriageReturnRegEx, "%0D");
  }
  if (filepath.includes("	")) {
    filepath = filepath.replace(tabRegEx, "%09");
  }
  return filepath;
}
function pathToFileURL(filepath) {
  const outURL = new URL("file://");
  if (isWindows && filepath.startsWith("\\\\")) {
    const paths = filepath.split("\\");
    if (paths.length <= 3) {
      throw new ERR_INVALID_ARG_VALUE(
        "filepath",
        filepath,
        "Missing UNC resource path",
      );
    }
    const hostname = paths[2];
    if (hostname.length === 0) {
      throw new ERR_INVALID_ARG_VALUE(
        "filepath",
        filepath,
        "Empty UNC servername",
      );
    }
    outURL.hostname = hostname;
    outURL.pathname = encodePathChars(paths.slice(3).join("/"));
  } else {
    let resolved = resolve3(filepath);
    const filePathLast = filepath.charCodeAt(filepath.length - 1);
    if (
      (filePathLast === CHAR_FORWARD_SLASH ||
        (isWindows && filePathLast === CHAR_BACKWARD_SLASH)) &&
      resolved[resolved.length - 1] !== sep3
    ) {
      resolved += "/";
    }
    outURL.pathname = encodePathChars(resolved);
  }
  return outURL;
}
var url_default = {
  fileURLToPath,
  pathToFileURL,
  URL,
};
export { _url as URL, fileURLToPath, pathToFileURL, url_default as default };
