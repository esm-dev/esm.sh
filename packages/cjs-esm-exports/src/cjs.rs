use indexmap::IndexMap;
use swc_common::DUMMY_SP;
use swc_ecma_ast::*;
use swc_ecma_visit::{noop_fold_type, Fold};

#[derive(Debug)]
pub enum IdentKind {
	Lit(Lit),
	Alias(String),
	Object(Vec<PropOrSpread>),
	Function(Vec<PropOrSpread>),
	Require(String),
	Unkonwn,
}

pub struct IdentRecorder {
	pub node_env: String,
	pub idents: IndexMap<String, IdentKind>,
}

impl IdentRecorder {
	fn record(&mut self, name: &str, expr: &Expr) {
		match expr {
			Expr::Lit(lit) => {
				self.idents.insert(name.into(), IdentKind::Lit(lit.clone()));
			}
			Expr::Ident(id) => {
				let conflict = if let Some(val) = self.idents.get(id.sym.as_ref().into()) {
					if let IdentKind::Alias(rename) = val {
						rename.eq(name)
					} else {
						false
					}
				} else {
					false
				};
				if !conflict {
					self
						.idents
						.insert(name.into(), IdentKind::Alias(id.sym.as_ref().into()));
				}
			}
			Expr::Call(call) => {
				if let Some(file) = is_require_call(&call) {
					self.idents.insert(name.into(), IdentKind::Require(file));
				}
			}
			Expr::Object(obj) => {
				self
					.idents
					.insert(name.into(), IdentKind::Object(obj.props.clone()));
			}
			Expr::Class(ClassExpr { class, .. }) => {
				class
					.body
					.iter()
					.filter(|&member| match member {
						ClassMember::Method(method) => method.is_static,
						ClassMember::ClassProp(prop) => prop.is_static,
						_ => false,
					})
					.map(|member| match member {
						ClassMember::Method(method) => {
							if let PropName::Ident(id) = &method.key  {
								id.sym.as_ref()
							} else {
								""
							}
						}
						ClassMember::ClassProp(prop) => {
							if let Expr::Ident(id) = prop.key.as_ref() {
								id.sym.as_ref()
							} else {
								""
							}
						}
						_ => "",
					});
			}
			_ => {
				self.idents.insert(name.into(), IdentKind::Unkonwn);
			}
		};
	}
}

impl Fold for IdentRecorder {
	noop_fold_type!();

	fn fold_var_decl(&mut self, var: VarDecl) -> VarDecl {
		for decl in &var.decls {
			match &decl.name {
				Pat::Ident(BindingIdent { id, .. }) => {
					let id = id.sym.as_ref();
					if let Some(init) = &decl.init {
						self.record(id, init);
					} else {
						self.idents.insert(id.into(), IdentKind::Unkonwn);
					}
				}
				Pat::Object(ObjectPat { props, .. }) => {
					let mut process_env_init = false;
					if let Some(init) = &decl.init {
						if let Expr::MetaProp(MetaPropExpr { meta, prop }) = init.as_ref() {
							process_env_init = meta.sym.as_ref().eq("process") && prop.sym.as_ref().eq("env");
						}
					};
					if process_env_init {
						for prop in props {
							match prop {
								ObjectPatProp::Assign(AssignPatProp { key, .. }) => {
									let key = key.sym.as_ref();
									if key.eq("NODE_ENV") {
										self.idents.insert(
											key.to_owned(),
											IdentKind::Lit(Lit::Str(quote_str(self.node_env.as_str()))),
										);
									}
								}
								ObjectPatProp::KeyValue(KeyValuePatProp { key, value, .. }) => {
									let key = stringify_prop_name(&key);
									if key.eq("NODE_ENV") {
										if let Pat::Ident(rename) = value.as_ref() {
											self.idents.insert(
												rename.id.sym.as_ref().to_owned(),
												IdentKind::Lit(Lit::Str(quote_str(self.node_env.as_str()))),
											);
										}
									}
								}
								_ => {}
							}
						}
					}
				}
				_ => {}
			}
		}
		var
	}

