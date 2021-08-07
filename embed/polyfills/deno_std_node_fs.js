/* deno mod bundle
 * entry: deno.land/std/node/fs.ts
 * version: 0.103.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild fs.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_fs.js
 */

var __defProp = Object.defineProperty;
var __markAsModule = (target) => __defProp(target, "__esModule", { value: true });
var __export = (target, all) => {
  __markAsModule(target);
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};

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
      }
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
var noop = () => {
};
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

// ../fmt/colors.ts
var { Deno: Deno2 } = globalThis;
var noColor = typeof Deno2?.noColor === "boolean" ? Deno2.noColor : true;
var ANSI_PATTERN = new RegExp([
  "[\\u001B\\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?\\u0007)",
  "(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))"
].join("|"), "g");

// ../testing/_diff.ts
var DiffType;
(function(DiffType2) {
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
    if (options.copy === false)
      return this.#buf.subarray(this.#off);
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
      const buf = shouldGrow ? tmp : new Uint8Array(this.#buf.buffer, this.length);
      const nread = await r.read(buf);
      if (nread === null) {
        return n;
      }
      if (shouldGrow)
        this.writeSync(buf.subarray(0, nread));
      else
        this.#reslice(this.length + nread);
      n += nread;
    }
  }
  readFromSync(r) {
    let n = 0;
    const tmp = new Uint8Array(MIN_READ);
    while (true) {
      const shouldGrow = this.capacity - this.length < MIN_READ;
      const buf = shouldGrow ? tmp : new Uint8Array(this.#buf.buffer, this.length);
      const nread = r.readSync(buf);
      if (nread === null) {
        return n;
      }
      if (shouldGrow)
        this.writeSync(buf.subarray(0, nread));
      else
        this.#reslice(this.length + nread);
      n += nread;
    }
  }
};

// ../io/util.ts
var DEFAULT_BUFFER_SIZE = 32 * 1024;
async function writeAll(w, arr) {
  let nwritten = 0;
  while (nwritten < arr.length) {
    nwritten += await w.write(arr.subarray(nwritten));
  }
}
function writeAllSync(w, arr) {
  let nwritten = 0;
  while (nwritten < arr.length) {
    nwritten += w.writeSync(arr.subarray(nwritten));
  }
}

// _utils.ts
function notImplemented(msg) {
  const message = msg ? `Not implemented: ${msg}` : "Not implemented";
  throw new Error(message);
}
function intoCallbackAPIWithIntercept(func, interceptor, cb, ...args) {
  func(...args).then((value) => cb && cb(null, interceptor(value)), (err) => cb && cb(err));
}
function normalizeEncoding(enc) {
  if (enc == null || enc === "utf8" || enc === "utf-8")
    return "utf8";
  return slowCases(enc);
}
function slowCases(enc) {
  switch (enc.length) {
    case 4:
      if (enc === "UTF8")
        return "utf8";
      if (enc === "ucs2" || enc === "UCS2")
        return "utf16le";
      enc = `${enc}`.toLowerCase();
      if (enc === "utf8")
        return "utf8";
      if (enc === "ucs2")
        return "utf16le";
      break;
    case 3:
      if (enc === "hex" || enc === "HEX" || `${enc}`.toLowerCase() === "hex") {
        return "hex";
      }
      break;
    case 5:
      if (enc === "ascii")
        return "ascii";
      if (enc === "ucs-2")
        return "utf16le";
      if (enc === "UTF-8")
        return "utf8";
      if (enc === "ASCII")
        return "ascii";
      if (enc === "UCS-2")
        return "utf16le";
      enc = `${enc}`.toLowerCase();
      if (enc === "utf-8")
        return "utf8";
      if (enc === "ascii")
        return "ascii";
      if (enc === "ucs-2")
        return "utf16le";
      break;
    case 6:
      if (enc === "base64")
        return "base64";
      if (enc === "latin1" || enc === "binary")
        return "latin1";
      if (enc === "BASE64")
        return "base64";
      if (enc === "LATIN1" || enc === "BINARY")
        return "latin1";
      enc = `${enc}`.toLowerCase();
      if (enc === "base64")
        return "base64";
      if (enc === "latin1" || enc === "binary")
        return "latin1";
      break;
    case 7:
      if (enc === "utf16le" || enc === "UTF16LE" || `${enc}`.toLowerCase() === "utf16le") {
        return "utf16le";
      }
      break;
    case 8:
      if (enc === "utf-16le" || enc === "UTF-16LE" || `${enc}`.toLowerCase() === "utf-16le") {
        return "utf16le";
      }
      break;
    default:
      if (enc === "")
        return "utf8";
  }
}

// _fs/_fs_access.ts
function access(_path, _modeOrCallback, _callback) {
  notImplemented("Not yet available");
}
function accessSync(_path, _mode) {
  notImplemented("Not yet available");
}

// _fs/_fs_common.ts
function isFileOptions(fileOptions) {
  if (!fileOptions)
    return false;
  return fileOptions.encoding != void 0 || fileOptions.flag != void 0 || fileOptions.mode != void 0;
}
function getEncoding(optOrCallback) {
  if (!optOrCallback || typeof optOrCallback === "function") {
    return null;
  }
  const encoding = typeof optOrCallback === "string" ? optOrCallback : optOrCallback.encoding;
  if (!encoding)
    return null;
  return encoding;
}
function checkEncoding(encoding) {
  if (!encoding)
    return null;
  encoding = encoding.toLowerCase();
  if (["utf8", "hex", "base64"].includes(encoding))
    return encoding;
  if (encoding === "utf-8") {
    return "utf8";
  }
  if (encoding === "binary") {
    return "binary";
  }
  const notImplementedEncodings2 = ["utf16le", "latin1", "ascii", "ucs2"];
  if (notImplementedEncodings2.includes(encoding)) {
    notImplemented(`"${encoding}" encoding`);
  }
  throw new Error(`The value "${encoding}" is invalid for option "encoding"`);
}
function getOpenOptions(flag) {
  if (!flag) {
    return { create: true, append: true };
  }
  let openOptions;
  switch (flag) {
    case "a": {
      openOptions = { create: true, append: true };
      break;
    }
    case "ax": {
      openOptions = { createNew: true, write: true, append: true };
      break;
    }
    case "a+": {
      openOptions = { read: true, create: true, append: true };
      break;
    }
    case "ax+": {
      openOptions = { read: true, createNew: true, append: true };
      break;
    }
    case "r": {
      openOptions = { read: true };
      break;
    }
    case "r+": {
      openOptions = { read: true, write: true };
      break;
    }
    case "w": {
      openOptions = { create: true, write: true, truncate: true };
      break;
    }
    case "wx": {
      openOptions = { createNew: true, write: true };
      break;
    }
    case "w+": {
      openOptions = { create: true, write: true, truncate: true, read: true };
      break;
    }
    case "wx+": {
      openOptions = { createNew: true, write: true, read: true };
      break;
    }
    case "as": {
      openOptions = { create: true, append: true };
      break;
    }
    case "as+": {
      openOptions = { create: true, read: true, append: true };
      break;
    }
    case "rs+": {
      openOptions = { create: true, read: true, write: true };
      break;
    }
    default: {
      throw new Error(`Unrecognized file system flag: ${flag}`);
    }
  }
  return openOptions;
}

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
  win32: () => win32
});

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
  toNamespacedPath: () => toNamespacedPath
});

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

