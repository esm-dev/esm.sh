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
use source_type::SourceType;
use std::collections::HashMap;
use std::{cell::RefCell, rc::Rc};
use swc::{EmitOptions, SWC};
use wasm_bindgen::prelude::{wasm_bindgen, JsValue};

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub struct Options {
	#[serde(default)]
	pub swc_options: SWCOptions,

	#[serde(default)]
	pub is_dev: bool,

	#[serde(default)]
	pub import_map: ImportHashMap,

	#[serde(default)]
	pub react: Option<ReactOptions>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub struct SWCOptions {
	#[serde(default)]
	pub source_type: SourceType,

	#[serde(default)]
	pub jsx_import_source: Option<String>,

	#[serde(default = "default_pragma")]
	pub jsx_factory: String,

	#[serde(default = "default_pragma_frag")]
	pub jsx_fragment_factory: String,
}

impl Default for SWCOptions {
	fn default() -> Self {
		SWCOptions {
			source_type: SourceType::TSX,
			jsx_import_source: None,
			jsx_factory: default_pragma(),
			jsx_fragment_factory: default_pragma_frag(),
		}
	}
}

fn default_pragma() -> String {
	"React.createElement".into()
}

fn default_pragma_frag() -> String {
	"React.Fragment".into()
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TransformOutput {
	pub code: String,

	#[serde(skip_serializing_if = "Vec::is_empty")]
	pub deps: Vec<DependencyDescriptor>,

	#[serde(skip_serializing_if = "Vec::is_empty")]
	pub star_exports: Vec<String>,

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
	let resolver = Rc::new(RefCell::new(Resolver::new(
		specifier,
		options.import_map,
		options.react,
	)));
	let module = SWC::parse(specifier, code, Some(options.swc_options.source_type))
		.expect("could not parse the module");
	let (code, map) = module
		.transform(
			resolver.clone(),
			&EmitOptions {
				jsx_import_source: options.swc_options.jsx_import_source,
				jsx_factory: options.swc_options.jsx_factory,
				jsx_fragment_factory: options.swc_options.jsx_fragment_factory,
				is_dev: options.is_dev,
			},
		)
		.expect("could not transform the module");
	let r = resolver.borrow();

	Ok(
		JsValue::from_serde(&TransformOutput {
			code,
			deps: r.deps.clone(),
			star_exports: r.star_exports.clone(),
			jsx_inline_styles: r.jsx_inline_styles.clone(),
			map,
		})
		.unwrap(),
	)
}
