// Copyright 2018-2020 the Deno authors. All rights reserved. MIT license.

// This is a specialised implementation of a System module loader.

"use strict";

// @ts-nocheck
/* eslint-disable */
let System, __instantiate;
(() => {
  const r = new Map();

  System = {
    register(id, d, f) {
      r.set(id, { d, f, exp: {} });
    },
  };
  async function dI(mid, src) {
    let id = mid.replace(/\.\w+$/i, "");
    if (id.includes("./")) {
      const [o, ...ia] = id.split("/").reverse(),
        [, ...sa] = src.split("/").reverse(),
        oa = [o];
      let s = 0,
        i;
      while ((i = ia.shift())) {
        if (i === "..") s++;
        else if (i === ".") break;
        else oa.push(i);
      }
      if (s < sa.length) oa.push(...sa.slice(s));
      id = oa.reverse().join("/");
    }
    return r.has(id) ? gExpA(id) : import(mid);
  }

  function gC(id, main) {
    return {
      id,
      import: (m) => dI(m, id),
      meta: { url: id, main },
    };
  }

  function gE(exp) {
    return (id, v) => {
      const e = typeof id === "string" ? { [id]: v } : id;
      for (const [id, value] of Object.entries(e)) {
        Object.defineProperty(exp, id, {
          value,
          writable: true,
          enumerable: true,
        });
      }
      return v;
    };
  }

  function rF(main) {
    for (const [id, m] of r.entries()) {
      const { f, exp } = m;
      const { execute: e, setters: s } = f(gE(exp), gC(id, id === main));
      delete m.f;
      m.e = e;
      m.s = s;
    }
  }

  async function gExpA(id) {
    if (!r.has(id)) return;
    const m = r.get(id);
    if (m.s) {
      const { d, e, s } = m;
      delete m.s;
      delete m.e;
      for (let i = 0; i < s.length; i++) s[i](await gExpA(d[i]));
      const r = e();
      if (r) await r;
    }
    return m.exp;
  }

  function gExp(id) {
    if (!r.has(id)) return;
    const m = r.get(id);
    if (m.s) {
      const { d, e, s } = m;
      delete m.s;
      delete m.e;
      for (let i = 0; i < s.length; i++) s[i](gExp(d[i]));
      e();
    }
    return m.exp;
  }
  __instantiate = (m, a) => {
    System = __instantiate = undefined;
    rF(m);
    return a ? gExpA(m) : gExp(m);
  };
})();