// ../path/_util.ts
function assertPath(path3) {
  if (typeof path3 !== "string") {
    throw new TypeError(`Path must be a string. Received ${JSON.stringify(path3)}`);
  }
}
function isPosixPathSeparator(code) {
  return code === CHAR_FORWARD_SLASH;
}
function isPathSeparator(code) {
  return isPosixPathSeparator(code) || code === CHAR_BACKWARD_SLASH;
}
function isWindowsDeviceRoot(code) {
  return code >= CHAR_LOWERCASE_A && code <= CHAR_LOWERCASE_Z || code >= CHAR_UPPERCASE_A && code <= CHAR_UPPERCASE_Z;
}
function normalizeString(path3, allowAboveRoot, separator, isPathSeparator2) {
  let res = "";
  let lastSegmentLength = 0;
  let lastSlash = -1;
  let dots = 0;
  let code;
  for (let i = 0, len = path3.length; i <= len; ++i) {
    if (i < len)
      code = path3.charCodeAt(i);
    else if (isPathSeparator2(code))
      break;
    else
      code = CHAR_FORWARD_SLASH;
    if (isPathSeparator2(code)) {
      if (lastSlash === i - 1 || dots === 1) {
      } else if (lastSlash !== i - 1 && dots === 2) {
        if (res.length < 2 || lastSegmentLength !== 2 || res.charCodeAt(res.length - 1) !== CHAR_DOT || res.charCodeAt(res.length - 2) !== CHAR_DOT) {
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
          if (res.length > 0)
            res += `${separator}..`;
          else
            res = "..";
          lastSegmentLength = 2;
        }
      } else {
        if (res.length > 0)
          res += separator + path3.slice(lastSlash + 1, i);
        else
          res = path3.slice(lastSlash + 1, i);
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
  const base = pathObject.base || (pathObject.name || "") + (pathObject.ext || "");
  if (!dir)
    return base;
  if (dir === pathObject.root)
    return dir + base;
  return dir + sep4 + base;
}
var WHITESPACE_ENCODINGS = {
  "	": "%09",
  "\n": "%0A",
  "\v": "%0B",
  "\f": "%0C",
  "\r": "%0D",
  " ": "%20"
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
    let path3;
    const { Deno: Deno3 } = globalThis;
    if (i >= 0) {
      path3 = pathSegments[i];
    } else if (!resolvedDevice) {
      if (typeof Deno3?.cwd !== "function") {
        throw new TypeError("Resolved a drive-letter-less path without a CWD.");
      }
      path3 = Deno3.cwd();
    } else {
      if (typeof Deno3?.env?.get !== "function" || typeof Deno3?.cwd !== "function") {
        throw new TypeError("Resolved a relative path without a CWD.");
      }
      path3 = Deno3.env.get(`=${resolvedDevice}`) || Deno3.cwd();
      if (path3 === void 0 || path3.slice(0, 3).toLowerCase() !== `${resolvedDevice.toLowerCase()}\\`) {
        path3 = `${resolvedDevice}\\`;
      }
    }
    assertPath(path3);
    const len = path3.length;
    if (len === 0)
      continue;
    let rootEnd = 0;
    let device = "";
    let isAbsolute4 = false;
    const code = path3.charCodeAt(0);
    if (len > 1) {
      if (isPathSeparator(code)) {
        isAbsolute4 = true;
        if (isPathSeparator(path3.charCodeAt(1))) {
          let j = 2;
          let last = j;
          for (; j < len; ++j) {
            if (isPathSeparator(path3.charCodeAt(j)))
              break;
          }
          if (j < len && j !== last) {
            const firstPart = path3.slice(last, j);
            last = j;
            for (; j < len; ++j) {
              if (!isPathSeparator(path3.charCodeAt(j)))
                break;
            }
            if (j < len && j !== last) {
              last = j;
              for (; j < len; ++j) {
                if (isPathSeparator(path3.charCodeAt(j)))
                  break;
              }
              if (j === len) {
                device = `\\\\${firstPart}\\${path3.slice(last)}`;
                rootEnd = j;
              } else if (j !== last) {
                device = `\\\\${firstPart}\\${path3.slice(last, j)}`;
                rootEnd = j;
              }
            }
          }
        } else {
          rootEnd = 1;
        }
      } else if (isWindowsDeviceRoot(code)) {
        if (path3.charCodeAt(1) === CHAR_COLON) {
          device = path3.slice(0, 2);
          rootEnd = 2;
          if (len > 2) {
            if (isPathSeparator(path3.charCodeAt(2))) {
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
    if (device.length > 0 && resolvedDevice.length > 0 && device.toLowerCase() !== resolvedDevice.toLowerCase()) {
      continue;
    }
    if (resolvedDevice.length === 0 && device.length > 0) {
      resolvedDevice = device;
    }
    if (!resolvedAbsolute) {
      resolvedTail = `${path3.slice(rootEnd)}\\${resolvedTail}`;
      resolvedAbsolute = isAbsolute4;
    }
    if (resolvedAbsolute && resolvedDevice.length > 0)
      break;
  }
  resolvedTail = normalizeString(resolvedTail, !resolvedAbsolute, "\\", isPathSeparator);
  return resolvedDevice + (resolvedAbsolute ? "\\" : "") + resolvedTail || ".";
}
function normalize(path3) {
  assertPath(path3);
  const len = path3.length;
  if (len === 0)
    return ".";
  let rootEnd = 0;
  let device;
  let isAbsolute4 = false;
  const code = path3.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      isAbsolute4 = true;
      if (isPathSeparator(path3.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path3.charCodeAt(j)))
            break;
        }
        if (j < len && j !== last) {
          const firstPart = path3.slice(last, j);
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path3.charCodeAt(j)))
              break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path3.charCodeAt(j)))
                break;
            }
            if (j === len) {
              return `\\\\${firstPart}\\${path3.slice(last)}\\`;
            } else if (j !== last) {
              device = `\\\\${firstPart}\\${path3.slice(last, j)}`;
              rootEnd = j;
            }
          }
        }
      } else {
        rootEnd = 1;
      }
    } else if (isWindowsDeviceRoot(code)) {
      if (path3.charCodeAt(1) === CHAR_COLON) {
        device = path3.slice(0, 2);
        rootEnd = 2;
        if (len > 2) {
          if (isPathSeparator(path3.charCodeAt(2))) {
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
    tail = normalizeString(path3.slice(rootEnd), !isAbsolute4, "\\", isPathSeparator);
  } else {
    tail = "";
  }
  if (tail.length === 0 && !isAbsolute4)
    tail = ".";
  if (tail.length > 0 && isPathSeparator(path3.charCodeAt(len - 1))) {
    tail += "\\";
  }
  if (device === void 0) {
    if (isAbsolute4) {
      if (tail.length > 0)
        return `\\${tail}`;
      else
        return "\\";
    } else if (tail.length > 0) {
      return tail;
    } else {
      return "";
    }
  } else if (isAbsolute4) {
    if (tail.length > 0)
      return `${device}\\${tail}`;
    else
      return `${device}\\`;
  } else if (tail.length > 0) {
    return device + tail;
  } else {
    return device;
  }
}
function isAbsolute(path3) {
  assertPath(path3);
  const len = path3.length;
  if (len === 0)
    return false;
  const code = path3.charCodeAt(0);
  if (isPathSeparator(code)) {
    return true;
  } else if (isWindowsDeviceRoot(code)) {
    if (len > 2 && path3.charCodeAt(1) === CHAR_COLON) {
      if (isPathSeparator(path3.charCodeAt(2)))
        return true;
    }
  }
  return false;
}
function join(...paths) {
  const pathsCount = paths.length;
  if (pathsCount === 0)
    return ".";
  let joined;
  let firstPart = null;
  for (let i = 0; i < pathsCount; ++i) {
    const path3 = paths[i];
    assertPath(path3);
    if (path3.length > 0) {
      if (joined === void 0)
        joined = firstPart = path3;
      else
        joined += `\\${path3}`;
    }
  }
  if (joined === void 0)
    return ".";
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
          if (isPathSeparator(firstPart.charCodeAt(2)))
            ++slashCount;
          else {
            needsReplace = false;
          }
        }
      }
    }
  }
  if (needsReplace) {
    for (; slashCount < joined.length; ++slashCount) {
      if (!isPathSeparator(joined.charCodeAt(slashCount)))
        break;
    }
    if (slashCount >= 2)
      joined = `\\${joined.slice(slashCount)}`;
  }
  return normalize(joined);
}
function relative(from, to) {
  assertPath(from);
  assertPath(to);
  if (from === to)
    return "";
  const fromOrig = resolve(from);
  const toOrig = resolve(to);
  if (fromOrig === toOrig)
    return "";
  from = fromOrig.toLowerCase();
  to = toOrig.toLowerCase();
  if (from === to)
    return "";
  let fromStart = 0;
  let fromEnd = from.length;
  for (; fromStart < fromEnd; ++fromStart) {
    if (from.charCodeAt(fromStart) !== CHAR_BACKWARD_SLASH)
      break;
  }
  for (; fromEnd - 1 > fromStart; --fromEnd) {
    if (from.charCodeAt(fromEnd - 1) !== CHAR_BACKWARD_SLASH)
      break;
  }
  const fromLen = fromEnd - fromStart;
  let toStart = 0;
  let toEnd = to.length;
  for (; toStart < toEnd; ++toStart) {
    if (to.charCodeAt(toStart) !== CHAR_BACKWARD_SLASH)
      break;
  }
  for (; toEnd - 1 > toStart; --toEnd) {
    if (to.charCodeAt(toEnd - 1) !== CHAR_BACKWARD_SLASH)
      break;
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
    if (fromCode !== toCode)
      break;
    else if (fromCode === CHAR_BACKWARD_SLASH)
      lastCommonSep = i;
  }
  if (i !== length && lastCommonSep === -1) {
    return toOrig;
  }
  let out = "";
  if (lastCommonSep === -1)
    lastCommonSep = 0;
  for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
    if (i === fromEnd || from.charCodeAt(i) === CHAR_BACKWARD_SLASH) {
      if (out.length === 0)
        out += "..";
      else
        out += "\\..";
    }
  }
  if (out.length > 0) {
    return out + toOrig.slice(toStart + lastCommonSep, toEnd);
  } else {
    toStart += lastCommonSep;
    if (toOrig.charCodeAt(toStart) === CHAR_BACKWARD_SLASH)
      ++toStart;
    return toOrig.slice(toStart, toEnd);
  }
}
function toNamespacedPath(path3) {
  if (typeof path3 !== "string")
    return path3;
  if (path3.length === 0)
    return "";
  const resolvedPath = resolve(path3);
  if (resolvedPath.length >= 3) {
    if (resolvedPath.charCodeAt(0) === CHAR_BACKWARD_SLASH) {
      if (resolvedPath.charCodeAt(1) === CHAR_BACKWARD_SLASH) {
        const code = resolvedPath.charCodeAt(2);
        if (code !== CHAR_QUESTION_MARK && code !== CHAR_DOT) {
          return `\\\\?\\UNC\\${resolvedPath.slice(2)}`;
        }
      }
    } else if (isWindowsDeviceRoot(resolvedPath.charCodeAt(0))) {
      if (resolvedPath.charCodeAt(1) === CHAR_COLON && resolvedPath.charCodeAt(2) === CHAR_BACKWARD_SLASH) {
        return `\\\\?\\${resolvedPath}`;
      }
    }
  }
  return path3;
}
function dirname(path3) {
  assertPath(path3);
  const len = path3.length;
  if (len === 0)
    return ".";
  let rootEnd = -1;
  let end = -1;
  let matchedSlash = true;
  let offset = 0;
  const code = path3.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      rootEnd = offset = 1;
      if (isPathSeparator(path3.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path3.charCodeAt(j)))
            break;
        }
        if (j < len && j !== last) {
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path3.charCodeAt(j)))
              break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path3.charCodeAt(j)))
                break;
            }
            if (j === len) {
              return path3;
            }
            if (j !== last) {
              rootEnd = offset = j + 1;
            }
          }
        }
      }
    } else if (isWindowsDeviceRoot(code)) {
      if (path3.charCodeAt(1) === CHAR_COLON) {
        rootEnd = offset = 2;
        if (len > 2) {
          if (isPathSeparator(path3.charCodeAt(2)))
            rootEnd = offset = 3;
        }
      }
    }
  } else if (isPathSeparator(code)) {
    return path3;
  }
  for (let i = len - 1; i >= offset; --i) {
    if (isPathSeparator(path3.charCodeAt(i))) {
      if (!matchedSlash) {
        end = i;
        break;
      }
    } else {
      matchedSlash = false;
    }
  }
  if (end === -1) {
    if (rootEnd === -1)
      return ".";
    else
      end = rootEnd;
  }
  return path3.slice(0, end);
}
function basename(path3, ext = "") {
  if (ext !== void 0 && typeof ext !== "string") {
    throw new TypeError('"ext" argument must be a string');
  }
  assertPath(path3);
  let start = 0;
  let end = -1;
  let matchedSlash = true;
  let i;
  if (path3.length >= 2) {
    const drive = path3.charCodeAt(0);
    if (isWindowsDeviceRoot(drive)) {
      if (path3.charCodeAt(1) === CHAR_COLON)
        start = 2;
    }
  }
  if (ext !== void 0 && ext.length > 0 && ext.length <= path3.length) {
    if (ext.length === path3.length && ext === path3)
      return "";
    let extIdx = ext.length - 1;
    let firstNonSlashEnd = -1;
    for (i = path3.length - 1; i >= start; --i) {
      const code = path3.charCodeAt(i);
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
    if (start === end)
      end = firstNonSlashEnd;
    else if (end === -1)
      end = path3.length;
    return path3.slice(start, end);
  } else {
    for (i = path3.length - 1; i >= start; --i) {
      if (isPathSeparator(path3.charCodeAt(i))) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
    }
    if (end === -1)
      return "";
    return path3.slice(start, end);
  }
}
function extname(path3) {
  assertPath(path3);
  let start = 0;
  let startDot = -1;
  let startPart = 0;
  let end = -1;
  let matchedSlash = true;
  let preDotState = 0;
  if (path3.length >= 2 && path3.charCodeAt(1) === CHAR_COLON && isWindowsDeviceRoot(path3.charCodeAt(0))) {
    start = startPart = 2;
  }
  for (let i = path3.length - 1; i >= start; --i) {
    const code = path3.charCodeAt(i);
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
      if (startDot === -1)
        startDot = i;
      else if (preDotState !== 1)
        preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (startDot === -1 || end === -1 || preDotState === 0 || preDotState === 1 && startDot === end - 1 && startDot === startPart + 1) {
    return "";
  }
  return path3.slice(startDot, end);
}
function format(pathObject) {
  if (pathObject === null || typeof pathObject !== "object") {
    throw new TypeError(`The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`);
  }
  return _format("\\", pathObject);
}
function parse(path3) {
  assertPath(path3);
  const ret = { root: "", dir: "", base: "", ext: "", name: "" };
  const len = path3.length;
  if (len === 0)
    return ret;
  let rootEnd = 0;
  let code = path3.charCodeAt(0);
  if (len > 1) {
    if (isPathSeparator(code)) {
      rootEnd = 1;
      if (isPathSeparator(path3.charCodeAt(1))) {
        let j = 2;
        let last = j;
        for (; j < len; ++j) {
          if (isPathSeparator(path3.charCodeAt(j)))
            break;
        }
        if (j < len && j !== last) {
          last = j;
          for (; j < len; ++j) {
            if (!isPathSeparator(path3.charCodeAt(j)))
              break;
          }
          if (j < len && j !== last) {
            last = j;
            for (; j < len; ++j) {
              if (isPathSeparator(path3.charCodeAt(j)))
                break;
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
      if (path3.charCodeAt(1) === CHAR_COLON) {
        rootEnd = 2;
        if (len > 2) {
          if (isPathSeparator(path3.charCodeAt(2))) {
            if (len === 3) {
              ret.root = ret.dir = path3;
              return ret;
            }
            rootEnd = 3;
          }
        } else {
          ret.root = ret.dir = path3;
          return ret;
        }
      }
    }
  } else if (isPathSeparator(code)) {
    ret.root = ret.dir = path3;
    return ret;
  }
  if (rootEnd > 0)
    ret.root = path3.slice(0, rootEnd);
  let startDot = -1;
  let startPart = rootEnd;
  let end = -1;
  let matchedSlash = true;
  let i = path3.length - 1;
  let preDotState = 0;
  for (; i >= rootEnd; --i) {
    code = path3.charCodeAt(i);
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
      if (startDot === -1)
        startDot = i;
      else if (preDotState !== 1)
        preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (startDot === -1 || end === -1 || preDotState === 0 || preDotState === 1 && startDot === end - 1 && startDot === startPart + 1) {
    if (end !== -1) {
      ret.base = ret.name = path3.slice(startPart, end);
    }
  } else {
    ret.name = path3.slice(startPart, startDot);
    ret.base = path3.slice(startPart, end);
    ret.ext = path3.slice(startDot, end);
  }
  if (startPart > 0 && startPart !== rootEnd) {
    ret.dir = path3.slice(0, startPart - 1);
  } else
    ret.dir = ret.root;
  return ret;
}
function fromFileUrl(url) {
  url = url instanceof URL ? url : new URL(url);
  if (url.protocol != "file:") {
    throw new TypeError("Must be a file URL.");
  }
  let path3 = decodeURIComponent(url.pathname.replace(/\//g, "\\").replace(/%(?![0-9A-Fa-f]{2})/g, "%25")).replace(/^\\*([A-Za-z]:)(\\|$)/, "$1\\");
  if (url.hostname != "") {
    path3 = `\\\\${url.hostname}${path3}`;
  }
  return path3;
}
function toFileUrl(path3) {
  if (!isAbsolute(path3)) {
    throw new TypeError("Must be an absolute path.");
  }
  const [, hostname, pathname] = path3.match(/^(?:[/\\]{2}([^/\\]+)(?=[/\\](?:[^/\\]|$)))?(.*)/);
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
  toNamespacedPath: () => toNamespacedPath2
});
var sep2 = "/";
var delimiter2 = ":";
function resolve2(...pathSegments) {
  let resolvedPath = "";
  let resolvedAbsolute = false;
  for (let i = pathSegments.length - 1; i >= -1 && !resolvedAbsolute; i--) {
    let path3;
    if (i >= 0)
      path3 = pathSegments[i];
    else {
      const { Deno: Deno3 } = globalThis;
      if (typeof Deno3?.cwd !== "function") {
        throw new TypeError("Resolved a relative path without a CWD.");
      }
      path3 = Deno3.cwd();
    }
    assertPath(path3);
    if (path3.length === 0) {
      continue;
    }
    resolvedPath = `${path3}/${resolvedPath}`;
    resolvedAbsolute = path3.charCodeAt(0) === CHAR_FORWARD_SLASH;
  }
  resolvedPath = normalizeString(resolvedPath, !resolvedAbsolute, "/", isPosixPathSeparator);
  if (resolvedAbsolute) {
    if (resolvedPath.length > 0)
      return `/${resolvedPath}`;
    else
      return "/";
  } else if (resolvedPath.length > 0)
    return resolvedPath;
  else
    return ".";
}
function normalize2(path3) {
  assertPath(path3);
  if (path3.length === 0)
    return ".";
  const isAbsolute4 = path3.charCodeAt(0) === CHAR_FORWARD_SLASH;
  const trailingSeparator = path3.charCodeAt(path3.length - 1) === CHAR_FORWARD_SLASH;
  path3 = normalizeString(path3, !isAbsolute4, "/", isPosixPathSeparator);
  if (path3.length === 0 && !isAbsolute4)
    path3 = ".";
  if (path3.length > 0 && trailingSeparator)
    path3 += "/";
  if (isAbsolute4)
    return `/${path3}`;
  return path3;
}
function isAbsolute2(path3) {
  assertPath(path3);
  return path3.length > 0 && path3.charCodeAt(0) === CHAR_FORWARD_SLASH;
}
function join2(...paths) {
  if (paths.length === 0)
    return ".";
  let joined;
  for (let i = 0, len = paths.length; i < len; ++i) {
    const path3 = paths[i];
    assertPath(path3);
    if (path3.length > 0) {
      if (!joined)
        joined = path3;
      else
        joined += `/${path3}`;
    }
  }
  if (!joined)
    return ".";
  return normalize2(joined);
}
function relative2(from, to) {
  assertPath(from);
  assertPath(to);
  if (from === to)
    return "";
  from = resolve2(from);
  to = resolve2(to);
  if (from === to)
    return "";
  let fromStart = 1;
  const fromEnd = from.length;
  for (; fromStart < fromEnd; ++fromStart) {
    if (from.charCodeAt(fromStart) !== CHAR_FORWARD_SLASH)
      break;
  }
  const fromLen = fromEnd - fromStart;
  let toStart = 1;
  const toEnd = to.length;
  for (; toStart < toEnd; ++toStart) {
    if (to.charCodeAt(toStart) !== CHAR_FORWARD_SLASH)
      break;
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
    if (fromCode !== toCode)
      break;
    else if (fromCode === CHAR_FORWARD_SLASH)
      lastCommonSep = i;
  }
  let out = "";
  for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
    if (i === fromEnd || from.charCodeAt(i) === CHAR_FORWARD_SLASH) {
      if (out.length === 0)
        out += "..";
      else
        out += "/..";
    }
  }
  if (out.length > 0)
    return out + to.slice(toStart + lastCommonSep);
  else {
    toStart += lastCommonSep;
    if (to.charCodeAt(toStart) === CHAR_FORWARD_SLASH)
      ++toStart;
    return to.slice(toStart);
  }
}
function toNamespacedPath2(path3) {
  return path3;
}
function dirname2(path3) {
  assertPath(path3);
  if (path3.length === 0)
    return ".";
  const hasRoot = path3.charCodeAt(0) === CHAR_FORWARD_SLASH;
  let end = -1;
  let matchedSlash = true;
  for (let i = path3.length - 1; i >= 1; --i) {
    if (path3.charCodeAt(i) === CHAR_FORWARD_SLASH) {
      if (!matchedSlash) {
        end = i;
        break;
      }
    } else {
      matchedSlash = false;
    }
  }
  if (end === -1)
    return hasRoot ? "/" : ".";
  if (hasRoot && end === 1)
    return "//";
  return path3.slice(0, end);
}
function basename2(path3, ext = "") {
  if (ext !== void 0 && typeof ext !== "string") {
    throw new TypeError('"ext" argument must be a string');
  }
  assertPath(path3);
  let start = 0;
  let end = -1;
  let matchedSlash = true;
  let i;
  if (ext !== void 0 && ext.length > 0 && ext.length <= path3.length) {
    if (ext.length === path3.length && ext === path3)
      return "";
    let extIdx = ext.length - 1;
    let firstNonSlashEnd = -1;
    for (i = path3.length - 1; i >= 0; --i) {
      const code = path3.charCodeAt(i);
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
    if (start === end)
      end = firstNonSlashEnd;
    else if (end === -1)
      end = path3.length;
    return path3.slice(start, end);
  } else {
    for (i = path3.length - 1; i >= 0; --i) {
      if (path3.charCodeAt(i) === CHAR_FORWARD_SLASH) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
    }
    if (end === -1)
      return "";
    return path3.slice(start, end);
  }
}
function extname2(path3) {
  assertPath(path3);
  let startDot = -1;
  let startPart = 0;
  let end = -1;
  let matchedSlash = true;
  let preDotState = 0;
  for (let i = path3.length - 1; i >= 0; --i) {
    const code = path3.charCodeAt(i);
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
      if (startDot === -1)
        startDot = i;
      else if (preDotState !== 1)
        preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (startDot === -1 || end === -1 || preDotState === 0 || preDotState === 1 && startDot === end - 1 && startDot === startPart + 1) {
    return "";
  }
  return path3.slice(startDot, end);
}
function format2(pathObject) {
  if (pathObject === null || typeof pathObject !== "object") {
    throw new TypeError(`The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`);
  }
  return _format("/", pathObject);
}
function parse2(path3) {
  assertPath(path3);
  const ret = { root: "", dir: "", base: "", ext: "", name: "" };
  if (path3.length === 0)
    return ret;
  const isAbsolute4 = path3.charCodeAt(0) === CHAR_FORWARD_SLASH;
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
  let i = path3.length - 1;
  let preDotState = 0;
  for (; i >= start; --i) {
    const code = path3.charCodeAt(i);
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
      if (startDot === -1)
        startDot = i;
      else if (preDotState !== 1)
        preDotState = 1;
    } else if (startDot !== -1) {
      preDotState = -1;
    }
  }
  if (startDot === -1 || end === -1 || preDotState === 0 || preDotState === 1 && startDot === end - 1 && startDot === startPart + 1) {
    if (end !== -1) {
      if (startPart === 0 && isAbsolute4) {
        ret.base = ret.name = path3.slice(1, end);
      } else {
        ret.base = ret.name = path3.slice(startPart, end);
      }
    }
  } else {
    if (startPart === 0 && isAbsolute4) {
      ret.name = path3.slice(1, startDot);
      ret.base = path3.slice(1, end);
    } else {
      ret.name = path3.slice(startPart, startDot);
      ret.base = path3.slice(startPart, end);
    }
    ret.ext = path3.slice(startDot, end);
  }
  if (startPart > 0)
    ret.dir = path3.slice(0, startPart - 1);
  else if (isAbsolute4)
    ret.dir = "/";
  return ret;
}
function fromFileUrl2(url) {
  url = url instanceof URL ? url : new URL(url);
  if (url.protocol != "file:") {
    throw new TypeError("Must be a file URL.");
  }
  return decodeURIComponent(url.pathname.replace(/%(?![0-9A-Fa-f]{2})/g, "%25"));
}
function toFileUrl2(path3) {
  if (!isAbsolute2(path3)) {
    throw new TypeError("Must be an absolute path.");
  }
  const url = new URL("file:///");
  url.pathname = encodeWhitespace(path3.replace(/%/g, "%25").replace(/\\/g, "%5C"));
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
  for (const path3 of remaining) {
    const compare = path3.split(sep4);
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
var regExpEscapeChars = ["!", "$", "(", ")", "*", "+", ".", "=", "?", "[", "\\", "^", "{", "|"];
var rangeEscapeChars = ["-", "\\", "]"];
function globToRegExp(glob, {
  extended = true,
  globstar: globstarOption = true,
  os: os2 = osType,
  caseInsensitive = false
} = {}) {
  if (glob == "") {
    return /(?!)/;
  }
  const sep4 = os2 == "windows" ? "(?:\\\\|/)+" : "/+";
  const sepMaybe = os2 == "windows" ? "(?:\\\\|/)*" : "/*";
  const seps = os2 == "windows" ? ["\\", "/"] : ["/"];
  const globstar = os2 == "windows" ? "(?:[^\\\\/]*(?:\\\\|/|$)+)*" : "(?:[^/]*(?:/|$)+)*";
  const wildcard = os2 == "windows" ? "[^\\\\/]*" : "[^/]*";
  const escapePrefix = os2 == "windows" ? "`" : "\\";
  let newLength = glob.length;
  for (; newLength > 1 && seps.includes(glob[newLength - 1]); newLength--)
    ;
  glob = glob.slice(0, newLength);
  let regExpString = "";
  for (let j = 0; j < glob.length; ) {
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
            if (value == "alnum")
              segment += "\\dA-Za-z";
            else if (value == "alpha")
              segment += "A-Za-z";
            else if (value == "ascii")
              segment += "\0-\x7F";
            else if (value == "blank")
              segment += "	 ";
            else if (value == "cntrl")
              segment += "\0-\x7F";
            else if (value == "digit")
              segment += "\\d";
            else if (value == "graph")
              segment += "!-~";
            else if (value == "lower")
              segment += "a-z";
            else if (value == "print")
              segment += " -~";
            else if (value == "punct") {
              segment += `!"#$%&'()*+,\\-./:;<=>?@[\\\\\\]^_\u2018{|}~`;
            } else if (value == "space")
              segment += "\\s\v";
            else if (value == "upper")
              segment += "A-Z";
            else if (value == "word")
              segment += "\\w";
            else if (value == "xdigit")
              segment += "\\dA-Fa-f";
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
      if (glob[i] == ")" && groupStack.length > 0 && groupStack[groupStack.length - 1] != "BRACE") {
        segment += ")";
        const type = groupStack.pop();
        if (type == "!") {
          segment += wildcard;
        } else if (type != "@") {
          segment += type;
        }
        continue;
      }
      if (glob[i] == "|" && groupStack.length > 0 && groupStack[groupStack.length - 1] != "BRACE") {
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
          if (globstarOption && numStars == 2 && [...seps, void 0].includes(prevChar) && [...seps, void 0].includes(nextChar)) {
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
    while (seps.includes(glob[i]))
      i++;
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
  const regex = /\\(.)|(^!|\*|\?|[\].+)]\?|\[[^\\\]]+\]|\{[^\\}]+\}|\(\?[:!=][^\\)]+\)|\([^|]+\|[^\\)]+\))/;
  if (str === "") {
    return false;
  }
  let match;
  while (match = regex.exec(str)) {
    if (match[2])
      return true;
    let idx = match.index + match[0].length;
    const open3 = match[1];
    const close2 = open3 ? chars[open3] : null;
    if (open3 && close2) {
      const n = str.indexOf(close2, idx);
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
  const badParentPattern = new RegExp(`(?<=(${s}|^)\\*\\*${s})\\.\\.(?=${s}|$)`, "g");
  return normalize3(glob.replace(badParentPattern, "\0")).replace(/\0/g, "..");
}
function joinGlobs(globs, { extended = false, globstar = false } = {}) {
  if (!globstar || globs.length == 0) {
    return join3(...globs);
  }
  if (globs.length === 0)
    return ".";
  let joined;
  for (const glob of globs) {
    const path3 = glob;
    if (path3.length > 0) {
      if (!joined)
        joined = path3;
      else
        joined += `${SEP}${path3}`;
    }
  }
  if (!joined)
    return ".";
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
  toNamespacedPath: toNamespacedPath3
} = path2;

// path.ts
var path_default = { ...mod_exports };

// _fs/_fs_appendFile.ts
function appendFile(pathOrRid, data, optionsOrCallback, callback) {
  pathOrRid = pathOrRid instanceof URL ? fromFileUrl3(pathOrRid) : pathOrRid;
  const callbackFn = optionsOrCallback instanceof Function ? optionsOrCallback : callback;
  const options = optionsOrCallback instanceof Function ? void 0 : optionsOrCallback;
  if (!callbackFn) {
    throw new Error("No callback function supplied");
  }
  validateEncoding(options);
  let rid = -1;
  const buffer = data instanceof Uint8Array ? data : new TextEncoder().encode(data);
  new Promise((resolve4, reject) => {
    if (typeof pathOrRid === "number") {
      rid = pathOrRid;
      Deno.write(rid, buffer).then(resolve4, reject);
    } else {
      const mode = isFileOptions(options) ? options.mode : void 0;
      const flag = isFileOptions(options) ? options.flag : void 0;
      if (mode) {
        notImplemented("Deno does not yet support setting mode on create");
      }
      Deno.open(pathOrRid, getOpenOptions(flag)).then(({ rid: openedFileRid }) => {
        rid = openedFileRid;
        return Deno.write(openedFileRid, buffer);
      }).then(resolve4, reject);
    }
  }).then(() => {
    closeRidIfNecessary(typeof pathOrRid === "string", rid);
    callbackFn(null);
  }, (err) => {
    closeRidIfNecessary(typeof pathOrRid === "string", rid);
    callbackFn(err);
  });
}
function closeRidIfNecessary(isPathString, rid) {
  if (isPathString && rid != -1) {
    Deno.close(rid);
  }
}
function appendFileSync(pathOrRid, data, options) {
  let rid = -1;
  validateEncoding(options);
  pathOrRid = pathOrRid instanceof URL ? fromFileUrl3(pathOrRid) : pathOrRid;
  try {
    if (typeof pathOrRid === "number") {
      rid = pathOrRid;
    } else {
      const mode = isFileOptions(options) ? options.mode : void 0;
      const flag = isFileOptions(options) ? options.flag : void 0;
      if (mode) {
        notImplemented("Deno does not yet support setting mode on create");
      }
      const file = Deno.openSync(pathOrRid, getOpenOptions(flag));
      rid = file.rid;
    }
    const buffer = data instanceof Uint8Array ? data : new TextEncoder().encode(data);
    Deno.writeSync(rid, buffer);
  } finally {
    closeRidIfNecessary(typeof pathOrRid === "string", rid);
  }
}
function validateEncoding(encodingOption) {
  if (!encodingOption)
    return;
  if (typeof encodingOption === "string") {
    if (encodingOption !== "utf8") {
      throw new Error("Only 'utf8' encoding is currently supported");
    }
  } else if (encodingOption.encoding && encodingOption.encoding !== "utf8") {
    throw new Error("Only 'utf8' encoding is currently supported");
  }
}

// _fs/_fs_chmod.ts
var allowedModes = /^[0-7]{3}/;
function chmod(path3, mode, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.chmod(path3, getResolvedMode(mode)).then(() => callback(null), callback);
}
function chmodSync(path3, mode) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.chmodSync(path3, getResolvedMode(mode));
}
function getResolvedMode(mode) {
  if (typeof mode === "number") {
    return mode;
  }
  if (typeof mode === "string" && !allowedModes.test(mode)) {
    throw new Error("Unrecognized mode: " + mode);
  }
  return parseInt(mode, 8);
}

// _fs/_fs_chown.ts
function chown(path3, uid, gid, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.chown(path3, uid, gid).then(() => callback(null), callback);
}
function chownSync(path3, uid, gid) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.chownSync(path3, uid, gid);
}

// _fs/_fs_close.ts
function close(fd, callback) {
  setTimeout(() => {
    let error = null;
    try {
      Deno.close(fd);
    } catch (err) {
      error = err;
    }
    callback(error);
  }, 0);
}
function closeSync(fd) {
  Deno.close(fd);
}

// _fs/_fs_constants.ts
var fs_constants_exports = {};
__export(fs_constants_exports, {
  F_OK: () => F_OK,
  R_OK: () => R_OK,
  S_IRGRP: () => S_IRGRP,
  S_IROTH: () => S_IROTH,
  S_IRUSR: () => S_IRUSR,
  S_IWGRP: () => S_IWGRP,
  S_IWOTH: () => S_IWOTH,
  S_IWUSR: () => S_IWUSR,
  S_IXGRP: () => S_IXGRP,
  S_IXOTH: () => S_IXOTH,
  S_IXUSR: () => S_IXUSR,
  W_OK: () => W_OK,
  X_OK: () => X_OK
});
var F_OK = 0;
var R_OK = 4;
var W_OK = 2;
var X_OK = 1;
var S_IRUSR = 256;
var S_IWUSR = 128;
var S_IXUSR = 64;
var S_IRGRP = 32;
var S_IWGRP = 16;
var S_IXGRP = 8;
var S_IROTH = 4;
var S_IWOTH = 2;
var S_IXOTH = 1;

// _fs/_fs_copy.ts
function copyFile(source, destination, callback) {
  source = source instanceof URL ? fromFileUrl3(source) : source;
  Deno.copyFile(source, destination).then(() => callback(null), callback);
}
function copyFileSync(source, destination) {
  source = source instanceof URL ? fromFileUrl3(source) : source;
  Deno.copyFileSync(source, destination);
}

// _fs/_fs_dirent.ts
var Dirent = class {
  constructor(entry) {
    this.entry = entry;
  }
  isBlockDevice() {
    notImplemented("Deno does not yet support identification of block devices");
    return false;
  }
  isCharacterDevice() {
    notImplemented("Deno does not yet support identification of character devices");
    return false;
  }
  isDirectory() {
    return this.entry.isDirectory;
  }
  isFIFO() {
    notImplemented("Deno does not yet support identification of FIFO named pipes");
    return false;
  }
  isFile() {
    return this.entry.isFile;
  }
  isSocket() {
    notImplemented("Deno does not yet support identification of sockets");
    return false;
  }
  isSymbolicLink() {
    return this.entry.isSymlink;
  }
  get name() {
    return this.entry.name;
  }
};

// _fs/_fs_dir.ts
var Dir = class {
  constructor(path3) {
    this.dirPath = path3;
  }
  get path() {
    if (this.dirPath instanceof Uint8Array) {
      return new TextDecoder().decode(this.dirPath);
    }
    return this.dirPath;
  }
  read(callback) {
    return new Promise((resolve4, reject) => {
      if (!this.asyncIterator) {
        this.asyncIterator = Deno.readDir(this.path)[Symbol.asyncIterator]();
      }
      assert(this.asyncIterator);
      this.asyncIterator.next().then(({ value }) => {
        resolve4(value ? value : null);
        if (callback) {
          callback(null, value ? value : null);
        }
      }, (err) => {
        if (callback) {
          callback(err);
        }
        reject(err);
      });
    });
  }
  readSync() {
    if (!this.syncIterator) {
      this.syncIterator = Deno.readDirSync(this.path)[Symbol.iterator]();
    }
    const file = this.syncIterator.next().value;
    return file ? new Dirent(file) : null;
  }
  close(callback) {
    return new Promise((resolve4) => {
      if (callback) {
        callback(null);
      }
      resolve4();
    });
  }
  closeSync() {
  }
  async *[Symbol.asyncIterator]() {
    try {
      while (true) {
        const dirent = await this.read();
        if (dirent === null) {
          break;
        }
        yield dirent;
      }
    } finally {
      await this.close();
    }
  }
};

// _fs/_fs_exists.ts
function exists(path3, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.lstat(path3).then(() => callback(true), () => callback(false));
}
function existsSync(path3) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  try {
    Deno.lstatSync(path3);
    return true;
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return false;
    }
    throw err;
  }
}

// _fs/_fs_fdatasync.ts
function fdatasync(fd, callback) {
  Deno.fdatasync(fd).then(() => callback(null), callback);
}
function fdatasyncSync(fd) {
  Deno.fdatasyncSync(fd);
}

// _fs/_fs_stat.ts
function convertFileInfoToStats(origin) {
  return {
    dev: origin.dev,
    ino: origin.ino,
    mode: origin.mode,
    nlink: origin.nlink,
    uid: origin.uid,
    gid: origin.gid,
    rdev: origin.rdev,
    size: origin.size,
    blksize: origin.blksize,
    blocks: origin.blocks,
    mtime: origin.mtime,
    atime: origin.atime,
    birthtime: origin.birthtime,
    mtimeMs: origin.mtime?.getTime() || null,
    atimeMs: origin.atime?.getTime() || null,
    birthtimeMs: origin.birthtime?.getTime() || null,
    isFile: () => origin.isFile,
    isDirectory: () => origin.isDirectory,
    isSymbolicLink: () => origin.isSymlink,
    isBlockDevice: () => false,
    isFIFO: () => false,
    isCharacterDevice: () => false,
    isSocket: () => false,
    ctime: origin.mtime,
    ctimeMs: origin.mtime?.getTime() || null
  };
}
function toBigInt(number) {
  if (number === null || number === void 0)
    return null;
  return BigInt(number);
}
function convertFileInfoToBigIntStats(origin) {
  return {
    dev: toBigInt(origin.dev),
    ino: toBigInt(origin.ino),
    mode: toBigInt(origin.mode),
    nlink: toBigInt(origin.nlink),
    uid: toBigInt(origin.uid),
    gid: toBigInt(origin.gid),
    rdev: toBigInt(origin.rdev),
    size: toBigInt(origin.size) || 0n,
    blksize: toBigInt(origin.blksize),
    blocks: toBigInt(origin.blocks),
    mtime: origin.mtime,
    atime: origin.atime,
    birthtime: origin.birthtime,
    mtimeMs: origin.mtime ? BigInt(origin.mtime.getTime()) : null,
    atimeMs: origin.atime ? BigInt(origin.atime.getTime()) : null,
    birthtimeMs: origin.birthtime ? BigInt(origin.birthtime.getTime()) : null,
    mtimeNs: origin.mtime ? BigInt(origin.mtime.getTime()) * 1000000n : null,
    atimeNs: origin.atime ? BigInt(origin.atime.getTime()) * 1000000n : null,
    birthtimeNs: origin.birthtime ? BigInt(origin.birthtime.getTime()) * 1000000n : null,
    isFile: () => origin.isFile,
    isDirectory: () => origin.isDirectory,
    isSymbolicLink: () => origin.isSymlink,
    isBlockDevice: () => false,
    isFIFO: () => false,
    isCharacterDevice: () => false,
    isSocket: () => false,
    ctime: origin.mtime,
    ctimeMs: origin.mtime ? BigInt(origin.mtime.getTime()) : null,
    ctimeNs: origin.mtime ? BigInt(origin.mtime.getTime()) * 1000000n : null
  };
}
function CFISBIS(fileInfo, bigInt) {
  if (bigInt)
    return convertFileInfoToBigIntStats(fileInfo);
  return convertFileInfoToStats(fileInfo);
}
function stat(path3, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback === "function" ? optionsOrCallback : maybeCallback;
  const options = typeof optionsOrCallback === "object" ? optionsOrCallback : { bigint: false };
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.stat(path3).then((stat3) => callback(null, CFISBIS(stat3, options.bigint)), (err) => callback(err));
}
function statSync(path3, options = { bigint: false }) {
  const origin = Deno.statSync(path3);
  return CFISBIS(origin, options.bigint);
}

// _fs/_fs_fstat.ts
function fstat(fd, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback === "function" ? optionsOrCallback : maybeCallback;
  const options = typeof optionsOrCallback === "object" ? optionsOrCallback : { bigint: false };
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.fstat(fd).then((stat3) => callback(null, CFISBIS(stat3, options.bigint)), (err) => callback(err));
}
function fstatSync(fd, options) {
  const origin = Deno.fstatSync(fd);
  return CFISBIS(origin, options?.bigint || false);
}

// _fs/_fs_fsync.ts
function fsync(fd, callback) {
  Deno.fsync(fd).then(() => callback(null), callback);
}
function fsyncSync(fd) {
  Deno.fsyncSync(fd);
}

// _fs/_fs_ftruncate.ts
function ftruncate(fd, lenOrCallback, maybeCallback) {
  const len = typeof lenOrCallback === "number" ? lenOrCallback : void 0;
  const callback = typeof lenOrCallback === "function" ? lenOrCallback : maybeCallback;
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.ftruncate(fd, len).then(() => callback(null), callback);
}
function ftruncateSync(fd, len) {
  Deno.ftruncateSync(fd, len);
}

// _fs/_fs_futimes.ts
function getValidTime(time, name) {
  if (typeof time === "string") {
    time = Number(time);
  }
  if (typeof time === "number" && (Number.isNaN(time) || !Number.isFinite(time))) {
    throw new Deno.errors.InvalidData(`invalid ${name}, must not be infitiny or NaN`);
  }
  return time;
}
function futimes(fd, atime, mtime, callback) {
  if (!callback) {
    throw new Deno.errors.InvalidData("No callback function supplied");
  }
  atime = getValidTime(atime, "atime");
  mtime = getValidTime(mtime, "mtime");
  Deno.futime(fd, atime, mtime).then(() => callback(null), callback);
}
function futimesSync(fd, atime, mtime) {
  atime = getValidTime(atime, "atime");
  mtime = getValidTime(mtime, "mtime");
  Deno.futimeSync(fd, atime, mtime);
}

// _fs/_fs_link.ts
function link(existingPath, newPath, callback) {
  existingPath = existingPath instanceof URL ? fromFileUrl3(existingPath) : existingPath;
  newPath = newPath instanceof URL ? fromFileUrl3(newPath) : newPath;
  Deno.link(existingPath, newPath).then(() => callback(null), callback);
}
function linkSync(existingPath, newPath) {
  existingPath = existingPath instanceof URL ? fromFileUrl3(existingPath) : existingPath;
  newPath = newPath instanceof URL ? fromFileUrl3(newPath) : newPath;
  Deno.linkSync(existingPath, newPath);
}

// _fs/_fs_lstat.ts
function lstat(path3, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback === "function" ? optionsOrCallback : maybeCallback;
  const options = typeof optionsOrCallback === "object" ? optionsOrCallback : { bigint: false };
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.lstat(path3).then((stat3) => callback(null, CFISBIS(stat3, options.bigint)), (err) => callback(err));
}
function lstatSync(path3, options) {
  const origin = Deno.lstatSync(path3);
  return CFISBIS(origin, options?.bigint || false);
}

// _fs/_fs_mkdir.ts
function mkdir(path3, options, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  let mode = 511;
  let recursive = false;
  if (typeof options == "function") {
    callback = options;
  } else if (typeof options === "number") {
    mode = options;
  } else if (typeof options === "boolean") {
    recursive = options;
  } else if (options) {
    if (options.recursive !== void 0)
      recursive = options.recursive;
    if (options.mode !== void 0)
      mode = options.mode;
  }
  if (typeof recursive !== "boolean") {
    throw new Deno.errors.InvalidData("invalid recursive option , must be a boolean");
  }
  Deno.mkdir(path3, { recursive, mode }).then(() => {
    if (typeof callback === "function") {
      callback(null);
    }
  }, (err) => {
    if (typeof callback === "function") {
      callback(err);
    }
  });
}
function mkdirSync(path3, options) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  let mode = 511;
  let recursive = false;
  if (typeof options === "number") {
    mode = options;
  } else if (typeof options === "boolean") {
    recursive = options;
  } else if (options) {
    if (options.recursive !== void 0)
      recursive = options.recursive;
    if (options.mode !== void 0)
      mode = options.mode;
  }
  if (typeof recursive !== "boolean") {
    throw new Deno.errors.InvalidData("invalid recursive option , must be a boolean");
  }
  Deno.mkdirSync(path3, { recursive, mode });
}

// _util/_util_promisify.ts
var kCustomPromisifiedSymbol = Symbol.for("nodejs.util.promisify.custom");
var kCustomPromisifyArgsSymbol = Symbol.for("nodejs.util.promisify.customArgs");
var NodeInvalidArgTypeError = class extends TypeError {
  constructor(argumentName, type, received) {
    super(`The "${argumentName}" argument must be of type ${type}. Received ${typeof received}`);
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
      throw new NodeInvalidArgTypeError("util.promisify.custom", "Function", fn2);
    }
    return Object.defineProperty(fn2, kCustomPromisifiedSymbol, {
      value: fn2,
      enumerable: false,
      writable: false,
      configurable: true
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
    configurable: true
  });
  return Object.defineProperties(fn, Object.getOwnPropertyDescriptors(original));
}
promisify.custom = kCustomPromisifiedSymbol;

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
  getters: false
};
inspect.defaultOptions = DEFAULT_INSPECT_OPTIONS;
inspect.custom = Symbol.for("Deno.customInspect");
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
    showProxy: !!opts.showProxy
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
  "symbol"
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
    super(`The value of "${str}" is out of range. It must be ${range}. Received ${received}`);
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
  [-4027, ["EILSEQ", "illegal byte sequence"]]
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
  [-92, ["EILSEQ", "illegal byte sequence"]]
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
  [-84, ["EILSEQ", "illegal byte sequence"]]
];
var { os } = Deno.build;
var errorMap = new Map(os === "windows" ? windows : os === "darwin" ? darwin : os === "linux" ? linux : unreachable());
var ERR_INVALID_CALLBACK = class extends NodeTypeError {
  constructor(object) {
    super("ERR_INVALID_CALLBACK", `Callback must be a function. Received ${JSON.stringify(object)}`);
  }
};
var ERR_INVALID_OPT_VALUE_ENCODING = class extends NodeTypeError {
  constructor(x) {
    super("ERR_INVALID_OPT_VALUE_ENCODING", `The value "${x}" is invalid for option "encoding"`);
  }
};

// _fs/_fs_mkdtemp.ts
function mkdtemp(prefix, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback == "function" ? optionsOrCallback : maybeCallback;
  if (!callback)
    throw new ERR_INVALID_CALLBACK(callback);
  const encoding = parseEncoding(optionsOrCallback);
  const path3 = tempDirPath(prefix);
  mkdir(path3, { recursive: false, mode: 448 }, (err) => {
    if (err)
      callback(err);
    else
      callback(null, decode(path3, encoding));
  });
}
function mkdtempSync(prefix, options) {
  const encoding = parseEncoding(options);
  const path3 = tempDirPath(prefix);
  mkdirSync(path3, { recursive: false, mode: 448 });
  return decode(path3, encoding);
}
function parseEncoding(optionsOrCallback) {
  let encoding;
  if (typeof optionsOrCallback == "function")
    encoding = void 0;
  else if (optionsOrCallback instanceof Object) {
    encoding = optionsOrCallback?.encoding;
  } else
    encoding = optionsOrCallback;
  if (encoding) {
    try {
      new TextDecoder(encoding);
    } catch {
      throw new ERR_INVALID_OPT_VALUE_ENCODING(encoding);
    }
  }
  return encoding;
}
function decode(str, encoding) {
  if (!encoding)
    return str;
  else {
    const decoder = new TextDecoder(encoding);
    const encoder = new TextEncoder();
    return decoder.decode(encoder.encode(str));
  }
}
var CHARS = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";
function randomName() {
  return [...Array(6)].map(() => CHARS[Math.floor(Math.random() * CHARS.length)]).join("");
}
function tempDirPath(prefix) {
  let path3;
  do {
    path3 = prefix + randomName();
  } while (existsSync(path3));
  return path3;
}

// ../fs/exists.ts
function existsSync2(filePath) {
  try {
    Deno.lstatSync(filePath);
    return true;
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return false;
    }
    throw err;
  }
}

// _fs/_fs_open.ts
function convertFlagAndModeToOptions(flag, mode) {
  if (!flag && !mode)
    return void 0;
  if (!flag && mode)
    return { mode };
  return { ...getOpenOptions(flag), mode };
}
function open(path3, flagsOrCallback, callbackOrMode, maybeCallback) {
  const flags = typeof flagsOrCallback === "string" ? flagsOrCallback : void 0;
  const callback = typeof flagsOrCallback === "function" ? flagsOrCallback : typeof callbackOrMode === "function" ? callbackOrMode : maybeCallback;
  const mode = typeof callbackOrMode === "number" ? callbackOrMode : void 0;
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  if (!callback)
    throw new Error("No callback function supplied");
  if (["ax", "ax+", "wx", "wx+"].includes(flags || "") && existsSync2(path3)) {
    const err = new Error(`EEXIST: file already exists, open '${path3}'`);
    callback(err);
  } else {
    if (flags === "as" || flags === "as+") {
      let err = null, res;
      try {
        res = openSync(path3, flags, mode);
      } catch (error) {
        err = error;
      }
      if (err) {
        callback(err);
      } else {
        callback(null, res);
      }
      return;
    }
    Deno.open(path3, convertFlagAndModeToOptions(flags, mode)).then((file) => callback(null, file.rid), (err) => callback(err));
  }
}
function openSync(path3, flagsOrMode, maybeMode) {
  const flags = typeof flagsOrMode === "string" ? flagsOrMode : void 0;
  const mode = typeof flagsOrMode === "number" ? flagsOrMode : maybeMode;
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  if (["ax", "ax+", "wx", "wx+"].includes(flags || "") && existsSync2(path3)) {
    throw new Error(`EEXIST: file already exists, open '${path3}'`);
  }
  return Deno.openSync(path3, convertFlagAndModeToOptions(flags, mode)).rid;
}

// events.ts
function createIterResult(value, done) {
  return { value, done };
}
var defaultMaxListeners = 10;
function validateMaxListeners(n, name) {
  if (!Number.isInteger(n) || n < 0) {
    throw new ERR_OUT_OF_RANGE(name, "a non-negative number", inspect(n));
  }
}
var _EventEmitter = class {
  static get defaultMaxListeners() {
    return defaultMaxListeners;
  }
  static set defaultMaxListeners(value) {
    validateMaxListeners(value, "defaultMaxListeners");
    defaultMaxListeners = value;
  }
  constructor() {
    this._events = new Map();
  }
  _addListener(eventName, listener, prepend) {
    this.checkListenerArgument(listener);
    this.emit("newListener", eventName, this.unwrapListener(listener));
    if (this._events.has(eventName)) {
      const listeners = this._events.get(eventName);
      if (prepend) {
        listeners.unshift(listener);
      } else {
        listeners.push(listener);
      }
    } else {
      this._events.set(eventName, [listener]);
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
    if (this._events.has(eventName)) {
      if (eventName === "error" && this._events.get(_EventEmitter.errorMonitor)) {
        this.emit(_EventEmitter.errorMonitor, ...args);
      }
      const listeners = this._events.get(eventName).slice();
      for (const listener of listeners) {
        try {
          listener.apply(this, args);
        } catch (err) {
          this.emit("error", err);
        }
      }
      return true;
    } else if (eventName === "error") {
      if (this._events.get(_EventEmitter.errorMonitor)) {
        this.emit(_EventEmitter.errorMonitor, ...args);
      }
      const errMsg = args.length > 0 ? args[0] : Error("Unhandled error.");
      throw errMsg;
    }
    return false;
  }
  eventNames() {
    return Array.from(this._events.keys());
  }
  getMaxListeners() {
    return this.maxListeners == null ? _EventEmitter.defaultMaxListeners : this.maxListeners;
  }
  listenerCount(eventName) {
    if (this._events.has(eventName)) {
      return this._events.get(eventName).length;
    } else {
      return 0;
    }
  }
  static listenerCount(emitter, eventName) {
    return emitter.listenerCount(eventName);
  }
  _listeners(target, eventName, unwrap) {
    if (!target._events?.has(eventName)) {
      return [];
    }
    const eventListeners = target._events.get(eventName);
    return unwrap ? this.unwrapListeners(eventListeners) : eventListeners.slice(0);
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
  off(eventName, listener) {
  }
  on(eventName, listener) {
  }
  once(eventName, listener) {
    const wrapped = this.onceWrap(eventName, listener);
    this.on(eventName, wrapped);
    return this;
  }
  onceWrap(eventName, listener) {
    this.checkListenerArgument(listener);
    const wrapper = function(...args) {
      if (this.isCalled) {
        return;
      }
      this.context.removeListener(this.eventName, this.rawListener);
      this.isCalled = true;
      return this.listener.apply(this.context, args);
    };
    const wrapperContext = {
      eventName,
      listener,
      rawListener: wrapper,
      context: this
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
      if (this._events.has(eventName)) {
        const listeners = this._events.get(eventName).slice().reverse();
        for (const listener of listeners) {
          this.removeListener(eventName, this.unwrapListener(listener));
        }
      }
    } else {
      const eventList = this.eventNames();
      eventList.forEach((value) => {
        this.removeAllListeners(value);
      });
    }
    return this;
  }
  removeListener(eventName, listener) {
    this.checkListenerArgument(listener);
    if (this._events.has(eventName)) {
      const arr = this._events.get(eventName);
      assert(arr);
      let listenerIndex = -1;
      for (let i = arr.length - 1; i >= 0; i--) {
        if (arr[i] == listener || arr[i] && arr[i]["listener"] == listener) {
          listenerIndex = i;
          break;
        }
      }
      if (listenerIndex >= 0) {
        arr.splice(listenerIndex, 1);
        this.emit("removeListener", eventName, listener);
        if (arr.length === 0) {
          this._events.delete(eventName);
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
    return new Promise((resolve4, reject) => {
      if (emitter instanceof EventTarget) {
        emitter.addEventListener(name, (...args) => {
          resolve4(args);
        }, { once: true, passive: false, capture: false });
        return;
      } else if (emitter instanceof _EventEmitter) {
        const eventListener = (...args) => {
          if (errorListener !== void 0) {
            emitter.removeListener("error", errorListener);
          }
          resolve4(args);
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
        return new Promise(function(resolve4, reject) {
          unconsumedPromises.push({ resolve: resolve4, reject });
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
      }
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
    _EventEmitter._alreadyWarnedEvents ||= new Set();
    if (_EventEmitter._alreadyWarnedEvents.has(eventName)) {
      return;
    }
    _EventEmitter._alreadyWarnedEvents.add(eventName);
    console.warn(warning);
    const maybeProcess = globalThis.process;
    if (maybeProcess instanceof _EventEmitter) {
      maybeProcess.emit("warning", warning);
    }
  }
};
var EventEmitter = _EventEmitter;
EventEmitter.captureRejectionSymbol = Symbol.for("nodejs.rejection");
EventEmitter.errorMonitor = Symbol("events.errorMonitor");
EventEmitter.prototype.on = EventEmitter.prototype.addListener;
EventEmitter.prototype.off = EventEmitter.prototype.removeListener;
var MaxListenersExceededWarning = class extends Error {
  constructor(emitter, type) {
    const listenerCount2 = emitter.listenerCount(type);
    const message = `Possible EventEmitter memory leak detected. ${listenerCount2} ${type == null ? "null" : type.toString()} listeners added to [${emitter.constructor.name}].  Use emitter.setMaxListeners() to increase limit`;
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

// _fs/_fs_watch.ts
function asyncIterableToCallback(iter, callback) {
  const iterator = iter[Symbol.asyncIterator]();
  function next() {
    iterator.next().then((obj) => {
      if (obj.done) {
        callback(obj.value, true);
        return;
      }
      callback(obj.value);
      next();
    });
  }
  next();
}
function watch(filename, optionsOrListener, optionsOrListener2) {
  const listener = typeof optionsOrListener === "function" ? optionsOrListener : typeof optionsOrListener2 === "function" ? optionsOrListener2 : void 0;
  const options = typeof optionsOrListener === "object" ? optionsOrListener : typeof optionsOrListener2 === "object" ? optionsOrListener2 : void 0;
  filename = filename instanceof URL ? fromFileUrl3(filename) : filename;
  const iterator = Deno.watchFs(filename, {
    recursive: options?.recursive || false
  });
  if (!listener)
    throw new Error("No callback function supplied");
  const fsWatcher = new FSWatcher(() => {
    if (iterator.return)
      iterator.return();
  });
  fsWatcher.on("change", listener);
  asyncIterableToCallback(iterator, (val, done) => {
    if (done)
      return;
    fsWatcher.emit("change", val.kind, val.paths[0]);
  });
  return fsWatcher;
}
var FSWatcher = class extends EventEmitter {
  constructor(closer) {
    super();
    this.close = closer;
  }
  ref() {
    notImplemented("FSWatcher.ref() is not implemented");
  }
  unref() {
    notImplemented("FSWatcher.unref() is not implemented");
  }
};

// _fs/_fs_readdir.ts
function toDirent(val) {
  return new Dirent(val);
}
function readdir(path3, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback === "function" ? optionsOrCallback : maybeCallback;
  const options = typeof optionsOrCallback === "object" ? optionsOrCallback : null;
  const result = [];
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  if (!callback)
    throw new Error("No callback function supplied");
  if (options?.encoding) {
    try {
      new TextDecoder(options.encoding);
    } catch {
      throw new Error(`TypeError [ERR_INVALID_OPT_VALUE_ENCODING]: The value "${options.encoding}" is invalid for option "encoding"`);
    }
  }
  try {
    asyncIterableToCallback(Deno.readDir(path3), (val, done) => {
      if (typeof path3 !== "string")
        return;
      if (done) {
        callback(null, result);
        return;
      }
      if (options?.withFileTypes) {
        result.push(toDirent(val));
      } else
        result.push(decode2(val.name));
    });
  } catch (error) {
    callback(error);
  }
}
function decode2(str, encoding) {
  if (!encoding)
    return str;
  else {
    const decoder = new TextDecoder(encoding);
    const encoder = new TextEncoder();
    return decoder.decode(encoder.encode(str));
  }
}
function readdirSync(path3, options) {
  const result = [];
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  if (options?.encoding) {
    try {
      new TextDecoder(options.encoding);
    } catch {
      throw new Error(`TypeError [ERR_INVALID_OPT_VALUE_ENCODING]: The value "${options.encoding}" is invalid for option "encoding"`);
    }
  }
  for (const file of Deno.readDirSync(path3)) {
    if (options?.withFileTypes) {
      result.push(toDirent(file));
    } else
      result.push(decode2(file.name));
  }
  return result;
}

// ../encoding/hex.ts
var hexTable = new TextEncoder().encode("0123456789abcdef");
function errInvalidByte(byte) {
  return new TypeError(`Invalid byte '${String.fromCharCode(byte)}'`);
}
function errLength() {
  return new RangeError("Odd length hex string");
}
function fromHexChar(byte) {
  if (48 <= byte && byte <= 57)
    return byte - 48;
  if (97 <= byte && byte <= 102)
    return byte - 97 + 10;
  if (65 <= byte && byte <= 70)
    return byte - 65 + 10;
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
function decode3(src) {
  const dst = new Uint8Array(src.length / 2);
  for (let i = 0; i < dst.length; i++) {
    const a = fromHexChar(src[i * 2]);
    const b = fromHexChar(src[i * 2 + 1]);
    dst[i] = a << 4 | b;
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
  "/"
];
function encode2(data) {
  const uint8 = typeof data === "string" ? new TextEncoder().encode(data) : data instanceof Uint8Array ? data : new Uint8Array(data);
  let result = "", i;
  const l = uint8.length;
  for (i = 2; i < l; i += 3) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[(uint8[i - 2] & 3) << 4 | uint8[i - 1] >> 4];
    result += base64abc[(uint8[i - 1] & 15) << 2 | uint8[i] >> 6];
    result += base64abc[uint8[i] & 63];
  }
  if (i === l + 1) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[(uint8[i - 2] & 3) << 4];
    result += "==";
  }
  if (i === l) {
    result += base64abc[uint8[i - 2] >> 2];
    result += base64abc[(uint8[i - 2] & 3) << 4 | uint8[i - 1] >> 4];
    result += base64abc[(uint8[i - 1] & 15) << 2];
    result += "=";
  }
  return result;
}
function decode4(b64) {
  const binString = atob(b64);
  const size = binString.length;
  const bytes = new Uint8Array(size);
  for (let i = 0; i < size; i++) {
    bytes[i] = binString.charCodeAt(i);
  }
  return bytes;
}

// buffer.ts
var notImplementedEncodings = [
  "ascii",
  "binary",
  "latin1",
  "ucs2",
  "utf16le"
];
function checkEncoding2(encoding = "utf8", strict = true) {
  if (typeof encoding !== "string" || strict && encoding === "") {
    if (!strict)
      return "utf8";
    throw new TypeError(`Unkown encoding: ${encoding}`);
  }
  const normalized = normalizeEncoding(encoding);
  if (normalized === void 0) {
    throw new TypeError(`Unkown encoding: ${encoding}`);
  }
  if (notImplementedEncodings.includes(encoding)) {
    notImplemented(`"${encoding}" encoding`);
  }
  return normalized;
}
var encodingOps = {
  utf8: {
    byteLength: (string) => new TextEncoder().encode(string).byteLength
  },
  ucs2: {
    byteLength: (string) => string.length * 2
  },
  utf16le: {
    byteLength: (string) => string.length * 2
  },
  latin1: {
    byteLength: (string) => string.length
  },
  ascii: {
    byteLength: (string) => string.length
  },
  base64: {
    byteLength: (string) => base64ByteLength(string, string.length)
  },
  hex: {
    byteLength: (string) => string.length >>> 1
  }
};
function base64ByteLength(str, bytes) {
  if (str.charCodeAt(bytes - 1) === 61)
    bytes--;
  if (bytes > 1 && str.charCodeAt(bytes - 1) === 61)
    bytes--;
  return bytes * 3 >>> 2;
}
var Buffer3 = class extends Uint8Array {
  static alloc(size, fill, encoding = "utf8") {
    if (typeof size !== "number") {
      throw new TypeError(`The "size" argument must be of type number. Received type ${typeof size}`);
    }
    const buf = new Buffer3(size);
    if (size === 0)
      return buf;
    let bufFill;
    if (typeof fill === "string") {
      const clearEncoding = checkEncoding2(encoding);
      if (typeof fill === "string" && fill.length === 1 && clearEncoding === "utf8") {
        buf.fill(fill.charCodeAt(0));
      } else
        bufFill = Buffer3.from(fill, clearEncoding);
    } else if (typeof fill === "number") {
      buf.fill(fill);
    } else if (fill instanceof Uint8Array) {
      if (fill.length === 0) {
        throw new TypeError(`The argument "value" is invalid. Received ${fill.constructor.name} []`);
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
        if (offset + bufFill.length >= size)
          break;
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
    if (typeof string != "string")
      return string.byteLength;
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
    const offset = typeof offsetOrEncoding === "string" ? void 0 : offsetOrEncoding;
    let encoding = typeof offsetOrEncoding === "string" ? offsetOrEncoding : void 0;
    if (typeof value == "string") {
      encoding = checkEncoding2(encoding, false);
      if (encoding === "hex") {
        return new Buffer3(decode3(new TextEncoder().encode(value)).buffer);
      }
      if (encoding === "base64")
        return new Buffer3(decode4(value).buffer);
      return new Buffer3(new TextEncoder().encode(value).buffer);
    }
    return new Buffer3(value, offset, length);
  }
  static isBuffer(obj) {
    return obj instanceof Buffer3;
  }
  static isEncoding(encoding) {
    return typeof encoding === "string" && encoding.length !== 0 && normalizeEncoding(encoding) !== void 0;
  }
  copy(targetBuffer, targetStart = 0, sourceStart = 0, sourceEnd = this.length) {
    const sourceBuffer = this.subarray(sourceStart, sourceEnd).subarray(0, Math.max(0, targetBuffer.length - targetStart));
    if (sourceBuffer.length === 0)
      return 0;
    targetBuffer.set(sourceBuffer, targetStart);
    return sourceBuffer.length;
  }
  equals(otherBuffer) {
    if (!(otherBuffer instanceof Uint8Array)) {
      throw new TypeError(`The "otherBuffer" argument must be an instance of Buffer or Uint8Array. Received type ${typeof otherBuffer}`);
    }
    if (this === otherBuffer)
      return true;
    if (this.byteLength !== otherBuffer.byteLength)
      return false;
    for (let i = 0; i < this.length; i++) {
      if (this[i] !== otherBuffer[i])
        return false;
    }
    return true;
  }
  readBigInt64BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getBigInt64(offset);
  }
  readBigInt64LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getBigInt64(offset, true);
  }
  readBigUInt64BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getBigUint64(offset);
  }
  readBigUInt64LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getBigUint64(offset, true);
  }
  readDoubleBE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getFloat64(offset);
  }
  readDoubleLE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getFloat64(offset, true);
  }
  readFloatBE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getFloat32(offset);
  }
  readFloatLE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getFloat32(offset, true);
  }
  readInt8(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt8(offset);
  }
  readInt16BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt16(offset);
  }
  readInt16LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt16(offset, true);
  }
  readInt32BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt32(offset);
  }
  readInt32LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getInt32(offset, true);
  }
  readUInt8(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint8(offset);
  }
  readUInt16BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint16(offset);
  }
  readUInt16LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint16(offset, true);
  }
  readUInt32BE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint32(offset);
  }
  readUInt32LE(offset = 0) {
    return new DataView(this.buffer, this.byteOffset, this.byteLength).getUint32(offset, true);
  }
  slice(begin = 0, end = this.length) {
    return this.subarray(begin, end);
  }
  toJSON() {
    return { type: "Buffer", data: Array.from(this) };
  }
  toString(encoding = "utf8", start = 0, end = this.length) {
    encoding = checkEncoding2(encoding);
    const b = this.subarray(start, end);
    if (encoding === "hex")
      return new TextDecoder().decode(encode(b));
    if (encoding === "base64")
      return encode2(b.buffer);
    return new TextDecoder(encoding).decode(b);
  }
  write(string, offset = 0, length = this.length) {
    return new TextEncoder().encodeInto(string, this.subarray(offset, offset + length)).written;
  }
  writeBigInt64BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigInt64(offset, value);
    return offset + 4;
  }
  writeBigInt64LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigInt64(offset, value, true);
    return offset + 4;
  }
  writeBigUInt64BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigUint64(offset, value);
    return offset + 4;
  }
  writeBigUInt64LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setBigUint64(offset, value, true);
    return offset + 4;
  }
  writeDoubleBE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat64(offset, value);
    return offset + 8;
  }
  writeDoubleLE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat64(offset, value, true);
    return offset + 8;
  }
  writeFloatBE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat32(offset, value);
    return offset + 4;
  }
  writeFloatLE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setFloat32(offset, value, true);
    return offset + 4;
  }
  writeInt8(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt8(offset, value);
    return offset + 1;
  }
  writeInt16BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt16(offset, value);
    return offset + 2;
  }
  writeInt16LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt16(offset, value, true);
    return offset + 2;
  }
  writeInt32BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(offset, value);
    return offset + 4;
  }
  writeInt32LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setInt32(offset, value, true);
    return offset + 4;
  }
  writeUInt8(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint8(offset, value);
    return offset + 1;
  }
  writeUInt16BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint16(offset, value);
    return offset + 2;
  }
  writeUInt16LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint16(offset, value, true);
    return offset + 2;
  }
  writeUInt32BE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(offset, value);
    return offset + 4;
  }
  writeUInt32LE(value, offset = 0) {
    new DataView(this.buffer, this.byteOffset, this.byteLength).setUint32(offset, value, true);
    return offset + 4;
  }
};

