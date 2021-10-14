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
	fn clear(&mut self) {
		self.exports.clear();
		self.reexports.clear();
	}

	fn reset(&mut self, expr: &Expr) {
		if let Some(props) = self.as_obj(&expr) {
			self.clear();
			self.use_object_as_exports(props);
		} else if let Some(reexport) = self.as_reexport(&expr) {
			self.clear();
			self.reexports.insert(reexport);
		} else if let Some(fields) = self.as_function(&expr) {
			self.clear();
			for field in fields {
				self.exports.insert(field);
			}
		}
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

	fn as_function(&self, expr: &Expr) -> Option<Vec<String>> {
		match expr {
			Expr::Class(ClassExpr { class, .. }) => Some(get_class_static_names(&class)),
			Expr::Fn(_) => Some(vec![]),
			Expr::Ident(id) => {
				if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
					match value {
						IdentKind::Function(fields) => return Some(fields.clone()),
						IdentKind::Alias(id) => return self.as_function(&Expr::Ident(quote_ident(id))),
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
				Stmt::Decl(decl) => match decl {
					Decl::Var(var) => {
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
					Decl::Fn(FnDecl {
						ident, function, ..
					}) => {
						self.record_ident(
							ident.sym.as_ref(),
							&Expr::Fn(FnExpr {
								ident: Some(ident.clone()),
								function: function.clone(),
							}),
						);
					}
					Decl::Class(ClassDecl { ident, class, .. }) => {
						self.record_ident(
							ident.sym.as_ref(),
							&Expr::Class(ClassExpr {
								ident: Some(ident.clone()),
								class: class.clone(),
							}),
						);
					}
					_ => {}
				},
				Stmt::Expr(ExprStmt { expr, .. }) => {
					match expr.as_ref() {
						Expr::Assign(assign) => {
							if assign.op == AssignOp::Assign {
								let left_expr = match &assign.left {
									PatOrExpr::Expr(expr) => Some(expr.as_ref()),
									PatOrExpr::Pat(pat) => match pat.as_ref() {
										Pat::Expr(expr) => Some(expr.as_ref()),
										_ => None,
									},
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
											let key = match prop.as_ref() {
												Expr::Ident(id) => Some(id.sym.as_ref()),
												Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref()),
												_ => None,
											};
											if let Some(key) = key {
												if let Expr::Ident(obj_id) = obj.as_ref() {
													let obj_name = obj_id.sym.as_ref();
													if let Some(mut props) = self.as_obj(obj) {
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
															.insert(obj_name.into(), IdentKind::Object(props));
													} else if let Some(mut fields) = self.as_function(obj) {
														fields.push(key.to_owned());
														self
															.idents
															.insert(obj_name.into(), IdentKind::Function(fields));
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
		println!("{:?}", self.idents);

		// parse exports
		for item in &items {
			if let Stmt::Expr(ExprStmt { expr, .. }) = item {
				match expr.as_ref() {
					// exports.foo = 'bar'
					// module.exports.foo = 'bar'
					// module.exports = { foo: 'bar' }
					// module.exports = { ...require('a'), ...require('b') }
					// module.exports = require('lib')
					Expr::Assign(assign) => {
						if assign.op == AssignOp::Assign {
							let left_expr = match &assign.left {
								PatOrExpr::Expr(expr) => Some(expr.as_ref()),
								PatOrExpr::Pat(pat) => match pat.as_ref() {
									Pat::Expr(expr) => Some(expr.as_ref()),
									_ => None,
								},
							};
							if let Some(Expr::Member(MemberExpr {
								obj: ExprOrSuper::Expr(obj),
								prop,
								..
							})) = left_expr
							{
								let prop = match prop.as_ref() {
									Expr::Ident(prop) => Some(prop.sym.as_ref().to_owned()),
									Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref().to_owned()),
									_ => None,
								};
								if let Some(prop) = prop {
									match obj.as_ref() {
										Expr::Ident(obj) => {
											let obj_name = obj.sym.as_ref();
											if obj_name.eq("exports") {
												self.exports.insert(prop);
											} else if obj_name.eq("module") && prop.eq("exports") {
												let right_expr = assign.right.as_ref();
												self.reset(right_expr)
											}
										}
										Expr::Member(_) => {
											if is_member(obj, "module", "exports") {
												self.exports.insert(prop);
											}
										}
										_ => {}
									}
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
					// Object.assign(module, { exports: require('lib') })
					Expr::Call(call) => {
						if is_object_static_mothod_call(&call, "defineProperty") && call.args.len() >= 3 {
							let arg0 = &call.args[0];
							let arg1 = &call.args[1];
							let arg2 = &call.args[2];
							let (is_module, is_exports) = is_module_exports(arg0.expr.as_ref());
							let name = self.as_str(arg1.expr.as_ref());
							let mut with_value_or_getter = false;
							let mut with_value: Option<Expr> = None;
							if let Some(props) = self.as_obj(arg2.expr.as_ref()) {
								for prop in props {
									if let PropOrSpread::Prop(prop) = prop {
										let key = match prop.as_ref() {
											Prop::KeyValue(KeyValueProp { key, value, .. }) => {
												let key = stringify_prop_name(key);
												if let Some(key) = &key {
													if key.eq("value") {
														with_value = Some(value.as_ref().clone());
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
								if let Some(expr) = with_value {
									self.reset(&expr);
								}
							}
						} else if is_object_static_mothod_call(&call, "assign") && call.args.len() >= 2 {
							let (is_module, is_exports) = is_module_exports(call.args[0].expr.as_ref());
							for arg in &call.args[1..] {
								if let Some(props) = self.as_obj(arg.expr.as_ref()) {
									if is_module {
										let mut with_exports: Option<Expr> = None;
										for prop in props {
											if let PropOrSpread::Prop(prop) = prop {
												if let Prop::KeyValue(KeyValueProp { key, value, .. }) = prop.as_ref() {
													let key = stringify_prop_name(key);
													if let Some(key) = &key {
														if key.eq("exports") {
															with_exports = Some(value.as_ref().clone());
															break;
														}
													}
												};
											}
										}
										if let Some(exports_expr) = with_exports {
											self.reset(&exports_expr);
										}
									} else if is_exports {
										self.use_object_as_exports(props);
									}
								} else if let Some(reexport) = self.as_reexport(arg.expr.as_ref()) {
									if is_exports {
										self.reexports.insert(reexport);
									}
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
fn is_module_exports(expr: &Expr) -> (bool, bool) {
	match expr {
		Expr::Ident(id) => {
			let id = id.sym.as_ref();
			return (id.eq("module"), id.eq("exports"));
		}
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
					Expr::Ident(prop) => prop.sym.as_ref().eq(prop_name),
					Expr::Lit(Lit::Str(Str { value, .. })) => value.as_ref().eq(prop_name),
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
