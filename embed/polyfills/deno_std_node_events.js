/* deno mod bundle
 * entry: deno.land/std/node/events.ts
 * version: 0.110.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild events.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_child_process.js
 */

var __accessCheck = (obj, member, msg) => {
  if (!member.has(obj)) throw TypeError("Cannot " + msg);
};
var __privateAdd = (obj, member, value) => {
  if (member.has(obj)) {
    throw TypeError("Cannot add the same private member more than once");
  }
  member instanceof WeakSet ? member.add(obj) : member.set(obj, value);
};
var __privateMethod = (obj, member, method) => {
  __accessCheck(obj, member, "access private method");
  return method;
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
    return new Promise((resolve, reject) => {
      original.call(this, ...args, (err, ...values) => {
        if (err) {
          return reject(err);
        }
        if (argumentNames !== void 0 && values.length > 1) {
          const obj = {};
          for (let i = 0; i < argumentNames.length; i++) {
            obj[argumentNames[i]] = values[i];
          }
          resolve(obj);
        } else {
          resolve(values[0]);
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
  const promise = new Promise((resolve, reject) => {
    methods = {
      async resolve(value) {
        await value;
        state = "fulfilled";
        resolve(value);
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
    this.currentPromise = new Promise((resolve) => {
      this.resolveCurrent = resolve;
    });
    this.consumed = new Promise((resolve) => {
      this.consume = resolve;
    });
  }
  reset() {
    this.currentPromise = new Promise((resolve) => {
      this.resolveCurrent = resolve;
    });
    this.consumed = new Promise((resolve) => {
      this.consume = resolve;
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
var ERR_OUT_OF_RANGE = class extends RangeError {
  constructor(str, range, received) {
    super(
      `The value of "${str}" is out of range. It must be ${range}. Received ${received}`,
    );
    this.code = "ERR_OUT_OF_RANGE";
    const { name } = this;
    this.name = `${name} [${this.code}]`;
    this.stack;
    this.name = name;
  }
};
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

// events.ts
function ensureArray(maybeArray) {
  return Array.isArray(maybeArray) ? maybeArray : [maybeArray];
}
function createIterResult(value, done) {
  return { value, done };
}
var defaultMaxListeners = 10;
function validateMaxListeners(n, name) {
  if (!Number.isInteger(n) || n < 0) {
    throw new ERR_OUT_OF_RANGE(name, "a non-negative number", inspect(n));
  }
}
var _init, init_fn;
var _EventEmitter = class {
  static get defaultMaxListeners() {
    return defaultMaxListeners;
  }
  static set defaultMaxListeners(value) {
    validateMaxListeners(value, "defaultMaxListeners");
    defaultMaxListeners = value;
  }
  constructor() {
    var _a;
    __privateMethod((_a = _EventEmitter), _init, init_fn).call(_a, this);
  }
  _addListener(eventName, listener, prepend) {
    var _a;
    this.checkListenerArgument(listener);
    this.emit("newListener", eventName, this.unwrapListener(listener));
    if (this.hasListeners(eventName)) {
      let listeners = this._events[eventName];
      if (!Array.isArray(listeners)) {
        listeners = [listeners];
        this._events[eventName] = listeners;
      }
      if (prepend) {
        listeners.unshift(listener);
      } else {
        listeners.push(listener);
      }
    } else if (this._events) {
      this._events[eventName] = listener;
    } else {
      __privateMethod((_a = _EventEmitter), _init, init_fn).call(_a, this);
      this._events[eventName] = listener;
    }
    const max = this.getMaxListeners();
    if (max > 0 && this.listenerCount(eventName) > max) {
      const warning = new MaxListenersExceededWarning(this, eventName);
      this.warnIfNeeded(eventName, warning);
    }
    return this;
  }
  addListener(eventName, listener) {
    return this._addListener(eventName, listener, false);
  }
  emit(eventName, ...args) {
    if (this.hasListeners(eventName)) {
      if (
        eventName === "error" && this.hasListeners(_EventEmitter.errorMonitor)
      ) {
        this.emit(_EventEmitter.errorMonitor, ...args);
      }
      const listeners = ensureArray(this._events[eventName]).slice();
      for (const listener of listeners) {
        try {
          listener.apply(this, args);
        } catch (err) {
          this.emit("error", err);
        }
      }
      return true;
    } else if (eventName === "error") {
      if (this.hasListeners(_EventEmitter.errorMonitor)) {
        this.emit(_EventEmitter.errorMonitor, ...args);
      }
      const errMsg = args.length > 0 ? args[0] : Error("Unhandled error.");
      throw errMsg;
    }
    return false;
  }
  eventNames() {
    return Reflect.ownKeys(this._events);
  }
  getMaxListeners() {
    return this.maxListeners == null
      ? _EventEmitter.defaultMaxListeners
      : this.maxListeners;
  }
  listenerCount(eventName) {
    if (this.hasListeners(eventName)) {
      const maybeListeners = this._events[eventName];
      return Array.isArray(maybeListeners) ? maybeListeners.length : 1;
    } else {
      return 0;
    }
  }
  static listenerCount(emitter, eventName) {
    return emitter.listenerCount(eventName);
  }
  _listeners(target, eventName, unwrap) {
    if (!target.hasListeners(eventName)) {
      return [];
    }
    const eventListeners = target._events[eventName];
    if (Array.isArray(eventListeners)) {
      return unwrap
        ? this.unwrapListeners(eventListeners)
        : eventListeners.slice(0);
    } else {
      return [unwrap ? this.unwrapListener(eventListeners) : eventListeners];
    }
  }
  unwrapListeners(arr) {
    const unwrappedListeners = new Array(arr.length);
    for (let i = 0; i < arr.length; i++) {
      unwrappedListeners[i] = this.unwrapListener(arr[i]);
    }
    return unwrappedListeners;
  }
  unwrapListener(listener) {
    return listener["listener"] ?? listener;
  }
  listeners(eventName) {
    return this._listeners(this, eventName, true);
  }
  rawListeners(eventName) {
    return this._listeners(this, eventName, false);
  }
  off(eventName, listener) {}
  on(eventName, listener) {}
  once(eventName, listener) {
    const wrapped = this.onceWrap(eventName, listener);
    this.on(eventName, wrapped);
    return this;
  }
  onceWrap(eventName, listener) {
    this.checkListenerArgument(listener);
    const wrapper = function (...args) {
      if (this.isCalled) {
        return;
      }
      this.context.removeListener(this.eventName, this.listener);
      this.isCalled = true;
      return this.listener.apply(this.context, args);
    };
    const wrapperContext = {
      eventName,
      listener,
      rawListener: wrapper,
      context: this,
    };
    const wrapped = wrapper.bind(wrapperContext);
    wrapperContext.rawListener = wrapped;
    wrapped.listener = listener;
    return wrapped;
  }
  prependListener(eventName, listener) {
    return this._addListener(eventName, listener, true);
  }
  prependOnceListener(eventName, listener) {
    const wrapped = this.onceWrap(eventName, listener);
    this.prependListener(eventName, wrapped);
    return this;
  }
  removeAllListeners(eventName) {
    if (this._events === void 0) {
      return this;
    }
    if (eventName) {
      if (this.hasListeners(eventName)) {
        const listeners = ensureArray(this._events[eventName]).slice()
          .reverse();
        for (const listener of listeners) {
          this.removeListener(eventName, this.unwrapListener(listener));
        }
      }
    } else {
      const eventList = this.eventNames();
      eventList.forEach((eventName2) => {
        if (eventName2 === "removeListener") return;
        this.removeAllListeners(eventName2);
      });
      this.removeAllListeners("removeListener");
    }
    return this;
  }
  removeListener(eventName, listener) {
    this.checkListenerArgument(listener);
    if (this.hasListeners(eventName)) {
      const maybeArr = this._events[eventName];
      assert(maybeArr);
      const arr = ensureArray(maybeArr);
      let listenerIndex = -1;
      for (let i = arr.length - 1; i >= 0; i--) {
        if (arr[i] == listener || (arr[i] && arr[i]["listener"] == listener)) {
          listenerIndex = i;
          break;
        }
      }
      if (listenerIndex >= 0) {
        arr.splice(listenerIndex, 1);
        if (arr.length === 0) {
          delete this._events[eventName];
        } else if (arr.length === 1) {
          this._events[eventName] = arr[0];
        }
        if (this._events.removeListener) {
          this.emit("removeListener", eventName, listener);
        }
      }
    }
    return this;
  }
  setMaxListeners(n) {
    if (n !== Infinity) {
      validateMaxListeners(n, "n");
    }
    this.maxListeners = n;
    return this;
  }
  static once(emitter, name) {
    return new Promise((resolve, reject) => {
      if (emitter instanceof EventTarget) {
        emitter.addEventListener(
          name,
          (...args) => {
            resolve(args);
          },
          { once: true, passive: false, capture: false },
        );
        return;
      } else if (emitter instanceof _EventEmitter) {
        const eventListener = (...args) => {
          if (errorListener !== void 0) {
            emitter.removeListener("error", errorListener);
          }
          resolve(args);
        };
        let errorListener;
        if (name !== "error") {
          errorListener = (err) => {
            emitter.removeListener(name, eventListener);
            reject(err);
          };
          emitter.once("error", errorListener);
        }
        emitter.once(name, eventListener);
        return;
      }
    });
  }
  static on(emitter, event) {
    const unconsumedEventValues = [];
    const unconsumedPromises = [];
    let error = null;
    let finished = false;
    const iterator = {
      next() {
        const value = unconsumedEventValues.shift();
        if (value) {
          return Promise.resolve(createIterResult(value, false));
        }
        if (error) {
          const p = Promise.reject(error);
          error = null;
          return p;
        }
        if (finished) {
          return Promise.resolve(createIterResult(void 0, true));
        }
        return new Promise(function (resolve, reject) {
          unconsumedPromises.push({ resolve, reject });
        });
      },
      return() {
        emitter.removeListener(event, eventHandler);
        emitter.removeListener("error", errorHandler);
        finished = true;
        for (const promise of unconsumedPromises) {
          promise.resolve(createIterResult(void 0, true));
        }
        return Promise.resolve(createIterResult(void 0, true));
      },
      throw(err) {
        error = err;
        emitter.removeListener(event, eventHandler);
        emitter.removeListener("error", errorHandler);
      },
      [Symbol.asyncIterator]() {
        return this;
      },
    };
    emitter.on(event, eventHandler);
    emitter.on("error", errorHandler);
    return iterator;
    function eventHandler(...args) {
      const promise = unconsumedPromises.shift();
      if (promise) {
        promise.resolve(createIterResult(args, false));
      } else {
        unconsumedEventValues.push(args);
      }
    }
    function errorHandler(err) {
      finished = true;
      const toError = unconsumedPromises.shift();
      if (toError) {
        toError.reject(err);
      } else {
        error = err;
      }
      iterator.return();
    }
  }
  checkListenerArgument(listener) {
    if (typeof listener !== "function") {
      throw new ERR_INVALID_ARG_TYPE("listener", "function", listener);
    }
  }
  warnIfNeeded(eventName, warning) {
    const listeners = this._events[eventName];
    if (listeners.warned) {
      return;
    }
    listeners.warned = true;
    console.warn(warning);
    const maybeProcess = globalThis.process;
    if (maybeProcess instanceof _EventEmitter) {
      maybeProcess.emit("warning", warning);
    }
  }
  hasListeners(eventName) {
    return this._events && Boolean(this._events[eventName]);
  }
};
var EventEmitter = _EventEmitter;
_init = new WeakSet();
init_fn = function (emitter) {
  if (
    emitter._events == null ||
    emitter._events === Object.getPrototypeOf(emitter)._events
  ) {
    emitter._events = Object.create(null);
  }
};
__privateAdd(EventEmitter, _init);
EventEmitter.captureRejectionSymbol = Symbol.for("nodejs.rejection");
EventEmitter.errorMonitor = Symbol("events.errorMonitor");
EventEmitter.call = function call(thisArg) {
  var _a;
  __privateMethod((_a = _EventEmitter), _init, init_fn).call(_a, thisArg);
};
EventEmitter.prototype.on = EventEmitter.prototype.addListener;
EventEmitter.prototype.off = EventEmitter.prototype.removeListener;
var MaxListenersExceededWarning = class extends Error {
  constructor(emitter, type) {
    const listenerCount2 = emitter.listenerCount(type);
    const message =
      `Possible EventEmitter memory leak detected. ${listenerCount2} ${
        type == null ? "null" : type.toString()
      } listeners added to [${emitter.constructor.name}].  Use emitter.setMaxListeners() to increase limit`;
    super(message);
    this.emitter = emitter;
    this.type = type;
    this.count = listenerCount2;
    this.name = "MaxListenersExceededWarning";
  }
};
var events_default = Object.assign(EventEmitter, { EventEmitter });
var captureRejectionSymbol = EventEmitter.captureRejectionSymbol;
var errorMonitor = EventEmitter.errorMonitor;
var listenerCount = EventEmitter.listenerCount;
var on = EventEmitter.on;
var once = EventEmitter.once;
export {
  captureRejectionSymbol,
  defaultMaxListeners,
  errorMonitor,
  EventEmitter,
  events_default as default,
  listenerCount,
  on,
  once,
};