// _fs/_fs_readFile.ts
function maybeDecode(data, encoding) {
  const buffer = new Buffer3(data.buffer, data.byteOffset, data.byteLength);
  if (encoding && encoding !== "binary")
    return buffer.toString(encoding);
  return buffer;
}
function readFile(path3, optOrCallback, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  let cb;
  if (typeof optOrCallback === "function") {
    cb = optOrCallback;
  } else {
    cb = callback;
  }
  const encoding = getEncoding(optOrCallback);
  const p = Deno.readFile(path3);
  if (cb) {
    p.then((data) => {
      if (encoding && encoding !== "binary") {
        const text = maybeDecode(data, encoding);
        return cb(null, text);
      }
      const buffer = maybeDecode(data, encoding);
      cb(null, buffer);
    }, (err) => cb && cb(err));
  }
}
function readFileSync(path3, opt) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  const data = Deno.readFileSync(path3);
  const encoding = getEncoding(opt);
  if (encoding && encoding !== "binary") {
    const text = maybeDecode(data, encoding);
    return text;
  }
  const buffer = maybeDecode(data, encoding);
  return buffer;
}

// _fs/_fs_readlink.ts
function maybeEncode(data, encoding) {
  if (encoding === "buffer") {
    return new TextEncoder().encode(data);
  }
  return data;
}
function getEncoding2(optOrCallback) {
  if (!optOrCallback || typeof optOrCallback === "function") {
    return null;
  } else {
    if (optOrCallback.encoding) {
      if (optOrCallback.encoding === "utf8" || optOrCallback.encoding === "utf-8") {
        return "utf8";
      } else if (optOrCallback.encoding === "buffer") {
        return "buffer";
      } else {
        notImplemented();
      }
    }
    return null;
  }
}
function readlink(path3, optOrCallback, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  let cb;
  if (typeof optOrCallback === "function") {
    cb = optOrCallback;
  } else {
    cb = callback;
  }
  const encoding = getEncoding2(optOrCallback);
  intoCallbackAPIWithIntercept(Deno.readLink, (data) => maybeEncode(data, encoding), cb, path3);
}
function readlinkSync(path3, opt) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  return maybeEncode(Deno.readLinkSync(path3), getEncoding2(opt));
}

