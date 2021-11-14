use crate::import_map::{ImportHashMap, ImportMap};
use path_slash::PathBufExt;
use regex::Regex;
use relative_path::RelativePath;
use serde::{Deserialize, Serialize};
use std::{collections::HashMap, path::PathBuf, str::FromStr};
use url::Url;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DependencyDescriptor {
	pub specifier: String,
	pub is_dynamic: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InlineStyle {
	pub r#type: String,
	pub quasis: Vec<String>,
	pub exprs: Vec<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub struct ReactOptions {
	#[serde(default)]
	pub version: String,
	#[serde(default)]
	pub esm_sh_build_version: usize,
}

/// A Resolver to resolve aleph.js import/export URL.
pub struct Resolver {
	/// the text specifier associated with the import/export statement.
	pub specifier: String,
	/// a flag indicating if the specifier is a remote(http) url.
	pub specifier_is_remote: bool,
	/// a ordered dependencies of the module
	pub deps: Vec<DependencyDescriptor>,
	/// star exports of the module
	pub star_exports: Vec<String>,
	/// parsed jsx inline styles
	pub jsx_inline_styles: HashMap<String, InlineStyle>,

	// internal
	import_map: ImportMap,
	react: Option<ReactOptions>,
}

impl Resolver {
	pub fn new(specifier: &str, import_map: ImportHashMap, react: Option<ReactOptions>) -> Self {
		Resolver {
			specifier: specifier.into(),
			specifier_is_remote: is_remote_url(specifier),
			deps: Vec::new(),
			star_exports: Vec::new(),
			jsx_inline_styles: HashMap::new(),
			import_map: ImportMap::from_hashmap(import_map),
			react,
		}
	}

	/// resolve import/export url.
	pub fn resolve(&mut self, url: &str, is_dynamic: bool) -> String {
		// apply import map
		let url = self.import_map.resolve(self.specifier.as_str(), url);
		let mut fixed_url: String = if is_remote_url(url.as_str()) {
			url.into()
		} else {
			if self.specifier_is_remote {
				let mut new_url = Url::from_str(self.specifier.as_str()).unwrap();
				if url.starts_with("/") {
					new_url.set_path(url.as_str());
				} else {
					let mut buf = PathBuf::from(new_url.path());
					buf.pop();
					buf.push(url);
					let path = "/".to_owned()
						+ RelativePath::new(buf.to_slash().unwrap().as_str())
							.normalize()
							.as_str();
					new_url.set_path(path.as_str());
				}
				new_url.as_str().into()
			} else {
				if url.starts_with("/") {
					url.into()
				} else {
					let mut buf = PathBuf::from(self.specifier.as_str());
					buf.pop();
					buf.push(url);
					"/".to_owned()
						+ RelativePath::new(buf.to_slash().unwrap().as_str())
							.normalize()
							.as_str()
				}
			}
		};

		// fix react/react-dom url
		if let Some(react) = &self.react {
			let re_react_url =
				Regex::new(r"^https?://(esm\.sh|cdn\.esm\.sh)(/v\d+)?/react(\-dom)?(@[^/]+)?(/.*)?$")
					.unwrap();
			if re_react_url.is_match(fixed_url.as_str()) {
				let caps = re_react_url.captures(fixed_url.as_str()).unwrap();
				let mut host = caps.get(1).map_or("", |m| m.as_str());
				let build_version = caps
					.get(2)
					.map_or("", |m| m.as_str().strip_prefix("/v").unwrap());
				let dom = caps.get(3).map_or("", |m| m.as_str());
				let ver = caps.get(4).map_or("", |m| m.as_str());
				let path = caps.get(5).map_or("", |m| m.as_str());
				let (target_build_version, should_replace_build_version) = if build_version != ""
					&& react.esm_sh_build_version > 0
					&& !build_version.eq(react.esm_sh_build_version.to_string().as_str())
				{
					(react.esm_sh_build_version.to_string(), true)
				} else {
					("".to_owned(), false)
				};
				let non_esm_sh_cdn = match host {
					"esm.sh" | "cdn.esm.sh" => false,
					_ => true,
				};
				if non_esm_sh_cdn {
					host = "esm.sh"
				}
				if non_esm_sh_cdn || ver != react.version || should_replace_build_version {
					if should_replace_build_version {
						fixed_url = format!(
							"https://{}/v{}/react{}@{}{}",
							host, target_build_version, dom, react.version, path
						);
					} else if build_version != "" {
						fixed_url = format!(
							"https://{}/v{}/react{}@{}{}",
							host, build_version, dom, react.version, path
						);
					} else {
						fixed_url = format!("https://{}/react{}@{}{}", host, dom, react.version, path);
					}
				}
			}
		}

		self.deps.push(DependencyDescriptor {
			specifier: fixed_url.clone(),
			is_dynamic,
		});
		fixed_url
	}
}

pub fn is_remote_url(url: &str) -> bool {
	return url.starts_with("https://") || url.starts_with("http://");
}
