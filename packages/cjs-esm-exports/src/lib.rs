mod cjs;
mod error;
mod swc;
mod test;

use serde::Serialize;
use swc::SWC;
use wasm_bindgen::prelude::{wasm_bindgen, JsValue};

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Output {
	pub exports: Vec<String>,
	pub reexports: Vec<String>,
}

#[wasm_bindgen(js_name = "parseCjsExportsSync")]
pub fn parse_cjs_exports_sync(
	specifier: &str,
	code: &str,
	node_env: &str,
	call_mode: bool,
) -> Result<JsValue, JsValue> {
	console_error_panic_hook::set_once();

	let swc = SWC::parse(specifier, code).expect("could not parse module");
	let (exports, reexports) = swc.parse_cjs_exports(node_env, call_mode).unwrap();
	let output = &Output {
		exports: exports,
		reexports: reexports,
	};

	Ok(JsValue::from_serde(output).unwrap())
}
