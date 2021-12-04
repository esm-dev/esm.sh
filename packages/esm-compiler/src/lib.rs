mod error;
mod import_map;
mod resolve_fold;
mod resolver;
mod source_type;
mod swc;

#[cfg(test)]
mod tests;

use import_map::ImportHashMap;
use resolver::{DependencyDescriptor, InlineStyle, ReactOptions, Resolver};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::{cell::RefCell, rc::Rc};
use swc::{EmitOptions, SWC};
use wasm_bindgen::prelude::{wasm_bindgen, JsValue};

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub struct Options {
	#[serde(default)]
	pub is_dev: bool,

	#[serde(default)]
	pub import_map: ImportHashMap,

	#[serde(default)]
	pub react: Option<ReactOptions>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TransformOutput {
	pub code: String,

	#[serde(skip_serializing_if = "Vec::is_empty")]
	pub deps: Vec<DependencyDescriptor>,

	#[serde(skip_serializing_if = "HashMap::is_empty")]
	pub jsx_inline_styles: HashMap<String, InlineStyle>,

	#[serde(skip_serializing_if = "Option::is_none")]
	pub map: Option<String>,
}

#[wasm_bindgen(js_name = "transformSync")]
pub fn transform_sync(specifier: &str, code: &str, options: JsValue) -> Result<JsValue, JsValue> {
	console_error_panic_hook::set_once();

	let options: Options = options
		.into_serde()
		.map_err(|err| format!("failed to parse options: {}", err))
		.unwrap();
	let jsx_import_source =	if options.import_map.imports.contains_key(options.import_map.jsx.as_str()) {
		options.import_map.imports.get(options.import_map.jsx.as_str()).unwrap().into()
	} else {
		options.import_map.jsx.clone()
	};
	let resolver = Rc::new(RefCell::new(Resolver::new(
		specifier,
		options.import_map,
		options.react,
	)));
	let module = SWC::parse(specifier, code).expect("could not parse the module");
	let (code, map) = module
		.transform(
			resolver.clone(),
			&EmitOptions {
				jsx_import_source,
				is_dev: options.is_dev,
			},
		)
		.expect("could not transform the module");
	let r = resolver.borrow();

	Ok(
		JsValue::from_serde(&TransformOutput {
			code,
			deps: r.deps.clone(),
			jsx_inline_styles: r.jsx_inline_styles.clone(),
			map,
		})
		.unwrap(),
	)
}
