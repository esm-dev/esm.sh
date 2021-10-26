/* deno mod bundle
 * entry: deno.land/std/node/stream.ts
 * version: 0.110.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild stream.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_stream.js
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

// _util/_util_types.ts
var util_types_exports = {};
__export(util_types_exports, {
  isAnyArrayBuffer: () => isAnyArrayBuffer,
  isArgumentsObject: () => isArgumentsObject,
  isArrayBuffer: () => isArrayBuffer,
  isArrayBufferView: () => isArrayBufferView,
  isAsyncFunction: () => isAsyncFunction,
  isBigInt64Array: () => isBigInt64Array,
  isBigIntObject: () => isBigIntObject,
  isBigUint64Array: () => isBigUint64Array,
  isBooleanObject: () => isBooleanObject,
  isBoxedPrimitive: () => isBoxedPrimitive,
  isDataView: () => isDataView,
  isDate: () => isDate,
  isFloat32Array: () => isFloat32Array,
  isFloat64Array: () => isFloat64Array,
  isGeneratorFunction: () => isGeneratorFunction,
  isGeneratorObject: () => isGeneratorObject,
  isInt16Array: () => isInt16Array,
  isInt32Array: () => isInt32Array,
  isInt8Array: () => isInt8Array,
  isMap: () => isMap,
  isMapIterator: () => isMapIterator,
  isModuleNamespaceObject: () => isModuleNamespaceObject,
  isNativeError: () => isNativeError,
  isNumberObject: () => isNumberObject,
  isPromise: () => isPromise,
  isRegExp: () => isRegExp,
  isSet: () => isSet,
  isSetIterator: () => isSetIterator,
  isSharedArrayBuffer: () => isSharedArrayBuffer,
  isStringObject: () => isStringObject,
  isSymbolObject: () => isSymbolObject,
  isTypedArray: () => isTypedArray,
  isUint16Array: () => isUint16Array,
  isUint32Array: () => isUint32Array,
  isUint8Array: () => isUint8Array,
  isUint8ClampedArray: () => isUint8ClampedArray,
  isWeakMap: () => isWeakMap,
  isWeakSet: () => isWeakSet,
});
var _toString = Object.prototype.toString;
var _isObjectLike = (value) => value !== null && typeof value === "object";
var _isFunctionLike = (value) => value !== null && typeof value === "function";
function isAnyArrayBuffer(value) {
  return (
    _isObjectLike(value) &&
    (_toString.call(value) === "[object ArrayBuffer]" ||
      _toString.call(value) === "[object SharedArrayBuffer]")
  );
}
function isArrayBufferView(value) {
  return ArrayBuffer.isView(value);
}
function isArgumentsObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Arguments]";
}
function isArrayBuffer(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object ArrayBuffer]";
}
function isAsyncFunction(value) {
  return _isFunctionLike(value) &&
    _toString.call(value) === "[object AsyncFunction]";
}
function isBigInt64Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object BigInt64Array]";
}
function isBigUint64Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object BigUint64Array]";
}
function isBooleanObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Boolean]";
}
function isBoxedPrimitive(value) {
  return (
    isBooleanObject(value) ||
    isStringObject(value) ||
    isNumberObject(value) ||
    isSymbolObject(value) ||
    isBigIntObject(value)
  );
}
function isDataView(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object DataView]";
}
function isDate(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Date]";
}
function isFloat32Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Float32Array]";
}
function isFloat64Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Float64Array]";
}
function isGeneratorFunction(value) {
  return _isFunctionLike(value) &&
    _toString.call(value) === "[object GeneratorFunction]";
}
function isGeneratorObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Generator]";
}
function isInt8Array(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Int8Array]";
}
function isInt16Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Int16Array]";
}
function isInt32Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Int32Array]";
}
function isMap(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Map]";
}
function isMapIterator(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Map Iterator]";
}
function isModuleNamespaceObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Module]";
}
function isNativeError(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Error]";
}
function isNumberObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Number]";
}
function isBigIntObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object BigInt]";
}
function isPromise(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Promise]";
}
function isRegExp(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object RegExp]";
}
function isSet(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Set]";
}
function isSetIterator(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Set Iterator]";
}
function isSharedArrayBuffer(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object SharedArrayBuffer]";
}
function isStringObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object String]";
}
function isSymbolObject(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object Symbol]";
}
function isTypedArray(value) {
  const reTypedTag =
    /^\[object (?:Float(?:32|64)|(?:Int|Uint)(?:8|16|32)|Uint8Clamped)Array\]$/;
  return _isObjectLike(value) && reTypedTag.test(_toString.call(value));
}
function isUint8Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Uint8Array]";
}
function isUint8ClampedArray(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Uint8ClampedArray]";
}
function isUint16Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Uint16Array]";
}
function isUint32Array(value) {
  return _isObjectLike(value) &&
    _toString.call(value) === "[object Uint32Array]";
}
function isWeakMap(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object WeakMap]";
}
function isWeakSet(value) {
  return _isObjectLike(value) && _toString.call(value) === "[object WeakSet]";
}

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

// _utils.ts
function notImplemented(msg) {
  const message = msg ? `Not implemented: ${msg}` : "Not implemented";
  throw new Error(message);
}
function normalizeEncoding(enc) {
  if (enc == null || enc === "utf8" || enc === "utf-8") return "utf8";
  return slowCases(enc);
}
function slowCases(enc) {
  switch (enc.length) {
    case 4:
      if (enc === "UTF8") return "utf8";
      if (enc === "ucs2" || enc === "UCS2") return "utf16le";
      enc = `${enc}`.toLowerCase();
      if (enc === "utf8") return "utf8";
      if (enc === "ucs2") return "utf16le";
      break;
    case 3:
      if (enc === "hex" || enc === "HEX" || `${enc}`.toLowerCase() === "hex") {
        return "hex";
      }
      break;
    case 5:
      if (enc === "ascii") return "ascii";
      if (enc === "ucs-2") return "utf16le";
      if (enc === "UTF-8") return "utf8";
      if (enc === "ASCII") return "ascii";
      if (enc === "UCS-2") return "utf16le";
      enc = `${enc}`.toLowerCase();
      if (enc === "utf-8") return "utf8";
      if (enc === "ascii") return "ascii";
      if (enc === "ucs-2") return "utf16le";
      break;
    case 6:
      if (enc === "base64") return "base64";
      if (enc === "latin1" || enc === "binary") return "latin1";
      if (enc === "BASE64") return "base64";
      if (enc === "LATIN1" || enc === "BINARY") return "latin1";
      enc = `${enc}`.toLowerCase();
      if (enc === "base64") return "base64";
      if (enc === "latin1" || enc === "binary") return "latin1";
      break;
    case 7:
      if (
        enc === "utf16le" || enc === "UTF16LE" ||
        `${enc}`.toLowerCase() === "utf16le"
      ) {
        return "utf16le";
      }
      break;
    case 8:
      if (
        enc === "utf-16le" || enc === "UTF-16LE" ||
        `${enc}`.toLowerCase() === "utf-16le"
      ) {
        return "utf16le";
      }
      break;
    default:
      if (enc === "") return "utf8";
  }
}
function once(callback) {
  let called = false;
  return function (...args) {
    if (called) return;
    called = true;
    callback.apply(this, args);
  };
}

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
var NodeError = class extends NodeErrorAbstraction {
  constructor(code, message) {
    super(Error.prototype.name, code, message);
  }
};
var NodeRangeError = class extends NodeErrorAbstraction {
  constructor(code, message) {
    super(RangeError.prototype.name, code, message);
    Object.setPrototypeOf(this, RangeError.prototype);
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
var ERR_BUFFER_OUT_OF_BOUNDS = class extends NodeRangeError {
  constructor(name) {
    super(
      "ERR_BUFFER_OUT_OF_BOUNDS",
      name
        ? `"${name}" is outside of buffer bounds`
        : "Attempt to access memory outside buffer bounds",
    );
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
var ERR_INVALID_CALLBACK = class extends NodeTypeError {
  constructor(object) {
    super(
      "ERR_INVALID_CALLBACK",
      `Callback must be a function. Received ${JSON.stringify(object)}`,
    );
  }
};
var ERR_METHOD_NOT_IMPLEMENTED = class extends NodeError {
  constructor(x) {
    super("ERR_METHOD_NOT_IMPLEMENTED", `The ${x} method is not implemented`);
  }
};
var ERR_MISSING_ARGS = class extends NodeTypeError {
  constructor(...args) {
    args = args.map((a) => `"${a}"`);
    let msg = "The ";
    switch (args.length) {
      case 1:
        msg += `${args[0]} argument`;
        break;
      case 2:
        msg += `${args[0]} and ${args[1]} arguments`;
        break;
      default:
        msg += args.slice(0, args.length - 1).join(", ");
        msg += `, and ${args[args.length - 1]} arguments`;
        break;
    }
    super("ERR_MISSING_ARGS", `${msg} must be specified`);
  }
};
var ERR_MULTIPLE_CALLBACK = class extends NodeError {
  constructor() {
    super("ERR_MULTIPLE_CALLBACK", `Callback called multiple times`);
  }
};
var ERR_STREAM_ALREADY_FINISHED = class extends NodeError {
  constructor(x) {
    super(
      "ERR_STREAM_ALREADY_FINISHED",
      `Cannot call ${x} after a stream was finished`,
    );
  }
};
var ERR_STREAM_CANNOT_PIPE = class extends NodeError {
  constructor() {
    super("ERR_STREAM_CANNOT_PIPE", `Cannot pipe, not readable`);
  }
};
var ERR_STREAM_DESTROYED = class extends NodeError {
  constructor(x) {
    super(
      "ERR_STREAM_DESTROYED",
      `Cannot call ${x} after a stream was destroyed`,
    );
  }
};
var ERR_STREAM_NULL_VALUES = class extends NodeTypeError {
  constructor() {
    super("ERR_STREAM_NULL_VALUES", `May not write null values to stream`);
  }
};
var ERR_STREAM_PREMATURE_CLOSE = class extends NodeError {
  constructor() {
    super("ERR_STREAM_PREMATURE_CLOSE", `Premature close`);
  }
};
var ERR_STREAM_PUSH_AFTER_EOF = class extends NodeError {
  constructor() {
    super("ERR_STREAM_PUSH_AFTER_EOF", `stream.push() after EOF`);
  }
};
var ERR_STREAM_UNSHIFT_AFTER_END_EVENT = class extends NodeError {
  constructor() {
    super(
      "ERR_STREAM_UNSHIFT_AFTER_END_EVENT",
      `stream.unshift() after end event`,
    );
  }
};
var ERR_STREAM_WRITE_AFTER_END = class extends NodeError {
  constructor() {
    super("ERR_STREAM_WRITE_AFTER_END", `write after end`);
  }
};
var ERR_UNKNOWN_ENCODING = class extends NodeTypeError {
  constructor(x) {
    super("ERR_UNKNOWN_ENCODING", `Unknown encoding: ${x}`);
  }
};
var ERR_INVALID_OPT_VALUE = class extends NodeTypeError {
  constructor(name, value) {
    super(
      "ERR_INVALID_OPT_VALUE",
      `The value "${value}" is invalid for option "${name}"`,
    );
  }
};
function buildReturnPropertyType(value) {
  if (value && value.constructor && value.constructor.name) {
    return `instance of ${value.constructor.name}`;
  } else {
    return `type ${typeof value}`;
  }
}
var ERR_INVALID_RETURN_VALUE = class extends NodeTypeError {
  constructor(input, name, value) {
    super(
      "ERR_INVALID_RETURN_VALUE",
      `Expected ${input} to be returned from the "${name}" function but got ${
        buildReturnPropertyType(value)
      }.`,
    );
  }
};

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
    var _a4;
    __privateMethod((_a4 = _EventEmitter), _init, init_fn).call(_a4, this);
  }
  _addListener(eventName, listener, prepend) {
    var _a4;
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
      __privateMethod((_a4 = _EventEmitter), _init, init_fn).call(_a4, this);
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
    let finished2 = false;
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
        if (finished2) {
          return Promise.resolve(createIterResult(void 0, true));
        }
        return new Promise(function (resolve, reject) {
          unconsumedPromises.push({ resolve, reject });
        });
      },
      return() {
        emitter.removeListener(event, eventHandler);
        emitter.removeListener("error", errorHandler);
        finished2 = true;
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
      finished2 = true;
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
  var _a4;
  __privateMethod((_a4 = _EventEmitter), _init, init_fn).call(_a4, thisArg);
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
var once2 = EventEmitter.once;

// ../encoding/hex.ts
var hexTable = new TextEncoder().encode("0123456789abcdef");
function errInvalidByte(byte) {
  return new TypeError(`Invalid byte '${String.fromCharCode(byte)}'`);
}
function errLength() {
  return new RangeError("Odd length hex string");
}
function fromHexChar(byte) {
  if (48 <= byte && byte <= 57) return byte - 48;
  if (97 <= byte && byte <= 102) return byte - 97 + 10;
  if (65 <= byte && byte <= 70) return byte - 65 + 10;
  throw errInvalidByte(byte);
}
function encode(src) {
  const dst = new Uint8Array(src.length * 2);
  for (let i = 0; i < dst.length; i++) {
    const v = src[i];
    dst[i * 2] = hexTable[v >> 4];
    dst[i * 2 + 1] = hexTable[v & 15];
  }
  return dst;
}
function decode(src) {
  const dst = new Uint8Array(src.length / 2);
  for (let i = 0; i < dst.length; i++) {
    const a = fromHexChar(src[i * 2]);
    const b = fromHexChar(src[i * 2 + 1]);
    dst[i] = (a << 4) | b;
  }
  if (src.length % 2 == 1) {
    fromHexChar(src[dst.length * 2]);
    throw errLength();
  }
  return dst;
}

// ../encoding/base64.ts
var base64abc = [
  "A",
  "B",
  "C",
  "D",
  "E",
  "F",
  "G",
  "H",
  "I",
  "J",
  "K",
  "L",
  "M",
  "N",
  "O",
  "P",
  "Q",
  "R",
  "S",
  "T",
  "U",
  "V",
  "W",
  "X",
  "Y",
  "Z",
  "a",
  "b",
  "c",
  "d",
  "e",
  "f",
  "g",
  "h",
  "i",
  "j",
  "k",
  "l",
  "m",
  "n",
  "o",
  "p",
  "q",
  "r",
  "s",
  "t",
  "u",
  "v",
  "w",
  "x",
  "y",
  "z",
  "0",
  "1",
  "2",
  "3",
  "4",
  "5",
  "6",
  "7",
  "8",
  "9",
  "+",
  "/",
];
function encode2(data) {
  const uint8 = typeof data === "string"
    ? new TextEncoder().encode(data)
    : data instanceof Uint8Array
    ? data
    : new Uint8Array(data);
  let result = "",
    i;
  const l = uint8.length;
  for (i = 2; i < l; i += 3) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[((uint8[i - 2] & 3) << 4) | (uint8[i - 1] >> 4)];
    result += base64abc[((uint8[i - 1] & 15) << 2) | (uint8[i] >> 6)];
    result += base64abc[uint8[i] & 63];
  }
  if (i === l + 1) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[(uint8[i - 2] & 3) << 4];
    result += "==";
  }
  if (i === l) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[((uint8[i - 2] & 3) << 4) | (uint8[i - 1] >> 4)];
    result += base64abc[(uint8[i - 1] & 15) << 2];
    result += "=";
  }
  return result;
}
function decode2(b64) {
  const binString = atob(b64);
  const size = binString.length;
  const bytes = new Uint8Array(size);
  for (let i = 0; i < size; i++) {
    bytes[i] = binString.charCodeAt(i);
  }
  return bytes;
}

// buffer.ts
var notImplementedEncodings = ["ascii", "binary", "latin1", "ucs2", "utf16le"];
function checkEncoding(encoding = "utf8", strict = true) {
  if (typeof encoding !== "string" || (strict && encoding === "")) {
    if (!strict) return "utf8";
    throw new TypeError(`Unknown encoding: ${encoding}`);
  }
  const normalized = normalizeEncoding(encoding);
  if (normalized === void 0) {
    throw new TypeError(`Unknown encoding: ${encoding}`);
  }
  if (notImplementedEncodings.includes(encoding)) {
    notImplemented(`"${encoding}" encoding`);
  }
  return normalized;
}
var encodingOps = {
  utf8: {
    byteLength: (string) => new TextEncoder().encode(string).byteLength,
  },
  ucs2: {
    byteLength: (string) => string.length * 2,
  },
  utf16le: {
    byteLength: (string) => string.length * 2,
  },
  latin1: {
    byteLength: (string) => string.length,
  },
  ascii: {
    byteLength: (string) => string.length,
  },
  base64: {
    byteLength: (string) => base64ByteLength(string, string.length),
  },
  hex: {
    byteLength: (string) => string.length >>> 1,
  },
};
function base64ByteLength(str, bytes) {
  if (str.charCodeAt(bytes - 1) === 61) bytes--;
  if (bytes > 1 && str.charCodeAt(bytes - 1) === 61) bytes--;
  return (bytes * 3) >>> 2;
}
var Buffer3 = class extends Uint8Array {
  static alloc(size, fill, encoding = "utf8") {
    if (typeof size !== "number") {
      throw new TypeError(
        `The "size" argument must be of type number. Received type ${typeof size}`,
      );
    }
    const buf = new Buffer3(size);
    if (size === 0) return buf;
    let bufFill;
    if (typeof fill === "string") {
      const clearEncoding = checkEncoding(encoding);
      if (
        typeof fill === "string" && fill.length === 1 &&
        clearEncoding === "utf8"
      ) {
        buf.fill(fill.charCodeAt(0));
      } else bufFill = Buffer3.from(fill, clearEncoding);
    } else if (typeof fill === "number") {
      buf.fill(fill);
    } else if (fill instanceof Uint8Array) {
      if (fill.length === 0) {
        throw new TypeError(
          `The argument "value" is invalid. Received ${fill.constructor.name} []`,
        );
      }
      bufFill = fill;
    }
    if (bufFill) {
      if (bufFill.length > buf.length) {
        bufFill = bufFill.subarray(0, buf.length);
      }
      let offset = 0;
      while (offset < size) {
        buf.set(bufFill, offset);
        offset += bufFill.length;
        if (offset + bufFill.length >= size) break;
      }
      if (offset !== size) {
        buf.set(bufFill.subarray(0, size - offset), offset);
      }
    }
    return buf;
  }
  static allocUnsafe(size) {
    return new Buffer3(size);
  }
  static byteLength(string, encoding = "utf8") {
    if (typeof string != "string") return string.byteLength;
    encoding = normalizeEncoding(encoding) || "utf8";
    return encodingOps[encoding].byteLength(string);
  }
  static concat(list, totalLength) {
    if (totalLength == void 0) {
      totalLength = 0;
      for (const buf of list) {
        totalLength += buf.length;
      }
    }
    const buffer = Buffer3.allocUnsafe(totalLength);
    let pos = 0;
    for (const item of list) {
      let buf;
      if (!(item instanceof Buffer3)) {
        buf = Buffer3.from(item);
      } else {
        buf = item;
      }
      buf.copy(buffer, pos);
      pos += buf.length;
    }
    return buffer;
  }
  static from(value, offsetOrEncoding, length) {
    const offset = typeof offsetOrEncoding === "string"
      ? void 0
      : offsetOrEncoding;
    let encoding = typeof offsetOrEncoding === "string"
      ? offsetOrEncoding
      : void 0;
    if (typeof value == "string") {
      encoding = checkEncoding(encoding, false);
      if (encoding === "hex") {
        return new Buffer3(decode(new TextEncoder().encode(value)).buffer);
      }
      if (encoding === "base64") return new Buffer3(decode2(value).buffer);
      return new Buffer3(new TextEncoder().encode(value).buffer);
    }
    return new Buffer3(value, offset, length);
  }
  static isBuffer(obj) {
    return obj instanceof Buffer3;
  }
  static isEncoding(encoding) {
    return typeof encoding === "string" && encoding.length !== 0 &&
      normalizeEncoding(encoding) !== void 0;
  }
  boundsError(value, length, type) {
    if (Math.floor(value) !== value) {
      throw new ERR_OUT_OF_RANGE(type || "offset", "an integer", value);
    }
    if (length < 0) throw new ERR_BUFFER_OUT_OF_BOUNDS();
    throw new ERR_OUT_OF_RANGE(
      type || "offset",
      `>= ${type ? 1 : 0} and <= ${length}`,
      value,
    );
  }
  readUIntBE(offset = 0, byteLength) {
    if (byteLength === 3 || byteLength === 5 || byteLength === 6) {
      notImplemented(`byteLength ${byteLength}`);
    }
    if (byteLength === 4) return this.readUInt32BE(offset);
    if (byteLength === 2) return this.readUInt16BE(offset);
    if (byteLength === 1) return this.readUInt8(offset);
    this.boundsError(byteLength, 4, "byteLength");
  }
  readUIntLE(offset = 0, byteLength) {
    if (byteLength === 3 || byteLength === 5 || byteLength === 6) {
      notImplemented(`byteLength ${byteLength}`);
    }
    if (byteLength === 4) return this.readUInt32LE(offset);
    if (byteLength === 2) return this.readUInt16LE(offset);
    if (byteLength === 1) return this.readUInt8(offset);
    this.boundsError(byteLength, 4, "byteLength");
  }
  copy(
    targetBuffer,
    targetStart = 0,
    sourceStart = 0,
    sourceEnd = this.length,
  ) {
    const sourceBuffer = this.subarray(sourceStart, sourceEnd).subarray(
      0,
      Math.max(0, targetBuffer.length - targetStart),
    );
    if (sourceBuffer.length === 0) return 0;
    targetBuffer.set(sourceBuffer, targetStart);
    return sourceBuffer.length;
  }
  equals(otherBuffer) {
    if (!(otherBuffer instanceof Uint8Array)) {
      throw new TypeError(
        `The "otherBuffer" argument must be an instance of Buffer or Uint8Array. Received type ${typeof otherBuffer}`,
      );
    }
    if (this === otherBuffer) return true;
    if (this.byteLength !== otherBuffer.byteLength) return false;
    for (let i = 0; i < this.length; i++) {
      if (this[i] !== otherBuffer[i]) return false;
    }
    return true;
  }
  readBigInt64BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getBigInt64(offset);
  }
  readBigInt64LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getBigInt64(offset, true);
  }
  readBigUInt64BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getBigUint64(offset);
  }
  readBigUInt64LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getBigUint64(offset, true);
  }
  readDoubleBE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getFloat64(offset);
  }
  readDoubleLE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getFloat64(offset, true);
  }
  readFloatBE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getFloat32(offset);
  }
  readFloatLE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getFloat32(offset, true);
  }
  readInt8(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt8(
      offset,
    );
  }
  readInt16BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt16(
      offset,
    );
  }
  readInt16LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt16(
      offset,
      true,
    );
  }
  readInt32BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt32(
      offset,
    );
  }
  readInt32LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt32(
      offset,
      true,
    );
  }
  readUInt8(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint8(
      offset,
    );
  }
  readUInt16BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getUint16(offset);
  }
  readUInt16LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getUint16(offset, true);
  }
  readUInt32BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getUint32(offset);
  }
  readUInt32LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength)
      .getUint32(offset, true);
  }
  slice(begin = 0, end = this.length) {
    return this.subarray(begin, end);
  }
  toJSON() {
    return { type: "Buffer", data: Array.from(this) };
  }
  toString(encoding = "utf8", start = 0, end = this.length) {
    encoding = checkEncoding(encoding);
    const b = this.subarray(start, end);
    if (encoding === "hex") return new TextDecoder().decode(encode(b));
    if (encoding === "base64") return encode2(b);
    return new TextDecoder(encoding).decode(b);
  }
  write(string, offset = 0, length = this.length) {
    return new TextEncoder().encodeInto(
      string,
      this.subarray(offset, offset + length),
    ).written;
  }
  writeBigInt64BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigInt64(
      offset,
      value,
    );
    return offset + 4;
  }
  writeBigInt64LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigInt64(
      offset,
      value,
      true,
    );
    return offset + 4;
  }
  writeBigUInt64BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigUint64(
      offset,
      value,
    );
    return offset + 4;
  }
  writeBigUInt64LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigUint64(
      offset,
      value,
      true,
    );
    return offset + 4;
  }
  writeDoubleBE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat64(
      offset,
      value,
    );
    return offset + 8;
  }
  writeDoubleLE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat64(
      offset,
      value,
      true,
    );
    return offset + 8;
  }
  writeFloatBE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat32(
      offset,
      value,
    );
    return offset + 4;
  }
  writeFloatLE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat32(
      offset,
      value,
      true,
    );
    return offset + 4;
  }
  writeInt8(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt8(
      offset,
      value,
    );
    return offset + 1;
  }
  writeInt16BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt16(
      offset,
      value,
    );
    return offset + 2;
  }
  writeInt16LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt16(
      offset,
      value,
      true,
    );
    return offset + 2;
  }
  writeInt32BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(
      offset,
      value,
    );
    return offset + 4;
  }
  writeInt32LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt32(
      offset,
      value,
      true,
    );
    return offset + 4;
  }
  writeUInt8(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint8(
      offset,
      value,
    );
    return offset + 1;
  }
  writeUInt16BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint16(
      offset,
      value,
    );
    return offset + 2;
  }
  writeUInt16LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint16(
      offset,
      value,
      true,
    );
    return offset + 2;
  }
  writeUInt32BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(
      offset,
      value,
    );
    return offset + 4;
  }
  writeUInt32LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(
      offset,
      value,
      true,
    );
    return offset + 4;
  }
};
var atob2 = globalThis.atob;
var btoa = globalThis.btoa;

// _stream/stream.ts
var Stream = class extends events_default {
  constructor() {
    super();
  }
  pipe(dest, options) {
    const source = this;
    if (options?.end ?? true) {
      source.on("end", onend);
      source.on("close", onclose);
    }
    let didOnEnd = false;
    function onend() {
      if (didOnEnd) return;
      didOnEnd = true;
      dest.end();
    }
    function onclose() {
      if (didOnEnd) return;
      didOnEnd = true;
      if (typeof dest.destroy === "function") dest.destroy();
    }
    function onerror(er) {
      cleanup();
      if (this.listenerCount("error") === 0) {
        throw er;
      }
    }
    source.on("error", onerror);
    dest.on("error", onerror);
    function cleanup() {
      source.removeListener("end", onend);
      source.removeListener("close", onclose);
      source.removeListener("error", onerror);
      dest.removeListener("error", onerror);
      source.removeListener("end", cleanup);
      source.removeListener("close", cleanup);
      dest.removeListener("close", cleanup);
    }
    source.on("end", cleanup);
    source.on("close", cleanup);
    dest.on("close", cleanup);
    dest.emit("pipe", source);
    return dest;
  }
};
Stream._isUint8Array = util_types_exports.isUint8Array;
Stream._uint8ArrayToBuffer = (chunk) => Buffer3.from(chunk);
var stream_default = Stream;

// _stream/buffer_list.ts
var BufferList = class {
  constructor() {
    this.head = null;
    this.tail = null;
    this.head = null;
    this.tail = null;
    this.length = 0;
  }
  push(v) {
    const entry = { data: v, next: null };
    if (this.length > 0) {
      this.tail.next = entry;
    } else {
      this.head = entry;
    }
    this.tail = entry;
    ++this.length;
  }
  unshift(v) {
    const entry = { data: v, next: this.head };
    if (this.length === 0) {
      this.tail = entry;
    }
    this.head = entry;
    ++this.length;
  }
  shift() {
    if (this.length === 0) {
      return;
    }
    const ret = this.head.data;
    if (this.length === 1) {
      this.head = this.tail = null;
    } else {
      this.head = this.head.next;
    }
    --this.length;
    return ret;
  }
  clear() {
    this.head = this.tail = null;
    this.length = 0;
  }
  join(s) {
    if (this.length === 0) {
      return "";
    }
    let p = this.head;
    let ret = "" + p.data;
    p = p.next;
    while (p) {
      ret += s + p.data;
      p = p.next;
    }
    return ret;
  }
  concat(n) {
    if (this.length === 0) {
      return Buffer3.alloc(0);
    }
    const ret = Buffer3.allocUnsafe(n >>> 0);
    let p = this.head;
    let i = 0;
    while (p) {
      ret.set(p.data, i);
      i += p.data.length;
      p = p.next;
    }
    return ret;
  }
  consume(n, hasStrings) {
    const data = this.head.data;
    if (n < data.length) {
      const slice = data.slice(0, n);
      this.head.data = data.slice(n);
      return slice;
    }
    if (n === data.length) {
      return this.shift();
    }
    return hasStrings ? this._getString(n) : this._getBuffer(n);
  }
  first() {
    return this.head.data;
  }
  *[Symbol.iterator]() {
    for (let p = this.head; p; p = p.next) {
      yield p.data;
    }
  }
  _getString(n) {
    let ret = "";
    let p = this.head;
    let c = 0;
    p = p.next;
    do {
      const str = p.data;
      if (n > str.length) {
        ret += str;
        n -= str.length;
      } else {
        if (n === str.length) {
          ret += str;
          ++c;
          if (p.next) {
            this.head = p.next;
          } else {
            this.head = this.tail = null;
          }
        } else {
          ret += str.slice(0, n);
          this.head = p;
          p.data = str.slice(n);
        }
        break;
      }
      ++c;
      p = p.next;
    } while (p);
    this.length -= c;
    return ret;
  }
  _getBuffer(n) {
    const ret = Buffer3.allocUnsafe(n);
    const retLen = n;
    let p = this.head;
    let c = 0;
    p = p.next;
    do {
      const buf = p.data;
      if (n > buf.length) {
        ret.set(buf, retLen - n);
        n -= buf.length;
      } else {
        if (n === buf.length) {
          ret.set(buf, retLen - n);
          ++c;
          if (p.next) {
            this.head = p.next;
          } else {
            this.head = this.tail = null;
          }
        } else {
          ret.set(new Uint8Array(buf.buffer, buf.byteOffset, n), retLen - n);
          this.head = p;
          p.data = buf.slice(n);
        }
        break;
      }
      ++c;
      p = p.next;
    } while (p);
    this.length -= c;
    return ret;
  }
};

// string_decoder.ts
var NotImplemented;
(function (NotImplemented2) {
  NotImplemented2[(NotImplemented2["ascii"] = 0)] = "ascii";
  NotImplemented2[(NotImplemented2["latin1"] = 1)] = "latin1";
  NotImplemented2[(NotImplemented2["utf16le"] = 2)] = "utf16le";
})(NotImplemented || (NotImplemented = {}));
function normalizeEncoding2(enc) {
  const encoding = normalizeEncoding(enc ?? null);
  if (encoding && encoding in NotImplemented) notImplemented(encoding);
  if (!encoding && typeof enc === "string" && enc.toLowerCase() !== "raw") {
    throw new Error(`Unknown encoding: ${enc}`);
  }
  return String(encoding);
}
function utf8CheckByte(byte) {
  if (byte <= 127) return 0;
  else if (byte >> 5 === 6) return 2;
  else if (byte >> 4 === 14) return 3;
  else if (byte >> 3 === 30) return 4;
  return byte >> 6 === 2 ? -1 : -2;
}
function utf8CheckIncomplete(self, buf, i) {
  let j = buf.length - 1;
  if (j < i) return 0;
  let nb = utf8CheckByte(buf[j]);
  if (nb >= 0) {
    if (nb > 0) self.lastNeed = nb - 1;
    return nb;
  }
  if (--j < i || nb === -2) return 0;
  nb = utf8CheckByte(buf[j]);
  if (nb >= 0) {
    if (nb > 0) self.lastNeed = nb - 2;
    return nb;
  }
  if (--j < i || nb === -2) return 0;
  nb = utf8CheckByte(buf[j]);
  if (nb >= 0) {
    if (nb > 0) {
      if (nb === 2) nb = 0;
      else self.lastNeed = nb - 3;
    }
    return nb;
  }
  return 0;
}
function utf8CheckExtraBytes(self, buf) {
  if ((buf[0] & 192) !== 128) {
    self.lastNeed = 0;
    return "\uFFFD";
  }
  if (self.lastNeed > 1 && buf.length > 1) {
    if ((buf[1] & 192) !== 128) {
      self.lastNeed = 1;
      return "\uFFFD";
    }
    if (self.lastNeed > 2 && buf.length > 2) {
      if ((buf[2] & 192) !== 128) {
        self.lastNeed = 2;
        return "\uFFFD";
      }
    }
  }
}
function utf8FillLastComplete(buf) {
  const p = this.lastTotal - this.lastNeed;
  const r = utf8CheckExtraBytes(this, buf);
  if (r !== void 0) return r;
  if (this.lastNeed <= buf.length) {
    buf.copy(this.lastChar, p, 0, this.lastNeed);
    return this.lastChar.toString(this.encoding, 0, this.lastTotal);
  }
  buf.copy(this.lastChar, p, 0, buf.length);
  this.lastNeed -= buf.length;
}
function utf8FillLastIncomplete(buf) {
  if (this.lastNeed <= buf.length) {
    buf.copy(this.lastChar, this.lastTotal - this.lastNeed, 0, this.lastNeed);
    return this.lastChar.toString(this.encoding, 0, this.lastTotal);
  }
  buf.copy(this.lastChar, this.lastTotal - this.lastNeed, 0, buf.length);
  this.lastNeed -= buf.length;
}
function utf8Text(buf, i) {
  const total = utf8CheckIncomplete(this, buf, i);
  if (!this.lastNeed) return buf.toString("utf8", i);
  this.lastTotal = total;
  const end = buf.length - (total - this.lastNeed);
  buf.copy(this.lastChar, 0, end);
  return buf.toString("utf8", i, end);
}
function utf8End(buf) {
  const r = buf && buf.length ? this.write(buf) : "";
  if (this.lastNeed) return r + "\uFFFD";
  return r;
}
function utf8Write(buf) {
  if (typeof buf === "string") {
    return buf;
  }
  if (buf.length === 0) return "";
  let r;
  let i;
  if (this.lastNeed) {
    r = this.fillLast(buf);
    if (r === void 0) return "";
    i = this.lastNeed;
    this.lastNeed = 0;
  } else {
    i = 0;
  }
  if (i < buf.length) return r ? r + this.text(buf, i) : this.text(buf, i);
  return r || "";
}
function base64Text(buf, i) {
  const n = (buf.length - i) % 3;
  if (n === 0) return buf.toString("base64", i);
  this.lastNeed = 3 - n;
  this.lastTotal = 3;
  if (n === 1) {
    this.lastChar[0] = buf[buf.length - 1];
  } else {
    this.lastChar[0] = buf[buf.length - 2];
    this.lastChar[1] = buf[buf.length - 1];
  }
  return buf.toString("base64", i, buf.length - n);
}
function base64End(buf) {
  const r = buf && buf.length ? this.write(buf) : "";
  if (this.lastNeed) {
    return r + this.lastChar.toString("base64", 0, 3 - this.lastNeed);
  }
  return r;
}
function simpleWrite(buf) {
  if (typeof buf === "string") {
    return buf;
  }
  return buf.toString(this.encoding);
}
function simpleEnd(buf) {
  return buf && buf.length ? this.write(buf) : "";
}
var StringDecoderBase = class {
  constructor(encoding, nb) {
    this.encoding = encoding;
    this.lastNeed = 0;
    this.lastTotal = 0;
    this.lastChar = Buffer3.allocUnsafe(nb);
  }
};
var Base64Decoder = class extends StringDecoderBase {
  constructor(encoding) {
    super(normalizeEncoding2(encoding), 3);
    this.end = base64End;
    this.fillLast = utf8FillLastIncomplete;
    this.text = base64Text;
    this.write = utf8Write;
  }
};
var GenericDecoder = class extends StringDecoderBase {
  constructor(encoding) {
    super(normalizeEncoding2(encoding), 4);
    this.end = simpleEnd;
    this.fillLast = void 0;
    this.text = utf8Text;
    this.write = simpleWrite;
  }
};
var Utf8Decoder = class extends StringDecoderBase {
  constructor(encoding) {
    super(normalizeEncoding2(encoding), 4);
    this.end = utf8End;
    this.fillLast = utf8FillLastComplete;
    this.text = utf8Text;
    this.write = utf8Write;
  }
};
var StringDecoder = class {
  constructor(encoding) {
    let decoder;
    switch (encoding) {
      case "utf8":
        decoder = new Utf8Decoder(encoding);
        break;
      case "base64":
        decoder = new Base64Decoder(encoding);
        break;
      default:
        decoder = new GenericDecoder(encoding);
    }
    this.encoding = decoder.encoding;
    this.end = decoder.end;
    this.fillLast = decoder.fillLast;
    this.lastChar = decoder.lastChar;
    this.lastNeed = decoder.lastNeed;
    this.lastTotal = decoder.lastTotal;
    this.text = decoder.text;
    this.write = decoder.write;
  }
};

// _stream/end_of_stream.ts
function isReadable(stream) {
  return typeof stream.readable === "boolean" ||
    typeof stream.readableEnded === "boolean" || !!stream._readableState;
}
function isWritable(stream) {
  return typeof stream.writable === "boolean" ||
    typeof stream.writableEnded === "boolean" || !!stream._writableState;
}
function isWritableFinished(stream) {
  if (stream.writableFinished) return true;
  const wState = stream._writableState;
  if (!wState || wState.errored) return false;
  return wState.finished || (wState.ended && wState.length === 0);
}
function nop() {}
function isReadableEnded(stream) {
  if (stream.readableEnded) return true;
  const rState = stream._readableState;
  if (!rState || rState.errored) return false;
  return rState.endEmitted || (rState.ended && rState.length === 0);
}
function eos(stream, x, y) {
  let opts;
  let callback;
  if (!y) {
    if (typeof x !== "function") {
      throw new ERR_INVALID_ARG_TYPE("callback", "function", x);
    }
    opts = {};
    callback = x;
  } else {
    if (!x || Array.isArray(x) || typeof x !== "object") {
      throw new ERR_INVALID_ARG_TYPE("opts", "object", x);
    }
    opts = x;
    if (typeof y !== "function") {
      throw new ERR_INVALID_ARG_TYPE("callback", "function", y);
    }
    callback = y;
  }
  callback = once(callback);
  const readable = opts.readable ?? isReadable(stream);
  const writable = opts.writable ?? isWritable(stream);
  const wState = stream._writableState;
  const rState = stream._readableState;
  const validState = wState || rState;
  const onlegacyfinish = () => {
    if (!stream.writable) {
      onfinish();
    }
  };
  let willEmitClose = validState?.autoDestroy &&
    validState?.emitClose &&
    validState?.closed === false &&
    isReadable(stream) === readable &&
    isWritable(stream) === writable;
  let writableFinished = stream.writableFinished || wState?.finished;
  const onfinish = () => {
    writableFinished = true;
    if (stream.destroyed) {
      willEmitClose = false;
    }
    if (willEmitClose && (!stream.readable || readable)) {
      return;
    }
    if (!readable || readableEnded) {
      callback.call(stream);
    }
  };
  let readableEnded = stream.readableEnded || rState?.endEmitted;
  const onend = () => {
    readableEnded = true;
    if (stream.destroyed) {
      willEmitClose = false;
    }
    if (willEmitClose && (!stream.writable || writable)) {
      return;
    }
    if (!writable || writableFinished) {
      callback.call(stream);
    }
  };
  const onerror = (err) => {
    callback.call(stream, err);
  };
  const onclose = () => {
    if (readable && !readableEnded) {
      if (!isReadableEnded(stream)) {
        return callback.call(stream, new ERR_STREAM_PREMATURE_CLOSE());
      }
    }
    if (writable && !writableFinished) {
      if (!isWritableFinished(stream)) {
        return callback.call(stream, new ERR_STREAM_PREMATURE_CLOSE());
      }
    }
    callback.call(stream);
  };
  if (writable && !wState) {
    stream.on("end", onlegacyfinish);
    stream.on("close", onlegacyfinish);
  }
  stream.on("end", onend);
  stream.on("finish", onfinish);
  if (opts.error !== false) stream.on("error", onerror);
  stream.on("close", onclose);
  const closed = wState?.closed ||
    rState?.closed ||
    wState?.errorEmitted ||
    rState?.errorEmitted ||
    ((!writable || wState?.finished) && (!readable || rState?.endEmitted));
  if (closed) {
    queueMicrotask(callback);
  }
  return function () {
    callback = nop;
    stream.removeListener("aborted", onclose);
    stream.removeListener("complete", onfinish);
    stream.removeListener("abort", onclose);
    stream.removeListener("end", onlegacyfinish);
    stream.removeListener("close", onlegacyfinish);
    stream.removeListener("finish", onfinish);
    stream.removeListener("end", onend);
    stream.removeListener("error", onerror);
    stream.removeListener("close", onclose);
  };
}

// _stream/destroy.ts
function destroyer(stream, err) {
  if (typeof stream.destroy === "function") {
    return stream.destroy(err);
  }
  if (typeof stream.close === "function") {
    return stream.close();
  }
}

// _stream/async_iterator.ts
var kLastResolve = Symbol("lastResolve");
var kLastReject = Symbol("lastReject");
var kError = Symbol("error");
var kEnded = Symbol("ended");
var kLastPromise = Symbol("lastPromise");
var kHandlePromise = Symbol("handlePromise");
var kStream = Symbol("stream");
function initIteratorSymbols(o, symbols) {
  const properties = {};
  for (const sym in symbols) {
    properties[sym] = {
      configurable: false,
      enumerable: false,
      writable: true,
    };
  }
  Object.defineProperties(o, properties);
}
function createIterResult2(value, done) {
  return { value, done };
}
function readAndResolve(iter) {
  const resolve = iter[kLastResolve];
  if (resolve !== null) {
    const data = iter[kStream].read();
    if (data !== null) {
      iter[kLastPromise] = null;
      iter[kLastResolve] = null;
      iter[kLastReject] = null;
      resolve(createIterResult2(data, false));
    }
  }
}
function onReadable(iter) {
  queueMicrotask(() => readAndResolve(iter));
}
function wrapForNext(lastPromise, iter) {
  return (resolve, reject) => {
    lastPromise.then(() => {
      if (iter[kEnded]) {
        resolve(createIterResult2(void 0, true));
        return;
      }
      iter[kHandlePromise](resolve, reject);
    }, reject);
  };
}
function finish(self, err) {
  return new Promise((resolve, reject) => {
    const stream = self[kStream];
    eos(stream, (err2) => {
      if (err2 && err2.code !== "ERR_STREAM_PREMATURE_CLOSE") {
        reject(err2);
      } else {
        resolve(createIterResult2(void 0, true));
      }
    });
    destroyer(stream, err);
  });
}
var AsyncIteratorPrototype = Object.getPrototypeOf(
  Object.getPrototypeOf(async function* () {}).prototype,
);
var _a, _b, _c, _d, _e;
var ReadableStreamAsyncIterator = class {
  constructor(stream) {
    this[_a] = null;
    this[_b] = (resolve, reject) => {
      const data = this[kStream].read();
      if (data) {
        this[kLastPromise] = null;
        this[kLastResolve] = null;
        this[kLastReject] = null;
        resolve(createIterResult2(data, false));
      } else {
        this[kLastResolve] = resolve;
        this[kLastReject] = reject;
      }
    };
    this[_c] = null;
    this[_d] = null;
    this[_e] = AsyncIteratorPrototype[Symbol.asyncIterator];
    this[kEnded] = stream.readableEnded || stream._readableState.endEmitted;
    this[kStream] = stream;
    initIteratorSymbols(this, [
      kEnded,
      kError,
      kHandlePromise,
      kLastPromise,
      kLastReject,
      kLastResolve,
      kStream,
    ]);
  }
  get stream() {
    return this[kStream];
  }
  next() {
    const error = this[kError];
    if (error !== null) {
      return Promise.reject(error);
    }
    if (this[kEnded]) {
      return Promise.resolve(createIterResult2(void 0, true));
    }
    if (this[kStream].destroyed) {
      return new Promise((resolve, reject) => {
        if (this[kError]) {
          reject(this[kError]);
        } else if (this[kEnded]) {
          resolve(createIterResult2(void 0, true));
        } else {
          eos(this[kStream], (err) => {
            if (err && err.code !== "ERR_STREAM_PREMATURE_CLOSE") {
              reject(err);
            } else {
              resolve(createIterResult2(void 0, true));
            }
          });
        }
      });
    }
    const lastPromise = this[kLastPromise];
    let promise;
    if (lastPromise) {
      promise = new Promise(wrapForNext(lastPromise, this));
    } else {
      const data = this[kStream].read();
      if (data !== null) {
        return Promise.resolve(createIterResult2(data, false));
      }
      promise = new Promise(this[kHandlePromise]);
    }
    this[kLastPromise] = promise;
    return promise;
  }
  return() {
    return finish(this);
  }
  throw(err) {
    return finish(this, err);
  }
};
kEnded,
  (_a = kError),
  (_b = kHandlePromise),
  kLastPromise,
  (_c = kLastReject),
  (_d = kLastResolve),
  kStream,
  (_e = Symbol.asyncIterator);
var createReadableStreamAsyncIterator = (stream) => {
  if (typeof stream.read !== "function") {
    const src = stream;
    stream = new readable_default({ objectMode: true }).wrap(src);
    eos(stream, (err) => destroyer(src, err));
  }
  const iterator = new ReadableStreamAsyncIterator(stream);
  iterator[kLastPromise] = null;
  eos(stream, { writable: false }, (err) => {
    if (err && err.code !== "ERR_STREAM_PREMATURE_CLOSE") {
      const reject = iterator[kLastReject];
      if (reject !== null) {
        iterator[kLastPromise] = null;
        iterator[kLastResolve] = null;
        iterator[kLastReject] = null;
        reject(err);
      }
      iterator[kError] = err;
      return;
    }
    const resolve = iterator[kLastResolve];
    if (resolve !== null) {
      iterator[kLastPromise] = null;
      iterator[kLastResolve] = null;
      iterator[kLastReject] = null;
      resolve(createIterResult2(void 0, true));
    }
    iterator[kEnded] = true;
  });
  stream.on("readable", onReadable.bind(null, iterator));
  return iterator;
};
var async_iterator_default = createReadableStreamAsyncIterator;

// _stream/from.ts
function from(iterable, opts) {
  let iterator;
  if (typeof iterable === "string" || iterable instanceof Buffer3) {
    return new readable_default({
      objectMode: true,
      ...opts,
      read() {
        this.push(iterable);
        this.push(null);
      },
    });
  }
  if (Symbol.asyncIterator in iterable) {
    iterator = iterable[Symbol.asyncIterator]();
  } else if (Symbol.iterator in iterable) {
    iterator = iterable[Symbol.iterator]();
  } else {
    throw new ERR_INVALID_ARG_TYPE("iterable", ["Iterable"], iterable);
  }
  const readable = new readable_default({
    objectMode: true,
    highWaterMark: 1,
    ...opts,
  });
  let reading = false;
  let needToClose = false;
  readable._read = function () {
    if (!reading) {
      reading = true;
      next();
    }
  };
  readable._destroy = function (error, cb) {
    if (needToClose) {
      needToClose = false;
      close().then(
        () => queueMicrotask(() => cb(error)),
        (e) => queueMicrotask(() => cb(error || e)),
      );
    } else {
      cb(error);
    }
  };
  async function close() {
    if (typeof iterator.return === "function") {
      const { value } = await iterator.return();
      await value;
    }
  }
  async function next() {
    try {
      needToClose = false;
      const { value, done } = await iterator.next();
      needToClose = !done;
      if (done) {
        readable.push(null);
      } else if (readable.destroyed) {
        await close();
      } else {
        const res = await value;
        if (res === null) {
          reading = false;
          throw new ERR_STREAM_NULL_VALUES();
        } else if (readable.push(res)) {
          next();
        } else {
          reading = false;
        }
      }
    } catch (err) {
      if (err instanceof Error) {
        readable.destroy(err);
      }
    }
  }
  return readable;
}

// _stream/symbols.ts
var kConstruct = Symbol("kConstruct");
var kDestroy = Symbol("kDestroy");
var kPaused = Symbol("kPaused");

// _stream/readable_internal.ts
function _destroy(self, err, cb) {
  self._destroy(err || null, (err2) => {
    const r = self._readableState;
    if (err2) {
      err2.stack;
      if (!r.errored) {
        r.errored = err2;
      }
    }
    r.closed = true;
    if (typeof cb === "function") {
      cb(err2);
    }
    if (err2) {
      queueMicrotask(() => {
        if (!r.errorEmitted) {
          r.errorEmitted = true;
          self.emit("error", err2);
        }
        r.closeEmitted = true;
        if (r.emitClose) {
          self.emit("close");
        }
      });
    } else {
      queueMicrotask(() => {
        r.closeEmitted = true;
        if (r.emitClose) {
          self.emit("close");
        }
      });
    }
  });
}
function addChunk(stream, state, chunk, addToFront) {
  if (state.flowing && state.length === 0 && !state.sync) {
    if (state.multiAwaitDrain) {
      state.awaitDrainWriters.clear();
    } else {
      state.awaitDrainWriters = null;
    }
    stream.emit("data", chunk);
  } else {
    state.length += state.objectMode ? 1 : chunk.length;
    if (addToFront) {
      state.buffer.unshift(chunk);
    } else {
      state.buffer.push(chunk);
    }
    if (state.needReadable) {
      emitReadable(stream);
    }
  }
  maybeReadMore(stream, state);
}
var MAX_HWM = 1073741824;
function computeNewHighWaterMark(n) {
  if (n >= MAX_HWM) {
    n = MAX_HWM;
  } else {
    n--;
    n |= n >>> 1;
    n |= n >>> 2;
    n |= n >>> 4;
    n |= n >>> 8;
    n |= n >>> 16;
    n++;
  }
  return n;
}
function emitReadable(stream) {
  const state = stream._readableState;
  state.needReadable = false;
  if (!state.emittedReadable) {
    state.emittedReadable = true;
    queueMicrotask(() => emitReadable_(stream));
  }
}
function emitReadable_(stream) {
  const state = stream._readableState;
  if (!state.destroyed && !state.errored && (state.length || state.ended)) {
    stream.emit("readable");
    state.emittedReadable = false;
  }
  state.needReadable = !state.flowing && !state.ended &&
    state.length <= state.highWaterMark;
  flow(stream);
}
function endReadable(stream) {
  const state = stream._readableState;
  if (!state.endEmitted) {
    state.ended = true;
    queueMicrotask(() => endReadableNT(state, stream));
  }
}
function endReadableNT(state, stream) {
  if (
    !state.errorEmitted && !state.closeEmitted && !state.endEmitted &&
    state.length === 0
  ) {
    state.endEmitted = true;
    stream.emit("end");
    if (state.autoDestroy) {
      stream.destroy();
    }
  }
}
function errorOrDestroy(stream, err, sync = false) {
  const r = stream._readableState;
  if (r.destroyed) {
    return stream;
  }
  if (r.autoDestroy) {
    stream.destroy(err);
  } else if (err) {
    err.stack;
    if (!r.errored) {
      r.errored = err;
    }
    if (sync) {
      queueMicrotask(() => {
        if (!r.errorEmitted) {
          r.errorEmitted = true;
          stream.emit("error", err);
        }
      });
    } else if (!r.errorEmitted) {
      r.errorEmitted = true;
      stream.emit("error", err);
    }
  }
}
function flow(stream) {
  const state = stream._readableState;
  while (state.flowing && stream.read() !== null);
}
function fromList(n, state) {
  if (state.length === 0) {
    return null;
  }
  let ret;
  if (state.objectMode) {
    ret = state.buffer.shift();
  } else if (!n || n >= state.length) {
    if (state.decoder) {
      ret = state.buffer.join("");
    } else if (state.buffer.length === 1) {
      ret = state.buffer.first();
    } else {
      ret = state.buffer.concat(state.length);
    }
    state.buffer.clear();
  } else {
    ret = state.buffer.consume(n, !!state.decoder);
  }
  return ret;
}
function howMuchToRead(n, state) {
  if (n <= 0 || (state.length === 0 && state.ended)) {
    return 0;
  }
  if (state.objectMode) {
    return 1;
  }
  if (Number.isNaN(n)) {
    if (state.flowing && state.length) {
      return state.buffer.first().length;
    }
    return state.length;
  }
  if (n <= state.length) {
    return n;
  }
  return state.ended ? state.length : 0;
}
function maybeReadMore(stream, state) {
  if (!state.readingMore && state.constructed) {
    state.readingMore = true;
    queueMicrotask(() => maybeReadMore_(stream, state));
  }
}
function maybeReadMore_(stream, state) {
  while (
    !state.reading &&
    !state.ended &&
    (state.length < state.highWaterMark ||
      (state.flowing && state.length === 0))
  ) {
    const len = state.length;
    stream.read(0);
    if (len === state.length) {
      break;
    }
  }
  state.readingMore = false;
}
function nReadingNextTick(self) {
  self.read(0);
}
function onEofChunk(stream, state) {
  if (state.ended) return;
  if (state.decoder) {
    const chunk = state.decoder.end();
    if (chunk && chunk.length) {
      state.buffer.push(chunk);
      state.length += state.objectMode ? 1 : chunk.length;
    }
  }
  state.ended = true;
  if (state.sync) {
    emitReadable(stream);
  } else {
    state.needReadable = false;
    state.emittedReadable = true;
    emitReadable_(stream);
  }
}
function pipeOnDrain(src, dest) {
  return function pipeOnDrainFunctionResult() {
    const state = src._readableState;
    if (state.awaitDrainWriters === dest) {
      state.awaitDrainWriters = null;
    } else if (state.multiAwaitDrain) {
      state.awaitDrainWriters.delete(dest);
    }
    if (
      (!state.awaitDrainWriters || state.awaitDrainWriters.size === 0) &&
      src.listenerCount("data")
    ) {
      state.flowing = true;
      flow(src);
    }
  };
}
function prependListener(emitter, event, fn) {
  if (typeof emitter.prependListener === "function") {
    return emitter.prependListener(event, fn);
  }
  if (emitter._events.get(event)?.length) {
    const listeners = [fn, ...emitter._events.get(event)];
    emitter._events.set(event, listeners);
  } else {
    emitter.on(event, fn);
  }
}
function readableAddChunk(stream, chunk, encoding = void 0, addToFront) {
  const state = stream._readableState;
  let usedEncoding = encoding;
  let err;
  if (!state.objectMode) {
    if (typeof chunk === "string") {
      usedEncoding = encoding || state.defaultEncoding;
      if (state.encoding !== usedEncoding) {
        if (addToFront && state.encoding) {
          chunk = Buffer3.from(chunk, usedEncoding).toString(state.encoding);
        } else {
          chunk = Buffer3.from(chunk, usedEncoding);
          usedEncoding = "";
        }
      }
    } else if (chunk instanceof Uint8Array) {
      chunk = Buffer3.from(chunk);
    }
  }
  if (err) {
    errorOrDestroy(stream, err);
  } else if (chunk === null) {
    state.reading = false;
    onEofChunk(stream, state);
  } else if (state.objectMode || chunk.length > 0) {
    if (addToFront) {
      if (state.endEmitted) {
        errorOrDestroy(stream, new ERR_STREAM_UNSHIFT_AFTER_END_EVENT());
      } else {
        addChunk(stream, state, chunk, true);
      }
    } else if (state.ended) {
      errorOrDestroy(stream, new ERR_STREAM_PUSH_AFTER_EOF());
    } else if (state.destroyed || state.errored) {
      return false;
    } else {
      state.reading = false;
      if (state.decoder && !usedEncoding) {
        chunk = state.decoder.write(Buffer3.from(chunk));
        if (state.objectMode || chunk.length !== 0) {
          addChunk(stream, state, chunk, false);
        } else {
          maybeReadMore(stream, state);
        }
      } else {
        addChunk(stream, state, chunk, false);
      }
    }
  } else if (!addToFront) {
    state.reading = false;
    maybeReadMore(stream, state);
  }
  return !state.ended &&
    (state.length < state.highWaterMark || state.length === 0);
}
function resume(stream, state) {
  if (!state.resumeScheduled) {
    state.resumeScheduled = true;
    queueMicrotask(() => resume_(stream, state));
  }
}
function resume_(stream, state) {
  if (!state.reading) {
    stream.read(0);
  }
  state.resumeScheduled = false;
  stream.emit("resume");
  flow(stream);
  if (state.flowing && !state.reading) {
    stream.read(0);
  }
}
function updateReadableListening(self) {
  const state = self._readableState;
  state.readableListening = self.listenerCount("readable") > 0;
  if (state.resumeScheduled && state[kPaused] === false) {
    state.flowing = true;
  } else if (self.listenerCount("data") > 0) {
    self.resume();
  } else if (!state.readableListening) {
    state.flowing = null;
  }
}

// _stream/writable_internal.ts
var kOnFinished = Symbol("kOnFinished");
function _destroy2(self, err, cb) {
  self._destroy(err || null, (err2) => {
    const w = self._writableState;
    if (err2) {
      err2.stack;
      if (!w.errored) {
        w.errored = err2;
      }
    }
    w.closed = true;
    if (typeof cb === "function") {
      cb(err2);
    }
    if (err2) {
      queueMicrotask(() => {
        if (!w.errorEmitted) {
          w.errorEmitted = true;
          self.emit("error", err2);
        }
        w.closeEmitted = true;
        if (w.emitClose) {
          self.emit("close");
        }
      });
    } else {
      queueMicrotask(() => {
        w.closeEmitted = true;
        if (w.emitClose) {
          self.emit("close");
        }
      });
    }
  });
}
function afterWrite(stream, state, count, cb) {
  const needDrain = !state.ending && !stream.destroyed && state.length === 0 &&
    state.needDrain;
  if (needDrain) {
    state.needDrain = false;
    stream.emit("drain");
  }
  while (count-- > 0) {
    state.pendingcb--;
    cb();
  }
  if (state.destroyed) {
    errorBuffer(state);
  }
  finishMaybe(stream, state);
}
function afterWriteTick({ cb, count, state, stream }) {
  state.afterWriteTickInfo = null;
  return afterWrite(stream, state, count, cb);
}
function clearBuffer(stream, state) {
  if (
    state.corked || state.bufferProcessing || state.destroyed ||
    !state.constructed
  ) {
    return;
  }
  const { buffered, bufferedIndex, objectMode } = state;
  const bufferedLength = buffered.length - bufferedIndex;
  if (!bufferedLength) {
    return;
  }
  const i = bufferedIndex;
  state.bufferProcessing = true;
  if (bufferedLength > 1 && stream._writev) {
    state.pendingcb -= bufferedLength - 1;
    const callback = state.allNoop ? nop2 : (err) => {
      for (let n = i; n < buffered.length; ++n) {
        buffered[n].callback(err);
      }
    };
    const chunks = state.allNoop && i === 0 ? buffered : buffered.slice(i);
    doWrite(stream, state, true, state.length, chunks, "", callback);
    resetBuffer(state);
  } else {
    do {
      const { chunk, encoding, callback } = buffered[i];
      const len = objectMode ? 1 : chunk.length;
      doWrite(stream, state, false, len, chunk, encoding, callback);
    } while (i < buffered.length && !state.writing);
    if (i === buffered.length) {
      resetBuffer(state);
    } else if (i > 256) {
      buffered.splice(0, i);
      state.bufferedIndex = 0;
    } else {
      state.bufferedIndex = i;
    }
  }
  state.bufferProcessing = false;
}
function destroy(err, cb) {
  const w = this._writableState;
  if (w.destroyed) {
    if (typeof cb === "function") {
      cb();
    }
    return this;
  }
  if (err) {
    err.stack;
    if (!w.errored) {
      w.errored = err;
    }
  }
  w.destroyed = true;
  if (!w.constructed) {
    this.once(kDestroy, (er) => {
      _destroy2(this, err || er, cb);
    });
  } else {
    _destroy2(this, err, cb);
  }
  return this;
}
function doWrite(stream, state, writev, len, chunk, encoding, cb) {
  state.writelen = len;
  state.writecb = cb;
  state.writing = true;
  state.sync = true;
  if (state.destroyed) {
    state.onwrite(new ERR_STREAM_DESTROYED("write"));
  } else if (writev) {
    stream._writev(chunk, state.onwrite);
  } else {
    stream._write(chunk, encoding, state.onwrite);
  }
  state.sync = false;
}
function errorBuffer(state) {
  if (state.writing) {
    return;
  }
  for (let n = state.bufferedIndex; n < state.buffered.length; ++n) {
    const { chunk, callback } = state.buffered[n];
    const len = state.objectMode ? 1 : chunk.length;
    state.length -= len;
    callback(new ERR_STREAM_DESTROYED("write"));
  }
  for (const callback of state[kOnFinished].splice(0)) {
    callback(new ERR_STREAM_DESTROYED("end"));
  }
  resetBuffer(state);
}
function errorOrDestroy2(stream, err, sync = false) {
  const w = stream._writableState;
  if (w.destroyed) {
    return stream;
  }
  if (w.autoDestroy) {
    stream.destroy(err);
  } else if (err) {
    err.stack;
    if (!w.errored) {
      w.errored = err;
    }
    if (sync) {
      queueMicrotask(() => {
        if (w.errorEmitted) {
          return;
        }
        w.errorEmitted = true;
        stream.emit("error", err);
      });
    } else {
      if (w.errorEmitted) {
        return;
      }
      w.errorEmitted = true;
      stream.emit("error", err);
    }
  }
}
function finish2(stream, state) {
  state.pendingcb--;
  if (state.errorEmitted || state.closeEmitted) {
    return;
  }
  state.finished = true;
  for (const callback of state[kOnFinished].splice(0)) {
    callback();
  }
  stream.emit("finish");
  if (state.autoDestroy) {
    stream.destroy();
  }
}
function finishMaybe(stream, state, sync) {
  if (needFinish(state)) {
    prefinish(stream, state);
    if (state.pendingcb === 0 && needFinish(state)) {
      state.pendingcb++;
      if (sync) {
        queueMicrotask(() => finish2(stream, state));
      } else {
        finish2(stream, state);
      }
    }
  }
}
function needFinish(state) {
  return (
    state.ending &&
    state.constructed &&
    state.length === 0 &&
    !state.errored &&
    state.buffered.length === 0 &&
    !state.finished &&
    !state.writing
  );
}
function nop2() {}
function resetBuffer(state) {
  state.buffered = [];
  state.bufferedIndex = 0;
  state.allBuffers = true;
  state.allNoop = true;
}
function onwriteError(stream, state, er, cb) {
  --state.pendingcb;
  cb(er);
  errorBuffer(state);
  errorOrDestroy2(stream, er);
}
function onwrite(stream, er) {
  const state = stream._writableState;
  const sync = state.sync;
  const cb = state.writecb;
  if (typeof cb !== "function") {
    errorOrDestroy2(stream, new ERR_MULTIPLE_CALLBACK());
    return;
  }
  state.writing = false;
  state.writecb = null;
  state.length -= state.writelen;
  state.writelen = 0;
  if (er) {
    er.stack;
    if (!state.errored) {
      state.errored = er;
    }
    if (sync) {
      queueMicrotask(() => onwriteError(stream, state, er, cb));
    } else {
      onwriteError(stream, state, er, cb);
    }
  } else {
    if (state.buffered.length > state.bufferedIndex) {
      clearBuffer(stream, state);
    }
    if (sync) {
      if (
        state.afterWriteTickInfo !== null && state.afterWriteTickInfo.cb === cb
      ) {
        state.afterWriteTickInfo.count++;
      } else {
        state.afterWriteTickInfo = {
          count: 1,
          cb,
          stream,
          state,
        };
        queueMicrotask(() => afterWriteTick(state.afterWriteTickInfo));
      }
    } else {
      afterWrite(stream, state, 1, cb);
    }
  }
}
function prefinish(stream, state) {
  if (!state.prefinished && !state.finalCalled) {
    if (typeof stream._final === "function" && !state.destroyed) {
      state.finalCalled = true;
      state.sync = true;
      state.pendingcb++;
      stream._final((err) => {
        state.pendingcb--;
        if (err) {
          for (const callback of state[kOnFinished].splice(0)) {
            callback(err);
          }
          errorOrDestroy2(stream, err, state.sync);
        } else if (needFinish(state)) {
          state.prefinished = true;
          stream.emit("prefinish");
          state.pendingcb++;
          queueMicrotask(() => finish2(stream, state));
        }
      });
      state.sync = false;
    } else {
      state.prefinished = true;
      stream.emit("prefinish");
    }
  }
}
function writeOrBuffer(stream, state, chunk, encoding, callback) {
  const len = state.objectMode ? 1 : chunk.length;
  state.length += len;
  if (state.writing || state.corked || state.errored || !state.constructed) {
    state.buffered.push({ chunk, encoding, callback });
    if (state.allBuffers && encoding !== "buffer") {
      state.allBuffers = false;
    }
    if (state.allNoop && callback !== nop2) {
      state.allNoop = false;
    }
  } else {
    state.writelen = len;
    state.writecb = callback;
    state.writing = true;
    state.sync = true;
    stream._write(chunk, encoding, state.onwrite);
    state.sync = false;
  }
  const ret = state.length < state.highWaterMark;
  if (!ret) {
    state.needDrain = true;
  }
  return ret && !state.errored && !state.destroyed;
}

// _stream/readable.ts
var _a2;
var ReadableState = class {
  constructor(options) {
    this[_a2] = null;
    this.awaitDrainWriters = null;
    this.buffer = new BufferList();
    this.closed = false;
    this.closeEmitted = false;
    this.decoder = null;
    this.destroyed = false;
    this.emittedReadable = false;
    this.encoding = null;
    this.ended = false;
    this.endEmitted = false;
    this.errored = null;
    this.errorEmitted = false;
    this.flowing = null;
    this.length = 0;
    this.multiAwaitDrain = false;
    this.needReadable = false;
    this.pipes = [];
    this.readable = true;
    this.readableListening = false;
    this.reading = false;
    this.readingMore = false;
    this.resumeScheduled = false;
    this.sync = true;
    this.objectMode = !!options?.objectMode;
    this.highWaterMark = options?.highWaterMark ??
      (this.objectMode ? 16 : 16 * 1024);
    if (Number.isInteger(this.highWaterMark) && this.highWaterMark >= 0) {
      this.highWaterMark = Math.floor(this.highWaterMark);
    } else {
      throw new ERR_INVALID_OPT_VALUE("highWaterMark", this.highWaterMark);
    }
    this.emitClose = options?.emitClose ?? true;
    this.autoDestroy = options?.autoDestroy ?? true;
    this.defaultEncoding = options?.defaultEncoding || "utf8";
    if (options?.encoding) {
      this.decoder = new StringDecoder(options.encoding);
      this.encoding = options.encoding;
    }
    this.constructed = true;
  }
};
_a2 = kPaused;
var Readable = class extends stream_default {
  constructor(options) {
    super();
    this.off = this.removeListener;
    if (options) {
      if (typeof options.read === "function") {
        this._read = options.read;
      }
      if (typeof options.destroy === "function") {
        this._destroy = options.destroy;
      }
    }
    this._readableState = new ReadableState(options);
  }
  static from(iterable, opts) {
    return from(iterable, opts);
  }
  read(n) {
    if (n === void 0) {
      n = NaN;
    }
    const state = this._readableState;
    const nOrig = n;
    if (n > state.highWaterMark) {
      state.highWaterMark = computeNewHighWaterMark(n);
    }
    if (n !== 0) {
      state.emittedReadable = false;
    }
    if (
      n === 0 &&
      state.needReadable &&
      ((state.highWaterMark !== 0
        ? state.length >= state.highWaterMark
        : state.length > 0) || state.ended)
    ) {
      if (state.length === 0 && state.ended) {
        endReadable(this);
      } else {
        emitReadable(this);
      }
      return null;
    }
    n = howMuchToRead(n, state);
    if (n === 0 && state.ended) {
      if (state.length === 0) {
        endReadable(this);
      }
      return null;
    }
    let doRead = state.needReadable;
    if (state.length === 0 || state.length - n < state.highWaterMark) {
      doRead = true;
    }
    if (
      state.ended || state.reading || state.destroyed || state.errored ||
      !state.constructed
    ) {
      doRead = false;
    } else if (doRead) {
      state.reading = true;
      state.sync = true;
      if (state.length === 0) {
        state.needReadable = true;
      }
      this._read();
      state.sync = false;
      if (!state.reading) {
        n = howMuchToRead(nOrig, state);
      }
    }
    let ret;
    if (n > 0) {
      ret = fromList(n, state);
    } else {
      ret = null;
    }
    if (ret === null) {
      state.needReadable = state.length <= state.highWaterMark;
      n = 0;
    } else {
      state.length -= n;
      if (state.multiAwaitDrain) {
        state.awaitDrainWriters.clear();
      } else {
        state.awaitDrainWriters = null;
      }
    }
    if (state.length === 0) {
      if (!state.ended) {
        state.needReadable = true;
      }
      if (nOrig !== n && state.ended) {
        endReadable(this);
      }
    }
    if (ret !== null) {
      this.emit("data", ret);
    }
    return ret;
  }
  _read(_size) {
    throw new ERR_METHOD_NOT_IMPLEMENTED("_read()");
  }
  pipe(dest, pipeOpts) {
    const src = this;
    const state = this._readableState;
    if (state.pipes.length === 1) {
      if (!state.multiAwaitDrain) {
        state.multiAwaitDrain = true;
        state.awaitDrainWriters = new Set(
          state.awaitDrainWriters ? [state.awaitDrainWriters] : [],
        );
      }
    }
    state.pipes.push(dest);
    const doEnd = !pipeOpts || pipeOpts.end !== false;
    const endFn = doEnd ? onend : unpipe;
    if (state.endEmitted) {
      queueMicrotask(endFn);
    } else {
      this.once("end", endFn);
    }
    dest.on("unpipe", onunpipe);
    function onunpipe(readable, unpipeInfo) {
      if (readable === src) {
        if (unpipeInfo && unpipeInfo.hasUnpiped === false) {
          unpipeInfo.hasUnpiped = true;
          cleanup();
        }
      }
    }
    function onend() {
      dest.end();
    }
    let ondrain;
    let cleanedUp = false;
    function cleanup() {
      dest.removeListener("close", onclose);
      dest.removeListener("finish", onfinish);
      if (ondrain) {
        dest.removeListener("drain", ondrain);
      }
      dest.removeListener("error", onerror);
      dest.removeListener("unpipe", onunpipe);
      src.removeListener("end", onend);
      src.removeListener("end", unpipe);
      src.removeListener("data", ondata);
      cleanedUp = true;
      if (
        ondrain && state.awaitDrainWriters &&
        (!dest._writableState || dest._writableState.needDrain)
      ) {
        ondrain();
      }
    }
    this.on("data", ondata);
    function ondata(chunk) {
      const ret = dest.write(chunk);
      if (ret === false) {
        if (!cleanedUp) {
          if (state.pipes.length === 1 && state.pipes[0] === dest) {
            state.awaitDrainWriters = dest;
            state.multiAwaitDrain = false;
          } else if (state.pipes.length > 1 && state.pipes.includes(dest)) {
            state.awaitDrainWriters.add(dest);
          }
          src.pause();
        }
        if (!ondrain) {
          ondrain = pipeOnDrain(src, dest);
          dest.on("drain", ondrain);
        }
      }
    }
    function onerror(er) {
      unpipe();
      dest.removeListener("error", onerror);
      if (dest.listenerCount("error") === 0) {
        const s = dest._writableState || dest._readableState;
        if (s && !s.errorEmitted) {
          if (dest instanceof duplex_default) {
            errorOrDestroy3(dest, er);
          } else {
            errorOrDestroy2(dest, er);
          }
        } else {
          dest.emit("error", er);
        }
      }
    }
    prependListener(dest, "error", onerror);
    function onclose() {
      dest.removeListener("finish", onfinish);
      unpipe();
    }
    dest.once("close", onclose);
    function onfinish() {
      dest.removeListener("close", onclose);
      unpipe();
    }
    dest.once("finish", onfinish);
    function unpipe() {
      src.unpipe(dest);
    }
    dest.emit("pipe", this);
    if (!state.flowing) {
      this.resume();
    }
    return dest;
  }
  isPaused() {
    return this._readableState[kPaused] === true ||
      this._readableState.flowing === false;
  }
  setEncoding(enc) {
    const decoder = new StringDecoder(enc);
    this._readableState.decoder = decoder;
    this._readableState.encoding = this._readableState.decoder.encoding;
    const buffer = this._readableState.buffer;
    let content = "";
    for (const data of buffer) {
      content += decoder.write(data);
    }
    buffer.clear();
    if (content !== "") {
      buffer.push(content);
    }
    this._readableState.length = content.length;
    return this;
  }
  on(ev, fn) {
    const res = super.on.call(this, ev, fn);
    const state = this._readableState;
    if (ev === "data") {
      state.readableListening = this.listenerCount("readable") > 0;
      if (state.flowing !== false) {
        this.resume();
      }
    } else if (ev === "readable") {
      if (!state.endEmitted && !state.readableListening) {
        state.readableListening = state.needReadable = true;
        state.flowing = false;
        state.emittedReadable = false;
        if (state.length) {
          emitReadable(this);
        } else if (!state.reading) {
          queueMicrotask(() => nReadingNextTick(this));
        }
      }
    }
    return res;
  }
  removeListener(ev, fn) {
    const res = super.removeListener.call(this, ev, fn);
    if (ev === "readable") {
      queueMicrotask(() => updateReadableListening(this));
    }
    return res;
  }
  destroy(err, cb) {
    const r = this._readableState;
    if (r.destroyed) {
      if (typeof cb === "function") {
        cb();
      }
      return this;
    }
    if (err) {
      err.stack;
      if (!r.errored) {
        r.errored = err;
      }
    }
    r.destroyed = true;
    if (!r.constructed) {
      this.once(kDestroy, (er) => {
        _destroy(this, err || er, cb);
      });
    } else {
      _destroy(this, err, cb);
    }
    return this;
  }
  _undestroy() {
    const r = this._readableState;
    r.constructed = true;
    r.closed = false;
    r.closeEmitted = false;
    r.destroyed = false;
    r.errored = null;
    r.errorEmitted = false;
    r.reading = false;
    r.ended = false;
    r.endEmitted = false;
  }
  _destroy(error, callback) {
    callback(error);
  }
  [captureRejectionSymbol](err) {
    this.destroy(err);
  }
  push(chunk, encoding) {
    return readableAddChunk(this, chunk, encoding, false);
  }
  unshift(chunk, encoding) {
    return readableAddChunk(this, chunk, encoding, true);
  }
  unpipe(dest) {
    const state = this._readableState;
    const unpipeInfo = { hasUnpiped: false };
    if (state.pipes.length === 0) {
      return this;
    }
    if (!dest) {
      const dests = state.pipes;
      state.pipes = [];
      this.pause();
      for (const dest2 of dests) {
        dest2.emit("unpipe", this, { hasUnpiped: false });
      }
      return this;
    }
    const index = state.pipes.indexOf(dest);
    if (index === -1) {
      return this;
    }
    state.pipes.splice(index, 1);
    if (state.pipes.length === 0) {
      this.pause();
    }
    dest.emit("unpipe", this, unpipeInfo);
    return this;
  }
  removeAllListeners(ev) {
    const res = super.removeAllListeners(ev);
    if (ev === "readable" || ev === void 0) {
      queueMicrotask(() => updateReadableListening(this));
    }
    return res;
  }
  resume() {
    const state = this._readableState;
    if (!state.flowing) {
      state.flowing = !state.readableListening;
      resume(this, state);
    }
    state[kPaused] = false;
    return this;
  }
  pause() {
    if (this._readableState.flowing !== false) {
      this._readableState.flowing = false;
      this.emit("pause");
    }
    this._readableState[kPaused] = true;
    return this;
  }
  wrap(stream) {
    const state = this._readableState;
    let paused = false;
    stream.on("end", () => {
      if (state.decoder && !state.ended) {
        const chunk = state.decoder.end();
        if (chunk && chunk.length) {
          this.push(chunk);
        }
      }
      this.push(null);
    });
    stream.on("data", (chunk) => {
      if (state.decoder) {
        chunk = state.decoder.write(chunk);
      }
      if (state.objectMode && (chunk === null || chunk === void 0)) {
        return;
      } else if (!state.objectMode && (!chunk || !chunk.length)) {
        return;
      }
      const ret = this.push(chunk);
      if (!ret) {
        paused = true;
        stream.pause();
      }
    });
    for (const i in stream) {
      if (this[i] === void 0 && typeof stream[i] === "function") {
        this[i] = (function methodWrap(method) {
          return function methodWrapReturnFunction() {
            return stream[method].apply(stream);
          };
        })(i);
      }
    }
    stream.on("error", (err) => {
      errorOrDestroy(this, err);
    });
    stream.on("close", () => {
      this.emit("close");
    });
    stream.on("destroy", () => {
      this.emit("destroy");
    });
    stream.on("pause", () => {
      this.emit("pause");
    });
    stream.on("resume", () => {
      this.emit("resume");
    });
    this._read = () => {
      if (paused) {
        paused = false;
        stream.resume();
      }
    };
    return this;
  }
  [Symbol.asyncIterator]() {
    return async_iterator_default(this);
  }
  get readable() {
    return (
      this._readableState?.readable &&
      !this._readableState?.destroyed &&
      !this._readableState?.errorEmitted &&
      !this._readableState?.endEmitted
    );
  }
  set readable(val) {
    if (this._readableState) {
      this._readableState.readable = val;
    }
  }
  get readableHighWaterMark() {
    return this._readableState.highWaterMark;
  }
  get readableBuffer() {
    return this._readableState && this._readableState.buffer;
  }
  get readableFlowing() {
    return this._readableState.flowing;
  }
  set readableFlowing(state) {
    if (this._readableState) {
      this._readableState.flowing = state;
    }
  }
  get readableLength() {
    return this._readableState.length;
  }
  get readableObjectMode() {
    return this._readableState ? this._readableState.objectMode : false;
  }
  get readableEncoding() {
    return this._readableState ? this._readableState.encoding : null;
  }
  get destroyed() {
    if (this._readableState === void 0) {
      return false;
    }
    return this._readableState.destroyed;
  }
  set destroyed(value) {
    if (!this._readableState) {
      return;
    }
    this._readableState.destroyed = value;
  }
  get readableEnded() {
    return this._readableState ? this._readableState.endEmitted : false;
  }
};
Readable.ReadableState = ReadableState;
Readable._fromList = fromList;
Object.defineProperties(Readable, {
  _readableState: { enumerable: false },
  destroyed: { enumerable: false },
  readableBuffer: { enumerable: false },
  readableEncoding: { enumerable: false },
  readableEnded: { enumerable: false },
  readableFlowing: { enumerable: false },
  readableHighWaterMark: { enumerable: false },
  readableLength: { enumerable: false },
  readableObjectMode: { enumerable: false },
});
var readable_default = Readable;

// _stream/writable.ts
var _a3;
var WritableState = class {
  constructor(options, stream) {
    this[_a3] = [];
    this.afterWriteTickInfo = null;
    this.allBuffers = true;
    this.allNoop = true;
    this.buffered = [];
    this.bufferedIndex = 0;
    this.bufferProcessing = false;
    this.closed = false;
    this.closeEmitted = false;
    this.corked = 0;
    this.destroyed = false;
    this.ended = false;
    this.ending = false;
    this.errored = null;
    this.errorEmitted = false;
    this.finalCalled = false;
    this.finished = false;
    this.length = 0;
    this.needDrain = false;
    this.pendingcb = 0;
    this.prefinished = false;
    this.sync = true;
    this.writecb = null;
    this.writable = true;
    this.writelen = 0;
    this.writing = false;
    this.objectMode = !!options?.objectMode;
    this.highWaterMark = options?.highWaterMark ??
      (this.objectMode ? 16 : 16 * 1024);
    if (Number.isInteger(this.highWaterMark) && this.highWaterMark >= 0) {
      this.highWaterMark = Math.floor(this.highWaterMark);
    } else {
      throw new ERR_INVALID_OPT_VALUE("highWaterMark", this.highWaterMark);
    }
    this.decodeStrings = !options?.decodeStrings === false;
    this.defaultEncoding = options?.defaultEncoding || "utf8";
    this.onwrite = onwrite.bind(void 0, stream);
    resetBuffer(this);
    this.emitClose = options?.emitClose ?? true;
    this.autoDestroy = options?.autoDestroy ?? true;
    this.constructed = true;
  }
  getBuffer() {
    return this.buffered.slice(this.bufferedIndex);
  }
  get bufferedRequestCount() {
    return this.buffered.length - this.bufferedIndex;
  }
};
_a3 = kOnFinished;
var Writable = class extends stream_default {
  constructor(options) {
    super();
    this._writev = null;
    this._writableState = new WritableState(options, this);
    if (options) {
      if (typeof options.write === "function") {
        this._write = options.write;
      }
      if (typeof options.writev === "function") {
        this._writev = options.writev;
      }
      if (typeof options.destroy === "function") {
        this._destroy = options.destroy;
      }
      if (typeof options.final === "function") {
        this._final = options.final;
      }
    }
  }
  [captureRejectionSymbol](err) {
    this.destroy(err);
  }
  get destroyed() {
    return this._writableState ? this._writableState.destroyed : false;
  }
  set destroyed(value) {
    if (this._writableState) {
      this._writableState.destroyed = value;
    }
  }
  get writable() {
    const w = this._writableState;
    return !w.destroyed && !w.errored && !w.ending && !w.ended;
  }
  set writable(val) {
    if (this._writableState) {
      this._writableState.writable = !!val;
    }
  }
  get writableFinished() {
    return this._writableState ? this._writableState.finished : false;
  }
  get writableObjectMode() {
    return this._writableState ? this._writableState.objectMode : false;
  }
  get writableBuffer() {
    return this._writableState && this._writableState.getBuffer();
  }
  get writableEnded() {
    return this._writableState ? this._writableState.ending : false;
  }
  get writableHighWaterMark() {
    return this._writableState && this._writableState.highWaterMark;
  }
  get writableCorked() {
    return this._writableState ? this._writableState.corked : 0;
  }
  get writableLength() {
    return this._writableState && this._writableState.length;
  }
  _undestroy() {
    const w = this._writableState;
    w.constructed = true;
    w.destroyed = false;
    w.closed = false;
    w.closeEmitted = false;
    w.errored = null;
    w.errorEmitted = false;
    w.ended = false;
    w.ending = false;
    w.finalCalled = false;
    w.prefinished = false;
    w.finished = false;
  }
  _destroy(err, cb) {
    cb(err);
  }
  destroy(err, cb) {
    const state = this._writableState;
    if (!state.destroyed) {
      queueMicrotask(() => errorBuffer(state));
    }
    destroy.call(this, err, cb);
    return this;
  }
  end(x, y, z) {
    const state = this._writableState;
    let chunk;
    let encoding;
    let cb;
    if (typeof x === "function") {
      chunk = null;
      encoding = null;
      cb = x;
    } else if (typeof y === "function") {
      chunk = x;
      encoding = null;
      cb = y;
    } else {
      chunk = x;
      encoding = y;
      cb = z;
    }
    if (chunk !== null && chunk !== void 0) {
      this.write(chunk, encoding);
    }
    if (state.corked) {
      state.corked = 1;
      this.uncork();
    }
    let err;
    if (!state.errored && !state.ending) {
      state.ending = true;
      finishMaybe(this, state, true);
      state.ended = true;
    } else if (state.finished) {
      err = new ERR_STREAM_ALREADY_FINISHED("end");
    } else if (state.destroyed) {
      err = new ERR_STREAM_DESTROYED("end");
    }
    if (typeof cb === "function") {
      if (err || state.finished) {
        queueMicrotask(() => {
          cb(err);
        });
      } else {
        state[kOnFinished].push(cb);
      }
    }
    return this;
  }
  _write(chunk, encoding, cb) {
    if (this._writev) {
      this._writev([{ chunk, encoding }], cb);
    } else {
      throw new ERR_METHOD_NOT_IMPLEMENTED("_write()");
    }
  }
  pipe(dest) {
    errorOrDestroy2(this, new ERR_STREAM_CANNOT_PIPE());
    return dest;
  }
  write(chunk, x, y) {
    const state = this._writableState;
    let encoding;
    let cb;
    if (typeof x === "function") {
      cb = x;
      encoding = state.defaultEncoding;
    } else {
      if (!x) {
        encoding = state.defaultEncoding;
      } else if (x !== "buffer" && !Buffer3.isEncoding(x)) {
        throw new ERR_UNKNOWN_ENCODING(x);
      } else {
        encoding = x;
      }
      if (typeof y !== "function") {
        cb = nop2;
      } else {
        cb = y;
      }
    }
    if (chunk === null) {
      throw new ERR_STREAM_NULL_VALUES();
    } else if (!state.objectMode) {
      if (typeof chunk === "string") {
        if (state.decodeStrings !== false) {
          chunk = Buffer3.from(chunk, encoding);
          encoding = "buffer";
        }
      } else if (chunk instanceof Buffer3) {
        encoding = "buffer";
      } else if (stream_default._isUint8Array(chunk)) {
        chunk = stream_default._uint8ArrayToBuffer(chunk);
        encoding = "buffer";
      } else {
        throw new ERR_INVALID_ARG_TYPE("chunk", [
          "string",
          "Buffer",
          "Uint8Array",
        ], chunk);
      }
    }
    let err;
    if (state.ending) {
      err = new ERR_STREAM_WRITE_AFTER_END();
    } else if (state.destroyed) {
      err = new ERR_STREAM_DESTROYED("write");
    }
    if (err) {
      queueMicrotask(() => cb(err));
      errorOrDestroy2(this, err, true);
      return false;
    }
    state.pendingcb++;
    return writeOrBuffer(this, state, chunk, encoding, cb);
  }
  cork() {
    this._writableState.corked++;
  }
  uncork() {
    const state = this._writableState;
    if (state.corked) {
      state.corked--;
      if (!state.writing) {
        clearBuffer(this, state);
      }
    }
  }
  setDefaultEncoding(encoding) {
    if (typeof encoding === "string") {
      encoding = encoding.toLowerCase();
    }
    if (!Buffer3.isEncoding(encoding)) {
      throw new ERR_UNKNOWN_ENCODING(encoding);
    }
    this._writableState.defaultEncoding = encoding;
    return this;
  }
};
Writable.WritableState = WritableState;
var writable_default = Writable;

// _stream/duplex_internal.ts
function endDuplex(stream) {
  const state = stream._readableState;
  if (!state.endEmitted) {
    state.ended = true;
    queueMicrotask(() => endReadableNT2(state, stream));
  }
}
function endReadableNT2(state, stream) {
  if (
    !state.errorEmitted && !state.closeEmitted && !state.endEmitted &&
    state.length === 0
  ) {
    state.endEmitted = true;
    stream.emit("end");
    if (stream.writable && stream.allowHalfOpen === false) {
      queueMicrotask(() => endWritableNT(state, stream));
    } else if (state.autoDestroy) {
      const wState = stream._writableState;
      const autoDestroy = !wState ||
        (wState.autoDestroy && (wState.finished || wState.writable === false));
      if (autoDestroy) {
        stream.destroy();
      }
    }
  }
}
function endWritableNT(_state, stream) {
  const writable = stream.writable && !stream.writableEnded &&
    !stream.destroyed;
  if (writable) {
    stream.end();
  }
}
function errorOrDestroy3(stream, err, sync = false) {
  const r = stream._readableState;
  const w = stream._writableState;
  if (w.destroyed || r.destroyed) {
    return this;
  }
  if (r.autoDestroy || w.autoDestroy) {
    stream.destroy(err);
  } else if (err) {
    err.stack;
    if (w && !w.errored) {
      w.errored = err;
    }
    if (r && !r.errored) {
      r.errored = err;
    }
    if (sync) {
      queueMicrotask(() => {
        if (w.errorEmitted || r.errorEmitted) {
          return;
        }
        w.errorEmitted = true;
        r.errorEmitted = true;
        stream.emit("error", err);
      });
    } else {
      if (w.errorEmitted || r.errorEmitted) {
        return;
      }
      w.errorEmitted = true;
      r.errorEmitted = true;
      stream.emit("error", err);
    }
  }
}
function finish3(stream, state) {
  state.pendingcb--;
  if (state.errorEmitted || state.closeEmitted) {
    return;
  }
  state.finished = true;
  for (const callback of state[kOnFinished].splice(0)) {
    callback();
  }
  stream.emit("finish");
  if (state.autoDestroy) {
    stream.destroy();
  }
}
function finishMaybe2(stream, state, sync) {
  if (needFinish(state)) {
    prefinish(stream, state);
    if (state.pendingcb === 0 && needFinish(state)) {
      state.pendingcb++;
      if (sync) {
        queueMicrotask(() => finish3(stream, state));
      } else {
        finish3(stream, state);
      }
    }
  }
}
function onwrite2(stream, er) {
  const state = stream._writableState;
  const sync = state.sync;
  const cb = state.writecb;
  if (typeof cb !== "function") {
    errorOrDestroy3(stream, new ERR_MULTIPLE_CALLBACK());
    return;
  }
  state.writing = false;
  state.writecb = null;
  state.length -= state.writelen;
  state.writelen = 0;
  if (er) {
    er.stack;
    if (!state.errored) {
      state.errored = er;
    }
    if (stream._readableState && !stream._readableState.errored) {
      stream._readableState.errored = er;
    }
    if (sync) {
      queueMicrotask(() => onwriteError2(stream, state, er, cb));
    } else {
      onwriteError2(stream, state, er, cb);
    }
  } else {
    if (state.buffered.length > state.bufferedIndex) {
      clearBuffer(stream, state);
    }
    if (sync) {
      if (
        state.afterWriteTickInfo !== null && state.afterWriteTickInfo.cb === cb
      ) {
        state.afterWriteTickInfo.count++;
      } else {
        state.afterWriteTickInfo = {
          count: 1,
          cb,
          stream,
          state,
        };
        queueMicrotask(() => afterWriteTick(state.afterWriteTickInfo));
      }
    } else {
      afterWrite(stream, state, 1, cb);
    }
  }
}
function onwriteError2(stream, state, er, cb) {
  --state.pendingcb;
  cb(er);
  errorBuffer(state);
  errorOrDestroy3(stream, er);
}
function readableAddChunk2(stream, chunk, encoding = void 0, addToFront) {
  const state = stream._readableState;
  let usedEncoding = encoding;
  let err;
  if (!state.objectMode) {
    if (typeof chunk === "string") {
      usedEncoding = encoding || state.defaultEncoding;
      if (state.encoding !== usedEncoding) {
        if (addToFront && state.encoding) {
          chunk = Buffer3.from(chunk, usedEncoding).toString(state.encoding);
        } else {
          chunk = Buffer3.from(chunk, usedEncoding);
          usedEncoding = "";
        }
      }
    } else if (chunk instanceof Uint8Array) {
      chunk = Buffer3.from(chunk);
    }
  }
  if (err) {
    errorOrDestroy3(stream, err);
  } else if (chunk === null) {
    state.reading = false;
    onEofChunk(stream, state);
  } else if (state.objectMode || chunk.length > 0) {
    if (addToFront) {
      if (state.endEmitted) {
        errorOrDestroy3(stream, new ERR_STREAM_UNSHIFT_AFTER_END_EVENT());
      } else {
        addChunk(stream, state, chunk, true);
      }
    } else if (state.ended) {
      errorOrDestroy3(stream, new ERR_STREAM_PUSH_AFTER_EOF());
    } else if (state.destroyed || state.errored) {
      return false;
    } else {
      state.reading = false;
      if (state.decoder && !usedEncoding) {
        chunk = state.decoder.write(Buffer3.from(chunk));
        if (state.objectMode || chunk.length !== 0) {
          addChunk(stream, state, chunk, false);
        } else {
          maybeReadMore(stream, state);
        }
      } else {
        addChunk(stream, state, chunk, false);
      }
    }
  } else if (!addToFront) {
    state.reading = false;
    maybeReadMore(stream, state);
  }
  return !state.ended &&
    (state.length < state.highWaterMark || state.length === 0);
}

// _stream/duplex.ts
var Duplex = class extends stream_default {
  constructor(options) {
    super();
    this.allowHalfOpen = true;
    this._read = readable_default.prototype._read;
    this._undestroy = readable_default.prototype._undestroy;
    this.isPaused = readable_default.prototype.isPaused;
    this.off = this.removeListener;
    this.pause = readable_default.prototype.pause;
    this.pipe = readable_default.prototype.pipe;
    this.resume = readable_default.prototype.resume;
    this.setEncoding = readable_default.prototype.setEncoding;
    this.unpipe = readable_default.prototype.unpipe;
    this.wrap = readable_default.prototype.wrap;
    this._write = writable_default.prototype._write;
    this.write = writable_default.prototype.write;
    this.cork = writable_default.prototype.cork;
    this.uncork = writable_default.prototype.uncork;
    if (options) {
      if (options.allowHalfOpen === false) {
        this.allowHalfOpen = false;
      }
      if (typeof options.destroy === "function") {
        this._destroy = options.destroy;
      }
      if (typeof options.final === "function") {
        this._final = options.final;
      }
      if (typeof options.read === "function") {
        this._read = options.read;
      }
      if (options.readable === false) {
        this.readable = false;
      }
      if (options.writable === false) {
        this.writable = false;
      }
      if (typeof options.write === "function") {
        this._write = options.write;
      }
      if (typeof options.writev === "function") {
        this._writev = options.writev;
      }
    }
    const readableOptions = {
      autoDestroy: options?.autoDestroy,
      defaultEncoding: options?.defaultEncoding,
      destroy: options?.destroy,
      emitClose: options?.emitClose,
      encoding: options?.encoding,
      highWaterMark: options?.highWaterMark ?? options?.readableHighWaterMark,
      objectMode: options?.objectMode ?? options?.readableObjectMode,
      read: options?.read,
    };
    const writableOptions = {
      autoDestroy: options?.autoDestroy,
      decodeStrings: options?.decodeStrings,
      defaultEncoding: options?.defaultEncoding,
      destroy: options?.destroy,
      emitClose: options?.emitClose,
      final: options?.final,
      highWaterMark: options?.highWaterMark ?? options?.writableHighWaterMark,
      objectMode: options?.objectMode ?? options?.writableObjectMode,
      write: options?.write,
      writev: options?.writev,
    };
    this._readableState = new ReadableState(readableOptions);
    this._writableState = new WritableState(writableOptions, this);
    this._writableState.onwrite = onwrite2.bind(void 0, this);
  }
  [captureRejectionSymbol](err) {
    this.destroy(err);
  }
  [Symbol.asyncIterator]() {
    return async_iterator_default(this);
  }
  _destroy(error, callback) {
    callback(error);
  }
  destroy(err, cb) {
    const r = this._readableState;
    const w = this._writableState;
    if (w.destroyed || r.destroyed) {
      if (typeof cb === "function") {
        cb();
      }
      return this;
    }
    if (err) {
      err.stack;
      if (!w.errored) {
        w.errored = err;
      }
      if (!r.errored) {
        r.errored = err;
      }
    }
    w.destroyed = true;
    r.destroyed = true;
    this._destroy(err || null, (err2) => {
      if (err2) {
        err2.stack;
        if (!w.errored) {
          w.errored = err2;
        }
        if (!r.errored) {
          r.errored = err2;
        }
      }
      w.closed = true;
      r.closed = true;
      if (typeof cb === "function") {
        cb(err2);
      }
      if (err2) {
        queueMicrotask(() => {
          const r2 = this._readableState;
          const w2 = this._writableState;
          if (!w2.errorEmitted && !r2.errorEmitted) {
            w2.errorEmitted = true;
            r2.errorEmitted = true;
            this.emit("error", err2);
          }
          r2.closeEmitted = true;
          if (w2.emitClose || r2.emitClose) {
            this.emit("close");
          }
        });
      } else {
        queueMicrotask(() => {
          const r2 = this._readableState;
          const w2 = this._writableState;
          r2.closeEmitted = true;
          if (w2.emitClose || r2.emitClose) {
            this.emit("close");
          }
        });
      }
    });
    return this;
  }
  on(ev, fn) {
    const res = super.on.call(this, ev, fn);
    const state = this._readableState;
    if (ev === "data") {
      state.readableListening = this.listenerCount("readable") > 0;
      if (state.flowing !== false) {
        this.resume();
      }
    } else if (ev === "readable") {
      if (!state.endEmitted && !state.readableListening) {
        state.readableListening = state.needReadable = true;
        state.flowing = false;
        state.emittedReadable = false;
        if (state.length) {
          emitReadable(this);
        } else if (!state.reading) {
          queueMicrotask(() => nReadingNextTick(this));
        }
      }
    }
    return res;
  }
  push(chunk, encoding) {
    return readableAddChunk2(this, chunk, encoding, false);
  }
  read(n) {
    if (n === void 0) {
      n = NaN;
    }
    const state = this._readableState;
    const nOrig = n;
    if (n > state.highWaterMark) {
      state.highWaterMark = computeNewHighWaterMark(n);
    }
    if (n !== 0) {
      state.emittedReadable = false;
    }
    if (
      n === 0 &&
      state.needReadable &&
      ((state.highWaterMark !== 0
        ? state.length >= state.highWaterMark
        : state.length > 0) || state.ended)
    ) {
      if (state.length === 0 && state.ended) {
        endDuplex(this);
      } else {
        emitReadable(this);
      }
      return null;
    }
    n = howMuchToRead(n, state);
    if (n === 0 && state.ended) {
      if (state.length === 0) {
        endDuplex(this);
      }
      return null;
    }
    let doRead = state.needReadable;
    if (state.length === 0 || state.length - n < state.highWaterMark) {
      doRead = true;
    }
    if (
      state.ended || state.reading || state.destroyed || state.errored ||
      !state.constructed
    ) {
      doRead = false;
    } else if (doRead) {
      state.reading = true;
      state.sync = true;
      if (state.length === 0) {
        state.needReadable = true;
      }
      this._read();
      state.sync = false;
      if (!state.reading) {
        n = howMuchToRead(nOrig, state);
      }
    }
    let ret;
    if (n > 0) {
      ret = fromList(n, state);
    } else {
      ret = null;
    }
    if (ret === null) {
      state.needReadable = state.length <= state.highWaterMark;
      n = 0;
    } else {
      state.length -= n;
      if (state.multiAwaitDrain) {
        state.awaitDrainWriters.clear();
      } else {
        state.awaitDrainWriters = null;
      }
    }
    if (state.length === 0) {
      if (!state.ended) {
        state.needReadable = true;
      }
      if (nOrig !== n && state.ended) {
        endDuplex(this);
      }
    }
    if (ret !== null) {
      this.emit("data", ret);
    }
    return ret;
  }
  removeAllListeners(ev) {
    const res = super.removeAllListeners(ev);
    if (ev === "readable" || ev === void 0) {
      queueMicrotask(() => updateReadableListening(this));
    }
    return res;
  }
  removeListener(ev, fn) {
    const res = super.removeListener.call(this, ev, fn);
    if (ev === "readable") {
      queueMicrotask(() => updateReadableListening(this));
    }
    return res;
  }
  unshift(chunk, encoding) {
    return readableAddChunk2(this, chunk, encoding, true);
  }
  get readable() {
    return (
      this._readableState?.readable &&
      !this._readableState?.destroyed &&
      !this._readableState?.errorEmitted &&
      !this._readableState?.endEmitted
    );
  }
  set readable(val) {
    if (this._readableState) {
      this._readableState.readable = val;
    }
  }
  get readableHighWaterMark() {
    return this._readableState.highWaterMark;
  }
  get readableBuffer() {
    return this._readableState && this._readableState.buffer;
  }
  get readableFlowing() {
    return this._readableState.flowing;
  }
  set readableFlowing(state) {
    if (this._readableState) {
      this._readableState.flowing = state;
    }
  }
  get readableLength() {
    return this._readableState.length;
  }
  get readableObjectMode() {
    return this._readableState ? this._readableState.objectMode : false;
  }
  get readableEncoding() {
    return this._readableState ? this._readableState.encoding : null;
  }
  get readableEnded() {
    return this._readableState ? this._readableState.endEmitted : false;
  }
  setDefaultEncoding(encoding) {
    if (typeof encoding === "string") {
      encoding = encoding.toLowerCase();
    }
    if (!Buffer3.isEncoding(encoding)) {
      throw new ERR_UNKNOWN_ENCODING(encoding);
    }
    this._writableState.defaultEncoding = encoding;
    return this;
  }
  end(x, y, z) {
    const state = this._writableState;
    let chunk;
    let encoding;
    let cb;
    if (typeof x === "function") {
      chunk = null;
      encoding = null;
      cb = x;
    } else if (typeof y === "function") {
      chunk = x;
      encoding = null;
      cb = y;
    } else {
      chunk = x;
      encoding = y;
      cb = z;
    }
    if (chunk !== null && chunk !== void 0) {
      this.write(chunk, encoding);
    }
    if (state.corked) {
      state.corked = 1;
      this.uncork();
    }
    let err;
    if (!state.errored && !state.ending) {
      state.ending = true;
      finishMaybe2(this, state, true);
      state.ended = true;
    } else if (state.finished) {
      err = new ERR_STREAM_ALREADY_FINISHED("end");
    } else if (state.destroyed) {
      err = new ERR_STREAM_DESTROYED("end");
    }
    if (typeof cb === "function") {
      if (err || state.finished) {
        queueMicrotask(() => {
          cb(err);
        });
      } else {
        state[kOnFinished].push(cb);
      }
    }
    return this;
  }
  get destroyed() {
    if (this._readableState === void 0 || this._writableState === void 0) {
      return false;
    }
    return this._readableState.destroyed && this._writableState.destroyed;
  }
  set destroyed(value) {
    if (this._readableState && this._writableState) {
      this._readableState.destroyed = value;
      this._writableState.destroyed = value;
    }
  }
  get writable() {
    const w = this._writableState;
    return !w.destroyed && !w.errored && !w.ending && !w.ended;
  }
  set writable(val) {
    if (this._writableState) {
      this._writableState.writable = !!val;
    }
  }
  get writableFinished() {
    return this._writableState ? this._writableState.finished : false;
  }
  get writableObjectMode() {
    return this._writableState ? this._writableState.objectMode : false;
  }
  get writableBuffer() {
    return this._writableState && this._writableState.getBuffer();
  }
  get writableEnded() {
    return this._writableState ? this._writableState.ending : false;
  }
  get writableHighWaterMark() {
    return this._writableState && this._writableState.highWaterMark;
  }
  get writableCorked() {
    return this._writableState ? this._writableState.corked : 0;
  }
  get writableLength() {
    return this._writableState && this._writableState.length;
  }
};
var duplex_default = Duplex;

// _stream/transform.ts
var kCallback = Symbol("kCallback");
var Transform = class extends duplex_default {
  constructor(options) {
    super(options);
    this._read = () => {
      if (this[kCallback]) {
        const callback = this[kCallback];
        this[kCallback] = null;
        callback();
      }
    };
    this._write = (chunk, encoding, callback) => {
      const rState = this._readableState;
      const wState = this._writableState;
      const length = rState.length;
      this._transform(chunk, encoding, (err, val) => {
        if (err) {
          callback(err);
          return;
        }
        if (val != null) {
          this.push(val);
        }
        if (
          wState.ended || length === rState.length ||
          rState.length < rState.highWaterMark || rState.length === 0
        ) {
          callback();
        } else {
          this[kCallback] = callback;
        }
      });
    };
    this._readableState.sync = false;
    this[kCallback] = null;
    if (options) {
      if (typeof options.transform === "function") {
        this._transform = options.transform;
      }
      if (typeof options.flush === "function") {
        this._flush = options.flush;
      }
    }
    this.on("prefinish", function () {
      if (typeof this._flush === "function" && !this.destroyed) {
        this._flush((er, data) => {
          if (er) {
            this.destroy(er);
            return;
          }
          if (data != null) {
            this.push(data);
          }
          this.push(null);
        });
      } else {
        this.push(null);
      }
    });
  }
  _transform(_chunk, _encoding, _callback) {
    throw new ERR_METHOD_NOT_IMPLEMENTED("_transform()");
  }
};
kCallback;

// _stream/passthrough.ts
var PassThrough = class extends Transform {
  constructor(options) {
    super(options);
  }
  _transform(chunk, _encoding, cb) {
    cb(null, chunk);
  }
};

// _stream/pipeline.ts
function destroyer2(stream, reading, writing, callback) {
  callback = once(callback);
  let finished2 = false;
  stream.on("close", () => {
    finished2 = true;
  });
  eos(stream, { readable: reading, writable: writing }, (err) => {
    finished2 = !err;
    const rState = stream?._readableState;
    if (
      err &&
      err.code === "ERR_STREAM_PREMATURE_CLOSE" &&
      reading &&
      rState?.ended &&
      !rState?.errored &&
      !rState?.errorEmitted
    ) {
      stream.once("end", callback).once("error", callback);
    } else {
      callback(err);
    }
  });
  return (err) => {
    if (finished2) return;
    finished2 = true;
    destroyer(stream, err);
    callback(err || new ERR_STREAM_DESTROYED("pipe"));
  };
}
function popCallback(streams) {
  if (typeof streams[streams.length - 1] !== "function") {
    throw new ERR_INVALID_CALLBACK(streams[streams.length - 1]);
  }
  return streams.pop();
}
function isReadable2(obj) {
  return !!(obj && typeof obj.pipe === "function");
}
function isWritable2(obj) {
  return !!(obj && typeof obj.write === "function");
}
function isStream(obj) {
  return isReadable2(obj) || isWritable2(obj);
}
function isIterable(obj, isAsync) {
  if (!obj) return false;
  if (isAsync === true) return typeof obj[Symbol.asyncIterator] === "function";
  if (isAsync === false) return typeof obj[Symbol.iterator] === "function";
  return typeof obj[Symbol.asyncIterator] === "function" ||
    typeof obj[Symbol.iterator] === "function";
}
function makeAsyncIterable(val) {
  if (isIterable(val)) {
    return val;
  } else if (isReadable2(val)) {
    return fromReadable(val);
  }
  throw new ERR_INVALID_ARG_TYPE("val", [
    "Readable",
    "Iterable",
    "AsyncIterable",
  ], val);
}
async function* fromReadable(val) {
  yield* async_iterator_default(val);
}
async function pump(iterable, writable, finish4) {
  let error = null;
  try {
    for await (const chunk of iterable) {
      if (!writable.write(chunk)) {
        if (writable.destroyed) return;
        await once2(writable, "drain");
      }
    }
    writable.end();
  } catch (err) {
    if (err instanceof NodeErrorAbstraction) {
      error = err;
    }
  } finally {
    finish4(error);
  }
}
function pipeline(...args) {
  const callback = once(popCallback(args));
  let streams;
  if (args.length > 1) {
    streams = args;
  } else {
    throw new ERR_MISSING_ARGS("streams");
  }
  let error;
  let value;
  const destroys = [];
  let finishCount = 0;
  function finish4(err) {
    const final = --finishCount === 0;
    if (err && (!error || error.code === "ERR_STREAM_PREMATURE_CLOSE")) {
      error = err;
    }
    if (!error && !final) {
      return;
    }
    while (destroys.length) {
      destroys.shift()(error);
    }
    if (final) {
      callback(error, value);
    }
  }
  let ret;
  for (let i = 0; i < streams.length; i++) {
    const stream = streams[i];
    const reading = i < streams.length - 1;
    const writing = i > 0;
    if (isStream(stream)) {
      finishCount++;
      destroys.push(destroyer2(stream, reading, writing, finish4));
    }
    if (i === 0) {
      if (typeof stream === "function") {
        ret = stream();
        if (!isIterable(ret)) {
          throw new ERR_INVALID_RETURN_VALUE(
            "Iterable, AsyncIterable or Stream",
            "source",
            ret,
          );
        }
      } else if (isIterable(stream) || isReadable2(stream)) {
        ret = stream;
      } else {
        throw new ERR_INVALID_ARG_TYPE("source", [
          "Stream",
          "Iterable",
          "AsyncIterable",
          "Function",
        ], stream);
      }
    } else if (typeof stream === "function") {
      ret = makeAsyncIterable(ret);
      ret = stream(ret);
      if (reading) {
        if (!isIterable(ret, true)) {
          throw new ERR_INVALID_RETURN_VALUE(
            "AsyncIterable",
            `transform[${i - 1}]`,
            ret,
          );
        }
      } else {
        const pt = new PassThrough({
          objectMode: true,
        });
        if (ret instanceof Promise) {
          ret.then(
            (val) => {
              value = val;
              pt.end(val);
            },
            (err) => {
              pt.destroy(err);
            },
          );
        } else if (isIterable(ret, true)) {
          finishCount++;
          pump(ret, pt, finish4);
        } else {
          throw new ERR_INVALID_RETURN_VALUE(
            "AsyncIterable or Promise",
            "destination",
            ret,
          );
        }
        ret = pt;
        finishCount++;
        destroys.push(destroyer2(ret, false, true, finish4));
      }
    } else if (isStream(stream)) {
      if (isReadable2(ret)) {
        ret.pipe(stream);
      } else {
        ret = makeAsyncIterable(ret);
        finishCount++;
        pump(ret, stream, finish4);
      }
      ret = stream;
    } else {
      const name = reading ? `transform[${i - 1}]` : "destination";
      throw new ERR_INVALID_ARG_TYPE(name, ["Stream", "Function"], ret);
    }
  }
  return ret;
}

// _stream/promises.ts
var promises_exports = {};
__export(promises_exports, {
  finished: () => finished,
  pipeline: () => pipeline2,
});
function pipeline2(...streams) {
  return new Promise((resolve, reject) => {
    pipeline(...streams, (err, value) => {
      if (err) {
        reject(err);
      } else {
        resolve(value);
      }
    });
  });
}
function finished(stream, opts) {
  return new Promise((resolve, reject) => {
    eos(stream, opts || null, (err) => {
      if (err) {
        reject(err);
      } else {
        resolve();
      }
    });
  });
}

// stream.ts
stream_default.Readable = readable_default;
stream_default.Writable = writable_default;
stream_default.Duplex = duplex_default;
stream_default.Transform = Transform;
stream_default.PassThrough = PassThrough;
stream_default.pipeline = pipeline;
stream_default.finished = eos;
stream_default.promises = promises_exports;
stream_default.Stream = stream_default;
var stream_default2 = stream_default;
var { _isUint8Array, _uint8ArrayToBuffer } = stream_default;
export {
  _isUint8Array,
  _uint8ArrayToBuffer,
  duplex_default as Duplex,
  eos as finished,
  PassThrough,
  pipeline,
  promises_exports as promises,
  readable_default as Readable,
  stream_default as Stream,
  stream_default2 as default,
  Transform,
  writable_default as Writable,
};
