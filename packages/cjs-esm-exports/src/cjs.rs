use indexmap::{IndexMap, IndexSet};
use swc_common::DUMMY_SP;
use swc_ecma_ast::*;
use swc_ecma_visit::{noop_fold_type, Fold};

#[derive(Debug)]
pub enum IdentKind {
	Lit(Lit),
	Alias(String),
	Object(Vec<PropOrSpread>),
	Function(Vec<String>),
	Reexport(String),
	Unkonwn,
}

pub struct ExportsParser {
	pub node_env: String,
	pub idents: IndexMap<String, IdentKind>,
	pub exports: IndexSet<String>,
	pub reexports: IndexSet<String>,
}

impl ExportsParser {
	fn reset(&mut self) {
		self.exports.clear();
		self.reexports.clear();
	}

	fn record_ident(&mut self, name: &str, expr: &Expr) {
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
					self.idents.insert(name.into(), IdentKind::Reexport(file));
				}
			}
			Expr::Object(obj) => {
				self
					.idents
					.insert(name.into(), IdentKind::Object(obj.props.clone()));
			}
			Expr::Class(ClassExpr { class, .. }) => {
				let names = get_class_static_names(&class);
				self.idents.insert(name.into(), IdentKind::Function(names));
			}
			Expr::Fn(_) => {
				self.idents.insert(name.into(), IdentKind::Function(vec![]));
			}
			_ => {
				self.idents.insert(name.into(), IdentKind::Unkonwn);
			}
		};
	}

	fn as_str(&self, expr: &Expr) -> Option<String> {
		match expr {
			Expr::Lit(Lit::Str(Str { value, .. })) => return Some(value.as_ref().into()),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Lit(Lit::Str(Str { value, .. })) => return Some(value.as_ref().into()),
						IdentKind::Alias(id) => return self.as_str(&Expr::Ident(quote_ident(id))),
						_ => {}
					}
				}
			}
			_ => {}
		};
		None
	}

	fn as_obj(&self, expr: &Expr) -> Option<Vec<PropOrSpread>> {
		match expr {
			Expr::Object(ObjectLit { props, .. }) => Some(props.to_vec()),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Object(props) => return Some(props.to_vec()),
						IdentKind::Alias(id) => return self.as_obj(&Expr::Ident(quote_ident(id))),
						_ => {}
					}
				}
				None
			}
			_ => None,
		}
	}

	fn as_reexport(&self, expr: &Expr) -> Option<String> {
		match expr {
			Expr::Call(call) => is_require_call(&call),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Reexport(file) => return Some(file.to_owned()),
						IdentKind::Alias(id) => return self.as_reexport(&Expr::Ident(quote_ident(id))),
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
						Prop::Shorthand(id) => Some(id.sym.as_ref().to_owned()),
						Prop::KeyValue(KeyValueProp { key, .. }) => stringify_prop_name(key),
						Prop::Method(MethodProp { key, .. }) => stringify_prop_name(key),
						_ => None,
					};
					if let Some(name) = name {
						self.exports.insert(name);
					}
				}
				PropOrSpread::Spread(SpreadElement { expr, .. }) => match expr.as_ref() {
					Expr::Ident(_) => {
						if let Some(props) = self.as_obj(expr.as_ref()) {
							self.use_object_as_exports(props);
						}
						if let Some(reexport) = self.as_reexport(expr.as_ref()) {
							self.reexports.insert(reexport);
						}
					}
					Expr::Call(call) => {
						if let Some(reexport) = is_require_call(call) {
							self.reexports.insert(reexport);
						}
					}
					_ => {}
				},
			}
		}
	}

	fn parse(&mut self, items: Vec<Stmt>) {
		// record idents
		for item in &items {
			match item {
				Stmt::Decl(Decl::Var(var)) => {
					for decl in &var.decls {
						match &decl.name {
							Pat::Ident(BindingIdent { id, .. }) => {
								let id = id.sym.as_ref();
								if let Some(init) = &decl.init {
									self.record_ident(id, init);
								} else {
									self.idents.insert(id.into(), IdentKind::Unkonwn);
								}
							}
							Pat::Object(ObjectPat { props, .. }) => {
								let mut process_env_init = false;
								if let Some(init) = &decl.init {
									process_env_init = is_member(init.as_ref(), "process", "env");
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
												if let (Some(key), Pat::Ident(rename)) = (key, value.as_ref()) {
													if key.eq("NODE_ENV") {
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
				}
				Stmt::Expr(ExprStmt { expr, .. }) => {
					match expr.as_ref() {
						Expr::Fn(FnExpr { ident, .. }) => {
							if let Some(id) = ident {
								self.record_ident(id.sym.as_ref(), expr);
							}
						}
						Expr::Class(ClassExpr { ident, .. }) => {
							if let Some(id) = ident {
								self.record_ident(id.sym.as_ref(), expr);
							}
						}
						Expr::Assign(assign) => {
							if assign.op == AssignOp::Assign {
								let left_expr = match &assign.left {
									// var foo = 'boo'
									// foo = 'bar'
									PatOrExpr::Expr(expr) => Some(expr.as_ref()),
									// var foo = {}
									// foo.bar = 'bar'
									PatOrExpr::Pat(pat) => match pat.as_ref() {
										Pat::Expr(expr) => Some(expr.as_ref()),
										_ => None,
									},
									_ => None,
								};
								if let Some(expr) = left_expr {
									match expr {
										// var foo = 'boo'
										// foo = 'bar'
										Expr::Ident(id) => {
											let id = id.sym.as_ref();
											if self.idents.contains_key(id) {
												self.record_ident(id, &assign.right.as_ref())
											}
										}
										// var foo = {}
										// foo.bar = 'bar'
										Expr::Member(MemberExpr {
											obj: ExprOrSuper::Expr(obj),
											prop,
											..
										}) => {
											if let Expr::Ident(obj_id) = obj.as_ref() {
												if let Some(mut props) = self.as_obj(obj) {
													if let Some(key) = match prop.as_ref() {
														Expr::Ident(id) => Some(id.sym.as_ref()),
														Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref()),
														_ => None,
													} {
														props.push(PropOrSpread::Prop(Box::new(Prop::KeyValue(
															KeyValueProp {
																key: PropName::Ident(quote_ident(key)),
																value: Box::new(Expr::Lit(Lit::Bool(Bool {
																	span: DUMMY_SP,
																	value: true,
																}))),
															},
														))));
														self
															.idents
															.insert(obj_id.sym.as_ref().into(), IdentKind::Object(props));
													}
												}
											}
										}
										_ => {}
									}
								}
							}
						}
						_ => {}
					};
				}
				_ => {}
			}
		}
		// parse exports
		for item in &items {
			if let Stmt::Expr(ExprStmt { expr, .. }) = item {
				match expr.as_ref() {
					// exports.foo = 'bar'
					// module.exports.foo = 'bar'
					// module.exports = { foo: 'bar' }
					// module.exports = require('lib')
					// module.exports = { ...require('a'), ...require('b') }
					Expr::Assign(assign) => {
						if assign.op == AssignOp::Assign {
							if let PatOrExpr::Expr(expr) = &assign.left {
								match expr.as_ref() {
									Expr::Member(MemberExpr { obj, prop, .. }) => {}
									// Expr::MetaProp(MetaPropExpr { meta, prop }) => {
									// 	let meta = meta.sym.as_ref();
									// 	let prop = prop.sym.as_ref();
									// 	if meta.eq("exports") {
									// 		self.exports.insert(prop.to_owned());
									// 	} else if (meta.eq("module") && prop.eq("exports")) {
									// 	}
									// }
									_ => {}
								}
							}
						}
					}
					// Object.defineProperty(exports, 'foo', { value: 'bar' })
					// Object.defineProperty(module.exports, 'foo', { value: 'bar' })
					// Object.defineProperty(module, 'exports', { value: { foo: 'bar' }})
					// Object.assign(exports, { foo: 'bar' })
					// Object.assign(module.exports, { foo: 'bar' }, { ...require('a') }, require('b'))
					// Object.assign(module, { exports: { foo: 'bar' } })
					Expr::Call(call) => {
						if is_object_static_mothod_call(&call, "defineProperty") && call.args.len() >= 3 {
							let arg0 = &call.args[0];
							let arg1 = &call.args[1];
							let arg2 = &call.args[2];
							let (is_module, is_exports) = is_module_exports_expr(arg0.expr.as_ref());
							let name = self.as_str(arg1.expr.as_ref());
							let mut with_value_or_getter = false;
							let mut with_value_as_object: Option<Vec<PropOrSpread>> = None;
							if let Some(props) = self.as_obj(arg2.expr.as_ref()) {
								for prop in props {
									if let PropOrSpread::Prop(prop) = prop {
										let key = match prop.as_ref() {
											Prop::KeyValue(KeyValueProp { key, value, .. }) => {
												let key = stringify_prop_name(key);
												if let Some(key) = &key {
													if key.eq("value") {
														with_value_as_object = self.as_obj(value.as_ref());
													}
												}
												key
											}
											Prop::Method(MethodProp { key, .. }) => stringify_prop_name(key),
											_ => None,
										};
										if let Some(key) = key {
											if key.eq("value") || key.eq("get") {
												with_value_or_getter = true;
												break;
											}
										}
									}
								}
							}
							if is_exports && with_value_or_getter {
								if let Some(name) = name {
									self.exports.insert(name);
								}
							}
							if is_module {
								if let Some(props) = with_value_as_object {
									self.reset();
									self.use_object_as_exports(props);
								}
							}
						} else if is_object_static_mothod_call(&call, "assign") && call.args.len() >= 2 {
							let (is_module, is_exports) = is_module_exports_expr(call.args[0].expr.as_ref());
							for arg in &call.args[1..] {
								if let Some(props) = self.as_obj(arg.expr.as_ref()) {
									if is_module {
										let mut with_exports_as_object: Option<Vec<PropOrSpread>> = None;
										for prop in props {
											if let PropOrSpread::Prop(prop) = prop {
												if let Prop::KeyValue(KeyValueProp { key, value, .. }) = prop.as_ref() {
													let key = stringify_prop_name(key);
													if let Some(key) = &key {
														if key.eq("exports") {
															with_exports_as_object = self.as_obj(value.as_ref());
															break;
														}
													}
												};
											}
										}
										if let Some(props) = with_exports_as_object {
											self.reset();
											self.use_object_as_exports(props);
										}
									} else if is_exports {
										self.use_object_as_exports(props);
									}
								} else if let Some(reexports) = self.as_reexport(arg.expr.as_ref()) {
									self.reexports.insert(reexports);
								}
							}
						}
					}
					_ => {}
				}
			}
		}
	}
}

impl Fold for ExportsParser {
	noop_fold_type!();

	fn fold_module_items(&mut self, items: Vec<ModuleItem>) -> Vec<ModuleItem> {
		let stmts = items
			.iter()
			.filter(|&item| match item {
				ModuleItem::Stmt(_) => true,
				_ => false,
			})
			.map(|item| match item {
				ModuleItem::Stmt(stmt) => stmt.clone(),
				_ => Stmt::Empty(EmptyStmt { span: DUMMY_SP }),
			})
			.collect::<Vec<Stmt>>();
		self.parse(stmts);
		items
	}
}

// module | exports | module.exports
fn is_module_exports_expr(expr: &Expr) -> (bool, bool) {
	match expr {
		Expr::Ident(id) => {
			let id = id.sym.as_ref();
			return (id.eq("module"), id.eq("exports"));
		}
		// Expr::MetaProp(MetaPropExpr { meta, prop }) => {
		// 	return (
		// 		false,
		// 		meta.sym.as_ref().eq("module") && prop.sym.as_ref().eq("exports"),
		// 	)
		// }
		Expr::Member(_) => return (false, is_member(expr, "module", "exports")),
		_ => {}
	}
	(false, false)
}

fn is_member(expr: &Expr, obj_name: &str, prop_name: &str) -> bool {
	if let Expr::Member(MemberExpr {
		obj: ExprOrSuper::Expr(obj),
		prop,
		..
	}) = expr
	{
		if let Expr::Ident(obj) = obj.as_ref() {
			if obj.sym.as_ref().eq(obj_name) {
				return match prop.as_ref() {
					Expr::Ident(prop) => prop_name == "*" || prop.sym.as_ref().eq(prop_name),
					Expr::Lit(Lit::Str(Str { value, .. })) => {
						prop_name == "*" || value.as_ref().eq(prop_name)
					}
					_ => false,
				};
			}
		}
	}
	false
}

// require('lib')
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

// Object.defineProperty()
// Object.assgin()
fn is_object_static_mothod_call(call: &CallExpr, method: &str) -> bool {
	let callee = match &call.callee {
		ExprOrSuper::Super(_) => return false,
		ExprOrSuper::Expr(callee) => callee.as_ref(),
	};
	is_member(callee, "Object", method)
}

fn get_class_static_names(class: &Class) -> Vec<String> {
	class
		.body
		.iter()
		.filter(|&member| match member {
			ClassMember::ClassProp(prop) => prop.is_static,
			ClassMember::Method(method) => method.is_static,
			_ => false,
		})
		.map(|member| {
			match member {
				ClassMember::ClassProp(prop) => {
					if let Expr::Ident(id) = prop.key.as_ref() {
						return id.sym.as_ref().into();
					}
				}
				ClassMember::Method(method) => {
					if let PropName::Ident(id) = &method.key {
						return id.sym.as_ref().into();
					}
				}
				_ => {}
			};
			"".to_owned()
		})
		.collect()
}

fn stringify_prop_name(name: &PropName) -> Option<String> {
	match name {
		PropName::Ident(id) => Some(id.sym.as_ref().into()),
		PropName::Str(Str { value, .. }) => Some(value.as_ref().into()),
		_ => None,
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
