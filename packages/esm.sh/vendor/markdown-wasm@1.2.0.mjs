import { loadWasm } from "./wasm-loader.mjs";

const wasmURL = "https://esm.sh/markdown-wasm@1.2.0/dist/markdown.wasm";

var e,
  n,
  o,
  a,
  d,
  m,
  h,
  w,
  g,
  A,
  E,
  b,
  _,
  R,
  S,
  I,
  T,
  U,
  W,
  M,
  H,
  P,
  F,
  O,
  k,
  B,
  N,
  x,
  initSync;
function D(e) {
  throw new Error("wasm abort" + (e ? ": " + (e.stack || e) : ""));
}
n = {
  preRun: [],
  postRun: [],
  print: console.log.bind(console),
  printErr: console.error.bind(console),
};
function G(e) {
  m.delete(_.get(e)), d.push(e);
}
function K(e, n) {
  return function (e, n) {
    var t, r, o, i;
    if (!m) {
      for (m = new WeakMap(), t = 0; t < _.length; t++) {
        (r = _.get(t)) && m.set(r, t);
      }
    }
    if (m.has(e)) return m.get(e);
    o = function () {
      if (d.length) return d.pop();
      try {
        _.grow(1);
      } catch (e) {
        if (!(e instanceof RangeError)) throw e;
        throw "Unable to grow wasm table. Set ALLOW_TABLE_GROWTH.";
      }
      return _.length - 1;
    }();
    try {
      _.set(o, e);
    } catch (t) {
      if (!(t instanceof TypeError)) throw t;
      i = function (e, n) {
        var t, r, o, i, u, a, s, f, c;
        if ("function" == typeof WebAssembly.Function) {
          for (
            t = { i: "i32", j: "i64", f: "f32", d: "f64" },
              r = { parameters: [], results: "v" == n[0] ? [] : [t[n[0]]] },
              o = 1;
            o < n.length;
            ++o
          ) r.parameters.push(t[n[o]]);
          return new WebAssembly.Function(r, e);
        }
        for (
          i = [1, 0, 1, 96],
            u = n.slice(0, 1),
            a = n.slice(1),
            s = { i: 127, j: 126, f: 125, d: 124 },
            i.push(a.length),
            o = 0;
          o < a.length;
          ++o
        ) i.push(s[a[o]]);
        return "v" == u ? i.push(0) : i = i.concat([1, s[u]]),
          i[1] = i.length - 2,
          f = new Uint8Array(
            [0, 97, 115, 109, 1, 0, 0, 0].concat(i, [
              2,
              7,
              1,
              1,
              101,
              1,
              102,
              0,
              0,
              7,
              5,
              1,
              1,
              102,
              0,
              0,
            ]),
          ),
          c = new WebAssembly.Module(f),
          new WebAssembly.Instance(c, { e: { f: e } }).exports.f;
      }(e, n), _.set(o, i);
    }
    return m.set(e, o), o;
  }(e, n);
}
function X(e) {
  g = e,
    n.HEAP8 = new Int8Array(e),
    n.HEAP16 = new Int16Array(e),
    n.HEAP32 = E = new Int32Array(e),
    n.HEAPU8 = A = new Uint8Array(e),
    n.HEAPU16 = new Uint16Array(e),
    n.HEAPU32 = b = new Uint32Array(e),
    n.HEAPF32 = new Float32Array(e),
    n.HEAPF64 = new Float64Array(e);
}
function z(e) {
  return e.startsWith(U);
}
function $(e) {
  for (var t, r; e.length > 0;) {
    "function" != typeof (t = e.shift())
      ? "number" == typeof (r = t.func)
        ? void 0 === t.arg ? _.get(r)() : _.get(r)(t.arg)
        : r(void 0 === t.arg ? null : t.arg)
      : t(n);
  }
}
function Y(e) {
  try {
    return h.grow(e - g.byteLength + 65535 >>> 16), X(h.buffer), 1;
  } catch (e) {}
}
function Q(e) {
  function t() {
    N ||
      (N = !0,
        n.calledRun = !0,
        w ||
        ($(S),
          n.onRuntimeInitialized && n.onRuntimeInitialized(),
          function () {
            if (n.postRun) {
              for (
                "function" == typeof n.postRun && (n.postRun = [n.postRun]);
                n.postRun.length;
              ) e = n.postRun.shift(), I.unshift(e);
            }
            var e;
            $(I);
          }()));
  }
  e = e || o,
    T > 0 || (function () {
      if (n.preRun) {
        for (
          "function" == typeof n.preRun && (n.preRun = [n.preRun]);
          n.preRun.length;
        ) e = n.preRun.shift(), R.unshift(e);
      }
      var e;
      $(R);
    }(),
      T > 0 ||
      (n.setStatus
        ? (n.setStatus("Running..."),
          setTimeout(function () {
            setTimeout(function () {
              n.setStatus("");
            }, 1), t();
          }, 1))
        : t()));
}
{
  n.arguments && (o = n.arguments),
    n.thisProgram && n.thisProgram,
    n.quit && n.quit,
    d = [],
    n.wasmBinary && (y = n.wasmBinary),
    n.noExitRuntime || !0,
    "object" != typeof WebAssembly && D("no native wasm support detected"),
    w = !1,
    n.INITIAL_MEMORY,
    R = [],
    S = [],
    I = [],
    0,
    T = 0,
    U = "data:application/octet-stream;base64,",
    z(W = "markdown.wasm") ||
    (x = W, W = n.locateFile ? n.locateFile(x, a) : a + x),
    M = {
      a: function (e) {
        var n, t, r, o, i = A.length;
        if ((e >>>= 0) > 2147483648) return !1;
        for (n = 1; n <= 4; n *= 2) {
          if (
            t = i * (1 + .2 / n),
              t = Math.min(t, e + 100663296),
              Y(Math.min(
                2147483648,
                ((r = Math.max(e, t)) % (o = 65536) > 0 && (r += o - r % o), r),
              ))
          ) return !0;
        }
        return !1;
      },
    },
    initSync = (e) => {
      var r, o = e.exports;
      n.asm = o,
        X((h = n.asm.b).buffer),
        _ = n.asm.i,
        r = n.asm.c,
        S.unshift(r);
    },
    n.___wasm_call_ctors = function () {
      return (n.___wasm_call_ctors = n.asm.c).apply(null, arguments);
    },
    H = n._wrealloc = function () {
      return (H = n._wrealloc = n.asm.d).apply(null, arguments);
    },
    P = n._wfree = function () {
      return (P = n._wfree = n.asm.e).apply(null, arguments);
    },
    F = n._WErrGetCode = function () {
      return (F = n._WErrGetCode = n.asm.f).apply(null, arguments);
    },
    O = n._WErrGetMsg = function () {
      return (O = n._WErrGetMsg = n.asm.g).apply(null, arguments);
    },
    k = n._WErrClear = function () {
      return (k = n._WErrClear = n.asm.h).apply(null, arguments);
    },
    B = n._parseUTF8 = function () {
      return (B = n._parseUTF8 = n.asm.j).apply(null, arguments);
    },
    n.addFunction = K,
    n.removeFunction = G,
    n.run = Q;
}
Q(), n.inspect = () => "[asm]", void 0 !== e && (module = e, e = void 0);
class WError extends Error {
  constructor(e, n, t, r) {
    super(n, t || "wasm", r || 0), this.name = "WError", this.code = e;
  }
}
function Z(e, n) {
  const t = H(0, n);
  return A.set(e, t), t;
}
let ee = 0;
n.postRun.push(() => {
  ee = H(0, 4);
});
const ne = (() => {
    const e = new TextEncoder("utf-8"), n = new TextDecoder("utf-8");
    return { encode: (n) => e.encode(n), decode: (e) => n.decode(e) };
  })(),
  re = {
    COLLAPSE_WHITESPACE: 1,
    PERMISSIVE_ATX_HEADERS: 2,
    PERMISSIVE_URL_AUTO_LINKS: 4,
    PERMISSIVE_EMAIL_AUTO_LINKS: 8,
    NO_INDENTED_CODE_BLOCKS: 16,
    NO_HTML_BLOCKS: 32,
    NO_HTML_SPANS: 64,
    TABLES: 256,
    STRIKETHROUGH: 512,
    PERMISSIVE_WWW_AUTOLINKS: 1024,
    TASK_LISTS: 2048,
    LATEX_MATH_SPANS: 4096,
    WIKI_LINKS: 8192,
    UNDERLINE: 16384,
    DEFAULT: 2823,
    NO_HTML: 96,
  },
  oe = { HTML: 1, XHTML: 2, AllowJSURI: 4 };
