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

#[wasm_bindgen(js_name = "parse")]
pub fn parse(
	specifier: &str,
	code: &str,
	node_env: Option<String>,
	call_mode: Option<bool>,
) -> Result<JsValue, JsValue> {
	console_error_panic_hook::set_once();

	let swc = SWC::parse(specifier, code).expect("could not parse module");
	let node_env = if let Some(env) = node_env {
		env
	} else {
		"production".to_owned()
	};
	let call_mode = if let Some(ok) = call_mode { ok } else { false };
	let (exports, reexports) = swc.parse_cjs_exports(node_env.as_str(), call_mode).unwrap();
	let output = &Output {
		exports: exports,
		reexports: reexports,
	};

	Ok(JsValue::from_serde(output).unwrap())
}