	fn fold_assign_expr(&mut self, assign: AssignExpr) -> AssignExpr {
		if assign.op == AssignOp::Assign {
			if let PatOrExpr::Expr(expr) = &assign.left {
				if let Expr::Ident(id) = expr.as_ref() {
					let id = id.sym.as_ref();
					if self.idents.contains_key(id) {
						self.record(id, &assign.right.as_ref())
					}
				}
			}
		}
		assign
	}
}

pub struct ExportsParser {
	pub idents: IndexMap<String, IdentKind>,
	pub exports: Vec<String>,
	pub reexports: Vec<String>,
}

impl ExportsParser {
	fn get_str(&self, expr: &Expr) -> String {
		match expr {
			Expr::Lit(Lit::Str(Str { value, .. })) => return value.as_ref().into(),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Lit(Lit::Str(Str { value, .. })) => return value.as_ref().into(),
						IdentKind::Alias(id) => return self.get_str(&Expr::Ident(quote_ident(id))),
						_ => {}
					}
				}
			}
			_ => {}
		}
		"".to_owned()
	}

	fn get_obj(&self, expr: &Expr) -> Option<Vec<PropOrSpread>> {
		match expr {
			Expr::Object(ObjectLit { props, .. }) => Some(props.clone()),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Object(props) => return Some(props.clone()),
						IdentKind::Alias(id) => return self.get_obj(&Expr::Ident(quote_ident(id))),
						_ => {}
					}
				}
				None
			}
			_ => None,
		}
	}

	fn use_object_as_exports(&mut self, props: Vec<PropOrSpread>) {
		for prop in props {
			match prop {
				PropOrSpread::Prop(prop) => {
					let name = match prop.as_ref() {
						Prop::Shorthand(id) => self.get_str(&Expr::Ident(id.clone())),
						Prop::KeyValue(KeyValueProp { key, .. }) => stringify_prop_name(key),
						Prop::Method(MethodProp { key, .. }) => stringify_prop_name(key),
						_ => "".to_owned(),
					};
				}
				PropOrSpread::Spread(spread) => {}
			}
		}
	}
}

impl Fold for ExportsParser {
	noop_fold_type!();

	// exports.foo = 'bar'
	// module.exports.foo = 'bar'
	// module.exports = { foo: 'bar' }
	// module.exports = require('lib')
	// module.exports = {...require('a'), ...require('b')}
	fn fold_assign_expr(&mut self, assign: AssignExpr) -> AssignExpr {
		if assign.op == AssignOp::Assign {
			if let PatOrExpr::Expr(expr) = &assign.left {
				match expr.as_ref() {
					Expr::Member(MemberExpr { obj, prop, .. }) => {}
					Expr::MetaProp(MetaPropExpr { meta, prop }) => {
						let meta = meta.sym.as_ref();
						let prop = prop.sym.as_ref();
						if meta.eq("exports") {
							self.exports.push(prop.to_owned());
						} else if (meta.eq("module") && prop.eq("exports")) {
						}
					}
					_ => {}
				}
			}
		}
		assign
	}