System.register("node/_utils", [], function (exports_1, context_1) {
    "use strict";
    var _TextDecoder, _TextEncoder;
    var __moduleName = context_1 && context_1.id;
    function notImplemented(msg) {
        const message = msg ? `Not implemented: ${msg}` : "Not implemented";
        throw new Error(message);
    }
    exports_1("notImplemented", notImplemented);
    function intoCallbackAPI(func, cb, ...args) {
        func(...args)
            .then((value) => cb && cb(null, value))
            .catch((err) => cb && cb(err, null));
    }
    exports_1("intoCallbackAPI", intoCallbackAPI);
    function intoCallbackAPIWithIntercept(func, interceptor, cb, ...args) {
        func(...args)
            .then((value) => cb && cb(null, interceptor(value)))
            .catch((err) => cb && cb(err, null));
    }
    exports_1("intoCallbackAPIWithIntercept", intoCallbackAPIWithIntercept);
    function spliceOne(list, index) {
        for (; index + 1 < list.length; index++)
            list[index] = list[index + 1];
        list.pop();
    }
    exports_1("spliceOne", spliceOne);
    function normalizeEncoding(enc) {
        if (enc == null || enc === "utf8" || enc === "utf-8")
            return "utf8";
        return slowCases(enc);
    }
    exports_1("normalizeEncoding", normalizeEncoding);
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
                if (enc === "utf16le" ||
                    enc === "UTF16LE" ||
                    `${enc}`.toLowerCase() === "utf16le") {
                    return "utf16le";
                }
                break;
            case 8:
                if (enc === "utf-16le" ||
                    enc === "UTF-16LE" ||
                    `${enc}`.toLowerCase() === "utf-16le") {
                    return "utf16le";
                }
                break;
            default:
                if (enc === "")
                    return "utf8";
        }
    }
    function validateIntegerRange(value, name, min = -2147483648, max = 2147483647) {
        if (!Number.isInteger(value)) {
            throw new Error(`${name} must be 'an integer' but was ${value}`);
        }
        if (value < min || value > max) {
            throw new Error(`${name} must be >= ${min} && <= ${max}. Value was ${value}`);
        }
    }
    exports_1("validateIntegerRange", validateIntegerRange);
    return {
        setters: [],
        execute: function () {
            exports_1("_TextDecoder", _TextDecoder = TextDecoder);
            exports_1("_TextEncoder", _TextEncoder = TextEncoder);
        }
    };
});
System.register("node/_fs/_fs_common", ["node/_utils"], function (exports_2, context_2) {
    "use strict";
    var _utils_ts_1;
    var __moduleName = context_2 && context_2.id;
    function isFileOptions(fileOptions) {
        if (!fileOptions)
            return false;
        return (fileOptions.encoding != undefined ||
            fileOptions.flag != undefined ||
            fileOptions.mode != undefined);
    }
    exports_2("isFileOptions", isFileOptions);
    function getEncoding(optOrCallback) {
        if (!optOrCallback || typeof optOrCallback === "function") {
            return null;
        }
        const encoding = typeof optOrCallback === "string"
            ? optOrCallback
            : optOrCallback.encoding;
        if (!encoding)
            return null;
        return encoding;
    }
    exports_2("getEncoding", getEncoding);
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
        const notImplementedEncodings = ["utf16le", "latin1", "ascii", "ucs2"];
        if (notImplementedEncodings.includes(encoding)) {
            _utils_ts_1.notImplemented(`"${encoding}" encoding`);
        }
        throw new Error(`The value "${encoding}" is invalid for option "encoding"`);
    }
    exports_2("checkEncoding", checkEncoding);
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
    exports_2("getOpenOptions", getOpenOptions);
    return {
        setters: [
            function (_utils_ts_1_1) {
                _utils_ts_1 = _utils_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_access", ["node/_utils"], function (exports_3, context_3) {
    "use strict";
    var _utils_ts_2;
    var __moduleName = context_3 && context_3.id;
    function access(_path, _modeOrCallback, _callback) {
        _utils_ts_2.notImplemented("Not yet available");
    }
    exports_3("access", access);
    function accessSync(path, mode) {
        _utils_ts_2.notImplemented("Not yet available");
    }
    exports_3("accessSync", accessSync);
    return {
        setters: [
            function (_utils_ts_2_1) {
                _utils_ts_2 = _utils_ts_2_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("path/_constants", [], function (exports_4, context_4) {
    "use strict";
    var CHAR_UPPERCASE_A, CHAR_LOWERCASE_A, CHAR_UPPERCASE_Z, CHAR_LOWERCASE_Z, CHAR_DOT, CHAR_FORWARD_SLASH, CHAR_BACKWARD_SLASH, CHAR_VERTICAL_LINE, CHAR_COLON, CHAR_QUESTION_MARK, CHAR_UNDERSCORE, CHAR_LINE_FEED, CHAR_CARRIAGE_RETURN, CHAR_TAB, CHAR_FORM_FEED, CHAR_EXCLAMATION_MARK, CHAR_HASH, CHAR_SPACE, CHAR_NO_BREAK_SPACE, CHAR_ZERO_WIDTH_NOBREAK_SPACE, CHAR_LEFT_SQUARE_BRACKET, CHAR_RIGHT_SQUARE_BRACKET, CHAR_LEFT_ANGLE_BRACKET, CHAR_RIGHT_ANGLE_BRACKET, CHAR_LEFT_CURLY_BRACKET, CHAR_RIGHT_CURLY_BRACKET, CHAR_HYPHEN_MINUS, CHAR_PLUS, CHAR_DOUBLE_QUOTE, CHAR_SINGLE_QUOTE, CHAR_PERCENT, CHAR_SEMICOLON, CHAR_CIRCUMFLEX_ACCENT, CHAR_GRAVE_ACCENT, CHAR_AT, CHAR_AMPERSAND, CHAR_EQUAL, CHAR_0, CHAR_9, NATIVE_OS, navigator, isWindows;
    var __moduleName = context_4 && context_4.id;
    return {
        setters: [],
        execute: function () {
            exports_4("CHAR_UPPERCASE_A", CHAR_UPPERCASE_A = 65);
            exports_4("CHAR_LOWERCASE_A", CHAR_LOWERCASE_A = 97);
            exports_4("CHAR_UPPERCASE_Z", CHAR_UPPERCASE_Z = 90);
            exports_4("CHAR_LOWERCASE_Z", CHAR_LOWERCASE_Z = 122);
            exports_4("CHAR_DOT", CHAR_DOT = 46);
            exports_4("CHAR_FORWARD_SLASH", CHAR_FORWARD_SLASH = 47);
            exports_4("CHAR_BACKWARD_SLASH", CHAR_BACKWARD_SLASH = 92);
            exports_4("CHAR_VERTICAL_LINE", CHAR_VERTICAL_LINE = 124);
            exports_4("CHAR_COLON", CHAR_COLON = 58);
            exports_4("CHAR_QUESTION_MARK", CHAR_QUESTION_MARK = 63);
            exports_4("CHAR_UNDERSCORE", CHAR_UNDERSCORE = 95);
            exports_4("CHAR_LINE_FEED", CHAR_LINE_FEED = 10);
            exports_4("CHAR_CARRIAGE_RETURN", CHAR_CARRIAGE_RETURN = 13);
            exports_4("CHAR_TAB", CHAR_TAB = 9);
            exports_4("CHAR_FORM_FEED", CHAR_FORM_FEED = 12);
            exports_4("CHAR_EXCLAMATION_MARK", CHAR_EXCLAMATION_MARK = 33);
            exports_4("CHAR_HASH", CHAR_HASH = 35);
            exports_4("CHAR_SPACE", CHAR_SPACE = 32);
            exports_4("CHAR_NO_BREAK_SPACE", CHAR_NO_BREAK_SPACE = 160);
            exports_4("CHAR_ZERO_WIDTH_NOBREAK_SPACE", CHAR_ZERO_WIDTH_NOBREAK_SPACE = 65279);
            exports_4("CHAR_LEFT_SQUARE_BRACKET", CHAR_LEFT_SQUARE_BRACKET = 91);
            exports_4("CHAR_RIGHT_SQUARE_BRACKET", CHAR_RIGHT_SQUARE_BRACKET = 93);
            exports_4("CHAR_LEFT_ANGLE_BRACKET", CHAR_LEFT_ANGLE_BRACKET = 60);
            exports_4("CHAR_RIGHT_ANGLE_BRACKET", CHAR_RIGHT_ANGLE_BRACKET = 62);
            exports_4("CHAR_LEFT_CURLY_BRACKET", CHAR_LEFT_CURLY_BRACKET = 123);
            exports_4("CHAR_RIGHT_CURLY_BRACKET", CHAR_RIGHT_CURLY_BRACKET = 125);
            exports_4("CHAR_HYPHEN_MINUS", CHAR_HYPHEN_MINUS = 45);
            exports_4("CHAR_PLUS", CHAR_PLUS = 43);
            exports_4("CHAR_DOUBLE_QUOTE", CHAR_DOUBLE_QUOTE = 34);
            exports_4("CHAR_SINGLE_QUOTE", CHAR_SINGLE_QUOTE = 39);
            exports_4("CHAR_PERCENT", CHAR_PERCENT = 37);
            exports_4("CHAR_SEMICOLON", CHAR_SEMICOLON = 59);
            exports_4("CHAR_CIRCUMFLEX_ACCENT", CHAR_CIRCUMFLEX_ACCENT = 94);
            exports_4("CHAR_GRAVE_ACCENT", CHAR_GRAVE_ACCENT = 96);
            exports_4("CHAR_AT", CHAR_AT = 64);
            exports_4("CHAR_AMPERSAND", CHAR_AMPERSAND = 38);
            exports_4("CHAR_EQUAL", CHAR_EQUAL = 61);
            exports_4("CHAR_0", CHAR_0 = 48);
            exports_4("CHAR_9", CHAR_9 = 57);
            NATIVE_OS = "linux";
            exports_4("NATIVE_OS", NATIVE_OS);
            navigator = globalThis.navigator;
            if (globalThis.Deno != null) {
                exports_4("NATIVE_OS", NATIVE_OS = Deno.build.os);
            }
            else if (navigator?.appVersion?.includes?.("Win") ?? false) {
                exports_4("NATIVE_OS", NATIVE_OS = "windows");
            }
            exports_4("isWindows", isWindows = NATIVE_OS == "windows");
        }
    };
});
System.register("path/_interface", [], function (exports_5, context_5) {
    "use strict";
    var __moduleName = context_5 && context_5.id;
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("path/_util", ["path/_constants"], function (exports_6, context_6) {
    "use strict";
    var _constants_ts_1;
    var __moduleName = context_6 && context_6.id;
    function assertPath(path) {
        if (typeof path !== "string") {
            throw new TypeError(`Path must be a string. Received ${JSON.stringify(path)}`);
        }
    }
    exports_6("assertPath", assertPath);
    function isPosixPathSeparator(code) {
        return code === _constants_ts_1.CHAR_FORWARD_SLASH;
    }
    exports_6("isPosixPathSeparator", isPosixPathSeparator);
    function isPathSeparator(code) {
        return isPosixPathSeparator(code) || code === _constants_ts_1.CHAR_BACKWARD_SLASH;
    }
    exports_6("isPathSeparator", isPathSeparator);
    function isWindowsDeviceRoot(code) {
        return ((code >= _constants_ts_1.CHAR_LOWERCASE_A && code <= _constants_ts_1.CHAR_LOWERCASE_Z) ||
            (code >= _constants_ts_1.CHAR_UPPERCASE_A && code <= _constants_ts_1.CHAR_UPPERCASE_Z));
    }
    exports_6("isWindowsDeviceRoot", isWindowsDeviceRoot);
    function normalizeString(path, allowAboveRoot, separator, isPathSeparator) {
        let res = "";
        let lastSegmentLength = 0;
        let lastSlash = -1;
        let dots = 0;
        let code;
        for (let i = 0, len = path.length; i <= len; ++i) {
            if (i < len)
                code = path.charCodeAt(i);
            else if (isPathSeparator(code))
                break;
            else
                code = _constants_ts_1.CHAR_FORWARD_SLASH;
            if (isPathSeparator(code)) {
                if (lastSlash === i - 1 || dots === 1) {
                }
                else if (lastSlash !== i - 1 && dots === 2) {
                    if (res.length < 2 ||
                        lastSegmentLength !== 2 ||
                        res.charCodeAt(res.length - 1) !== _constants_ts_1.CHAR_DOT ||
                        res.charCodeAt(res.length - 2) !== _constants_ts_1.CHAR_DOT) {
                        if (res.length > 2) {
                            const lastSlashIndex = res.lastIndexOf(separator);
                            if (lastSlashIndex === -1) {
                                res = "";
                                lastSegmentLength = 0;
                            }
                            else {
                                res = res.slice(0, lastSlashIndex);
                                lastSegmentLength = res.length - 1 - res.lastIndexOf(separator);
                            }
                            lastSlash = i;
                            dots = 0;
                            continue;
                        }
                        else if (res.length === 2 || res.length === 1) {
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
                }
                else {
                    if (res.length > 0)
                        res += separator + path.slice(lastSlash + 1, i);
                    else
                        res = path.slice(lastSlash + 1, i);
                    lastSegmentLength = i - lastSlash - 1;
                }
                lastSlash = i;
                dots = 0;
            }
            else if (code === _constants_ts_1.CHAR_DOT && dots !== -1) {
                ++dots;
            }
            else {
                dots = -1;
            }
        }
        return res;
    }
    exports_6("normalizeString", normalizeString);
    function _format(sep, pathObject) {
        const dir = pathObject.dir || pathObject.root;
        const base = pathObject.base ||
            (pathObject.name || "") + (pathObject.ext || "");
        if (!dir)
            return base;
        if (dir === pathObject.root)
            return dir + base;
        return dir + sep + base;
    }
    exports_6("_format", _format);
    return {
        setters: [
            function (_constants_ts_1_1) {
                _constants_ts_1 = _constants_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("_util/assert", [], function (exports_7, context_7) {
    "use strict";
    var DenoStdInternalError;
    var __moduleName = context_7 && context_7.id;
    function assert(expr, msg = "") {
        if (!expr) {
            throw new DenoStdInternalError(msg);
        }
    }
    exports_7("assert", assert);
    return {
        setters: [],
        execute: function () {
            DenoStdInternalError = class DenoStdInternalError extends Error {
                constructor(message) {
                    super(message);
                    this.name = "DenoStdInternalError";
                }
            };
            exports_7("DenoStdInternalError", DenoStdInternalError);
        }
    };
});
System.register("path/win32", ["path/_constants", "path/_util", "_util/assert"], function (exports_8, context_8) {
    "use strict";
    var _constants_ts_2, _util_ts_1, assert_ts_1, sep, delimiter;
    var __moduleName = context_8 && context_8.id;
    function resolve(...pathSegments) {
        let resolvedDevice = "";
        let resolvedTail = "";
        let resolvedAbsolute = false;
        for (let i = pathSegments.length - 1; i >= -1; i--) {
            let path;
            if (i >= 0) {
                path = pathSegments[i];
            }
            else if (!resolvedDevice) {
                if (globalThis.Deno == null) {
                    throw new TypeError("Resolved a drive-letter-less path without a CWD.");
                }
                path = Deno.cwd();
            }
            else {
                if (globalThis.Deno == null) {
                    throw new TypeError("Resolved a relative path without a CWD.");
                }
                path = Deno.env.get(`=${resolvedDevice}`) || Deno.cwd();
                if (path === undefined ||
                    path.slice(0, 3).toLowerCase() !== `${resolvedDevice.toLowerCase()}\\`) {
                    path = `${resolvedDevice}\\`;
                }
            }
            _util_ts_1.assertPath(path);
            const len = path.length;
            if (len === 0)
                continue;
            let rootEnd = 0;
            let device = "";
            let isAbsolute = false;
            const code = path.charCodeAt(0);
            if (len > 1) {
                if (_util_ts_1.isPathSeparator(code)) {
                    isAbsolute = true;
                    if (_util_ts_1.isPathSeparator(path.charCodeAt(1))) {
                        let j = 2;
                        let last = j;
                        for (; j < len; ++j) {
                            if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                break;
                        }
                        if (j < len && j !== last) {
                            const firstPart = path.slice(last, j);
                            last = j;
                            for (; j < len; ++j) {
                                if (!_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                    break;
                            }
                            if (j < len && j !== last) {
                                last = j;
                                for (; j < len; ++j) {
                                    if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                        break;
                                }
                                if (j === len) {
                                    device = `\\\\${firstPart}\\${path.slice(last)}`;
                                    rootEnd = j;
                                }
                                else if (j !== last) {
                                    device = `\\\\${firstPart}\\${path.slice(last, j)}`;
                                    rootEnd = j;
                                }
                            }
                        }
                    }
                    else {
                        rootEnd = 1;
                    }
                }
                else if (_util_ts_1.isWindowsDeviceRoot(code)) {
                    if (path.charCodeAt(1) === _constants_ts_2.CHAR_COLON) {
                        device = path.slice(0, 2);
                        rootEnd = 2;
                        if (len > 2) {
                            if (_util_ts_1.isPathSeparator(path.charCodeAt(2))) {
                                isAbsolute = true;
                                rootEnd = 3;
                            }
                        }
                    }
                }
            }
            else if (_util_ts_1.isPathSeparator(code)) {
                rootEnd = 1;
                isAbsolute = true;
            }
            if (device.length > 0 &&
                resolvedDevice.length > 0 &&
                device.toLowerCase() !== resolvedDevice.toLowerCase()) {
                continue;
            }
            if (resolvedDevice.length === 0 && device.length > 0) {
                resolvedDevice = device;
            }
            if (!resolvedAbsolute) {
                resolvedTail = `${path.slice(rootEnd)}\\${resolvedTail}`;
                resolvedAbsolute = isAbsolute;
            }
            if (resolvedAbsolute && resolvedDevice.length > 0)
                break;
        }
        resolvedTail = _util_ts_1.normalizeString(resolvedTail, !resolvedAbsolute, "\\", _util_ts_1.isPathSeparator);
        return resolvedDevice + (resolvedAbsolute ? "\\" : "") + resolvedTail || ".";
    }
    exports_8("resolve", resolve);
    function normalize(path) {
        _util_ts_1.assertPath(path);
        const len = path.length;
        if (len === 0)
            return ".";
        let rootEnd = 0;
        let device;
        let isAbsolute = false;
        const code = path.charCodeAt(0);
        if (len > 1) {
            if (_util_ts_1.isPathSeparator(code)) {
                isAbsolute = true;
                if (_util_ts_1.isPathSeparator(path.charCodeAt(1))) {
                    let j = 2;
                    let last = j;
                    for (; j < len; ++j) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                            break;
                    }
                    if (j < len && j !== last) {
                        const firstPart = path.slice(last, j);
                        last = j;
                        for (; j < len; ++j) {
                            if (!_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                break;
                        }
                        if (j < len && j !== last) {
                            last = j;
                            for (; j < len; ++j) {
                                if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                    break;
                            }
                            if (j === len) {
                                return `\\\\${firstPart}\\${path.slice(last)}\\`;
                            }
                            else if (j !== last) {
                                device = `\\\\${firstPart}\\${path.slice(last, j)}`;
                                rootEnd = j;
                            }
                        }
                    }
                }
                else {
                    rootEnd = 1;
                }
            }
            else if (_util_ts_1.isWindowsDeviceRoot(code)) {
                if (path.charCodeAt(1) === _constants_ts_2.CHAR_COLON) {
                    device = path.slice(0, 2);
                    rootEnd = 2;
                    if (len > 2) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(2))) {
                            isAbsolute = true;
                            rootEnd = 3;
                        }
                    }
                }
            }
        }
        else if (_util_ts_1.isPathSeparator(code)) {
            return "\\";
        }
        let tail;
        if (rootEnd < len) {
            tail = _util_ts_1.normalizeString(path.slice(rootEnd), !isAbsolute, "\\", _util_ts_1.isPathSeparator);
        }
        else {
            tail = "";
        }
        if (tail.length === 0 && !isAbsolute)
            tail = ".";
        if (tail.length > 0 && _util_ts_1.isPathSeparator(path.charCodeAt(len - 1))) {
            tail += "\\";
        }
        if (device === undefined) {
            if (isAbsolute) {
                if (tail.length > 0)
                    return `\\${tail}`;
                else
                    return "\\";
            }
            else if (tail.length > 0) {
                return tail;
            }
            else {
                return "";
            }
        }
        else if (isAbsolute) {
            if (tail.length > 0)
                return `${device}\\${tail}`;
            else
                return `${device}\\`;
        }
        else if (tail.length > 0) {
            return device + tail;
        }
        else {
            return device;
        }
    }
    exports_8("normalize", normalize);
    function isAbsolute(path) {
        _util_ts_1.assertPath(path);
        const len = path.length;
        if (len === 0)
            return false;
        const code = path.charCodeAt(0);
        if (_util_ts_1.isPathSeparator(code)) {
            return true;
        }
        else if (_util_ts_1.isWindowsDeviceRoot(code)) {
            if (len > 2 && path.charCodeAt(1) === _constants_ts_2.CHAR_COLON) {
                if (_util_ts_1.isPathSeparator(path.charCodeAt(2)))
                    return true;
            }
        }
        return false;
    }
    exports_8("isAbsolute", isAbsolute);
    function join(...paths) {
        const pathsCount = paths.length;
        if (pathsCount === 0)
            return ".";
        let joined;
        let firstPart = null;
        for (let i = 0; i < pathsCount; ++i) {
            const path = paths[i];
            _util_ts_1.assertPath(path);
            if (path.length > 0) {
                if (joined === undefined)
                    joined = firstPart = path;
                else
                    joined += `\\${path}`;
            }
        }
        if (joined === undefined)
            return ".";
        let needsReplace = true;
        let slashCount = 0;
        assert_ts_1.assert(firstPart != null);
        if (_util_ts_1.isPathSeparator(firstPart.charCodeAt(0))) {
            ++slashCount;
            const firstLen = firstPart.length;
            if (firstLen > 1) {
                if (_util_ts_1.isPathSeparator(firstPart.charCodeAt(1))) {
                    ++slashCount;
                    if (firstLen > 2) {
                        if (_util_ts_1.isPathSeparator(firstPart.charCodeAt(2)))
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
                if (!_util_ts_1.isPathSeparator(joined.charCodeAt(slashCount)))
                    break;
            }
            if (slashCount >= 2)
                joined = `\\${joined.slice(slashCount)}`;
        }
        return normalize(joined);
    }
    exports_8("join", join);
    function relative(from, to) {
        _util_ts_1.assertPath(from);
        _util_ts_1.assertPath(to);
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
            if (from.charCodeAt(fromStart) !== _constants_ts_2.CHAR_BACKWARD_SLASH)
                break;
        }
        for (; fromEnd - 1 > fromStart; --fromEnd) {
            if (from.charCodeAt(fromEnd - 1) !== _constants_ts_2.CHAR_BACKWARD_SLASH)
                break;
        }
        const fromLen = fromEnd - fromStart;
        let toStart = 0;
        let toEnd = to.length;
        for (; toStart < toEnd; ++toStart) {
            if (to.charCodeAt(toStart) !== _constants_ts_2.CHAR_BACKWARD_SLASH)
                break;
        }
        for (; toEnd - 1 > toStart; --toEnd) {
            if (to.charCodeAt(toEnd - 1) !== _constants_ts_2.CHAR_BACKWARD_SLASH)
                break;
        }
        const toLen = toEnd - toStart;
        const length = fromLen < toLen ? fromLen : toLen;
        let lastCommonSep = -1;
        let i = 0;
        for (; i <= length; ++i) {
            if (i === length) {
                if (toLen > length) {
                    if (to.charCodeAt(toStart + i) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                        return toOrig.slice(toStart + i + 1);
                    }
                    else if (i === 2) {
                        return toOrig.slice(toStart + i);
                    }
                }
                if (fromLen > length) {
                    if (from.charCodeAt(fromStart + i) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                        lastCommonSep = i;
                    }
                    else if (i === 2) {
                        lastCommonSep = 3;
                    }
                }
                break;
            }
            const fromCode = from.charCodeAt(fromStart + i);
            const toCode = to.charCodeAt(toStart + i);
            if (fromCode !== toCode)
                break;
            else if (fromCode === _constants_ts_2.CHAR_BACKWARD_SLASH)
                lastCommonSep = i;
        }
        if (i !== length && lastCommonSep === -1) {
            return toOrig;
        }
        let out = "";
        if (lastCommonSep === -1)
            lastCommonSep = 0;
        for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
            if (i === fromEnd || from.charCodeAt(i) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                if (out.length === 0)
                    out += "..";
                else
                    out += "\\..";
            }
        }
        if (out.length > 0) {
            return out + toOrig.slice(toStart + lastCommonSep, toEnd);
        }
        else {
            toStart += lastCommonSep;
            if (toOrig.charCodeAt(toStart) === _constants_ts_2.CHAR_BACKWARD_SLASH)
                ++toStart;
            return toOrig.slice(toStart, toEnd);
        }
    }
    exports_8("relative", relative);
    function toNamespacedPath(path) {
        if (typeof path !== "string")
            return path;
        if (path.length === 0)
            return "";
        const resolvedPath = resolve(path);
        if (resolvedPath.length >= 3) {
            if (resolvedPath.charCodeAt(0) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                if (resolvedPath.charCodeAt(1) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                    const code = resolvedPath.charCodeAt(2);
                    if (code !== _constants_ts_2.CHAR_QUESTION_MARK && code !== _constants_ts_2.CHAR_DOT) {
                        return `\\\\?\\UNC\\${resolvedPath.slice(2)}`;
                    }
                }
            }
            else if (_util_ts_1.isWindowsDeviceRoot(resolvedPath.charCodeAt(0))) {
                if (resolvedPath.charCodeAt(1) === _constants_ts_2.CHAR_COLON &&
                    resolvedPath.charCodeAt(2) === _constants_ts_2.CHAR_BACKWARD_SLASH) {
                    return `\\\\?\\${resolvedPath}`;
                }
            }
        }
        return path;
    }
    exports_8("toNamespacedPath", toNamespacedPath);
    function dirname(path) {
        _util_ts_1.assertPath(path);
        const len = path.length;
        if (len === 0)
            return ".";
        let rootEnd = -1;
        let end = -1;
        let matchedSlash = true;
        let offset = 0;
        const code = path.charCodeAt(0);
        if (len > 1) {
            if (_util_ts_1.isPathSeparator(code)) {
                rootEnd = offset = 1;
                if (_util_ts_1.isPathSeparator(path.charCodeAt(1))) {
                    let j = 2;
                    let last = j;
                    for (; j < len; ++j) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                            break;
                    }
                    if (j < len && j !== last) {
                        last = j;
                        for (; j < len; ++j) {
                            if (!_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                break;
                        }
                        if (j < len && j !== last) {
                            last = j;
                            for (; j < len; ++j) {
                                if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                    break;
                            }
                            if (j === len) {
                                return path;
                            }
                            if (j !== last) {
                                rootEnd = offset = j + 1;
                            }
                        }
                    }
                }
            }
            else if (_util_ts_1.isWindowsDeviceRoot(code)) {
                if (path.charCodeAt(1) === _constants_ts_2.CHAR_COLON) {
                    rootEnd = offset = 2;
                    if (len > 2) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(2)))
                            rootEnd = offset = 3;
                    }
                }
            }
        }
        else if (_util_ts_1.isPathSeparator(code)) {
            return path;
        }
        for (let i = len - 1; i >= offset; --i) {
            if (_util_ts_1.isPathSeparator(path.charCodeAt(i))) {
                if (!matchedSlash) {
                    end = i;
                    break;
                }
            }
            else {
                matchedSlash = false;
            }
        }
        if (end === -1) {
            if (rootEnd === -1)
                return ".";
            else
                end = rootEnd;
        }
        return path.slice(0, end);
    }
    exports_8("dirname", dirname);
    function basename(path, ext = "") {
        if (ext !== undefined && typeof ext !== "string") {
            throw new TypeError('"ext" argument must be a string');
        }
        _util_ts_1.assertPath(path);
        let start = 0;
        let end = -1;
        let matchedSlash = true;
        let i;
        if (path.length >= 2) {
            const drive = path.charCodeAt(0);
            if (_util_ts_1.isWindowsDeviceRoot(drive)) {
                if (path.charCodeAt(1) === _constants_ts_2.CHAR_COLON)
                    start = 2;
            }
        }
        if (ext !== undefined && ext.length > 0 && ext.length <= path.length) {
            if (ext.length === path.length && ext === path)
                return "";
            let extIdx = ext.length - 1;
            let firstNonSlashEnd = -1;
            for (i = path.length - 1; i >= start; --i) {
                const code = path.charCodeAt(i);
                if (_util_ts_1.isPathSeparator(code)) {
                    if (!matchedSlash) {
                        start = i + 1;
                        break;
                    }
                }
                else {
                    if (firstNonSlashEnd === -1) {
                        matchedSlash = false;
                        firstNonSlashEnd = i + 1;
                    }
                    if (extIdx >= 0) {
                        if (code === ext.charCodeAt(extIdx)) {
                            if (--extIdx === -1) {
                                end = i;
                            }
                        }
                        else {
                            extIdx = -1;
                            end = firstNonSlashEnd;
                        }
                    }
                }
            }
            if (start === end)
                end = firstNonSlashEnd;
            else if (end === -1)
                end = path.length;
            return path.slice(start, end);
        }
        else {
            for (i = path.length - 1; i >= start; --i) {
                if (_util_ts_1.isPathSeparator(path.charCodeAt(i))) {
                    if (!matchedSlash) {
                        start = i + 1;
                        break;
                    }
                }
                else if (end === -1) {
                    matchedSlash = false;
                    end = i + 1;
                }
            }
            if (end === -1)
                return "";
            return path.slice(start, end);
        }
    }
    exports_8("basename", basename);
    function extname(path) {
        _util_ts_1.assertPath(path);
        let start = 0;
        let startDot = -1;
        let startPart = 0;
        let end = -1;
        let matchedSlash = true;
        let preDotState = 0;
        if (path.length >= 2 &&
            path.charCodeAt(1) === _constants_ts_2.CHAR_COLON &&
            _util_ts_1.isWindowsDeviceRoot(path.charCodeAt(0))) {
            start = startPart = 2;
        }
        for (let i = path.length - 1; i >= start; --i) {
            const code = path.charCodeAt(i);
            if (_util_ts_1.isPathSeparator(code)) {
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
            if (code === _constants_ts_2.CHAR_DOT) {
                if (startDot === -1)
                    startDot = i;
                else if (preDotState !== 1)
                    preDotState = 1;
            }
            else if (startDot !== -1) {
                preDotState = -1;
            }
        }
        if (startDot === -1 ||
            end === -1 ||
            preDotState === 0 ||
            (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)) {
            return "";
        }
        return path.slice(startDot, end);
    }
    exports_8("extname", extname);
    function format(pathObject) {
        if (pathObject === null || typeof pathObject !== "object") {
            throw new TypeError(`The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`);
        }
        return _util_ts_1._format("\\", pathObject);
    }
    exports_8("format", format);
    function parse(path) {
        _util_ts_1.assertPath(path);
        const ret = { root: "", dir: "", base: "", ext: "", name: "" };
        const len = path.length;
        if (len === 0)
            return ret;
        let rootEnd = 0;
        let code = path.charCodeAt(0);
        if (len > 1) {
            if (_util_ts_1.isPathSeparator(code)) {
                rootEnd = 1;
                if (_util_ts_1.isPathSeparator(path.charCodeAt(1))) {
                    let j = 2;
                    let last = j;
                    for (; j < len; ++j) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                            break;
                    }
                    if (j < len && j !== last) {
                        last = j;
                        for (; j < len; ++j) {
                            if (!_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                break;
                        }
                        if (j < len && j !== last) {
                            last = j;
                            for (; j < len; ++j) {
                                if (_util_ts_1.isPathSeparator(path.charCodeAt(j)))
                                    break;
                            }
                            if (j === len) {
                                rootEnd = j;
                            }
                            else if (j !== last) {
                                rootEnd = j + 1;
                            }
                        }
                    }
                }
            }
            else if (_util_ts_1.isWindowsDeviceRoot(code)) {
                if (path.charCodeAt(1) === _constants_ts_2.CHAR_COLON) {
                    rootEnd = 2;
                    if (len > 2) {
                        if (_util_ts_1.isPathSeparator(path.charCodeAt(2))) {
                            if (len === 3) {
                                ret.root = ret.dir = path;
                                return ret;
                            }
                            rootEnd = 3;
                        }
                    }
                    else {
                        ret.root = ret.dir = path;
                        return ret;
                    }
                }
            }
        }
        else if (_util_ts_1.isPathSeparator(code)) {
            ret.root = ret.dir = path;
            return ret;
        }
        if (rootEnd > 0)
            ret.root = path.slice(0, rootEnd);
        let startDot = -1;
        let startPart = rootEnd;
        let end = -1;
        let matchedSlash = true;
        let i = path.length - 1;
        let preDotState = 0;
        for (; i >= rootEnd; --i) {
            code = path.charCodeAt(i);
            if (_util_ts_1.isPathSeparator(code)) {
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
            if (code === _constants_ts_2.CHAR_DOT) {
                if (startDot === -1)
                    startDot = i;
                else if (preDotState !== 1)
                    preDotState = 1;
            }
            else if (startDot !== -1) {
                preDotState = -1;
            }
        }
        if (startDot === -1 ||
            end === -1 ||
            preDotState === 0 ||
            (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)) {
            if (end !== -1) {
                ret.base = ret.name = path.slice(startPart, end);
            }
        }
        else {
            ret.name = path.slice(startPart, startDot);
            ret.base = path.slice(startPart, end);
            ret.ext = path.slice(startDot, end);
        }
        if (startPart > 0 && startPart !== rootEnd) {
            ret.dir = path.slice(0, startPart - 1);
        }
        else
            ret.dir = ret.root;
        return ret;
    }
    exports_8("parse", parse);
    function fromFileUrl(url) {
        url = url instanceof URL ? url : new URL(url);
        if (url.protocol != "file:") {
            throw new TypeError("Must be a file URL.");
        }
        let path = decodeURIComponent(url.pathname
            .replace(/^\/*([A-Za-z]:)(\/|$)/, "$1/")
            .replace(/\//g, "\\")
            .replace(/%(?![0-9A-Fa-f]{2})/g, "%25"));
        if (url.hostname != "") {
            path = `\\\\${url.hostname}${path}`;
        }
        return path;
    }
    exports_8("fromFileUrl", fromFileUrl);
    return {
        setters: [
            function (_constants_ts_2_1) {
                _constants_ts_2 = _constants_ts_2_1;
            },
            function (_util_ts_1_1) {
                _util_ts_1 = _util_ts_1_1;
            },
            function (assert_ts_1_1) {
                assert_ts_1 = assert_ts_1_1;
            }
        ],
        execute: function () {
            exports_8("sep", sep = "\\");
            exports_8("delimiter", delimiter = ";");
        }
    };
});
System.register("path/posix", ["path/_constants", "path/_util"], function (exports_9, context_9) {
    "use strict";
    var _constants_ts_3, _util_ts_2, sep, delimiter;
    var __moduleName = context_9 && context_9.id;
    function resolve(...pathSegments) {
        let resolvedPath = "";
        let resolvedAbsolute = false;
        for (let i = pathSegments.length - 1; i >= -1 && !resolvedAbsolute; i--) {
            let path;
            if (i >= 0)
                path = pathSegments[i];
            else {
                if (globalThis.Deno == null) {
                    throw new TypeError("Resolved a relative path without a CWD.");
                }
                path = Deno.cwd();
            }
            _util_ts_2.assertPath(path);
            if (path.length === 0) {
                continue;
            }
            resolvedPath = `${path}/${resolvedPath}`;
            resolvedAbsolute = path.charCodeAt(0) === _constants_ts_3.CHAR_FORWARD_SLASH;
        }
        resolvedPath = _util_ts_2.normalizeString(resolvedPath, !resolvedAbsolute, "/", _util_ts_2.isPosixPathSeparator);
        if (resolvedAbsolute) {
            if (resolvedPath.length > 0)
                return `/${resolvedPath}`;
            else
                return "/";
        }
        else if (resolvedPath.length > 0)
            return resolvedPath;
        else
            return ".";
    }
    exports_9("resolve", resolve);
    function normalize(path) {
        _util_ts_2.assertPath(path);
        if (path.length === 0)
            return ".";
        const isAbsolute = path.charCodeAt(0) === _constants_ts_3.CHAR_FORWARD_SLASH;
        const trailingSeparator = path.charCodeAt(path.length - 1) === _constants_ts_3.CHAR_FORWARD_SLASH;
        path = _util_ts_2.normalizeString(path, !isAbsolute, "/", _util_ts_2.isPosixPathSeparator);
        if (path.length === 0 && !isAbsolute)
            path = ".";
        if (path.length > 0 && trailingSeparator)
            path += "/";
        if (isAbsolute)
            return `/${path}`;
        return path;
    }
    exports_9("normalize", normalize);
    function isAbsolute(path) {
        _util_ts_2.assertPath(path);
        return path.length > 0 && path.charCodeAt(0) === _constants_ts_3.CHAR_FORWARD_SLASH;
    }
    exports_9("isAbsolute", isAbsolute);
    function join(...paths) {
        if (paths.length === 0)
            return ".";
        let joined;
        for (let i = 0, len = paths.length; i < len; ++i) {
            const path = paths[i];
            _util_ts_2.assertPath(path);
            if (path.length > 0) {
                if (!joined)
                    joined = path;
                else
                    joined += `/${path}`;
            }
        }
        if (!joined)
            return ".";
        return normalize(joined);
    }
    exports_9("join", join);
    function relative(from, to) {
        _util_ts_2.assertPath(from);
        _util_ts_2.assertPath(to);
        if (from === to)
            return "";
        from = resolve(from);
        to = resolve(to);
        if (from === to)
            return "";
        let fromStart = 1;
        const fromEnd = from.length;
        for (; fromStart < fromEnd; ++fromStart) {
            if (from.charCodeAt(fromStart) !== _constants_ts_3.CHAR_FORWARD_SLASH)
                break;
        }
        const fromLen = fromEnd - fromStart;
        let toStart = 1;
        const toEnd = to.length;
        for (; toStart < toEnd; ++toStart) {
            if (to.charCodeAt(toStart) !== _constants_ts_3.CHAR_FORWARD_SLASH)
                break;
        }
        const toLen = toEnd - toStart;
        const length = fromLen < toLen ? fromLen : toLen;
        let lastCommonSep = -1;
        let i = 0;
        for (; i <= length; ++i) {
            if (i === length) {
                if (toLen > length) {
                    if (to.charCodeAt(toStart + i) === _constants_ts_3.CHAR_FORWARD_SLASH) {
                        return to.slice(toStart + i + 1);
                    }
                    else if (i === 0) {
                        return to.slice(toStart + i);
                    }
                }
                else if (fromLen > length) {
                    if (from.charCodeAt(fromStart + i) === _constants_ts_3.CHAR_FORWARD_SLASH) {
                        lastCommonSep = i;
                    }
                    else if (i === 0) {
                        lastCommonSep = 0;
                    }
                }
                break;
            }
            const fromCode = from.charCodeAt(fromStart + i);
            const toCode = to.charCodeAt(toStart + i);
            if (fromCode !== toCode)
                break;
            else if (fromCode === _constants_ts_3.CHAR_FORWARD_SLASH)
                lastCommonSep = i;
        }
        let out = "";
        for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
            if (i === fromEnd || from.charCodeAt(i) === _constants_ts_3.CHAR_FORWARD_SLASH) {
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
            if (to.charCodeAt(toStart) === _constants_ts_3.CHAR_FORWARD_SLASH)
                ++toStart;
            return to.slice(toStart);
        }
    }
    exports_9("relative", relative);
    function toNamespacedPath(path) {
        return path;
    }
    exports_9("toNamespacedPath", toNamespacedPath);
    function dirname(path) {
        _util_ts_2.assertPath(path);
        if (path.length === 0)
            return ".";
        const hasRoot = path.charCodeAt(0) === _constants_ts_3.CHAR_FORWARD_SLASH;
        let end = -1;
        let matchedSlash = true;
        for (let i = path.length - 1; i >= 1; --i) {
            if (path.charCodeAt(i) === _constants_ts_3.CHAR_FORWARD_SLASH) {
                if (!matchedSlash) {
                    end = i;
                    break;
                }
            }
            else {
                matchedSlash = false;
            }
        }
        if (end === -1)
            return hasRoot ? "/" : ".";
        if (hasRoot && end === 1)
            return "//";
        return path.slice(0, end);
    }
    exports_9("dirname", dirname);
    function basename(path, ext = "") {
        if (ext !== undefined && typeof ext !== "string") {
            throw new TypeError('"ext" argument must be a string');
        }
        _util_ts_2.assertPath(path);
        let start = 0;
        let end = -1;
        let matchedSlash = true;
        let i;
        if (ext !== undefined && ext.length > 0 && ext.length <= path.length) {
            if (ext.length === path.length && ext === path)
                return "";
            let extIdx = ext.length - 1;
            let firstNonSlashEnd = -1;
            for (i = path.length - 1; i >= 0; --i) {
                const code = path.charCodeAt(i);
                if (code === _constants_ts_3.CHAR_FORWARD_SLASH) {
                    if (!matchedSlash) {
                        start = i + 1;
                        break;
                    }
                }
                else {
                    if (firstNonSlashEnd === -1) {
                        matchedSlash = false;
                        firstNonSlashEnd = i + 1;
                    }
                    if (extIdx >= 0) {
                        if (code === ext.charCodeAt(extIdx)) {
                            if (--extIdx === -1) {
                                end = i;
                            }
                        }
                        else {
                            extIdx = -1;
                            end = firstNonSlashEnd;
                        }
                    }
                }
            }
            if (start === end)
                end = firstNonSlashEnd;
            else if (end === -1)
                end = path.length;
            return path.slice(start, end);
        }
        else {
            for (i = path.length - 1; i >= 0; --i) {
                if (path.charCodeAt(i) === _constants_ts_3.CHAR_FORWARD_SLASH) {
                    if (!matchedSlash) {
                        start = i + 1;
                        break;
                    }
                }
                else if (end === -1) {
                    matchedSlash = false;
                    end = i + 1;
                }
            }
            if (end === -1)
                return "";
            return path.slice(start, end);
        }
    }
    exports_9("basename", basename);
    function extname(path) {
        _util_ts_2.assertPath(path);
        let startDot = -1;
        let startPart = 0;
        let end = -1;
        let matchedSlash = true;
        let preDotState = 0;
        for (let i = path.length - 1; i >= 0; --i) {
            const code = path.charCodeAt(i);
            if (code === _constants_ts_3.CHAR_FORWARD_SLASH) {
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
            if (code === _constants_ts_3.CHAR_DOT) {
                if (startDot === -1)
                    startDot = i;
                else if (preDotState !== 1)
                    preDotState = 1;
            }
            else if (startDot !== -1) {
                preDotState = -1;
            }
        }
        if (startDot === -1 ||
            end === -1 ||
            preDotState === 0 ||
            (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)) {
            return "";
        }
        return path.slice(startDot, end);
    }
    exports_9("extname", extname);
    function format(pathObject) {
        if (pathObject === null || typeof pathObject !== "object") {
            throw new TypeError(`The "pathObject" argument must be of type Object. Received type ${typeof pathObject}`);
        }
        return _util_ts_2._format("/", pathObject);
    }
    exports_9("format", format);
    function parse(path) {
        _util_ts_2.assertPath(path);
        const ret = { root: "", dir: "", base: "", ext: "", name: "" };
        if (path.length === 0)
            return ret;
        const isAbsolute = path.charCodeAt(0) === _constants_ts_3.CHAR_FORWARD_SLASH;
        let start;
        if (isAbsolute) {
            ret.root = "/";
            start = 1;
        }
        else {
            start = 0;
        }
        let startDot = -1;
        let startPart = 0;
        let end = -1;
        let matchedSlash = true;
        let i = path.length - 1;
        let preDotState = 0;
        for (; i >= start; --i) {
            const code = path.charCodeAt(i);
            if (code === _constants_ts_3.CHAR_FORWARD_SLASH) {
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
            if (code === _constants_ts_3.CHAR_DOT) {
                if (startDot === -1)
                    startDot = i;
                else if (preDotState !== 1)
                    preDotState = 1;
            }
            else if (startDot !== -1) {
                preDotState = -1;
            }
        }
        if (startDot === -1 ||
            end === -1 ||
            preDotState === 0 ||
            (preDotState === 1 && startDot === end - 1 && startDot === startPart + 1)) {
            if (end !== -1) {
                if (startPart === 0 && isAbsolute) {
                    ret.base = ret.name = path.slice(1, end);
                }
                else {
                    ret.base = ret.name = path.slice(startPart, end);
                }
            }
        }
        else {
            if (startPart === 0 && isAbsolute) {
                ret.name = path.slice(1, startDot);
                ret.base = path.slice(1, end);
            }
            else {
                ret.name = path.slice(startPart, startDot);
                ret.base = path.slice(startPart, end);
            }
            ret.ext = path.slice(startDot, end);
        }
        if (startPart > 0)
            ret.dir = path.slice(0, startPart - 1);
        else if (isAbsolute)
            ret.dir = "/";
        return ret;
    }
    exports_9("parse", parse);
    function fromFileUrl(url) {
        url = url instanceof URL ? url : new URL(url);
        if (url.protocol != "file:") {
            throw new TypeError("Must be a file URL.");
        }
        return decodeURIComponent(url.pathname.replace(/%(?![0-9A-Fa-f]{2})/g, "%25"));
    }
    exports_9("fromFileUrl", fromFileUrl);
    return {
        setters: [
            function (_constants_ts_3_1) {
                _constants_ts_3 = _constants_ts_3_1;
            },
            function (_util_ts_2_1) {
                _util_ts_2 = _util_ts_2_1;
            }
        ],
        execute: function () {
            exports_9("sep", sep = "/");
            exports_9("delimiter", delimiter = ":");
        }
    };
});
System.register("path/separator", ["path/_constants"], function (exports_10, context_10) {
    "use strict";
    var _constants_ts_4, SEP, SEP_PATTERN;
    var __moduleName = context_10 && context_10.id;
    return {
        setters: [
            function (_constants_ts_4_1) {
                _constants_ts_4 = _constants_ts_4_1;
            }
        ],
        execute: function () {
            exports_10("SEP", SEP = _constants_ts_4.isWindows ? "\\" : "/");
            exports_10("SEP_PATTERN", SEP_PATTERN = _constants_ts_4.isWindows ? /[\\/]+/ : /\/+/);
        }
    };
});
System.register("path/common", ["path/separator"], function (exports_11, context_11) {
    "use strict";
    var separator_ts_1;
    var __moduleName = context_11 && context_11.id;
    function common(paths, sep = separator_ts_1.SEP) {
        const [first = "", ...remaining] = paths;
        if (first === "" || remaining.length === 0) {
            return first.substring(0, first.lastIndexOf(sep) + 1);
        }
        const parts = first.split(sep);
        let endOfPrefix = parts.length;
        for (const path of remaining) {
            const compare = path.split(sep);
            for (let i = 0; i < endOfPrefix; i++) {
                if (compare[i] !== parts[i]) {
                    endOfPrefix = i;
                }
            }
            if (endOfPrefix === 0) {
                return "";
            }
        }
        const prefix = parts.slice(0, endOfPrefix).join(sep);
        return prefix.endsWith(sep) ? prefix : `${prefix}${sep}`;
    }
    exports_11("common", common);
    return {
        setters: [
            function (separator_ts_1_1) {
                separator_ts_1 = separator_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("path/glob", ["path/_constants", "path/mod", "path/separator"], function (exports_12, context_12) {
    "use strict";
    var _constants_ts_5, mod_ts_1, separator_ts_2, regExpEscapeChars, rangeEscapeChars;
    var __moduleName = context_12 && context_12.id;
    function globToRegExp(glob, { extended = true, globstar: globstarOption = true, os = _constants_ts_5.NATIVE_OS } = {}) {
        if (glob == "") {
            return /(?!)/;
        }
        const sep = os == "windows" ? "(?:\\\\|/)+" : "/+";
        const sepMaybe = os == "windows" ? "(?:\\\\|/)*" : "/*";
        const seps = os == "windows" ? ["\\", "/"] : ["/"];
        const globstar = os == "windows"
            ? "(?:[^\\\\/]*(?:\\\\|/|$)+)*"
            : "(?:[^/]*(?:/|$)+)*";
        const wildcard = os == "windows" ? "[^\\\\/]*" : "[^/]*";
        const escapePrefix = os == "windows" ? "`" : "\\";
        let newLength = glob.length;
        for (; newLength > 1 && seps.includes(glob[newLength - 1]); newLength--)
            ;
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
                        }
                        else if (glob[i + 1] == "^") {
                            i++;
                            segment += "\\^";
                        }
                        continue;
                    }
                    else if (glob[i + 1] == ":") {
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
                                segment += "\x00-\x7F";
                            else if (value == "blank")
                                segment += "\t ";
                            else if (value == "cntrl")
                                segment += "\x00-\x1F\x7F";
                            else if (value == "digit")
                                segment += "\\d";
                            else if (value == "graph")
                                segment += "\x21-\x7E";
                            else if (value == "lower")
                                segment += "a-z";
                            else if (value == "print")
                                segment += "\x20-\x7E";
                            else if (value == "punct") {
                                segment += "!\"#$%&'()*+,\\-./:;<=>?@[\\\\\\]^_‘{|}~";
                            }
                            else if (value == "space")
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
                    }
                    else {
                        segment += glob[i];
                    }
                    continue;
                }
                if (glob[i] == ")" && groupStack.length > 0 &&
                    groupStack[groupStack.length - 1] != "BRACE") {
                    segment += ")";
                    const type = groupStack.pop();
                    if (type == "!") {
                        segment += wildcard;
                    }
                    else if (type != "@") {
                        segment += type;
                    }
                    continue;
                }
                if (glob[i] == "|" && groupStack.length > 0 &&
                    groupStack[groupStack.length - 1] != "BRACE") {
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
                    }
                    else {
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
                    }
                    else {
                        const prevChar = glob[i - 1];
                        let numStars = 1;
                        while (glob[i + 1] == "*") {
                            i++;
                            numStars++;
                        }
                        const nextChar = glob[i + 1];
                        if (globstarOption && numStars == 2 &&
                            [...seps, undefined].includes(prevChar) &&
                            [...seps, undefined].includes(nextChar)) {
                            segment += globstar;
                            endsWithSep = true;
                        }
                        else {
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
                regExpString += i < glob.length ? sep : sepMaybe;
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
        return new RegExp(regExpString);
    }
    exports_12("globToRegExp", globToRegExp);
    function isGlob(str) {
        const chars = { "{": "}", "(": ")", "[": "]" };
        const regex = /\\(.)|(^!|\*|[\].+)]\?|\[[^\\\]]+\]|\{[^\\}]+\}|\(\?[:!=][^\\)]+\)|\([^|]+\|[^\\)]+\))/;
        if (str === "") {
            return false;
        }
        let match;
        while ((match = regex.exec(str))) {
            if (match[2])
                return true;
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
    exports_12("isGlob", isGlob);
    function normalizeGlob(glob, { globstar = false } = {}) {
        if (glob.match(/\0/g)) {
            throw new Error(`Glob contains invalid characters: "${glob}"`);
        }
        if (!globstar) {
            return mod_ts_1.normalize(glob);
        }
        const s = separator_ts_2.SEP_PATTERN.source;
        const badParentPattern = new RegExp(`(?<=(${s}|^)\\*\\*${s})\\.\\.(?=${s}|$)`, "g");
        return mod_ts_1.normalize(glob.replace(badParentPattern, "\0")).replace(/\0/g, "..");
    }
    exports_12("normalizeGlob", normalizeGlob);
    function joinGlobs(globs, { extended = false, globstar = false } = {}) {
        if (!globstar || globs.length == 0) {
            return mod_ts_1.join(...globs);
        }
        if (globs.length === 0)
            return ".";
        let joined;
        for (const glob of globs) {
            const path = glob;
            if (path.length > 0) {
                if (!joined)
                    joined = path;
                else
                    joined += `${separator_ts_2.SEP}${path}`;
            }
        }
        if (!joined)
            return ".";
        return normalizeGlob(joined, { extended, globstar });
    }
    exports_12("joinGlobs", joinGlobs);
    return {
        setters: [
            function (_constants_ts_5_1) {
                _constants_ts_5 = _constants_ts_5_1;
            },
            function (mod_ts_1_1) {
                mod_ts_1 = mod_ts_1_1;
            },
            function (separator_ts_2_1) {
                separator_ts_2 = separator_ts_2_1;
            }
        ],
        execute: function () {
            regExpEscapeChars = ["!", "$", "(", ")", "*", "+", ".", "=", "?", "[", "\\", "^", "{", "|"];
            rangeEscapeChars = ["-", "\\", "]"];
        }
    };
});
System.register("path/mod", ["path/_constants", "path/win32", "path/posix", "path/common", "path/separator", "path/_interface", "path/glob"], function (exports_13, context_13) {
    "use strict";
    var _constants_ts_6, _win32, _posix, path, win32, posix, basename, delimiter, dirname, extname, format, fromFileUrl, isAbsolute, join, normalize, parse, relative, resolve, sep, toNamespacedPath;
    var __moduleName = context_13 && context_13.id;
    var exportedNames_1 = {
        "win32": true,
        "posix": true,
        "basename": true,
        "delimiter": true,
        "dirname": true,
        "extname": true,
        "format": true,
        "fromFileUrl": true,
        "isAbsolute": true,
        "join": true,
        "normalize": true,
        "parse": true,
        "relative": true,
        "resolve": true,
        "sep": true,
        "toNamespacedPath": true,
        "SEP": true,
        "SEP_PATTERN": true
    };
    function exportStar_1(m) {
        var exports = {};
        for (var n in m) {
            if (n !== "default" && !exportedNames_1.hasOwnProperty(n)) exports[n] = m[n];
        }
        exports_13(exports);
    }
    return {
        setters: [
            function (_constants_ts_6_1) {
                _constants_ts_6 = _constants_ts_6_1;
            },
            function (_win32_1) {
                _win32 = _win32_1;
            },
            function (_posix_1) {
                _posix = _posix_1;
            },
            function (common_ts_1_1) {
                exportStar_1(common_ts_1_1);
            },
            function (separator_ts_3_1) {
                exports_13({
                    "SEP": separator_ts_3_1["SEP"],
                    "SEP_PATTERN": separator_ts_3_1["SEP_PATTERN"]
                });
            },
            function (_interface_ts_1_1) {
                exportStar_1(_interface_ts_1_1);
            },
            function (glob_ts_1_1) {
                exportStar_1(glob_ts_1_1);
            }
        ],
        execute: function () {
            path = _constants_ts_6.isWindows ? _win32 : _posix;
            exports_13("win32", win32 = _win32);
            exports_13("posix", posix = _posix);
            exports_13("basename", basename = path.basename), exports_13("delimiter", delimiter = path.delimiter), exports_13("dirname", dirname = path.dirname), exports_13("extname", extname = path.extname), exports_13("format", format = path.format), exports_13("fromFileUrl", fromFileUrl = path.fromFileUrl), exports_13("isAbsolute", isAbsolute = path.isAbsolute), exports_13("join", join = path.join), exports_13("normalize", normalize = path.normalize), exports_13("parse", parse = path.parse), exports_13("relative", relative = path.relative), exports_13("resolve", resolve = path.resolve), exports_13("sep", sep = path.sep), exports_13("toNamespacedPath", toNamespacedPath = path.toNamespacedPath);
        }
    };
});
System.register("node/path", ["path/mod"], function (exports_14, context_14) {
    "use strict";
    var __moduleName = context_14 && context_14.id;
    function exportStar_2(m) {
        var exports = {};
        for (var n in m) {
            if (n !== "default") exports[n] = m[n];
        }
        exports_14(exports);
    }
    return {
        setters: [
            function (mod_ts_2_1) {
                exportStar_2(mod_ts_2_1);
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_appendFile", ["node/_fs/_fs_common", "node/_utils", "node/path"], function (exports_15, context_15) {
    "use strict";
    var _fs_common_ts_1, _utils_ts_3, path_ts_1;
    var __moduleName = context_15 && context_15.id;
    function appendFile(pathOrRid, data, optionsOrCallback, callback) {
        pathOrRid = pathOrRid instanceof URL ? path_ts_1.fromFileUrl(pathOrRid) : pathOrRid;
        const callbackFn = optionsOrCallback instanceof Function ? optionsOrCallback : callback;
        const options = optionsOrCallback instanceof Function ? undefined : optionsOrCallback;
        if (!callbackFn) {
            throw new Error("No callback function supplied");
        }
        validateEncoding(options);
        let rid = -1;
        const buffer = data instanceof Uint8Array
            ? data
            : new TextEncoder().encode(data);
        new Promise((resolve, reject) => {
            if (typeof pathOrRid === "number") {
                rid = pathOrRid;
                Deno.write(rid, buffer).then(resolve).catch(reject);
            }
            else {
                const mode = _fs_common_ts_1.isFileOptions(options)
                    ? options.mode
                    : undefined;
                const flag = _fs_common_ts_1.isFileOptions(options)
                    ? options.flag
                    : undefined;
                if (mode) {
                    _utils_ts_3.notImplemented("Deno does not yet support setting mode on create");
                }
                Deno.open(pathOrRid, _fs_common_ts_1.getOpenOptions(flag))
                    .then(({ rid: openedFileRid }) => {
                    rid = openedFileRid;
                    return Deno.write(openedFileRid, buffer);
                })
                    .then(resolve)
                    .catch(reject);
            }
        })
            .then(() => {
            closeRidIfNecessary(typeof pathOrRid === "string", rid);
            callbackFn();
        })
            .catch((err) => {
            closeRidIfNecessary(typeof pathOrRid === "string", rid);
            callbackFn(err);
        });
    }
    exports_15("appendFile", appendFile);
    function closeRidIfNecessary(isPathString, rid) {
        if (isPathString && rid != -1) {
            Deno.close(rid);
        }
    }
    function appendFileSync(pathOrRid, data, options) {
        let rid = -1;
        validateEncoding(options);
        pathOrRid = pathOrRid instanceof URL ? path_ts_1.fromFileUrl(pathOrRid) : pathOrRid;
        try {
            if (typeof pathOrRid === "number") {
                rid = pathOrRid;
            }
            else {
                const mode = _fs_common_ts_1.isFileOptions(options)
                    ? options.mode
                    : undefined;
                const flag = _fs_common_ts_1.isFileOptions(options)
                    ? options.flag
                    : undefined;
                if (mode) {
                    _utils_ts_3.notImplemented("Deno does not yet support setting mode on create");
                }
                const file = Deno.openSync(pathOrRid, _fs_common_ts_1.getOpenOptions(flag));
                rid = file.rid;
            }
            const buffer = data instanceof Uint8Array
                ? data
                : new TextEncoder().encode(data);
            Deno.writeSync(rid, buffer);
        }
        finally {
            closeRidIfNecessary(typeof pathOrRid === "string", rid);
        }
    }
    exports_15("appendFileSync", appendFileSync);
    function validateEncoding(encodingOption) {
        if (!encodingOption)
            return;
        if (typeof encodingOption === "string") {
            if (encodingOption !== "utf8") {
                throw new Error("Only 'utf8' encoding is currently supported");
            }
        }
        else if (encodingOption.encoding && encodingOption.encoding !== "utf8") {
            throw new Error("Only 'utf8' encoding is currently supported");
        }
    }
    return {
        setters: [
            function (_fs_common_ts_1_1) {
                _fs_common_ts_1 = _fs_common_ts_1_1;
            },
            function (_utils_ts_3_1) {
                _utils_ts_3 = _utils_ts_3_1;
            },
            function (path_ts_1_1) {
                path_ts_1 = path_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_chmod", ["node/path"], function (exports_16, context_16) {
    "use strict";
    var path_ts_2, allowedModes;
    var __moduleName = context_16 && context_16.id;
    function chmod(path, mode, callback) {
        path = path instanceof URL ? path_ts_2.fromFileUrl(path) : path;
        Deno.chmod(path, getResolvedMode(mode))
            .then(() => callback())
            .catch(callback);
    }
    exports_16("chmod", chmod);
    function chmodSync(path, mode) {
        path = path instanceof URL ? path_ts_2.fromFileUrl(path) : path;
        Deno.chmodSync(path, getResolvedMode(mode));
    }
    exports_16("chmodSync", chmodSync);
    function getResolvedMode(mode) {
        if (typeof mode === "number") {
            return mode;
        }
        if (typeof mode === "string" && !allowedModes.test(mode)) {
            throw new Error("Unrecognized mode: " + mode);
        }
        return parseInt(mode, 8);
    }
    return {
        setters: [
            function (path_ts_2_1) {
                path_ts_2 = path_ts_2_1;
            }
        ],
        execute: function () {
            allowedModes = /^[0-7]{3}/;
        }
    };
});
System.register("node/_fs/_fs_chown", ["node/path"], function (exports_17, context_17) {
    "use strict";
    var path_ts_3;
    var __moduleName = context_17 && context_17.id;
    function chown(path, uid, gid, callback) {
        path = path instanceof URL ? path_ts_3.fromFileUrl(path) : path;
        Deno.chown(path, uid, gid)
            .then(() => callback())
            .catch(callback);
    }
    exports_17("chown", chown);
    function chownSync(path, uid, gid) {
        path = path instanceof URL ? path_ts_3.fromFileUrl(path) : path;
        Deno.chownSync(path, uid, gid);
    }
    exports_17("chownSync", chownSync);
    return {
        setters: [
            function (path_ts_3_1) {
                path_ts_3 = path_ts_3_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_close", [], function (exports_18, context_18) {
    "use strict";
    var __moduleName = context_18 && context_18.id;
    function close(fd, callback) {
        queueMicrotask(() => {
            try {
                Deno.close(fd);
                callback(null);
            }
            catch (err) {
                callback(err);
            }
        });
    }
    exports_18("close", close);
    function closeSync(fd) {
        Deno.close(fd);
    }
    exports_18("closeSync", closeSync);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_constants", [], function (exports_19, context_19) {
    "use strict";
    var F_OK, R_OK, W_OK, X_OK, S_IRUSR, S_IWUSR, S_IXUSR, S_IRGRP, S_IWGRP, S_IXGRP, S_IROTH, S_IWOTH, S_IXOTH;
    var __moduleName = context_19 && context_19.id;
    return {
        setters: [],
        execute: function () {
            exports_19("F_OK", F_OK = 0);
            exports_19("R_OK", R_OK = 4);
            exports_19("W_OK", W_OK = 2);
            exports_19("X_OK", X_OK = 1);
            exports_19("S_IRUSR", S_IRUSR = 0o400);
            exports_19("S_IWUSR", S_IWUSR = 0o200);
            exports_19("S_IXUSR", S_IXUSR = 0o100);
            exports_19("S_IRGRP", S_IRGRP = 0o40);
            exports_19("S_IWGRP", S_IWGRP = 0o20);
            exports_19("S_IXGRP", S_IXGRP = 0o10);
            exports_19("S_IROTH", S_IROTH = 0o4);
            exports_19("S_IWOTH", S_IWOTH = 0o2);
            exports_19("S_IXOTH", S_IXOTH = 0o1);
        }
    };
});
System.register("encoding/hex", [], function (exports_20, context_20) {
    "use strict";
    var hextable;
    var __moduleName = context_20 && context_20.id;
    function errInvalidByte(byte) {
        return new Error("encoding/hex: invalid byte: " +
            new TextDecoder().decode(new Uint8Array([byte])));
    }
    exports_20("errInvalidByte", errInvalidByte);
    function errLength() {
        return new Error("encoding/hex: odd length hex string");
    }
    exports_20("errLength", errLength);
    function fromHexChar(byte) {
        if (48 <= byte && byte <= 57)
            return byte - 48;
        if (97 <= byte && byte <= 102)
            return byte - 97 + 10;
        if (65 <= byte && byte <= 70)
            return byte - 65 + 10;
        throw errInvalidByte(byte);
    }
    function encodedLen(n) {
        return n * 2;
    }
    exports_20("encodedLen", encodedLen);
    function encode(src) {
        const dst = new Uint8Array(encodedLen(src.length));
        for (let i = 0; i < dst.length; i++) {
            const v = src[i];
            dst[i * 2] = hextable[v >> 4];
            dst[i * 2 + 1] = hextable[v & 0x0f];
        }
        return dst;
    }
    exports_20("encode", encode);
    function encodeToString(src) {
        return new TextDecoder().decode(encode(src));
    }
    exports_20("encodeToString", encodeToString);
    function decode(src) {
        const dst = new Uint8Array(decodedLen(src.length));
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
    exports_20("decode", decode);
    function decodedLen(x) {
        return x >>> 1;
    }
    exports_20("decodedLen", decodedLen);
    function decodeString(s) {
        return decode(new TextEncoder().encode(s));
    }
    exports_20("decodeString", decodeString);
    return {
        setters: [],
        execute: function () {
            hextable = new TextEncoder().encode("0123456789abcdef");
        }
    };
});
System.register("encoding/base64", [], function (exports_21, context_21) {
    "use strict";
    var __moduleName = context_21 && context_21.id;
    function encode(data) {
        if (typeof data === "string") {
            return btoa(data);
        }
        else {
            const d = new Uint8Array(data);
            let dataString = "";
            for (let i = 0; i < d.length; ++i) {
                dataString += String.fromCharCode(d[i]);
            }
            return btoa(dataString);
        }
    }
    exports_21("encode", encode);
    function decode(data) {
        const binaryString = decodeString(data);
        const binary = new Uint8Array(binaryString.length);
        for (let i = 0; i < binary.length; ++i) {
            binary[i] = binaryString.charCodeAt(i);
        }
        return binary.buffer;
    }
    exports_21("decode", decode);
    function decodeString(data) {
        return atob(data);
    }
    exports_21("decodeString", decodeString);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("node/buffer", ["encoding/hex", "encoding/base64", "node/_utils"], function (exports_22, context_22) {
    "use strict";
    var hex, base64, _utils_ts_4, notImplementedEncodings, encodingOps, Buffer;
    var __moduleName = context_22 && context_22.id;
    function checkEncoding(encoding = "utf8", strict = true) {
        if (typeof encoding !== "string" || (strict && encoding === "")) {
            if (!strict)
                return "utf8";
            throw new TypeError(`Unkown encoding: ${encoding}`);
        }
        const normalized = _utils_ts_4.normalizeEncoding(encoding);
        if (normalized === undefined) {
            throw new TypeError(`Unkown encoding: ${encoding}`);
        }
        if (notImplementedEncodings.includes(encoding)) {
            _utils_ts_4.notImplemented(`"${encoding}" encoding`);
        }
        return normalized;
    }
    function base64ByteLength(str, bytes) {
        if (str.charCodeAt(bytes - 1) === 0x3d)
            bytes--;
        if (bytes > 1 && str.charCodeAt(bytes - 1) === 0x3d)
            bytes--;
        return (bytes * 3) >>> 2;
    }
    return {
        setters: [
            function (hex_1) {
                hex = hex_1;
            },
            function (base64_1) {
                base64 = base64_1;
            },
            function (_utils_ts_4_1) {
                _utils_ts_4 = _utils_ts_4_1;
            }
        ],
        execute: function () {
            notImplementedEncodings = [
                "ascii",
                "binary",
                "latin1",
                "ucs2",
                "utf16le",
            ];
            encodingOps = {
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
            Buffer = class Buffer extends Uint8Array {
                static alloc(size, fill, encoding = "utf8") {
                    if (typeof size !== "number") {
                        throw new TypeError(`The "size" argument must be of type number. Received type ${typeof size}`);
                    }
                    const buf = new Buffer(size);
                    if (size === 0)
                        return buf;
                    let bufFill;
                    if (typeof fill === "string") {
                        encoding = checkEncoding(encoding);
                        if (typeof fill === "string" &&
                            fill.length === 1 &&
                            encoding === "utf8") {
                            buf.fill(fill.charCodeAt(0));
                        }
                        else
                            bufFill = Buffer.from(fill, encoding);
                    }
                    else if (typeof fill === "number") {
                        buf.fill(fill);
                    }
                    else if (fill instanceof Uint8Array) {
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
                    return new Buffer(size);
                }
                static byteLength(string, encoding = "utf8") {
                    if (typeof string != "string")
                        return string.byteLength;
                    encoding = _utils_ts_4.normalizeEncoding(encoding) || "utf8";
                    return encodingOps[encoding].byteLength(string);
                }
                static concat(list, totalLength) {
                    if (totalLength == undefined) {
                        totalLength = 0;
                        for (const buf of list) {
                            totalLength += buf.length;
                        }
                    }
                    const buffer = new Buffer(totalLength);
                    let pos = 0;
                    for (const buf of list) {
                        buffer.set(buf, pos);
                        pos += buf.length;
                    }
                    return buffer;
                }
                static from(value, offsetOrEncoding, length) {
                    const offset = typeof offsetOrEncoding === "string"
                        ? undefined
                        : offsetOrEncoding;
                    let encoding = typeof offsetOrEncoding === "string"
                        ? offsetOrEncoding
                        : undefined;
                    if (typeof value == "string") {
                        encoding = checkEncoding(encoding, false);
                        if (encoding === "hex")
                            return new Buffer(hex.decodeString(value).buffer);
                        if (encoding === "base64")
                            return new Buffer(base64.decode(value));
                        return new Buffer(new TextEncoder().encode(value).buffer);
                    }
                    return new Buffer(value, offset, length);
                }
                static isBuffer(obj) {
                    return obj instanceof Buffer;
                }
                static isEncoding(encoding) {
                    return (typeof encoding === "string" &&
                        encoding.length !== 0 &&
                        _utils_ts_4.normalizeEncoding(encoding) !== undefined);
                }
                copy(targetBuffer, targetStart = 0, sourceStart = 0, sourceEnd = this.length) {
                    const sourceBuffer = this.subarray(sourceStart, sourceEnd);
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
                    encoding = checkEncoding(encoding);
                    const b = this.subarray(start, end);
                    if (encoding === "hex")
                        return hex.encodeToString(b);
                    if (encoding === "base64")
                        return base64.encode(b.buffer);
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
            exports_22("default", Buffer);
            exports_22("Buffer", Buffer);
            Object.defineProperty(globalThis, "Buffer", {
                value: Buffer,
                enumerable: false,
                writable: true,
                configurable: true,
            });
        }
    };
});
System.register("node/_fs/_fs_readFile", ["node/_fs/_fs_common", "node/buffer", "node/path"], function (exports_23, context_23) {
    "use strict";
    var _fs_common_ts_2, buffer_ts_1, path_ts_4;
    var __moduleName = context_23 && context_23.id;
    function maybeDecode(data, encoding) {
        const buffer = new buffer_ts_1.Buffer(data.buffer, data.byteOffset, data.byteLength);
        if (encoding && encoding !== "binary")
            return buffer.toString(encoding);
        return buffer;
    }
    function readFile(path, optOrCallback, callback) {
        path = path instanceof URL ? path_ts_4.fromFileUrl(path) : path;
        let cb;
        if (typeof optOrCallback === "function") {
            cb = optOrCallback;
        }
        else {
            cb = callback;
        }
        const encoding = _fs_common_ts_2.getEncoding(optOrCallback);
        const p = Deno.readFile(path);
        if (cb) {
            p.then((data) => {
                if (encoding && encoding !== "binary") {
                    const text = maybeDecode(data, encoding);
                    return cb(null, text);
                }
                const buffer = maybeDecode(data, encoding);
                cb(null, buffer);
            }).catch((err) => cb && cb(err));
        }
    }
    exports_23("readFile", readFile);
    function readFileSync(path, opt) {
        path = path instanceof URL ? path_ts_4.fromFileUrl(path) : path;
        const data = Deno.readFileSync(path);
        const encoding = _fs_common_ts_2.getEncoding(opt);
        if (encoding && encoding !== "binary") {
            const text = maybeDecode(data, encoding);
            return text;
        }
        const buffer = maybeDecode(data, encoding);
        return buffer;
    }
    exports_23("readFileSync", readFileSync);
    return {
        setters: [
            function (_fs_common_ts_2_1) {
                _fs_common_ts_2 = _fs_common_ts_2_1;
            },
            function (buffer_ts_1_1) {
                buffer_ts_1 = buffer_ts_1_1;
            },
            function (path_ts_4_1) {
                path_ts_4 = path_ts_4_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_readlink", ["node/_utils", "node/path"], function (exports_24, context_24) {
    "use strict";
    var _utils_ts_5, path_ts_5;
    var __moduleName = context_24 && context_24.id;
    function maybeEncode(data, encoding) {
        if (encoding === "buffer") {
            return new TextEncoder().encode(data);
        }
        return data;
    }
    function getEncoding(optOrCallback) {
        if (!optOrCallback || typeof optOrCallback === "function") {
            return null;
        }
        else {
            if (optOrCallback.encoding) {
                if (optOrCallback.encoding === "utf8" ||
                    optOrCallback.encoding === "utf-8") {
                    return "utf8";
                }
                else if (optOrCallback.encoding === "buffer") {
                    return "buffer";
                }
                else {
                    _utils_ts_5.notImplemented();
                }
            }
            return null;
        }
    }
    function readlink(path, optOrCallback, callback) {
        path = path instanceof URL ? path_ts_5.fromFileUrl(path) : path;
        let cb;
        if (typeof optOrCallback === "function") {
            cb = optOrCallback;
        }
        else {
            cb = callback;
        }
        const encoding = getEncoding(optOrCallback);
        _utils_ts_5.intoCallbackAPIWithIntercept(Deno.readLink, (data) => maybeEncode(data, encoding), cb, path);
    }
    exports_24("readlink", readlink);
    function readlinkSync(path, opt) {
        path = path instanceof URL ? path_ts_5.fromFileUrl(path) : path;
        return maybeEncode(Deno.readLinkSync(path), getEncoding(opt));
    }
    exports_24("readlinkSync", readlinkSync);
    return {
        setters: [
            function (_utils_ts_5_1) {
                _utils_ts_5 = _utils_ts_5_1;
            },
            function (path_ts_5_1) {
                path_ts_5 = path_ts_5_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_exists", ["node/path"], function (exports_25, context_25) {
    "use strict";
    var path_ts_6;
    var __moduleName = context_25 && context_25.id;
    function exists(path, callback) {
        path = path instanceof URL ? path_ts_6.fromFileUrl(path) : path;
        Deno.lstat(path)
            .then(() => {
            callback(true);
        })
            .catch(() => callback(false));
    }
    exports_25("exists", exists);
    function existsSync(path) {
        path = path instanceof URL ? path_ts_6.fromFileUrl(path) : path;
        try {
            Deno.lstatSync(path);
            return true;
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                return false;
            }
            throw err;
        }
    }
    exports_25("existsSync", existsSync);
    return {
        setters: [
            function (path_ts_6_1) {
                path_ts_6 = path_ts_6_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_mkdir", ["node/path"], function (exports_26, context_26) {
    "use strict";
    var path_ts_7;
    var __moduleName = context_26 && context_26.id;
    function mkdir(path, options, callback) {
        path = path instanceof URL ? path_ts_7.fromFileUrl(path) : path;
        let mode = 0o777;
        let recursive = false;
        if (typeof options == "function") {
            callback = options;
        }
        else if (typeof options === "number") {
            mode = options;
        }
        else if (typeof options === "boolean") {
            recursive = options;
        }
        else if (options) {
            if (options.recursive !== undefined)
                recursive = options.recursive;
            if (options.mode !== undefined)
                mode = options.mode;
        }
        if (typeof recursive !== "boolean") {
            throw new Deno.errors.InvalidData("invalid recursive option , must be a boolean");
        }
        Deno.mkdir(path, { recursive, mode })
            .then(() => {
            if (typeof callback === "function") {
                callback();
            }
        })
            .catch((err) => {
            if (typeof callback === "function") {
                callback(err);
            }
        });
    }
    exports_26("mkdir", mkdir);
    function mkdirSync(path, options) {
        path = path instanceof URL ? path_ts_7.fromFileUrl(path) : path;
        let mode = 0o777;
        let recursive = false;
        if (typeof options === "number") {
            mode = options;
        }
        else if (typeof options === "boolean") {
            recursive = options;
        }
        else if (options) {
            if (options.recursive !== undefined)
                recursive = options.recursive;
            if (options.mode !== undefined)
                mode = options.mode;
        }
        if (typeof recursive !== "boolean") {
            throw new Deno.errors.InvalidData("invalid recursive option , must be a boolean");
        }
        Deno.mkdirSync(path, { recursive, mode });
    }
    exports_26("mkdirSync", mkdirSync);
    return {
        setters: [
            function (path_ts_7_1) {
                path_ts_7 = path_ts_7_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_copy", ["node/path"], function (exports_27, context_27) {
    "use strict";
    var path_ts_8;
    var __moduleName = context_27 && context_27.id;
    function copyFile(source, destination, callback) {
        source = source instanceof URL ? path_ts_8.fromFileUrl(source) : source;
        Deno.copyFile(source, destination)
            .then(() => callback())
            .catch(callback);
    }
    exports_27("copyFile", copyFile);
    function copyFileSync(source, destination) {
        source = source instanceof URL ? path_ts_8.fromFileUrl(source) : source;
        Deno.copyFileSync(source, destination);
    }
    exports_27("copyFileSync", copyFileSync);
    return {
        setters: [
            function (path_ts_8_1) {
                path_ts_8 = path_ts_8_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_writeFile", ["node/_utils", "node/path", "node/buffer", "node/_fs/_fs_common"], function (exports_28, context_28) {
    "use strict";
    var _utils_ts_6, path_ts_9, buffer_ts_2, _fs_common_ts_3;
    var __moduleName = context_28 && context_28.id;
    function writeFile(pathOrRid, data, optOrCallback, callback) {
        const callbackFn = optOrCallback instanceof Function ? optOrCallback : callback;
        const options = optOrCallback instanceof Function ? undefined : optOrCallback;
        if (!callbackFn) {
            throw new TypeError("Callback must be a function.");
        }
        pathOrRid = pathOrRid instanceof URL ? path_ts_9.fromFileUrl(pathOrRid) : pathOrRid;
        const flag = _fs_common_ts_3.isFileOptions(options)
            ? options.flag
            : undefined;
        const mode = _fs_common_ts_3.isFileOptions(options)
            ? options.mode
            : undefined;
        const encoding = _fs_common_ts_3.checkEncoding(_fs_common_ts_3.getEncoding(options)) || "utf8";
        const openOptions = _fs_common_ts_3.getOpenOptions(flag || "w");
        if (typeof data === "string")
            data = buffer_ts_2.Buffer.from(data, encoding);
        const isRid = typeof pathOrRid === "number";
        let file;
        let error = null;
        (async () => {
            try {
                file = isRid
                    ? new Deno.File(pathOrRid)
                    : await Deno.open(pathOrRid, openOptions);
                if (!isRid && mode) {
                    if (Deno.build.os === "windows")
                        _utils_ts_6.notImplemented(`"mode" on Windows`);
                    await Deno.chmod(pathOrRid, mode);
                }
                await Deno.writeAll(file, data);
            }
            catch (e) {
                error = e;
            }
            finally {
                if (!isRid && file)
                    file.close();
                callbackFn(error);
            }
        })();
    }
    exports_28("writeFile", writeFile);
    function writeFileSync(pathOrRid, data, options) {
        pathOrRid = pathOrRid instanceof URL ? path_ts_9.fromFileUrl(pathOrRid) : pathOrRid;
        const flag = _fs_common_ts_3.isFileOptions(options)
            ? options.flag
            : undefined;
        const mode = _fs_common_ts_3.isFileOptions(options)
            ? options.mode
            : undefined;
        const encoding = _fs_common_ts_3.checkEncoding(_fs_common_ts_3.getEncoding(options)) || "utf8";
        const openOptions = _fs_common_ts_3.getOpenOptions(flag || "w");
        if (typeof data === "string")
            data = buffer_ts_2.Buffer.from(data, encoding);
        const isRid = typeof pathOrRid === "number";
        let file;
        let error = null;
        try {
            file = isRid
                ? new Deno.File(pathOrRid)
                : Deno.openSync(pathOrRid, openOptions);
            if (!isRid && mode) {
                if (Deno.build.os === "windows")
                    _utils_ts_6.notImplemented(`"mode" on Windows`);
                Deno.chmodSync(pathOrRid, mode);
            }
            Deno.writeAllSync(file, data);
        }
        catch (e) {
            error = e;
        }
        finally {
            if (!isRid && file)
                file.close();
            if (error)
                throw error;
        }
    }
    exports_28("writeFileSync", writeFileSync);
    return {
        setters: [
            function (_utils_ts_6_1) {
                _utils_ts_6 = _utils_ts_6_1;
            },
            function (path_ts_9_1) {
                path_ts_9 = path_ts_9_1;
            },
            function (buffer_ts_2_1) {
                buffer_ts_2 = buffer_ts_2_1;
            },
            function (_fs_common_ts_3_1) {
                _fs_common_ts_3 = _fs_common_ts_3_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/events", ["node/_utils", "_util/assert"], function (exports_29, context_29) {
    "use strict";
    var _utils_ts_7, assert_ts_2, EventEmitter, captureRejectionSymbol;
    var __moduleName = context_29 && context_29.id;
    function once(emitter, name) {
        return new Promise((resolve, reject) => {
            if (emitter instanceof EventTarget) {
                emitter.addEventListener(name, (...args) => {
                    resolve(args);
                }, { once: true, passive: false, capture: false });
                return;
            }
            else if (emitter instanceof EventEmitter) {
                const eventListener = (...args) => {
                    if (errorListener !== undefined) {
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
    exports_29("once", once);
    function createIterResult(value, done) {
        return { value, done };
    }
    function on(emitter, event) {
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
                    return Promise.resolve(createIterResult(undefined, true));
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
                    promise.resolve(createIterResult(undefined, true));
                }
                return Promise.resolve(createIterResult(undefined, true));
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
            }
            else {
                unconsumedEventValues.push(args);
            }
        }
        function errorHandler(err) {
            finished = true;
            const toError = unconsumedPromises.shift();
            if (toError) {
                toError.reject(err);
            }
            else {
                error = err;
            }
            iterator.return();
        }
    }
    exports_29("on", on);
    return {
        setters: [
            function (_utils_ts_7_1) {
                _utils_ts_7 = _utils_ts_7_1;
            },
            function (assert_ts_2_1) {
                assert_ts_2 = assert_ts_2_1;
            }
        ],
        execute: function () {
            EventEmitter = class EventEmitter {
                constructor() {
                    this._events = new Map();
                }
                _addListener(eventName, listener, prepend) {
                    this.emit("newListener", eventName, listener);
                    if (this._events.has(eventName)) {
                        const listeners = this._events.get(eventName);
                        if (prepend) {
                            listeners.unshift(listener);
                        }
                        else {
                            listeners.push(listener);
                        }
                    }
                    else {
                        this._events.set(eventName, [listener]);
                    }
                    const max = this.getMaxListeners();
                    if (max > 0 && this.listenerCount(eventName) > max) {
                        const warning = new Error(`Possible EventEmitter memory leak detected.
         ${this.listenerCount(eventName)} ${eventName.toString()} listeners.
         Use emitter.setMaxListeners() to increase limit`);
                        warning.name = "MaxListenersExceededWarning";
                        console.warn(warning);
                    }
                    return this;
                }
                addListener(eventName, listener) {
                    return this._addListener(eventName, listener, false);
                }
                emit(eventName, ...args) {
                    if (this._events.has(eventName)) {
                        if (eventName === "error" &&
                            this._events.get(EventEmitter.errorMonitor)) {
                            this.emit(EventEmitter.errorMonitor, ...args);
                        }
                        const listeners = this._events.get(eventName).slice();
                        for (const listener of listeners) {
                            try {
                                listener.apply(this, args);
                            }
                            catch (err) {
                                this.emit("error", err);
                            }
                        }
                        return true;
                    }
                    else if (eventName === "error") {
                        if (this._events.get(EventEmitter.errorMonitor)) {
                            this.emit(EventEmitter.errorMonitor, ...args);
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
                    return this.maxListeners || EventEmitter.defaultMaxListeners;
                }
                listenerCount(eventName) {
                    if (this._events.has(eventName)) {
                        return this._events.get(eventName).length;
                    }
                    else {
                        return 0;
                    }
                }
                _listeners(target, eventName, unwrap) {
                    if (!target._events.has(eventName)) {
                        return [];
                    }
                    const eventListeners = target._events.get(eventName);
                    return unwrap
                        ? this.unwrapListeners(eventListeners)
                        : eventListeners.slice(0);
                }
                unwrapListeners(arr) {
                    const unwrappedListeners = new Array(arr.length);
                    for (let i = 0; i < arr.length; i++) {
                        unwrappedListeners[i] = arr[i]["listener"] || arr[i];
                    }
                    return unwrappedListeners;
                }
                listeners(eventName) {
                    return this._listeners(this, eventName, true);
                }
                rawListeners(eventName) {
                    return this._listeners(this, eventName, false);
                }
                off(eventName, listener) {
                    return this.removeListener(eventName, listener);
                }
                on(eventName, listener) {
                    return this.addListener(eventName, listener);
                }
                once(eventName, listener) {
                    const wrapped = this.onceWrap(eventName, listener);
                    this.on(eventName, wrapped);
                    return this;
                }
                onceWrap(eventName, listener) {
                    const wrapper = function (...args) {
                        this.context.removeListener(this.eventName, this.rawListener);
                        this.listener.apply(this.context, args);
                    };
                    const wrapperContext = {
                        eventName: eventName,
                        listener: listener,
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
                    if (this._events === undefined) {
                        return this;
                    }
                    if (eventName) {
                        if (this._events.has(eventName)) {
                            const listeners = this._events.get(eventName).slice();
                            this._events.delete(eventName);
                            for (const listener of listeners) {
                                this.emit("removeListener", eventName, listener);
                            }
                        }
                    }
                    else {
                        const eventList = this.eventNames();
                        eventList.map((value) => {
                            this.removeAllListeners(value);
                        });
                    }
                    return this;
                }
                removeListener(eventName, listener) {
                    if (this._events.has(eventName)) {
                        const arr = this._events.get(eventName);
                        assert_ts_2.assert(arr);
                        let listenerIndex = -1;
                        for (let i = arr.length - 1; i >= 0; i--) {
                            if (arr[i] == listener ||
                                (arr[i] && arr[i]["listener"] == listener)) {
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
                        if (n === 0) {
                            n = Infinity;
                        }
                        else {
                            _utils_ts_7.validateIntegerRange(n, "maxListeners", 0);
                        }
                    }
                    this.maxListeners = n;
                    return this;
                }
            };
            exports_29("default", EventEmitter);
            exports_29("EventEmitter", EventEmitter);
            EventEmitter.defaultMaxListeners = 10;
            EventEmitter.errorMonitor = Symbol("events.errorMonitor");
            exports_29("captureRejectionSymbol", captureRejectionSymbol = Symbol.for("nodejs.rejection"));
        }
    };
});
System.register("node/_fs/_fs_watch", ["node/path", "node/events", "node/_utils"], function (exports_30, context_30) {
    "use strict";
    var path_ts_10, events_ts_1, _utils_ts_8, FSWatcher;
    var __moduleName = context_30 && context_30.id;
    function asyncIterableIteratorToCallback(iterator, callback) {
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
    exports_30("asyncIterableIteratorToCallback", asyncIterableIteratorToCallback);
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
    exports_30("asyncIterableToCallback", asyncIterableToCallback);
    function watch(filename, optionsOrListener, optionsOrListener2) {
        const listener = typeof optionsOrListener === "function"
            ? optionsOrListener
            : typeof optionsOrListener2 === "function"
                ? optionsOrListener2
                : undefined;
        const options = typeof optionsOrListener === "object"
            ? optionsOrListener
            : typeof optionsOrListener2 === "object"
                ? optionsOrListener2
                : undefined;
        filename = filename instanceof URL ? path_ts_10.fromFileUrl(filename) : filename;
        const iterator = Deno.watchFs(filename, {
            recursive: options?.recursive || false,
        });
        if (!listener)
            throw new Error("No callback function supplied");
        const fsWatcher = new FSWatcher(() => {
            if (iterator.return)
                iterator.return();
        });
        fsWatcher.on("change", listener);
        asyncIterableIteratorToCallback(iterator, (val, done) => {
            if (done)
                return;
            fsWatcher.emit("change", val.kind, val.paths[0]);
        });
        return fsWatcher;
    }
    exports_30("watch", watch);
    return {
        setters: [
            function (path_ts_10_1) {
                path_ts_10 = path_ts_10_1;
            },
            function (events_ts_1_1) {
                events_ts_1 = events_ts_1_1;
            },
            function (_utils_ts_8_1) {
                _utils_ts_8 = _utils_ts_8_1;
            }
        ],
        execute: function () {
            FSWatcher = class FSWatcher extends events_ts_1.EventEmitter {
                constructor(closer) {
                    super();
                    this.close = closer;
                }
                ref() {
                    _utils_ts_8.notImplemented("FSWatcher.ref() is not implemented");
                }
                unref() {
                    _utils_ts_8.notImplemented("FSWatcher.unref() is not implemented");
                }
            };
        }
    };
});
System.register("node/_fs/_fs_dirent", ["node/_utils"], function (exports_31, context_31) {
    "use strict";
    var _utils_ts_9, Dirent;
    var __moduleName = context_31 && context_31.id;
    return {
        setters: [
            function (_utils_ts_9_1) {
                _utils_ts_9 = _utils_ts_9_1;
            }
        ],
        execute: function () {
            Dirent = class Dirent {
                constructor(entry) {
                    this.entry = entry;
                }
                isBlockDevice() {
                    _utils_ts_9.notImplemented("Deno does not yet support identification of block devices");
                    return false;
                }
                isCharacterDevice() {
                    _utils_ts_9.notImplemented("Deno does not yet support identification of character devices");
                    return false;
                }
                isDirectory() {
                    return this.entry.isDirectory;
                }
                isFIFO() {
                    _utils_ts_9.notImplemented("Deno does not yet support identification of FIFO named pipes");
                    return false;
                }
                isFile() {
                    return this.entry.isFile;
                }
                isSocket() {
                    _utils_ts_9.notImplemented("Deno does not yet support identification of sockets");
                    return false;
                }
                isSymbolicLink() {
                    return this.entry.isSymlink;
                }
                get name() {
                    return this.entry.name;
                }
            };
            exports_31("default", Dirent);
        }
    };
});
System.register("node/_fs/_fs_readdir", ["node/_fs/_fs_watch", "node/_fs/_fs_dirent", "node/path"], function (exports_32, context_32) {
    "use strict";
    var _fs_watch_ts_1, _fs_dirent_ts_1, path_ts_11;
    var __moduleName = context_32 && context_32.id;
    function toDirent(val) {
        return new _fs_dirent_ts_1.default(val);
    }
    function readdir(path, optionsOrCallback, maybeCallback) {
        const callback = (typeof optionsOrCallback === "function"
            ? optionsOrCallback
            : maybeCallback);
        const options = typeof optionsOrCallback === "object"
            ? optionsOrCallback
            : null;
        const result = [];
        path = path instanceof URL ? path_ts_11.fromFileUrl(path) : path;
        if (!callback)
            throw new Error("No callback function supplied");
        if (options?.encoding) {
            try {
                new TextDecoder(options.encoding);
            }
            catch (error) {
                throw new Error(`TypeError [ERR_INVALID_OPT_VALUE_ENCODING]: The value "${options.encoding}" is invalid for option "encoding"`);
            }
        }
        try {
            _fs_watch_ts_1.asyncIterableToCallback(Deno.readDir(path), (val, done) => {
                if (typeof path !== "string")
                    return;
                if (done) {
                    callback(undefined, result);
                    return;
                }
                if (options?.withFileTypes) {
                    result.push(toDirent(val));
                }
                else
                    result.push(decode(val.name));
            });
        }
        catch (error) {
            callback(error, result);
        }
    }
    exports_32("readdir", readdir);
    function decode(str, encoding) {
        if (!encoding)
            return str;
        else {
            const decoder = new TextDecoder(encoding);
            const encoder = new TextEncoder();
            return decoder.decode(encoder.encode(str));
        }
    }
    function readdirSync(path, options) {
        const result = [];
        path = path instanceof URL ? path_ts_11.fromFileUrl(path) : path;
        if (options?.encoding) {
            try {
                new TextDecoder(options.encoding);
            }
            catch (error) {
                throw new Error(`TypeError [ERR_INVALID_OPT_VALUE_ENCODING]: The value "${options.encoding}" is invalid for option "encoding"`);
            }
        }
        for (const file of Deno.readDirSync(path)) {
            if (options?.withFileTypes) {
                result.push(toDirent(file));
            }
            else
                result.push(decode(file.name));
        }
        return result;
    }
    exports_32("readdirSync", readdirSync);
    return {
        setters: [
            function (_fs_watch_ts_1_1) {
                _fs_watch_ts_1 = _fs_watch_ts_1_1;
            },
            function (_fs_dirent_ts_1_1) {
                _fs_dirent_ts_1 = _fs_dirent_ts_1_1;
            },
            function (path_ts_11_1) {
                path_ts_11 = path_ts_11_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_rename", ["node/path"], function (exports_33, context_33) {
    "use strict";
    var path_ts_12;
    var __moduleName = context_33 && context_33.id;
    function rename(oldPath, newPath, callback) {
        oldPath = oldPath instanceof URL ? path_ts_12.fromFileUrl(oldPath) : oldPath;
        newPath = newPath instanceof URL ? path_ts_12.fromFileUrl(newPath) : newPath;
        if (!callback)
            throw new Error("No callback function supplied");
        Deno.rename(oldPath, newPath)
            .then((_) => callback())
            .catch(callback);
    }
    exports_33("rename", rename);
    function renameSync(oldPath, newPath) {
        oldPath = oldPath instanceof URL ? path_ts_12.fromFileUrl(oldPath) : oldPath;
        newPath = newPath instanceof URL ? path_ts_12.fromFileUrl(newPath) : newPath;
        Deno.renameSync(oldPath, newPath);
    }
    exports_33("renameSync", renameSync);
    return {
        setters: [
            function (path_ts_12_1) {
                path_ts_12 = path_ts_12_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_rmdir", [], function (exports_34, context_34) {
    "use strict";
    var __moduleName = context_34 && context_34.id;
    function rmdir(path, optionsOrCallback, maybeCallback) {
        const callback = typeof optionsOrCallback === "function"
            ? optionsOrCallback
            : maybeCallback;
        const options = typeof optionsOrCallback === "object"
            ? optionsOrCallback
            : undefined;
        if (!callback)
            throw new Error("No callback function supplied");
        Deno.remove(path, { recursive: options?.recursive })
            .then((_) => callback())
            .catch(callback);
    }
    exports_34("rmdir", rmdir);
    function rmdirSync(path, options) {
        Deno.removeSync(path, { recursive: options?.recursive });
    }
    exports_34("rmdirSync", rmdirSync);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_unlink", [], function (exports_35, context_35) {
    "use strict";
    var __moduleName = context_35 && context_35.id;
    function unlink(path, callback) {
        if (!callback)
            throw new Error("No callback function supplied");
        Deno.remove(path)
            .then((_) => callback())
            .catch(callback);
    }
    exports_35("unlink", unlink);
    function unlinkSync(path) {
        Deno.removeSync(path);
    }
    exports_35("unlinkSync", unlinkSync);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("fs/empty_dir", ["path/mod"], function (exports_36, context_36) {
    "use strict";
    var mod_ts_3;
    var __moduleName = context_36 && context_36.id;
    async function emptyDir(dir) {
        try {
            const items = [];
            for await (const dirEntry of Deno.readDir(dir)) {
                items.push(dirEntry);
            }
            while (items.length) {
                const item = items.shift();
                if (item && item.name) {
                    const filepath = mod_ts_3.join(dir, item.name);
                    await Deno.remove(filepath, { recursive: true });
                }
            }
        }
        catch (err) {
            if (!(err instanceof Deno.errors.NotFound)) {
                throw err;
            }
            await Deno.mkdir(dir, { recursive: true });
        }
    }
    exports_36("emptyDir", emptyDir);
    function emptyDirSync(dir) {
        try {
            const items = [...Deno.readDirSync(dir)];
            while (items.length) {
                const item = items.shift();
                if (item && item.name) {
                    const filepath = mod_ts_3.join(dir, item.name);
                    Deno.removeSync(filepath, { recursive: true });
                }
            }
        }
        catch (err) {
            if (!(err instanceof Deno.errors.NotFound)) {
                throw err;
            }
            Deno.mkdirSync(dir, { recursive: true });
            return;
        }
    }
    exports_36("emptyDirSync", emptyDirSync);
    return {
        setters: [
            function (mod_ts_3_1) {
                mod_ts_3 = mod_ts_3_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/_util", ["path/mod"], function (exports_37, context_37) {
    "use strict";
    var path;
    var __moduleName = context_37 && context_37.id;
    function isSubdir(src, dest, sep = path.sep) {
        if (src === dest) {
            return false;
        }
        const srcArray = src.split(sep);
        const destArray = dest.split(sep);
        return srcArray.every((current, i) => destArray[i] === current);
    }
    exports_37("isSubdir", isSubdir);
    function getFileInfoType(fileInfo) {
        return fileInfo.isFile
            ? "file"
            : fileInfo.isDirectory
                ? "dir"
                : fileInfo.isSymlink
                    ? "symlink"
                    : undefined;
    }
    exports_37("getFileInfoType", getFileInfoType);
    return {
        setters: [
            function (path_1) {
                path = path_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/ensure_dir", ["fs/_util"], function (exports_38, context_38) {
    "use strict";
    var _util_ts_3;
    var __moduleName = context_38 && context_38.id;
    async function ensureDir(dir) {
        try {
            const fileInfo = await Deno.lstat(dir);
            if (!fileInfo.isDirectory) {
                throw new Error(`Ensure path exists, expected 'dir', got '${_util_ts_3.getFileInfoType(fileInfo)}'`);
            }
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                await Deno.mkdir(dir, { recursive: true });
                return;
            }
            throw err;
        }
    }
    exports_38("ensureDir", ensureDir);
    function ensureDirSync(dir) {
        try {
            const fileInfo = Deno.lstatSync(dir);
            if (!fileInfo.isDirectory) {
                throw new Error(`Ensure path exists, expected 'dir', got '${_util_ts_3.getFileInfoType(fileInfo)}'`);
            }
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                Deno.mkdirSync(dir, { recursive: true });
                return;
            }
            throw err;
        }
    }
    exports_38("ensureDirSync", ensureDirSync);
    return {
        setters: [
            function (_util_ts_3_1) {
                _util_ts_3 = _util_ts_3_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/ensure_file", ["path/mod", "fs/ensure_dir", "fs/_util"], function (exports_39, context_39) {
    "use strict";
    var path, ensure_dir_ts_1, _util_ts_4;
    var __moduleName = context_39 && context_39.id;
    async function ensureFile(filePath) {
        try {
            const stat = await Deno.lstat(filePath);
            if (!stat.isFile) {
                throw new Error(`Ensure path exists, expected 'file', got '${_util_ts_4.getFileInfoType(stat)}'`);
            }
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                await ensure_dir_ts_1.ensureDir(path.dirname(filePath));
                await Deno.writeFile(filePath, new Uint8Array());
                return;
            }
            throw err;
        }
    }
    exports_39("ensureFile", ensureFile);
    function ensureFileSync(filePath) {
        try {
            const stat = Deno.lstatSync(filePath);
            if (!stat.isFile) {
                throw new Error(`Ensure path exists, expected 'file', got '${_util_ts_4.getFileInfoType(stat)}'`);
            }
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                ensure_dir_ts_1.ensureDirSync(path.dirname(filePath));
                Deno.writeFileSync(filePath, new Uint8Array());
                return;
            }
            throw err;
        }
    }
    exports_39("ensureFileSync", ensureFileSync);
    return {
        setters: [
            function (path_2) {
                path = path_2;
            },
            function (ensure_dir_ts_1_1) {
                ensure_dir_ts_1 = ensure_dir_ts_1_1;
            },
            function (_util_ts_4_1) {
                _util_ts_4 = _util_ts_4_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/exists", [], function (exports_40, context_40) {
    "use strict";
    var __moduleName = context_40 && context_40.id;
    async function exists(filePath) {
        try {
            await Deno.lstat(filePath);
            return true;
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                return false;
            }
            throw err;
        }
    }
    exports_40("exists", exists);
    function existsSync(filePath) {
        try {
            Deno.lstatSync(filePath);
            return true;
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                return false;
            }
            throw err;
        }
    }
    exports_40("existsSync", existsSync);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("fs/ensure_link", ["path/mod", "fs/ensure_dir", "fs/exists", "fs/_util"], function (exports_41, context_41) {
    "use strict";
    var path, ensure_dir_ts_2, exists_ts_1, _util_ts_5;
    var __moduleName = context_41 && context_41.id;
    async function ensureLink(src, dest) {
        if (await exists_ts_1.exists(dest)) {
            const destStatInfo = await Deno.lstat(dest);
            const destFilePathType = _util_ts_5.getFileInfoType(destStatInfo);
            if (destFilePathType !== "file") {
                throw new Error(`Ensure path exists, expected 'file', got '${destFilePathType}'`);
            }
            return;
        }
        await ensure_dir_ts_2.ensureDir(path.dirname(dest));
        await Deno.link(src, dest);
    }
    exports_41("ensureLink", ensureLink);
    function ensureLinkSync(src, dest) {
        if (exists_ts_1.existsSync(dest)) {
            const destStatInfo = Deno.lstatSync(dest);
            const destFilePathType = _util_ts_5.getFileInfoType(destStatInfo);
            if (destFilePathType !== "file") {
                throw new Error(`Ensure path exists, expected 'file', got '${destFilePathType}'`);
            }
            return;
        }
        ensure_dir_ts_2.ensureDirSync(path.dirname(dest));
        Deno.linkSync(src, dest);
    }
    exports_41("ensureLinkSync", ensureLinkSync);
    return {
        setters: [
            function (path_3) {
                path = path_3;
            },
            function (ensure_dir_ts_2_1) {
                ensure_dir_ts_2 = ensure_dir_ts_2_1;
            },
            function (exists_ts_1_1) {
                exists_ts_1 = exists_ts_1_1;
            },
            function (_util_ts_5_1) {
                _util_ts_5 = _util_ts_5_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/ensure_symlink", ["path/mod", "fs/ensure_dir", "fs/exists", "fs/_util"], function (exports_42, context_42) {
    "use strict";
    var path, ensure_dir_ts_3, exists_ts_2, _util_ts_6;
    var __moduleName = context_42 && context_42.id;
    async function ensureSymlink(src, dest) {
        const srcStatInfo = await Deno.lstat(src);
        const srcFilePathType = _util_ts_6.getFileInfoType(srcStatInfo);
        if (await exists_ts_2.exists(dest)) {
            const destStatInfo = await Deno.lstat(dest);
            const destFilePathType = _util_ts_6.getFileInfoType(destStatInfo);
            if (destFilePathType !== "symlink") {
                throw new Error(`Ensure path exists, expected 'symlink', got '${destFilePathType}'`);
            }
            return;
        }
        await ensure_dir_ts_3.ensureDir(path.dirname(dest));
        if (Deno.build.os === "windows") {
            await Deno.symlink(src, dest, {
                type: srcFilePathType === "dir" ? "dir" : "file",
            });
        }
        else {
            await Deno.symlink(src, dest);
        }
    }
    exports_42("ensureSymlink", ensureSymlink);
    function ensureSymlinkSync(src, dest) {
        const srcStatInfo = Deno.lstatSync(src);
        const srcFilePathType = _util_ts_6.getFileInfoType(srcStatInfo);
        if (exists_ts_2.existsSync(dest)) {
            const destStatInfo = Deno.lstatSync(dest);
            const destFilePathType = _util_ts_6.getFileInfoType(destStatInfo);
            if (destFilePathType !== "symlink") {
                throw new Error(`Ensure path exists, expected 'symlink', got '${destFilePathType}'`);
            }
            return;
        }
        ensure_dir_ts_3.ensureDirSync(path.dirname(dest));
        if (Deno.build.os === "windows") {
            Deno.symlinkSync(src, dest, {
                type: srcFilePathType === "dir" ? "dir" : "file",
            });
        }
        else {
            Deno.symlinkSync(src, dest);
        }
    }
    exports_42("ensureSymlinkSync", ensureSymlinkSync);
    return {
        setters: [
            function (path_4) {
                path = path_4;
            },
            function (ensure_dir_ts_3_1) {
                ensure_dir_ts_3 = ensure_dir_ts_3_1;
            },
            function (exists_ts_2_1) {
                exists_ts_2 = exists_ts_2_1;
            },
            function (_util_ts_6_1) {
                _util_ts_6 = _util_ts_6_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/walk", ["_util/assert", "path/mod"], function (exports_43, context_43) {
    "use strict";
    var assert_ts_3, mod_ts_4;
    var __moduleName = context_43 && context_43.id;
    function _createWalkEntrySync(path) {
        path = mod_ts_4.normalize(path);
        const name = mod_ts_4.basename(path);
        const info = Deno.statSync(path);
        return {
            path,
            name,
            isFile: info.isFile,
            isDirectory: info.isDirectory,
            isSymlink: info.isSymlink,
        };
    }
    exports_43("_createWalkEntrySync", _createWalkEntrySync);
    async function _createWalkEntry(path) {
        path = mod_ts_4.normalize(path);
        const name = mod_ts_4.basename(path);
        const info = await Deno.stat(path);
        return {
            path,
            name,
            isFile: info.isFile,
            isDirectory: info.isDirectory,
            isSymlink: info.isSymlink,
        };
    }
    exports_43("_createWalkEntry", _createWalkEntry);
    function include(path, exts, match, skip) {
        if (exts && !exts.some((ext) => path.endsWith(ext))) {
            return false;
        }
        if (match && !match.some((pattern) => !!path.match(pattern))) {
            return false;
        }
        if (skip && skip.some((pattern) => !!path.match(pattern))) {
            return false;
        }
        return true;
    }
    async function* walk(root, { maxDepth = Infinity, includeFiles = true, includeDirs = true, followSymlinks = false, exts = undefined, match = undefined, skip = undefined, } = {}) {
        if (maxDepth < 0) {
            return;
        }
        if (includeDirs && include(root, exts, match, skip)) {
            yield await _createWalkEntry(root);
        }
        if (maxDepth < 1 || !include(root, undefined, undefined, skip)) {
            return;
        }
        for await (const entry of Deno.readDir(root)) {
            if (entry.isSymlink) {
                if (followSymlinks) {
                    throw new Error("unimplemented");
                }
                else {
                    continue;
                }
            }
            assert_ts_3.assert(entry.name != null);
            const path = mod_ts_4.join(root, entry.name);
            if (entry.isFile) {
                if (includeFiles && include(path, exts, match, skip)) {
                    yield { path, ...entry };
                }
            }
            else {
                yield* walk(path, {
                    maxDepth: maxDepth - 1,
                    includeFiles,
                    includeDirs,
                    followSymlinks,
                    exts,
                    match,
                    skip,
                });
            }
        }
    }
    exports_43("walk", walk);
    function* walkSync(root, { maxDepth = Infinity, includeFiles = true, includeDirs = true, followSymlinks = false, exts = undefined, match = undefined, skip = undefined, } = {}) {
        if (maxDepth < 0) {
            return;
        }
        if (includeDirs && include(root, exts, match, skip)) {
            yield _createWalkEntrySync(root);
        }
        if (maxDepth < 1 || !include(root, undefined, undefined, skip)) {
            return;
        }
        for (const entry of Deno.readDirSync(root)) {
            if (entry.isSymlink) {
                if (followSymlinks) {
                    throw new Error("unimplemented");
                }
                else {
                    continue;
                }
            }
            assert_ts_3.assert(entry.name != null);
            const path = mod_ts_4.join(root, entry.name);
            if (entry.isFile) {
                if (includeFiles && include(path, exts, match, skip)) {
                    yield { path, ...entry };
                }
            }
            else {
                yield* walkSync(path, {
                    maxDepth: maxDepth - 1,
                    includeFiles,
                    includeDirs,
                    followSymlinks,
                    exts,
                    match,
                    skip,
                });
            }
        }
    }
    exports_43("walkSync", walkSync);
    return {
        setters: [
            function (assert_ts_3_1) {
                assert_ts_3 = assert_ts_3_1;
            },
            function (mod_ts_4_1) {
                mod_ts_4 = mod_ts_4_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/expand_glob", ["path/mod", "fs/walk", "_util/assert"], function (exports_44, context_44) {
    "use strict";
    var mod_ts_5, walk_ts_1, assert_ts_4, isWindows;
    var __moduleName = context_44 && context_44.id;
    function split(path) {
        const s = mod_ts_5.SEP_PATTERN.source;
        const segments = path
            .replace(new RegExp(`^${s}|${s}$`, "g"), "")
            .split(mod_ts_5.SEP_PATTERN);
        const isAbsolute_ = mod_ts_5.isAbsolute(path);
        return {
            segments,
            isAbsolute: isAbsolute_,
            hasTrailingSep: !!path.match(new RegExp(`${s}$`)),
            winRoot: isWindows && isAbsolute_ ? segments.shift() : undefined,
        };
    }
    function throwUnlessNotFound(error) {
        if (!(error instanceof Deno.errors.NotFound)) {
            throw error;
        }
    }
    function comparePath(a, b) {
        if (a.path < b.path)
            return -1;
        if (a.path > b.path)
            return 1;
        return 0;
    }
    async function* expandGlob(glob, { root = Deno.cwd(), exclude = [], includeDirs = true, extended = false, globstar = false, } = {}) {
        const globOptions = { extended, globstar };
        const absRoot = mod_ts_5.isAbsolute(root)
            ? mod_ts_5.normalize(root)
            : mod_ts_5.joinGlobs([Deno.cwd(), root], globOptions);
        const resolveFromRoot = (path) => mod_ts_5.isAbsolute(path)
            ? mod_ts_5.normalize(path)
            : mod_ts_5.joinGlobs([absRoot, path], globOptions);
        const excludePatterns = exclude
            .map(resolveFromRoot)
            .map((s) => mod_ts_5.globToRegExp(s, globOptions));
        const shouldInclude = (path) => !excludePatterns.some((p) => !!path.match(p));
        const { segments, hasTrailingSep, winRoot } = split(resolveFromRoot(glob));
        let fixedRoot = winRoot != undefined ? winRoot : "/";
        while (segments.length > 0 && !mod_ts_5.isGlob(segments[0])) {
            const seg = segments.shift();
            assert_ts_4.assert(seg != null);
            fixedRoot = mod_ts_5.joinGlobs([fixedRoot, seg], globOptions);
        }
        let fixedRootInfo;
        try {
            fixedRootInfo = await walk_ts_1._createWalkEntry(fixedRoot);
        }
        catch (error) {
            return throwUnlessNotFound(error);
        }
        async function* advanceMatch(walkInfo, globSegment) {
            if (!walkInfo.isDirectory) {
                return;
            }
            else if (globSegment == "..") {
                const parentPath = mod_ts_5.joinGlobs([walkInfo.path, ".."], globOptions);
                try {
                    if (shouldInclude(parentPath)) {
                        return yield await walk_ts_1._createWalkEntry(parentPath);
                    }
                }
                catch (error) {
                    throwUnlessNotFound(error);
                }
                return;
            }
            else if (globSegment == "**") {
                return yield* walk_ts_1.walk(walkInfo.path, {
                    includeFiles: false,
                    skip: excludePatterns,
                });
            }
            yield* walk_ts_1.walk(walkInfo.path, {
                maxDepth: 1,
                match: [
                    mod_ts_5.globToRegExp(mod_ts_5.joinGlobs([walkInfo.path, globSegment], globOptions), globOptions),
                ],
                skip: excludePatterns,
            });
        }
        let currentMatches = [fixedRootInfo];
        for (const segment of segments) {
            const nextMatchMap = new Map();
            for (const currentMatch of currentMatches) {
                for await (const nextMatch of advanceMatch(currentMatch, segment)) {
                    nextMatchMap.set(nextMatch.path, nextMatch);
                }
            }
            currentMatches = [...nextMatchMap.values()].sort(comparePath);
        }
        if (hasTrailingSep) {
            currentMatches = currentMatches.filter((entry) => entry.isDirectory);
        }
        if (!includeDirs) {
            currentMatches = currentMatches.filter((entry) => !entry.isDirectory);
        }
        yield* currentMatches;
    }
    exports_44("expandGlob", expandGlob);
    function* expandGlobSync(glob, { root = Deno.cwd(), exclude = [], includeDirs = true, extended = false, globstar = false, } = {}) {
        const globOptions = { extended, globstar };
        const absRoot = mod_ts_5.isAbsolute(root)
            ? mod_ts_5.normalize(root)
            : mod_ts_5.joinGlobs([Deno.cwd(), root], globOptions);
        const resolveFromRoot = (path) => mod_ts_5.isAbsolute(path)
            ? mod_ts_5.normalize(path)
            : mod_ts_5.joinGlobs([absRoot, path], globOptions);
        const excludePatterns = exclude
            .map(resolveFromRoot)
            .map((s) => mod_ts_5.globToRegExp(s, globOptions));
        const shouldInclude = (path) => !excludePatterns.some((p) => !!path.match(p));
        const { segments, hasTrailingSep, winRoot } = split(resolveFromRoot(glob));
        let fixedRoot = winRoot != undefined ? winRoot : "/";
        while (segments.length > 0 && !mod_ts_5.isGlob(segments[0])) {
            const seg = segments.shift();
            assert_ts_4.assert(seg != null);
            fixedRoot = mod_ts_5.joinGlobs([fixedRoot, seg], globOptions);
        }
        let fixedRootInfo;
        try {
            fixedRootInfo = walk_ts_1._createWalkEntrySync(fixedRoot);
        }
        catch (error) {
            return throwUnlessNotFound(error);
        }
        function* advanceMatch(walkInfo, globSegment) {
            if (!walkInfo.isDirectory) {
                return;
            }
            else if (globSegment == "..") {
                const parentPath = mod_ts_5.joinGlobs([walkInfo.path, ".."], globOptions);
                try {
                    if (shouldInclude(parentPath)) {
                        return yield walk_ts_1._createWalkEntrySync(parentPath);
                    }
                }
                catch (error) {
                    throwUnlessNotFound(error);
                }
                return;
            }
            else if (globSegment == "**") {
                return yield* walk_ts_1.walkSync(walkInfo.path, {
                    includeFiles: false,
                    skip: excludePatterns,
                });
            }
            yield* walk_ts_1.walkSync(walkInfo.path, {
                maxDepth: 1,
                match: [
                    mod_ts_5.globToRegExp(mod_ts_5.joinGlobs([walkInfo.path, globSegment], globOptions), globOptions),
                ],
                skip: excludePatterns,
            });
        }
        let currentMatches = [fixedRootInfo];
        for (const segment of segments) {
            const nextMatchMap = new Map();
            for (const currentMatch of currentMatches) {
                for (const nextMatch of advanceMatch(currentMatch, segment)) {
                    nextMatchMap.set(nextMatch.path, nextMatch);
                }
            }
            currentMatches = [...nextMatchMap.values()].sort(comparePath);
        }
        if (hasTrailingSep) {
            currentMatches = currentMatches.filter((entry) => entry.isDirectory);
        }
        if (!includeDirs) {
            currentMatches = currentMatches.filter((entry) => !entry.isDirectory);
        }
        yield* currentMatches;
    }
    exports_44("expandGlobSync", expandGlobSync);
    return {
        setters: [
            function (mod_ts_5_1) {
                mod_ts_5 = mod_ts_5_1;
            },
            function (walk_ts_1_1) {
                walk_ts_1 = walk_ts_1_1;
            },
            function (assert_ts_4_1) {
                assert_ts_4 = assert_ts_4_1;
            }
        ],
        execute: function () {
            isWindows = Deno.build.os == "windows";
        }
    };
});
System.register("fs/move", ["fs/exists", "fs/_util"], function (exports_45, context_45) {
    "use strict";
    var exists_ts_3, _util_ts_7;
    var __moduleName = context_45 && context_45.id;
    async function move(src, dest, { overwrite = false } = {}) {
        const srcStat = await Deno.stat(src);
        if (srcStat.isDirectory && _util_ts_7.isSubdir(src, dest)) {
            throw new Error(`Cannot move '${src}' to a subdirectory of itself, '${dest}'.`);
        }
        if (overwrite) {
            if (await exists_ts_3.exists(dest)) {
                await Deno.remove(dest, { recursive: true });
            }
            await Deno.rename(src, dest);
        }
        else {
            if (await exists_ts_3.exists(dest)) {
                throw new Error("dest already exists.");
            }
            await Deno.rename(src, dest);
        }
        return;
    }
    exports_45("move", move);
    function moveSync(src, dest, { overwrite = false } = {}) {
        const srcStat = Deno.statSync(src);
        if (srcStat.isDirectory && _util_ts_7.isSubdir(src, dest)) {
            throw new Error(`Cannot move '${src}' to a subdirectory of itself, '${dest}'.`);
        }
        if (overwrite) {
            if (exists_ts_3.existsSync(dest)) {
                Deno.removeSync(dest, { recursive: true });
            }
            Deno.renameSync(src, dest);
        }
        else {
            if (exists_ts_3.existsSync(dest)) {
                throw new Error("dest already exists.");
            }
            Deno.renameSync(src, dest);
        }
    }
    exports_45("moveSync", moveSync);
    return {
        setters: [
            function (exists_ts_3_1) {
                exists_ts_3 = exists_ts_3_1;
            },
            function (_util_ts_7_1) {
                _util_ts_7 = _util_ts_7_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("fs/copy", ["path/mod", "fs/ensure_dir", "fs/_util", "_util/assert"], function (exports_46, context_46) {
    "use strict";
    var path, ensure_dir_ts_4, _util_ts_8, assert_ts_5, isWindows;
    var __moduleName = context_46 && context_46.id;
    async function ensureValidCopy(src, dest, options, isCopyFolder = false) {
        let destStat;
        try {
            destStat = await Deno.lstat(dest);
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                return;
            }
            throw err;
        }
        if (isCopyFolder && !destStat.isDirectory) {
            throw new Error(`Cannot overwrite non-directory '${dest}' with directory '${src}'.`);
        }
        if (!options.overwrite) {
            throw new Error(`'${dest}' already exists.`);
        }
        return destStat;
    }
    function ensureValidCopySync(src, dest, options, isCopyFolder = false) {
        let destStat;
        try {
            destStat = Deno.lstatSync(dest);
        }
        catch (err) {
            if (err instanceof Deno.errors.NotFound) {
                return;
            }
            throw err;
        }
        if (isCopyFolder && !destStat.isDirectory) {
            throw new Error(`Cannot overwrite non-directory '${dest}' with directory '${src}'.`);
        }
        if (!options.overwrite) {
            throw new Error(`'${dest}' already exists.`);
        }
        return destStat;
    }
    async function copyFile(src, dest, options) {
        await ensureValidCopy(src, dest, options);
        await Deno.copyFile(src, dest);
        if (options.preserveTimestamps) {
            const statInfo = await Deno.stat(src);
            assert_ts_5.assert(statInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(statInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            await Deno.utime(dest, statInfo.atime, statInfo.mtime);
        }
    }
    function copyFileSync(src, dest, options) {
        ensureValidCopySync(src, dest, options);
        Deno.copyFileSync(src, dest);
        if (options.preserveTimestamps) {
            const statInfo = Deno.statSync(src);
            assert_ts_5.assert(statInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(statInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            Deno.utimeSync(dest, statInfo.atime, statInfo.mtime);
        }
    }
    async function copySymLink(src, dest, options) {
        await ensureValidCopy(src, dest, options);
        const originSrcFilePath = await Deno.readLink(src);
        const type = _util_ts_8.getFileInfoType(await Deno.lstat(src));
        if (isWindows) {
            await Deno.symlink(originSrcFilePath, dest, {
                type: type === "dir" ? "dir" : "file",
            });
        }
        else {
            await Deno.symlink(originSrcFilePath, dest);
        }
        if (options.preserveTimestamps) {
            const statInfo = await Deno.lstat(src);
            assert_ts_5.assert(statInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(statInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            await Deno.utime(dest, statInfo.atime, statInfo.mtime);
        }
    }
    function copySymlinkSync(src, dest, options) {
        ensureValidCopySync(src, dest, options);
        const originSrcFilePath = Deno.readLinkSync(src);
        const type = _util_ts_8.getFileInfoType(Deno.lstatSync(src));
        if (isWindows) {
            Deno.symlinkSync(originSrcFilePath, dest, {
                type: type === "dir" ? "dir" : "file",
            });
        }
        else {
            Deno.symlinkSync(originSrcFilePath, dest);
        }
        if (options.preserveTimestamps) {
            const statInfo = Deno.lstatSync(src);
            assert_ts_5.assert(statInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(statInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            Deno.utimeSync(dest, statInfo.atime, statInfo.mtime);
        }
    }
    async function copyDir(src, dest, options) {
        const destStat = await ensureValidCopy(src, dest, options, true);
        if (!destStat) {
            await ensure_dir_ts_4.ensureDir(dest);
        }
        if (options.preserveTimestamps) {
            const srcStatInfo = await Deno.stat(src);
            assert_ts_5.assert(srcStatInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(srcStatInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            await Deno.utime(dest, srcStatInfo.atime, srcStatInfo.mtime);
        }
        for await (const entry of Deno.readDir(src)) {
            const srcPath = path.join(src, entry.name);
            const destPath = path.join(dest, path.basename(srcPath));
            if (entry.isSymlink) {
                await copySymLink(srcPath, destPath, options);
            }
            else if (entry.isDirectory) {
                await copyDir(srcPath, destPath, options);
            }
            else if (entry.isFile) {
                await copyFile(srcPath, destPath, options);
            }
        }
    }
    function copyDirSync(src, dest, options) {
        const destStat = ensureValidCopySync(src, dest, options, true);
        if (!destStat) {
            ensure_dir_ts_4.ensureDirSync(dest);
        }
        if (options.preserveTimestamps) {
            const srcStatInfo = Deno.statSync(src);
            assert_ts_5.assert(srcStatInfo.atime instanceof Date, `statInfo.atime is unavailable`);
            assert_ts_5.assert(srcStatInfo.mtime instanceof Date, `statInfo.mtime is unavailable`);
            Deno.utimeSync(dest, srcStatInfo.atime, srcStatInfo.mtime);
        }
        for (const entry of Deno.readDirSync(src)) {
            assert_ts_5.assert(entry.name != null, "file.name must be set");
            const srcPath = path.join(src, entry.name);
            const destPath = path.join(dest, path.basename(srcPath));
            if (entry.isSymlink) {
                copySymlinkSync(srcPath, destPath, options);
            }
            else if (entry.isDirectory) {
                copyDirSync(srcPath, destPath, options);
            }
            else if (entry.isFile) {
                copyFileSync(srcPath, destPath, options);
            }
        }
    }
    async function copy(src, dest, options = {}) {
        src = path.resolve(src);
        dest = path.resolve(dest);
        if (src === dest) {
            throw new Error("Source and destination cannot be the same.");
        }
        const srcStat = await Deno.lstat(src);
        if (srcStat.isDirectory && _util_ts_8.isSubdir(src, dest)) {
            throw new Error(`Cannot copy '${src}' to a subdirectory of itself, '${dest}'.`);
        }
        if (srcStat.isSymlink) {
            await copySymLink(src, dest, options);
        }
        else if (srcStat.isDirectory) {
            await copyDir(src, dest, options);
        }
        else if (srcStat.isFile) {
            await copyFile(src, dest, options);
        }
    }
    exports_46("copy", copy);
    function copySync(src, dest, options = {}) {
        src = path.resolve(src);
        dest = path.resolve(dest);
        if (src === dest) {
            throw new Error("Source and destination cannot be the same.");
        }
        const srcStat = Deno.lstatSync(src);
        if (srcStat.isDirectory && _util_ts_8.isSubdir(src, dest)) {
            throw new Error(`Cannot copy '${src}' to a subdirectory of itself, '${dest}'.`);
        }
        if (srcStat.isSymlink) {
            copySymlinkSync(src, dest, options);
        }
        else if (srcStat.isDirectory) {
            copyDirSync(src, dest, options);
        }
        else if (srcStat.isFile) {
            copyFileSync(src, dest, options);
        }
    }
    exports_46("copySync", copySync);
    return {
        setters: [
            function (path_5) {
                path = path_5;
            },
            function (ensure_dir_ts_4_1) {
                ensure_dir_ts_4 = ensure_dir_ts_4_1;
            },
            function (_util_ts_8_1) {
                _util_ts_8 = _util_ts_8_1;
            },
            function (assert_ts_5_1) {
                assert_ts_5 = assert_ts_5_1;
            }
        ],
        execute: function () {
            isWindows = Deno.build.os === "windows";
        }
    };
});
System.register("fs/eol", [], function (exports_47, context_47) {
    "use strict";
    var EOL, regDetect;
    var __moduleName = context_47 && context_47.id;
    function detect(content) {
        const d = content.match(regDetect);
        if (!d || d.length === 0) {
            return null;
        }
        const crlf = d.filter((x) => x === EOL.CRLF);
        if (crlf.length > 0) {
            return EOL.CRLF;
        }
        else {
            return EOL.LF;
        }
    }
    exports_47("detect", detect);
    function format(content, eol) {
        return content.replace(regDetect, eol);
    }
    exports_47("format", format);
    return {
        setters: [],
        execute: function () {
            (function (EOL) {
                EOL["LF"] = "\n";
                EOL["CRLF"] = "\r\n";
            })(EOL || (EOL = {}));
            exports_47("EOL", EOL);
            regDetect = /(?:\r?\n)/g;
        }
    };
});
System.register("fs/mod", ["fs/empty_dir", "fs/ensure_dir", "fs/ensure_file", "fs/ensure_link", "fs/ensure_symlink", "fs/exists", "fs/expand_glob", "fs/move", "fs/copy", "fs/walk", "fs/eol"], function (exports_48, context_48) {
    "use strict";
    var __moduleName = context_48 && context_48.id;
    function exportStar_3(m) {
        var exports = {};
        for (var n in m) {
            if (n !== "default") exports[n] = m[n];
        }
        exports_48(exports);
    }
    return {
        setters: [
            function (empty_dir_ts_1_1) {
                exportStar_3(empty_dir_ts_1_1);
            },
            function (ensure_dir_ts_5_1) {
                exportStar_3(ensure_dir_ts_5_1);
            },
            function (ensure_file_ts_1_1) {
                exportStar_3(ensure_file_ts_1_1);
            },
            function (ensure_link_ts_1_1) {
                exportStar_3(ensure_link_ts_1_1);
            },
            function (ensure_symlink_ts_1_1) {
                exportStar_3(ensure_symlink_ts_1_1);
            },
            function (exists_ts_4_1) {
                exportStar_3(exists_ts_4_1);
            },
            function (expand_glob_ts_1_1) {
                exportStar_3(expand_glob_ts_1_1);
            },
            function (move_ts_1_1) {
                exportStar_3(move_ts_1_1);
            },
            function (copy_ts_1_1) {
                exportStar_3(copy_ts_1_1);
            },
            function (walk_ts_2_1) {
                exportStar_3(walk_ts_2_1);
            },
            function (eol_ts_1_1) {
                exportStar_3(eol_ts_1_1);
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_open", ["fs/mod", "node/path", "node/_fs/_fs_common"], function (exports_49, context_49) {
    "use strict";
    var mod_ts_6, path_ts_13, _fs_common_ts_4;
    var __moduleName = context_49 && context_49.id;
    function convertFlagAndModeToOptions(flag, mode) {
        if (!flag && !mode)
            return undefined;
        if (!flag && mode)
            return { mode };
        return { ..._fs_common_ts_4.getOpenOptions(flag), mode };
    }
    function open(path, flagsOrCallback, callbackOrMode, maybeCallback) {
        const flags = typeof flagsOrCallback === "string"
            ? flagsOrCallback
            : undefined;
        const callback = typeof flagsOrCallback === "function"
            ? flagsOrCallback
            : typeof callbackOrMode === "function"
                ? callbackOrMode
                : maybeCallback;
        const mode = typeof callbackOrMode === "number" ? callbackOrMode : undefined;
        path = path instanceof URL ? path_ts_13.fromFileUrl(path) : path;
        if (!callback)
            throw new Error("No callback function supplied");
        if (["ax", "ax+", "wx", "wx+"].includes(flags || "") && mod_ts_6.existsSync(path)) {
            const err = new Error(`EEXIST: file already exists, open '${path}'`);
            callback(err, 0);
        }
        else {
            if (flags === "as" || flags === "as+") {
                try {
                    const res = openSync(path, flags, mode);
                    callback(undefined, res);
                }
                catch (error) {
                    callback(error, error);
                }
                return;
            }
            Deno.open(path, convertFlagAndModeToOptions(flags, mode))
                .then((file) => callback(undefined, file.rid))
                .catch((err) => callback(err, err));
        }
    }
    exports_49("open", open);
    function openSync(path, flagsOrMode, maybeMode) {
        const flags = typeof flagsOrMode === "string" ? flagsOrMode : undefined;
        const mode = typeof flagsOrMode === "number" ? flagsOrMode : maybeMode;
        path = path instanceof URL ? path_ts_13.fromFileUrl(path) : path;
        if (["ax", "ax+", "wx", "wx+"].includes(flags || "") && mod_ts_6.existsSync(path)) {
            throw new Error(`EEXIST: file already exists, open '${path}'`);
        }
        return Deno.openSync(path, convertFlagAndModeToOptions(flags, mode)).rid;
    }
    exports_49("openSync", openSync);
    return {
        setters: [
            function (mod_ts_6_1) {
                mod_ts_6 = mod_ts_6_1;
            },
            function (path_ts_13_1) {
                path_ts_13 = path_ts_13_1;
            },
            function (_fs_common_ts_4_1) {
                _fs_common_ts_4 = _fs_common_ts_4_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_stat", [], function (exports_50, context_50) {
    "use strict";
    var __moduleName = context_50 && context_50.id;
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
            ctimeMs: origin.mtime?.getTime() || null,
        };
    }
    exports_50("convertFileInfoToStats", convertFileInfoToStats);
    function convertFileInfoToBigIntStats(origin) {
        return {
            dev: BigInt(origin.dev),
            ino: BigInt(origin.ino),
            mode: BigInt(origin.mode),
            nlink: BigInt(origin.nlink),
            uid: BigInt(origin.uid),
            gid: BigInt(origin.gid),
            rdev: BigInt(origin.rdev),
            size: BigInt(origin.size),
            blksize: BigInt(origin.blksize),
            blocks: BigInt(origin.blocks),
            mtime: origin.mtime,
            atime: origin.atime,
            birthtime: origin.birthtime,
            mtimeMs: origin.mtime ? BigInt(origin.mtime.getTime()) : null,
            atimeMs: origin.atime ? BigInt(origin.atime.getTime()) : null,
            birthtimeMs: origin.birthtime ? BigInt(origin.birthtime.getTime()) : null,
            mtimeNs: origin.mtime ? BigInt(origin.mtime.getTime()) * 1000000n : null,
            atimeNs: origin.atime ? BigInt(origin.atime.getTime()) * 1000000n : null,
            birthtimeNs: origin.birthtime
                ? BigInt(origin.birthtime.getTime()) * 1000000n
                : null,
            isFile: () => origin.isFile,
            isDirectory: () => origin.isDirectory,
            isSymbolicLink: () => origin.isSymlink,
            isBlockDevice: () => false,
            isFIFO: () => false,
            isCharacterDevice: () => false,
            isSocket: () => false,
            ctime: origin.mtime,
            ctimeMs: origin.mtime ? BigInt(origin.mtime.getTime()) : null,
            ctimeNs: origin.mtime ? BigInt(origin.mtime.getTime()) * 1000000n : null,
        };
    }
    exports_50("convertFileInfoToBigIntStats", convertFileInfoToBigIntStats);
    function CFISBIS(fileInfo, bigInt) {
        if (bigInt)
            return convertFileInfoToBigIntStats(fileInfo);
        return convertFileInfoToStats(fileInfo);
    }
    exports_50("CFISBIS", CFISBIS);
    function stat(path, optionsOrCallback, maybeCallback) {
        const callback = (typeof optionsOrCallback === "function"
            ? optionsOrCallback
            : maybeCallback);
        const options = typeof optionsOrCallback === "object"
            ? optionsOrCallback
            : { bigint: false };
        if (!callback)
            throw new Error("No callback function supplied");
        Deno.stat(path)
            .then((stat) => callback(undefined, CFISBIS(stat, options.bigint)))
            .catch((err) => callback(err, err));
    }
    exports_50("stat", stat);
    function statSync(path, options = { bigint: false }) {
        const origin = Deno.statSync(path);
        return CFISBIS(origin, options.bigint);
    }
    exports_50("statSync", statSync);
    return {
        setters: [],
        execute: function () {
        }
    };
});
System.register("node/_fs/_fs_lstat", ["node/_fs/_fs_stat"], function (exports_51, context_51) {
    "use strict";
    var _fs_stat_ts_1;
    var __moduleName = context_51 && context_51.id;
    function lstat(path, optionsOrCallback, maybeCallback) {
        const callback = (typeof optionsOrCallback === "function"
            ? optionsOrCallback
            : maybeCallback);
        const options = typeof optionsOrCallback === "object"
            ? optionsOrCallback
            : { bigint: false };
        if (!callback)
            throw new Error("No callback function supplied");
        Deno.lstat(path)
            .then((stat) => callback(undefined, _fs_stat_ts_1.CFISBIS(stat, options.bigint)))
            .catch((err) => callback(err, err));
    }
    exports_51("lstat", lstat);
    function lstatSync(path, options) {
        const origin = Deno.lstatSync(path);
        return _fs_stat_ts_1.CFISBIS(origin, options?.bigint || false);
    }
    exports_51("lstatSync", lstatSync);
    return {
        setters: [
            function (_fs_stat_ts_1_1) {
                _fs_stat_ts_1 = _fs_stat_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/promises/_fs_writeFile", ["node/_fs/_fs_writeFile"], function (exports_52, context_52) {
    "use strict";
    var _fs_writeFile_ts_1;
    var __moduleName = context_52 && context_52.id;
    function writeFile(pathOrRid, data, options) {
        return new Promise((resolve, reject) => {
            _fs_writeFile_ts_1.writeFile(pathOrRid, data, options, (err) => {
                if (err)
                    return reject(err);
                resolve();
            });
        });
    }
    exports_52("writeFile", writeFile);
    return {
        setters: [
            function (_fs_writeFile_ts_1_1) {
                _fs_writeFile_ts_1 = _fs_writeFile_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/promises/_fs_readFile", ["node/_fs/_fs_readFile"], function (exports_53, context_53) {
    "use strict";
    var _fs_readFile_ts_1;
    var __moduleName = context_53 && context_53.id;
    function readFile(path, options) {
        return new Promise((resolve, reject) => {
            _fs_readFile_ts_1.readFile(path, options, (err, data) => {
                if (err)
                    return reject(err);
                if (data == null) {
                    return reject(new Error("Invalid state: data missing, but no error"));
                }
                resolve(data);
            });
        });
    }
    exports_53("readFile", readFile);
    return {
        setters: [
            function (_fs_readFile_ts_1_1) {
                _fs_readFile_ts_1 = _fs_readFile_ts_1_1;
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/_fs/promises/mod", ["node/_fs/promises/_fs_writeFile", "node/_fs/promises/_fs_readFile"], function (exports_54, context_54) {
    "use strict";
    var __moduleName = context_54 && context_54.id;
    return {
        setters: [
            function (_fs_writeFile_ts_2_1) {
                exports_54({
                    "writeFile": _fs_writeFile_ts_2_1["writeFile"]
                });
            },
            function (_fs_readFile_ts_2_1) {
                exports_54({
                    "readFile": _fs_readFile_ts_2_1["readFile"]
                });
            }
        ],
        execute: function () {
        }
    };
});
System.register("node/fs", ["node/_fs/_fs_access", "node/_fs/_fs_appendFile", "node/_fs/_fs_chmod", "node/_fs/_fs_chown", "node/_fs/_fs_close", "node/_fs/_fs_constants", "node/_fs/_fs_readFile", "node/_fs/_fs_readlink", "node/_fs/_fs_exists", "node/_fs/_fs_mkdir", "node/_fs/_fs_copy", "node/_fs/_fs_writeFile", "node/_fs/_fs_readdir", "node/_fs/_fs_rename", "node/_fs/_fs_rmdir", "node/_fs/_fs_unlink", "node/_fs/_fs_watch", "node/_fs/_fs_open", "node/_fs/_fs_stat", "node/_fs/_fs_lstat", "node/_fs/promises/mod"], function (exports_55, context_55) {
    "use strict";
    var _fs_access_ts_1, _fs_appendFile_ts_1, _fs_chmod_ts_1, _fs_chown_ts_1, _fs_close_ts_1, constants, _fs_readFile_ts_3, _fs_readlink_ts_1, _fs_exists_ts_1, _fs_mkdir_ts_1, _fs_copy_ts_1, _fs_writeFile_ts_3, _fs_readdir_ts_1, _fs_rename_ts_1, _fs_rmdir_ts_1, _fs_unlink_ts_1, _fs_watch_ts_2, _fs_open_ts_1, _fs_stat_ts_2, _fs_lstat_ts_1, promises;
    var __moduleName = context_55 && context_55.id;
    return {
        setters: [
            function (_fs_access_ts_1_1) {
                _fs_access_ts_1 = _fs_access_ts_1_1;
            },
            function (_fs_appendFile_ts_1_1) {
                _fs_appendFile_ts_1 = _fs_appendFile_ts_1_1;
            },
            function (_fs_chmod_ts_1_1) {
                _fs_chmod_ts_1 = _fs_chmod_ts_1_1;
            },
            function (_fs_chown_ts_1_1) {
                _fs_chown_ts_1 = _fs_chown_ts_1_1;
            },
            function (_fs_close_ts_1_1) {
                _fs_close_ts_1 = _fs_close_ts_1_1;
            },
            function (constants_1) {
                constants = constants_1;
            },
            function (_fs_readFile_ts_3_1) {
                _fs_readFile_ts_3 = _fs_readFile_ts_3_1;
            },
            function (_fs_readlink_ts_1_1) {
                _fs_readlink_ts_1 = _fs_readlink_ts_1_1;
            },
            function (_fs_exists_ts_1_1) {
                _fs_exists_ts_1 = _fs_exists_ts_1_1;
            },
            function (_fs_mkdir_ts_1_1) {
                _fs_mkdir_ts_1 = _fs_mkdir_ts_1_1;
            },
            function (_fs_copy_ts_1_1) {
                _fs_copy_ts_1 = _fs_copy_ts_1_1;
            },
            function (_fs_writeFile_ts_3_1) {
                _fs_writeFile_ts_3 = _fs_writeFile_ts_3_1;
            },
            function (_fs_readdir_ts_1_1) {
                _fs_readdir_ts_1 = _fs_readdir_ts_1_1;
            },
            function (_fs_rename_ts_1_1) {
                _fs_rename_ts_1 = _fs_rename_ts_1_1;
            },
            function (_fs_rmdir_ts_1_1) {
                _fs_rmdir_ts_1 = _fs_rmdir_ts_1_1;
            },
            function (_fs_unlink_ts_1_1) {
                _fs_unlink_ts_1 = _fs_unlink_ts_1_1;
            },
            function (_fs_watch_ts_2_1) {
                _fs_watch_ts_2 = _fs_watch_ts_2_1;
            },
            function (_fs_open_ts_1_1) {
                _fs_open_ts_1 = _fs_open_ts_1_1;
            },
            function (_fs_stat_ts_2_1) {
                _fs_stat_ts_2 = _fs_stat_ts_2_1;
            },
            function (_fs_lstat_ts_1_1) {
                _fs_lstat_ts_1 = _fs_lstat_ts_1_1;
            },
            function (promises_1) {
                promises = promises_1;
            }
        ],
        execute: function () {
            exports_55("access", _fs_access_ts_1.access);
            exports_55("accessSync", _fs_access_ts_1.accessSync);
            exports_55("appendFile", _fs_appendFile_ts_1.appendFile);
            exports_55("appendFileSync", _fs_appendFile_ts_1.appendFileSync);
            exports_55("chmod", _fs_chmod_ts_1.chmod);
            exports_55("chmodSync", _fs_chmod_ts_1.chmodSync);
            exports_55("chown", _fs_chown_ts_1.chown);
            exports_55("chownSync", _fs_chown_ts_1.chownSync);
            exports_55("close", _fs_close_ts_1.close);
            exports_55("closeSync", _fs_close_ts_1.closeSync);
            exports_55("constants", constants);
            exports_55("readFile", _fs_readFile_ts_3.readFile);
            exports_55("readFileSync", _fs_readFile_ts_3.readFileSync);
            exports_55("readlink", _fs_readlink_ts_1.readlink);
            exports_55("readlinkSync", _fs_readlink_ts_1.readlinkSync);
            exports_55("exists", _fs_exists_ts_1.exists);
            exports_55("existsSync", _fs_exists_ts_1.existsSync);
            exports_55("mkdir", _fs_mkdir_ts_1.mkdir);
            exports_55("mkdirSync", _fs_mkdir_ts_1.mkdirSync);
            exports_55("copyFile", _fs_copy_ts_1.copyFile);
            exports_55("copyFileSync", _fs_copy_ts_1.copyFileSync);
            exports_55("writeFile", _fs_writeFile_ts_3.writeFile);
            exports_55("writeFileSync", _fs_writeFile_ts_3.writeFileSync);
            exports_55("readdir", _fs_readdir_ts_1.readdir);
            exports_55("readdirSync", _fs_readdir_ts_1.readdirSync);
            exports_55("rename", _fs_rename_ts_1.rename);
            exports_55("renameSync", _fs_rename_ts_1.renameSync);
            exports_55("rmdir", _fs_rmdir_ts_1.rmdir);
            exports_55("rmdirSync", _fs_rmdir_ts_1.rmdirSync);
            exports_55("unlink", _fs_unlink_ts_1.unlink);
            exports_55("unlinkSync", _fs_unlink_ts_1.unlinkSync);
            exports_55("watch", _fs_watch_ts_2.watch);
            exports_55("open", _fs_open_ts_1.open);
            exports_55("openSync", _fs_open_ts_1.openSync);
            exports_55("stat", _fs_stat_ts_2.stat);
            exports_55("statSync", _fs_stat_ts_2.statSync);
            exports_55("lstat", _fs_lstat_ts_1.lstat);
            exports_55("lstatSync", _fs_lstat_ts_1.lstatSync);
            exports_55("promises", promises);
        }
    };
});

const __exp = __instantiate("node/fs", false);
export const access = __exp["access"];
export const accessSync = __exp["accessSync"];
export const appendFile = __exp["appendFile"];
export const appendFileSync = __exp["appendFileSync"];
export const chmod = __exp["chmod"];
export const chmodSync = __exp["chmodSync"];
export const chown = __exp["chown"];
export const chownSync = __exp["chownSync"];
export const close = __exp["close"];
export const closeSync = __exp["closeSync"];
export const constants = __exp["constants"];
export const copyFile = __exp["copyFile"];
export const copyFileSync = __exp["copyFileSync"];
export const exists = __exp["exists"];
export const existsSync = __exp["existsSync"];
export const lstat = __exp["lstat"];
export const lstatSync = __exp["lstatSync"];
export const mkdir = __exp["mkdir"];
export const mkdirSync = __exp["mkdirSync"];
export const open = __exp["open"];
export const openSync = __exp["openSync"];
export const promises = __exp["promises"];
export const readdir = __exp["readdir"];
export const readdirSync = __exp["readdirSync"];
export const readFile = __exp["readFile"];
export const readFileSync = __exp["readFileSync"];
export const readlink = __exp["readlink"];
export const readlinkSync = __exp["readlinkSync"];
export const rename = __exp["rename"];
export const renameSync = __exp["renameSync"];
export const rmdir = __exp["rmdir"];
export const rmdirSync = __exp["rmdirSync"];
export const stat = __exp["stat"];
export const statSync = __exp["statSync"];
export const unlink = __exp["unlink"];
export const unlinkSync = __exp["unlinkSync"];
export const watch = __exp["watch"];
export const writeFile = __exp["writeFile"];
export const writeFileSync = __exp["writeFileSync"];
export default {
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
    constants,
    copyFile,
    copyFileSync,
    exists,
    existsSync,
    lstat,
    lstatSync,
    mkdir,
    mkdirSync,
    open,
    openSync,
    promises,
    readdir,
    readdirSync,
    readFile,
    readFileSync,
    readlink,
    readlinkSync,
    rename,
    renameSync,
    rmdir,
    rmdirSync,
    stat,
    statSync,
    unlink,
    unlinkSync,
    watch,
    writeFile,
    writeFileSync
};
