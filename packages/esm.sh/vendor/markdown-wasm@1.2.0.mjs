import { loadWasm } from "./wasm-loader.mjs";

const wasmURL = "https://esm.sh/markdown-wasm@1.2.0/dist/markdown.wasm";

var U, t, T, y, H, v, g, C, F, d, k, x, M, b, W, P, R, N, D;
t = {};
function X(e) {
  y.delete(d.get(e)), T.push(e);
}
function j(e, r) {
  return function (n, f) {
    var u, m, h, p;
    if (!y) {
      for (y = new WeakMap(), u = 0; u < d.length; u++) {
        (m = d.get(u)) && y.set(m, u);
      }
    }
    if (y.has(n)) return y.get(n);
    h = function () {
      if (T.length) return T.pop();
      try {
        d.grow(1);
      } catch (l) {
        throw l instanceof RangeError
          ? "Unable to grow wasm table. Set ALLOW_TABLE_GROWTH."
          : l;
      }
      return d.length - 1;
    }();
    try {
      d.set(h, n);
    } catch (l) {
      if (!(l instanceof TypeError)) throw l;
      p = function (c, a) {
        var i, _, o, s, A, E, w, S, O;
        if (typeof WebAssembly.Function == "function") {
          for (
            i = { i: "i32", j: "i64", f: "f32", d: "f64" },
              _ = { parameters: [], results: a[0] == "v" ? [] : [i[a[0]]] },
              o = 1;
            o < a.length;
            ++o
          ) _.parameters.push(i[a[o]]);
          return new WebAssembly.Function(_, c);
        }
        for (
          s = [1, 0, 1, 96],
            A = a.slice(0, 1),
            E = a.slice(1),
            w = { i: 127, j: 126, f: 125, d: 124 },
            s.push(E.length),
            o = 0;
          o < E.length;
          ++o
        ) s.push(w[E[o]]);
        return A == "v" ? s.push(0) : s = s.concat([1, w[A]]),
          s[1] = s.length - 2,
          S = new Uint8Array(
            [0, 97, 115, 109, 1, 0, 0, 0].concat(s, [
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
          O = new WebAssembly.Module(S),
          new WebAssembly.Instance(O, { e: { f: c } }).exports.f;
      }(n, f), d.set(h, p);
    }
    return y.set(n, h), h;
  }(e, r);
}
function K(e) {
  v = e,
    t.HEAP32 = C = new Int32Array(e),
    t.HEAPU8 = g = new Uint8Array(e),
    t.HEAPU32 = F = new Uint32Array(e);
}
function $(e) {
  try {
    return H.grow(e - v.byteLength + 65535 >>> 16), K(H.buffer), 1;
  } catch {}
}
T = [],
  k = [],
  x = {
    a: function (e) {
      var r, n, f, u, m = g.length;
      if ((e >>>= 0) > 2147483648) return !1;
      for (r = 1; r <= 4; r *= 2) {
        if (
          n = m * (1 + .2 / r),
            n = Math.min(n, e + 100663296),
            $(Math.min(
              2147483648,
              ((f = Math.max(e, n)) % (u = 65536) > 0 && (f += u - f % u), f),
            ))
        ) return !0;
      }
      return !1;
    },
  },
  D = (e) => {
    var r, n = e.exports;
    t.asm = n, K((H = t.asm.b).buffer), d = t.asm.i, r = t.asm.c, k.unshift(r);
  },
  t.___wasm_call_ctors = function () {
    return (t.___wasm_call_ctors = t.asm.c).apply(null, arguments);
  },
  M = t._wrealloc = function () {
    return (M = t._wrealloc = t.asm.d).apply(null, arguments);
  },
  b = t._wfree = function () {
    return (b = t._wfree = t.asm.e).apply(null, arguments);
  },
  W = t._WErrGetCode = function () {
    return (W = t._WErrGetCode = t.asm.f).apply(null, arguments);
  },
  P = t._WErrGetMsg = function () {
    return (P = t._WErrGetMsg = t.asm.g).apply(null, arguments);
  },
  R = t._WErrClear = function () {
    return (R = t._WErrClear = t.asm.h).apply(null, arguments);
  },
  N = t._parseUTF8 = function () {
    return (N = t._parseUTF8 = t.asm.j).apply(null, arguments);
  },
  U !== void 0 && (module = U, U = void 0);
class z extends Error {
  constructor(r, n, f, u) {
    super(n, f || "wasm", u || 0), this.name = "WError", this.code = r;
  }
}
function B(e, r) {
  const n = M(0, r);
  return g.set(e, n), n;
}
let G = 0;
const L = (() => {
    const e = new TextEncoder("utf-8"), r = new TextDecoder("utf-8");
    return { encode: (n) => e.encode(n), decode: (n) => r.decode(n) };
  })(),
  V = {
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
  I = { HTML: 1, XHTML: 2, AllowJSURI: 4 };
function Y(e, r) {
  let n = (r = r || {}).parseFlags === void 0 ? V.DEFAULT : r.parseFlags,
    f = r.allowJSURIs ? I.AllowJSURI : 0;
  switch (r.format) {
    case "xhtml":
      f |= I.HTML | I.XHTML;
      break;
    case "html":
    case void 0:
    case null:
    case "":
      f |= I.HTML;
      break;
    default:
      throw new Error(`invalid format "${r.format}"`);
  }
  let u = r.onCodeBlock
    ? (m = r.onCodeBlock,
      j(function (l, c, a, i, _) {
        try {
          const o = c > 0 ? L.decode(g.subarray(l, l + c)) : "",
            s = g.subarray(a, a + i);
          let A;
          s.toString = () => A || (A = L.decode(s));
          let E = null;
          if ((E = m(o, s)) === null || E === void 0) return -1;
          let w = J(E);
          if (w.length > 0) {
            const S = B(w, w.length);
            F[_ >> 2] = S;
          }
          return w.length;
        } catch (o) {
          return console.error(
            `error in markdown onCodeBlock callback: ${o.stack || o}`,
          ),
            -1;
        }
      }, "iiiiii"))
    : 0;
  var m;
  let h = J(e),
    p = function (l) {
      let c = l(G), a = C[G >> 2];
      if (a == 0) return null;
      let i = g.subarray(a, a + c);
      return i.heapAddr = a, i;
    }((l) =>
      function (c, a) {
        const i = function (A) {
            return A instanceof Uint8Array ? A : new Uint8Array(A);
          }(c),
          _ = i.length,
          o = B(i, _),
          s = a(o, _);
        return function (A) {
          b(A);
        }(o),
          s;
      }(h, (c, a) => N(c, a, n, f, l, u))
    );
  return r.onCodeBlock && X(u),
    function () {
      let l = function () {
        let c = W();
        if (c != 0) {
          let a = P(), i = a != 0 ? UTF8ArrayToString(g, a) : "";
          return R(), new z(c, i);
        }
      }();
      if (l) throw l;
    }(),
    r.bytes || r.asMemoryView ? p : L.decode(p);
}
function J(e) {
  return typeof e == "string"
    ? L.encode(e)
    : e instanceof Uint8Array
    ? e
    : new Uint8Array(e);
}
export { V as ParseFlags, Y as parse };
export default async function init() {
  D(await loadWasm(wasmURL, { a: x }));
}

if (import.meta.main) {
  await init();
  console.log(Y(
    (new TextEncoder()).encode(
      "---\ntitle:Hello World\n---\n# Hello, world!\nhello, world!\nlink: https://example.com\n```js\nconsole.log('hello, world!')\n```\n<foo-bar></foo-bar>\n",
    ),
    {
      parseFlags: V.DEFAULT | V.NO_HTML,
      onCodeBlock: (lang, code) => {
        console.log(lang, code.toString());
        return "// " + code;
      },
    },
  ));
}