	// Object.defineProperty(exports, 'foo', { value: 'bar' })
	// Object.defineProperty(module.exports, 'foo', { value: 'bar' })
	// Object.defineProperty(module, 'exports', { value: { foo: 'bar' }})
	// Object.assign(exports, { foo: 'bar' })
	// Object.assign(module.exports, { foo: 'bar' }, { ...require('lib') })
	fn fold_call_expr(&mut self, call: CallExpr) -> CallExpr {
		if is_object_static_mothod_call(&call, "defineProperty") && call.args.len() >= 3 {
			let arg0 = &call.args[0];
			let arg1 = &call.args[1];
			let arg2 = &call.args[2];
			let (is_module, is_exports) = is_module_exports_expr(arg0.expr.as_ref());
			let name = self.get_str(arg1.expr.as_ref());
			let mut with_value_or_getter = false;
			let mut with_value_as_object: Option<Vec<PropOrSpread>> = None;
			if let Some(props) = self.get_obj(arg2.expr.as_ref()) {
				for prop in props {
					if let PropOrSpread::Prop(prop) = prop {
						let key = match prop.as_ref() {
							Prop::KeyValue(KeyValueProp { key, value, .. }) => {
								let key = stringify_prop_name(key);
								if key.eq("value") {
									with_value_as_object = self.get_obj(value.as_ref());
								}
								key
							}
							Prop::Method(MethodProp { key, .. }) => stringify_prop_name(key),
							_ => "".to_owned(),
						};
						if key.eq("value") || key.eq("get") {
							with_value_or_getter = true;
							break;
						}
					}
				}
			}
			if is_exports && !name.is_empty() && with_value_or_getter {
				self.exports.push(name.to_owned());
			}
			if is_module {
				if let Some(props) = with_value_as_object {
					self.use_object_as_exports(props)
				}
			}
		} else if is_object_static_mothod_call(&call, "assign") && call.args.len() >= 2 {
			// Object.assign(...)
		}
		call
	}
}

fn is_module_exports_expr(expr: &Expr) -> (bool, bool) {
	match expr {
		Expr::Ident(id) => {
			let id = id.sym.as_ref();
			return (id.eq("module"), id.eq("exports"));
		}
		Expr::MetaProp(MetaPropExpr { meta, prop }) => {
			return (
				false,
				meta.sym.as_ref().eq("module") && prop.sym.as_ref().eq("exports"),
			)
		}
		Expr::Member(MemberExpr {
			obj: ExprOrSuper::Expr(obj),
			prop,
			..
		}) => {
			if let Expr::Ident(obj) = obj.as_ref() {
				if let Expr::Ident(prop) = prop.as_ref() {
					return (
						false,
						obj.sym.as_ref().eq("module") && prop.sym.as_ref().eq("exports"),
					);
				}
			}
		}
		_ => {}
	}
	(false, false)
}

fn is_require_call(call: &CallExpr) -> Option<String> {
	let callee = match &call.callee {
		ExprOrSuper::Super(_) => return None,
		ExprOrSuper::Expr(callee) => callee.as_ref(),
	};
	match callee {
		Expr::Ident(id) => {
			if id.sym.as_ref().eq("require") && call.args.len() > 0 {
				match call.args[0].expr.as_ref() {
					Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref().to_owned()),
					_ => None,
				}
			} else {
				None
			}
		}
		_ => None,
	}
}

fn is_object_static_mothod_call(call: &CallExpr, method: &str) -> bool {
	let callee = match &call.callee {
		ExprOrSuper::Super(_) => return false,
		ExprOrSuper::Expr(callee) => callee.as_ref(),
	};
	match callee {
		Expr::MetaProp(MetaPropExpr { meta, prop }) => {
			return meta.sym.as_ref().eq("Object") && prop.sym.as_ref().eq(method)
		}
		Expr::Member(MemberExpr {
			obj: ExprOrSuper::Expr(obj),
			prop,
			..
		}) => {
			if let Expr::Ident(obj) = obj.as_ref() {
				if let Expr::Ident(prop) = prop.as_ref() {
					return obj.sym.as_ref().eq("Object") && prop.sym.as_ref().eq(method);
				}
			}
		}
		_ => {}
	}

	false
}

fn stringify_prop_name(name: &PropName) -> String {
	match name {
		PropName::Ident(id) => id.sym.as_ref().into(),
		PropName::Str(Str { value, .. }) => value.as_ref().into(),
		_ => "".to_owned(),
	}
}

fn quote_ident(value: &str) -> Ident {
	Ident {
		span: DUMMY_SP,
		sym: value.into(),
		optional: false,
	}
}

fn quote_str(value: &str) -> Str {
	Str {
		span: DUMMY_SP,
		value: value.into(),
		has_escape: false,
		kind: Default::default(),
	}
}