function ie(e, n) {
  let t = void 0 === (n = n || {}).parseFlags ? re.DEFAULT : n.parseFlags,
    r = n.allowJSURIs ? oe.AllowJSURI : 0;
  switch (n.format) {
    case "xhtml":
      r |= oe.HTML | oe.XHTML;
      break;
    case "html":
    case void 0:
    case null:
    case "":
      r |= oe.HTML;
      break;
    default:
      throw new Error(`invalid format "${n.format}"`);
  }
  let o = n.onCodeBlock
    ? (i = n.onCodeBlock,
      K(function (e, n, t, r, o) {
        try {
          const u = n > 0 ? ne.decode(A.subarray(e, e + n)) : "",
            a = A.subarray(t, t + r);
          let s = void 0;
          a.toString = () => s || (s = ne.decode(a));
          let f = null;
          if (null === (f = i(u, a)) || void 0 === f) return -1;
          let c = ue(f);
          if (c.length > 0) {
            const e = Z(c, c.length);
            b[o >> 2] = e;
          }
          return c.length;
        } catch (e) {
          return console.error(
            `error in markdown onCodeBlock callback: ${e.stack || e}`,
          ),
            -1;
        }
      }, "iiiiii"))
    : 0;
  var i;
  let u = ue(e),
    a = function (e) {
      let n = e(ee), t = E[ee >> 2];
      if (0 == t) return null;
      let r = A.subarray(t, t + n);
      return r.heapAddr = t, r;
    }((e) =>
      (function (e, n) {
        const t = function (e) {
            return e instanceof Uint8Array ? e : new Uint8Array(e);
          }(e),
          r = t.length,
          o = Z(t, r),
          i = n(o, r);
        return function (e) {
          P(e);
        }(o),
          i;
      })(u, (n, i) => B(n, i, t, r, e, o))
    );
  return n.onCodeBlock && G(o),
    function () {
      let e = function () {
        let e = F();
        if (0 != e) {
          let n = O(), t = 0 != n ? UTF8ArrayToString(A, n) : "";
          return k(), new WError(e, t);
        }
      }();
      if (e) throw e;
    }(),
    n.bytes || n.asMemoryView ? a : ne.decode(a);
}
function ue(e) {
  return "string" == typeof e
    ? ne.encode(e)
    : e instanceof Uint8Array
    ? e
    : new Uint8Array(e);
}

export { ie as parse, re as ParseFlags };

export default async function init() {
  initSync(await loadWasm(wasmURL, { a: M }));
}

if (import.meta.main) {
  init().then(() => {
    console.log(ie("# Hello, world!"));
  });
}