// _fs/_fs_realpath.ts
function realpath(path3, options, callback) {
  if (typeof options === "function") {
    callback = options;
  }
  if (!callback) {
    throw new Error("No callback function supplied");
  }
  Deno.realPath(path3).then((path4) => callback(null, path4), (err) => callback(err));
}
function realpathSync(path3) {
  return Deno.realPathSync(path3);
}

// _fs/_fs_rename.ts
function rename(oldPath, newPath, callback) {
  oldPath = oldPath instanceof URL ? fromFileUrl3(oldPath) : oldPath;
  newPath = newPath instanceof URL ? fromFileUrl3(newPath) : newPath;
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.rename(oldPath, newPath).then((_) => callback(), callback);
}
function renameSync(oldPath, newPath) {
  oldPath = oldPath instanceof URL ? fromFileUrl3(oldPath) : oldPath;
  newPath = newPath instanceof URL ? fromFileUrl3(newPath) : newPath;
  Deno.renameSync(oldPath, newPath);
}

// _fs/_fs_rmdir.ts
function rmdir(path3, optionsOrCallback, maybeCallback) {
  const callback = typeof optionsOrCallback === "function" ? optionsOrCallback : maybeCallback;
  const options = typeof optionsOrCallback === "object" ? optionsOrCallback : void 0;
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.remove(path3, { recursive: options?.recursive }).then((_) => callback(), callback);
}
function rmdirSync(path3, options) {
  Deno.removeSync(path3, { recursive: options?.recursive });
}

