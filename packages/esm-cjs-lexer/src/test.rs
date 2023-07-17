#[cfg(test)]
mod tests {
  use crate::swc::SWC;

  #[test]
  fn parse_cjs_exports_case_1() {
    let source = r#"
      const c = 'c'
      Object.defineProperty(exports, 'a', { value: true })
      Object.defineProperty(exports, 'b', { get: () => true })
      Object.defineProperty(exports, c, { get() { return true } })
      Object.defineProperty(exports, 'd', { "value": true })
      Object.defineProperty(exports, 'e', { "get": () => true })
      Object.defineProperty(exports, 'f', {})
      Object.defineProperty(module.exports, '__esModule', { value: true })
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "a,b,c,d,e,__esModule")
  }

  #[test]
  fn parse_cjs_exports_case_2() {
    let source = r#"
      const alas = true
      const obj = { bar: 123 }
      Object.defineProperty(exports, 'nope', { value: true })
      Object.defineProperty(module, 'exports', { value: { alas, foo: 'bar', ...obj, ...require('a'), ...require('b') } })
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "alas,foo,bar");
    assert_eq!(reexports.join(","), "a,b");
  }

  #[test]
  fn parse_cjs_exports_case_3() {
    let source = r#"
      const alas = true
      const obj = { bar: 1 }
      obj.meta = 1
      Object.assign(module.exports, { alas, foo: 'bar', ...obj }, { ...require('a') }, require('b'))
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "alas,foo,bar,meta");
    assert_eq!(reexports.join(","), "a,b");
  }

  #[test]
  fn parse_cjs_exports_case_4() {
    let source = r#"
      Object.assign(module.exports, { foo: 'bar', ...require('lib') })
      Object.assign(module, { exports: { nope: true } })
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "nope");
    assert_eq!(reexports.join(","), "");
  }

  #[test]
  fn parse_cjs_exports_case_5() {
    let source = r#"
      exports.foo = 'bar'
      module.exports.bar = 123
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_6() {
    let source = r#"
      const alas = true
      const obj = { boom: 1 }
      obj.coco = 1
      exports.foo = 'bar'
      module.exports.bar = 123
      module.exports = { alas,  ...obj, ...require('a'), ...require('b') }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "alas,boom,coco");
    assert_eq!(reexports.join(","), "a,b");
  }

  #[test]
  fn parse_cjs_exports_case_7() {
    let source = r#"
      exports['foo'] = 'bar'
      module['exports']['bar'] = 123
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_8() {
    let source = r#"
      module.exports = function() {}
      module.exports.foo = 'bar';
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_9() {
    let source = r#"
      module.exports = require("lib")
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (_, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(reexports.join(","), "lib");
  }

  #[test]
  fn parse_cjs_exports_case_9_1() {
    let source = r#"
      var lib = require("lib")
      module.exports = lib
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (_, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(reexports.join(","), "lib");
  }

  #[test]
  fn parse_cjs_exports_case_10() {
    let source = r#"
      function Module() {}
      Module.foo = 'bar'
      module.exports = Module
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_10_1() {
    let source = r#"
      let Module = function () {}
      Module.foo = 'bar'
      module.exports = Module
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_10_2() {
    let source = r#"
      let Module = () => {}
      Module.foo = 'bar'
      module.exports = Module
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_11() {
    let source = r#"
      class Module {
        static foo = 'bar'
        static greet() {}
        alas = true
        boom() {}
      }
      module.exports = Module
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,greet");
  }

  #[test]
  fn parse_cjs_exports_case_12() {
    let source = r#"
      (function() {
        module.exports = { foo: 'bar' }
      })()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_12_1() {
    let source = r#"
      (() => {
        module.exports = { foo: 'bar' }
      })()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_12_2() {
    let source = r#"
      (function() {
        module.exports = { foo: 'bar' }
      }())
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_12_3() {
    let source = r#"
      ~function() {
        module.exports = { foo: 'bar' }
      }()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_12_4() {
    let source = r#"
      let es = { foo: 'bar' };
      (function() {
        module.exports = es
      })()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_13() {
    let source = r#"
      {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_13_1() {
    let source = r#"
      const obj1 = { foo: 'bar' }
      {
        const obj2 = { bar: 123 }
        module.exports = { ...obj1, ...obj2 }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_14() {
    let source = r#"
      if (process.env.NODE_ENV === 'development') {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_14_1() {
    let source = r#"
      const { NODE_ENV } = process.env
      if (NODE_ENV === 'development') {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_14_2() {
    let source = r#"
      const { NODE_ENV: denv } = process.env
      if (denv === 'development') {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_14_3() {
    let source = r#"
      const denv = process.env.NODE_ENV
      if (denv === 'development') {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_14_4() {
    let source = r#"
      if (process.env.NODE_ENV !== 'development') {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "");
  }
  #[test]
  fn parse_cjs_exports_case_14_5() {
    let source = r#"
      if (typeof module !== 'undefined' && module.exports) {
        module.exports = { foo: 'bar' }
      }
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_15() {
    let source = r#"
      let es = { foo: 'bar' };
      (function() {
        const { NODE_ENV } = process.env
        es.bar = 123
        if (NODE_ENV === 'development') {
          module.exports = es
        }
      })()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_16() {
    let source = r#"
      function fn() { return { foo: 'bar' } };
      module.exports = fn()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_16_1() {
    let source = r#"
      let fn = () => ({ foo: 'bar' });
      module.exports = fn()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_16_2() {
    let source = r#"
      function fn() {
        const mod = { foo: 'bar' }
        mod.bar = 123
        return mod
      };
      module.exports = fn()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_17() {
    let source = r#"
      module.exports = require("lib")()
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (_, reexports) = swc
      .parse_cjs_exports("development", false)
      .expect("could not parse exports");
    assert_eq!(reexports.join(","), "lib()");
  }

  #[test]
  fn parse_cjs_exports_case_18() {
    let source = r#"
      module.exports = function () {
        const mod = { foo: 'bar' }
        mod.bar = 123
        return mod
      };
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_18_1() {
    let source = r#"
      function fn() {
        const mod = { foo: 'bar' }
        mod.bar = 123
        return mod
      }
      module.exports = fn;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_18_2() {
    let source = r#"
      const fn = () => {
        const mod = { foo: 'bar' }
        mod.bar = 123
        return mod
      }
      module.exports = fn;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("development", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_18_3() {
    let source = r#"
      function fn() {
        const { NODE_ENV } = process.env
        const mod = { foo: 'bar' }
        if (NODE_ENV === 'production') {
          return mod
        }
        mod.bar = 123
        return mod
      }
      module.exports = fn;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_18_4() {
    let source = r#"
      function fn() {
        const { NODE_ENV } = process.env
        const mod = { foo: 'bar' }
        if (NODE_ENV === 'development') {
          return mod
        }
        mod.bar = 123
        return mod
      }
      module.exports = fn;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_19() {
    let source = r#"
      require("tslib").__exportStar({foo: 'bar'}, exports)
      exports.bar = 123
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_19_2() {
    let source = r#"
      const tslib = require("tslib");
      (0, tslib.__exportStar)({foo: 'bar'}, exports)
      exports.bar = 123
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_19_3() {
    let source = r#"
      const { __exportStar } = require("tslib");
      (0, __exportStar)({foo: 'bar'}, exports)
      exports.bar = 123
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }

  #[test]
  fn parse_cjs_exports_case_19_4() {
    let source = r#"
      var tslib_1 = require("tslib");
      (0, tslib_1.__exportStar)(require("./crossPlatformSha256"), exports);
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (_, reexorts) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(reexorts.join(","), "./crossPlatformSha256");
  }

  #[test]
  fn parse_cjs_exports_case_19_5() {
    let source = r#"
      var __exportStar = function() {}
      Object.defineProperty(exports, "foo", { value: 1 });
      __exportStar(require("./bar"), exports);
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexorts) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
    assert_eq!(reexorts.join(","), "./bar");
  }

  #[test]
  fn parse_cjs_exports_case_20_1() {
    let source = r#"
      var foo;
      foo = exports.foo || (exports.foo = {});
      var  bar = exports.bar || (exports.bar = {});
      exports.greet = 123;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar,greet");
  }

  #[test]
  fn parse_cjs_exports_case_20_2() {
    let source = r#"
      var bar;
      ((foo, bar) => { })(exports.foo || (exports.foo = {}), bar = exports.bar || (exports.bar = {}));
      exports.greet = 123;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar,greet");
  }

  #[test]
  fn parse_cjs_exports_case_21_1() {
    let source = r#"
      (function (global, factory) {
        typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports) :
        typeof define === 'function' && define.amd ? define(['exports'], factory) :
        (factory((global.MMDParser = global.MMDParser || {})));
      }(this, function (exports) {
        exports.foo = "bar";
        Object.defineProperty(exports, '__esModule', { value: true });
      }))
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,__esModule");
  }

  #[test]
  fn parse_cjs_exports_case_21_2() {
    let source = r#"
      (function (global, factory) {
        typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports) :
        typeof define === 'function' && define.amd ? define(['exports'], factory) :
        (factory((global.MMDParser = global.MMDParser || {})));
      }(this, (function (exports) {
        exports.foo = "bar";
        Object.defineProperty(exports, '__esModule', { value: true });
      })))
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,__esModule");
  }

  #[test]
  fn parse_cjs_exports_case_21_3() {
    // Webpack 4 minified UMD output after replacing https://github.com/glennflanagan/react-collapsible/blob/1b987617fe7c20337977a0a877574263ed7ed657/src/Collapsible.js#L1
    // with:
    // ```
    // export const named = 'named-export';

    // export default 'default-export';
    // ```
    // Manually formatted to avoid ast changes from prettier
    let source = r#"
    !function (e, t) { 
      if ("object" == typeof exports && "object" == typeof module) module.exports = t(); 
      else if ("function" == typeof define && define.amd) define([], t); 
      else { var r = t(); for (var n in r) ("object" == typeof exports ? exports : e)[n] = r[n] } 
    }(this, (function () { 
      return function (e) { 
        var t = {}; 
        function r(n) { 
          if (t[n]) return t[n].exports; 
          var o = t[n] = { i: n, l: !1, exports: {} }; 
          return e[n].call(o.exports, o, o.exports, r), o.l = !0, o.exports 
        }
        return r.m = e, 
          r.c = t, 
          r.d = function (e, t, n) { 
            r.o(e, t) || Object.defineProperty(e, t, { enumerable: !0, get: n }) 
          }, 
          r.r = function (e) { 
            "undefined" != typeof Symbol && 
            Symbol.toStringTag && 
            Object.defineProperty(e, Symbol.toStringTag, { value: "Module" }), 
            Object.defineProperty(e, "__esModule", { value: !0 }) 
          }, 
          r.t = function (e, t) { 
            if (1 & t && (e = r(e)), 8 & t) return e; 
            if (4 & t && "object" == typeof e && e && e.__esModule) return e; 
            var n = Object.create(null); 
            if (
              r.r(n), 
              Object.defineProperty(n, "default", { enumerable: !0, value: e }), 
              2 & t && 
              "string" != typeof e
            ) for (var o in e) r.d(n, o, function (t) { return e[t] }.bind(null, o)); 
            return n 
          }, 
          r.n = function (e) { 
            var t = e && e.__esModule ? 
              function () { return e.default } : 
              function () { return e };
            return r.d(t, "a", t), t 
          }, 
          r.o = function (e, t) { 
            return Object.prototype.hasOwnProperty.call(e, t)
          }, 
          r.p = "", r(r.s = 0) 
        }([
          function (e, t, r) { 
            "use strict"; 
            r.r(t), r.d(t, "named", (function () { return n })); 
            var n = "named-export"; 
            t.default = "default-export";
          }
        ]) 
      }));
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,named,default");
  }

  #[test]
  fn parse_cjs_exports_case_21_4() {
    // Webpack 5 minified UMD output after replacing https://github.com/amrlabib/react-timer-hook/blob/46aad2022d5cfa69bb24d1f4a20a94c774ea13d7/src/index.js#L1:
    // with:
    // ```
    // export const named1 = 'named-export-1';
    // export const named2 = 'named-export-2';

    // export default 'default-export';
    // ```
    // Manually formatted to avoid ast changes from prettier
    let source = r#"
    !function (e, t) {
      "object" == typeof exports && "object" == typeof module ?
        module.exports = t() :
        "function" == typeof define && define.amd ?
          define([], t) :
          "object" == typeof exports ?
            exports["react-timer-hook"] = t() :
            e["react-timer-hook"] = t()
    }(
      "undefined" != typeof self ? self : this,
      (() =>
        (() => {
          "use strict";
          var e = {
            d: (t, o) => {
              for (var r in o)
                e.o(o, r) && !e.o(t, r) && Object.defineProperty(t, r, { enumerable: !0, get: o[r] })
            },
            o: (e, t) => Object.prototype.hasOwnProperty.call(e, t),
            r: e => {
              "undefined" != typeof Symbol && Symbol.toStringTag && Object.defineProperty(e, Symbol.toStringTag, { value: "Module" }),
                Object.defineProperty(e, "__esModule", { value: !0 })
            }
          },
            t = {};
          e.r(t),
            e.d(t, {
              default: () => n,
              named1: () => o,
              named2: () => r
            });
          const o = "named-export-1",
            r = "named-export-2",
            n = "default-export";
          return t
        })()
      ));
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,default,named1,named2");
  }

  #[test]
  fn parse_cjs_exports_case_21_5() {
    // Webpack 5 minified UMD output after replacing https://github.com/amrlabib/react-timer-hook/blob/46aad2022d5cfa69bb24d1f4a20a94c774ea13d7/src/index.js#L1:
    // with:
    // ```
    // import { useEffect } from "react";

    // export default function useFn() {
    //   useEffect(() => {}, []);
    // }

    // export const named1 = "named-export-1";
    // export const named2 = "named-export-2";
    // ```
    // Formatted with Deno to avoid ast changes from prettier
    let source = r#"
    !function (e, t) {
      "object" == typeof exports && "object" == typeof module
        ? module.exports = t(require("react"))
        : "function" == typeof define && define.amd
        ? define(["react"], t)
        : "object" == typeof exports
        ? exports["react-timer-hook"] = t(require("react"))
        : e["react-timer-hook"] = t(e.react);
    }("undefined" != typeof self ? self : this, (e) =>
      (() => {
        "use strict";
        var t = {
            156: (t) => {
              t.exports = e;
            },
          },
          r = {};
        function o(e) {
          var n = r[e];
          if (void 0 !== n) return n.exports;
          var a = r[e] = { exports: {} };
          return t[e](a, a.exports, o), a.exports;
        }
        o.n = (e) => {
          var t = e && e.__esModule ? () => e.default : () => e;
          return o.d(t, { a: t }), t;
        },
          o.d = (e, t) => {
            for (var r in t) {
              o.o(t, r) && !o.o(e, r) &&
                Object.defineProperty(e, r, { enumerable: !0, get: t[r] });
            }
          },
          o.o = (e, t) => Object.prototype.hasOwnProperty.call(e, t),
          o.r = (e) => {
            "undefined" != typeof Symbol && Symbol.toStringTag &&
            Object.defineProperty(e, Symbol.toStringTag, { value: "Module" }),
              Object.defineProperty(e, "__esModule", { value: !0 });
          };
        var n = {};
        return (() => {
          o.r(n), o.d(n, { default: () => t, named1: () => r, named2: () => a });
          var e = o(156);
          function t() {
            (0, e.useEffect)(() => {}, []);
          }
          const r = "named-export-1", a = "named-export-2";
        })(),
          n;
      })());    
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,default,named1,named2");
  }

  #[test]
  fn parse_cjs_exports_case_21_6() {
    // Webpack 5 minified UMD output after replacing https://github.com/xmtp/xmtp-js-content-types/blob/c3cac4842c98785a0436240148f228c654c919c8/remote-attachment/src/index.ts
    // with:
    // ```
    // export const named1 = 'named-export-1';
    // export const named2 = 'named-export-2';

    // export default 'default-export';
    // ```
    // Formatted with Deno to avoid ast changes from prettier
    let source = r#"
    !function (e, t) {
      "object" == typeof exports && "object" == typeof module
        ? module.exports = t()
        : "function" == typeof define && define.amd
        ? define([], t)
        : "object" == typeof exports
        ? exports.xmtp = t()
        : e.xmtp = t();
    }(this, () =>
      (() => {
        "use strict";
        var e = {};
        return (() => {
          var t = e;
          Object.defineProperty(t, "__esModule", { value: !0 }),
            t.named2 = t.named1 = void 0,
            t.named1 = "named-export-1",
            t.named2 = "named-export-2",
            t.default = "default-export";
        })(),
          e;
      })());    
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,named2,named1,default");
  }

  #[test]
  fn parse_cjs_exports_case_21_7() {
    // Webpack 5 minified UMD output after replacing https://github.com/xmtp/xmtp-js-content-types/blob/c3cac4842c98785a0436240148f228c654c919c8/remote-attachment/src/index.ts
    // with:
    // ```
    // export const named1 = 'named-export-1';
    // export const named2 = 'named-export-2';

    // export default 'default-export';
    // ```
    // Formatted with Deno to avoid ast changes from prettier
    let source = r#"
    !function (e, t) {
      "object" == typeof exports && "object" == typeof module
        ? module.exports = t()
        : "function" == typeof define && define.amd
        ? define([], t)
        : "object" == typeof exports
        ? exports.xmtp = t()
        : e.xmtp = t();
    }(this, () =>
      (() => {
        "use strict";
        var e = {};
        return (() => {
          "use strict";
          var t = e;
          Object.defineProperty(t, "__esModule", { value: !0 }),
            t.named2 = t.named1 = void 0,
            t.named1 = "named-export-1",
            t.named2 = "named-export-2",
            t.default = "default-export";
        })(),
          e;
      })());    
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,named2,named1,default");
  }

  #[test]
  fn parse_cjs_exports_case_21_8() {
    // Rollup 2 minified UMD output after replacing https://github.com/Hacker0x01/react-datepicker/blob/0c13f35e11577ca0979c363a19ea2c1f34bfdf1f/src/index.jsx#L1
    // with:
    // ```
    // import { useEffect } from "react";

    // export default function useFn() {
    //   useEffect(() => {}, []);
    // }

    // export const named1 = "named-export-1";
    // export const named2 = "named-export-2";
    // ```
    // Formatted with Deno to avoid ast changes from prettier
    let source = r#"
    !function (e, t) {
      "object" == typeof exports && "undefined" != typeof module
        ? t(exports, require("react"))
        : "function" == typeof define && define.amd
        ? define(["exports", "react"], t)
        : t(
          (e = "undefined" != typeof globalThis ? globalThis : e || self)
            .DatePicker = {},
          e.React,
        );
    }(this, function (e, t) {
      "use strict";
      e.default = function () {
        t.useEffect(function () {}, []);
      },
        e.named1 = "named-export-1",
        e.named2 = "named-export-2",
        Object.defineProperty(e, "__esModule", { value: !0 });
    });
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,named2,named1,default");
  }

  #[test]
  fn parse_cjs_exports_case_22() {
    let source = r#"
      var url = module.exports = {};
      url.foo = 'bar';
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo");
  }

  #[test]
  fn parse_cjs_exports_case_22_2() {
    let source = r#"
      exports.i18n = exports.use = exports.t = undefined;
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "i18n,use,t");
  }

  #[test]
  fn parse_cjs_exports_case_23() {
    let source = r#"
      Object.defineProperty(exports, "__esModule", { value: true });
      __export({foo:"bar"});
      __export(require("./lib"));
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, reexports) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "__esModule,foo");
    assert_eq!(reexports.join(","), "./lib");
  }

  #[test]
  fn parse_cjs_exports_case_24() {
    let source = r#"
    0 && (module.exports = {
      foo,
      bar
    });
    "#;
    let swc = SWC::parse("index.cjs", source).expect("could not parse module");
    let (exports, _) = swc
      .parse_cjs_exports("production", true)
      .expect("could not parse exports");
    assert_eq!(exports.join(","), "foo,bar");
  }
}
