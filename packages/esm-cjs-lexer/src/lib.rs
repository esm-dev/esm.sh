mod cjs;
mod error;
mod swc;
mod test;

use serde::{Deserialize, Serialize};
use swc::SWC;
use wasm_bindgen::prelude::{wasm_bindgen, JsValue};

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub struct Options {
  node_env: Option<String>,
  call_mode: Option<bool>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Output {
  pub exports: Vec<String>,
  pub reexports: Vec<String>,
}

#[wasm_bindgen(js_name = "parse")]
pub fn parse(specifier: &str, code: &str, options: JsValue) -> Result<JsValue, JsValue> {
  console_error_panic_hook::set_once();

  let options: Options = serde_wasm_bindgen::from_value(options).unwrap_or(Options{
    node_env: None,
    call_mode: None,
  });
  let swc = SWC::parse(specifier, code).expect("could not parse module");
  let node_env = if let Some(env) = options.node_env {
    env
  } else {
    "production".to_owned()
  };
  let call_mode = if let Some(ok) = options.call_mode { ok } else { false };
  let (exports, reexports) = swc.parse_cjs_exports(node_env.as_str(), call_mode).unwrap();
  Ok(
    serde_wasm_bindgen::to_value(&Output {
      exports: exports,
      reexports: reexports,
    })
    .unwrap(),
  )
}