// _fs/_fs_symlink.ts
function symlink(target, path3, typeOrCallback, maybeCallback) {
  target = target instanceof URL ? fromFileUrl3(target) : target;
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  const type = typeof typeOrCallback === "string" ? typeOrCallback : "file";
  const callback = typeof typeOrCallback === "function" ? typeOrCallback : maybeCallback;
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.symlink(target, path3, { type }).then(() => callback(null), callback);
}
function symlinkSync(target, path3, type) {
  target = target instanceof URL ? fromFileUrl3(target) : target;
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  type = type || "file";
  Deno.symlinkSync(target, path3, { type });
}

// _fs/_fs_truncate.ts
function truncate(path3, lenOrCallback, maybeCallback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  const len = typeof lenOrCallback === "number" ? lenOrCallback : void 0;
  const callback = typeof lenOrCallback === "function" ? lenOrCallback : maybeCallback;
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.truncate(path3, len).then(() => callback(null), callback);
}
function truncateSync(path3, len) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  Deno.truncateSync(path3, len);
}

// _fs/_fs_unlink.ts
function unlink(path3, callback) {
  if (!callback)
    throw new Error("No callback function supplied");
  Deno.remove(path3).then((_) => callback(), callback);
}
function unlinkSync(path3) {
  Deno.removeSync(path3);
}

