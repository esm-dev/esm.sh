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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
			.expect("could not parse exports");
		assert_eq!(exports.join(","), "foo");
	}

	#[test]
	fn parse_cjs_exports_case_12_2() {
		let source = r#"
			~function() {
				module.exports = { foo: 'bar' }
			}()
		"#;
		let swc = SWC::parse("index.cjs", source).expect("could not parse module");
		let (exports, _) = swc
			.parse_cjs_exports("development")
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
			.parse_cjs_exports("development")
			.expect("could not parse exports");
		assert_eq!(exports.join(","), "foo");
	}
}