// _fs/_fs_utimes.ts
function getValidTime2(time, name) {
  if (typeof time === "string") {
    time = Number(time);
  }
  if (typeof time === "number" && (Number.isNaN(time) || !Number.isFinite(time))) {
    throw new Deno.errors.InvalidData(`invalid ${name}, must not be infitiny or NaN`);
  }
  return time;
}
function utimes(path3, atime, mtime, callback) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  if (!callback) {
    throw new Deno.errors.InvalidData("No callback function supplied");
  }
  atime = getValidTime2(atime, "atime");
  mtime = getValidTime2(mtime, "mtime");
  Deno.utime(path3, atime, mtime).then(() => callback(null), callback);
}
function utimesSync(path3, atime, mtime) {
  path3 = path3 instanceof URL ? fromFileUrl3(path3) : path3;
  atime = getValidTime2(atime, "atime");
  mtime = getValidTime2(mtime, "mtime");
  Deno.utimeSync(path3, atime, mtime);
}

// _fs/_fs_writeFile.ts
function writeFile(pathOrRid, data, optOrCallback, callback) {
  const callbackFn = optOrCallback instanceof Function ? optOrCallback : callback;
  const options = optOrCallback instanceof Function ? void 0 : optOrCallback;
  if (!callbackFn) {
    throw new TypeError("Callback must be a function.");
  }
  pathOrRid = pathOrRid instanceof URL ? fromFileUrl3(pathOrRid) : pathOrRid;
  const flag = isFileOptions(options) ? options.flag : void 0;
  const mode = isFileOptions(options) ? options.mode : void 0;
  const encoding = checkEncoding(getEncoding(options)) || "utf8";
  const openOptions = getOpenOptions(flag || "w");
  if (typeof data === "string")
    data = Buffer3.from(data, encoding);
  const isRid = typeof pathOrRid === "number";
  let file;
  let error = null;
  (async () => {
    try {
      file = isRid ? new Deno.File(pathOrRid) : await Deno.open(pathOrRid, openOptions);
      if (!isRid && mode) {
        if (Deno.build.os === "windows")
          notImplemented(`"mode" on Windows`);
        await Deno.chmod(pathOrRid, mode);
      }
      await writeAll(file, data);
    } catch (e) {
      error = e;
    } finally {
      if (!isRid && file)
        file.close();
      callbackFn(error);
    }
  })();
}
function writeFileSync(pathOrRid, data, options) {
  pathOrRid = pathOrRid instanceof URL ? fromFileUrl3(pathOrRid) : pathOrRid;
  const flag = isFileOptions(options) ? options.flag : void 0;
  const mode = isFileOptions(options) ? options.mode : void 0;
  const encoding = checkEncoding(getEncoding(options)) || "utf8";
  const openOptions = getOpenOptions(flag || "w");
  if (typeof data === "string")
    data = Buffer3.from(data, encoding);
  const isRid = typeof pathOrRid === "number";
  let file;
  let error = null;
  try {
    file = isRid ? new Deno.File(pathOrRid) : Deno.openSync(pathOrRid, openOptions);
    if (!isRid && mode) {
      if (Deno.build.os === "windows")
        notImplemented(`"mode" on Windows`);
      Deno.chmodSync(pathOrRid, mode);
    }
    writeAllSync(file, data);
  } catch (e) {
    error = e;
  } finally {
    if (!isRid && file)
      file.close();
  }
  if (error)
    throw error;
}

// fs/promises.ts
var promises_exports = {};
__export(promises_exports, {
  access: () => access2,
  appendFile: () => appendFile2,
  chmod: () => chmod2,
  chown: () => chown2,
  copyFile: () => copyFile2,
  default: () => promises_default,
  link: () => link2,
  lstat: () => lstat2,
  mkdir: () => mkdir2,
  mkdtemp: () => mkdtemp2,
  open: () => open2,
  readFile: () => readFile2,
  readdir: () => readdir2,
  readlink: () => readlink2,
  realpath: () => realpath2,
  rename: () => rename2,
  rmdir: () => rmdir2,
  stat: () => stat2,
  symlink: () => symlink2,
  truncate: () => truncate2,
  unlink: () => unlink2,
  utimes: () => utimes2,
  watch: () => watch2,
  writeFile: () => writeFile2
});
var access2 = promisify(access);
var copyFile2 = promisify(copyFile);
var open2 = promisify(open);
var rename2 = promisify(rename);
var truncate2 = promisify(truncate);
var rmdir2 = promisify(rmdir);
var mkdir2 = promisify(mkdir);
var readdir2 = promisify(readdir);
var readlink2 = promisify(readlink);
var symlink2 = promisify(symlink);
var lstat2 = promisify(lstat);
var stat2 = promisify(stat);
var link2 = promisify(link);
var unlink2 = promisify(unlink);
var chmod2 = promisify(chmod);
var chown2 = promisify(chown);
var utimes2 = promisify(utimes);
var realpath2 = promisify(realpath);
var mkdtemp2 = promisify(mkdtemp);
var writeFile2 = promisify(writeFile);
var appendFile2 = promisify(appendFile);
var readFile2 = promisify(readFile);
var watch2 = promisify(watch);
var promises_default = {
  open: open2,
  rename: rename2,
  truncate: truncate2,
  rmdir: rmdir2,
  mkdir: mkdir2,
  readdir: readdir2,
  readlink: readlink2,
  symlink: symlink2,
  lstat: lstat2,
  stat: stat2,
  link: link2,
  unlink: unlink2,
  chmod: chmod2,
  chown: chown2,
  utimes: utimes2,
  realpath: realpath2,
  mkdtemp: mkdtemp2,
  writeFile: writeFile2,
  appendFile: appendFile2,
  readFile: readFile2,
  watch: watch2
};

// fs.ts
var fs_default = {
  access,
  accessSync,
  appendFile,
  appendFileSync,
  chmod,
  chmodSync,
  chown,
  chownSync,
  close,
  closeSync,
  constants: fs_constants_exports,
  copyFile,
  copyFileSync,
  Dir,
  Dirent,
  exists,
  existsSync,
  fdatasync,
  fdatasyncSync,
  fstat,
  fstatSync,
  fsync,
  fsyncSync,
  ftruncate,
  ftruncateSync,
  futimes,
  futimesSync,
  link,
  linkSync,
  lstat,
  lstatSync,
  mkdir,
  mkdirSync,
  mkdtemp,
  mkdtempSync,
  open,
  openSync,
  promises: promises_exports,
  readdir,
  readdirSync,
  readFile,
  readFileSync,
  readlink,
  readlinkSync,
  realpath,
  realpathSync,
  rename,
  renameSync,
  rmdir,
  rmdirSync,
  stat,
  statSync,
  symlink,
  symlinkSync,
  truncate,
  truncateSync,
  unlink,
  unlinkSync,
  utimes,
  utimesSync,
  watch,
  writeFile,
  writeFileSync
};
export {
  Dir,
  Dirent,
  access,
  accessSync,
  appendFile,
  appendFileSync,
  chmod,
  chmodSync,
  chown,
  chownSync,
  close,
  closeSync,
  fs_constants_exports as constants,
  copyFile,
  copyFileSync,
  fs_default as default,
  exists,
  existsSync,
  fdatasync,
  fdatasyncSync,
  fstat,
  fstatSync,
  fsync,
  fsyncSync,
  ftruncate,
  ftruncateSync,
  futimes,
  futimesSync,
  link,
  linkSync,
  lstat,
  lstatSync,
  mkdir,
  mkdirSync,
  mkdtemp,
  mkdtempSync,
  open,
  openSync,
  promises_exports as promises,
  readFile,
  readFileSync,
  readdir,
  readdirSync,
  readlink,
  readlinkSync,
  realpath,
  realpathSync,
  rename,
  renameSync,
  rmdir,
  rmdirSync,
  stat,
  statSync,
  symlink,
  symlinkSync,
  truncate,
  truncateSync,
  unlink,
  unlinkSync,
  utimes,
  utimesSync,
  watch,
  writeFile,
  writeFileSync
};
