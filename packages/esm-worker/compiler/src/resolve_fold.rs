use crate::resolver::Resolver;
use std::{cell::RefCell, rc::Rc};
use swc_common::DUMMY_SP;
use swc_ecma_ast::*;
use swc_ecma_utils::quote_ident;
use swc_ecma_visit::{noop_fold_type, Fold, FoldWith};

pub fn resolve_fold(resolver: Rc<RefCell<Resolver>>, is_dev: bool) -> impl Fold {
	ResolveFold { resolver, is_dev }
}

pub struct ResolveFold {
	resolver: Rc<RefCell<Resolver>>,
	is_dev: bool,
}

impl Fold for ResolveFold {
	noop_fold_type!();

	// resolve import/export url
	fn fold_module_items(&mut self, module_items: Vec<ModuleItem>) -> Vec<ModuleItem> {
		let mut items = Vec::<ModuleItem>::new();

		for item in module_items {
			match item {
				ModuleItem::ModuleDecl(decl) => {
					let item: ModuleItem = match decl {
						// match: import React, { useState } from "https://esm.sh/react"
						ModuleDecl::Import(import_decl) => {
							if import_decl.type_only {
								// ingore type import
								ModuleItem::ModuleDecl(ModuleDecl::Import(import_decl))
							} else {
								let mut resolver = self.resolver.borrow_mut();
								let fixed_url = resolver.resolve(import_decl.src.value.as_ref(), false);
								if resolver.bundle_mode && resolver.bundle_externals.contains(fixed_url.as_str()) {
									let mut names: Vec<(Ident, Option<String>)> = vec![];
									let mut ns: Option<Ident> = None;
									import_decl
										.specifiers
										.into_iter()
										.for_each(|specifier| match specifier {
											ImportSpecifier::Named(ImportNamedSpecifier {
												local, imported, ..
											}) => {
												names.push((
													local,
													match imported {
														Some(name) => Some(name.sym.as_ref().into()),
														None => None,
													},
												));
											}
											ImportSpecifier::Default(ImportDefaultSpecifier { local, .. }) => {
												names.push((local, Some("default".into())));
											}
											ImportSpecifier::Namespace(ImportStarAsSpecifier { local, .. }) => {
												ns = Some(local);
											}
										});
									if let Some(name) = ns {
										ModuleItem::Stmt(Stmt::Decl(Decl::Var(VarDecl {
											span: DUMMY_SP,
											kind: VarDeclKind::Const,
											declare: false,
											decls: vec![create_aleph_pack_var_decl(fixed_url.as_ref(), name)],
										})))
									} else if names.len() > 0 {
										// const {default: React, useState} = __ALEPH__.pack["https://esm.sh/react"];
										ModuleItem::Stmt(Stmt::Decl(Decl::Var(VarDecl {
											span: DUMMY_SP,
											kind: VarDeclKind::Const,
											declare: false,
											decls: vec![create_aleph_pack_var_decl_member(fixed_url.as_ref(), names)],
										})))
									} else {
										ModuleItem::Stmt(Stmt::Empty(EmptyStmt { span: DUMMY_SP }))
									}
								} else {
									ModuleItem::ModuleDecl(ModuleDecl::Import(ImportDecl {
										src: new_str(fixed_url),
										..import_decl
									}))
								}
							}
						}
						// match: export { default as React, useState } from "https://esm.sh/react"
						// match: export * as React from "https://esm.sh/react"
						ModuleDecl::ExportNamed(NamedExport {
							type_only,
							specifiers,
							src: Some(src),
							..
						}) => {
							if type_only {
								// ingore type export
								ModuleItem::ModuleDecl(ModuleDecl::ExportNamed(NamedExport {
									span: DUMMY_SP,
									specifiers,
									src: Some(src),
									type_only: true,
									asserts: None,
								}))
							} else {
								let mut resolver = self.resolver.borrow_mut();
								let fixed_url = resolver.resolve(src.value.as_ref(), false);
								if resolver.bundle_mode && resolver.bundle_externals.contains(fixed_url.as_str()) {
									let mut names: Vec<(Ident, Option<String>)> = vec![];
									let mut ns: Option<Ident> = None;
									specifiers
										.into_iter()
										.for_each(|specifier| match specifier {
											ExportSpecifier::Named(ExportNamedSpecifier { orig, exported, .. }) => {
												names.push((
													orig,
													match exported {
														Some(name) => Some(name.sym.as_ref().into()),
														None => None,
													},
												));
											}
											ExportSpecifier::Default(ExportDefaultSpecifier { exported, .. }) => {
												names.push((exported, Some("default".into())));
											}
											ExportSpecifier::Namespace(ExportNamespaceSpecifier { name, .. }) => {
												ns = Some(name);
											}
										});
									if let Some(name) = ns {
										ModuleItem::ModuleDecl(ModuleDecl::ExportDecl(ExportDecl {
											span: DUMMY_SP,
											decl: Decl::Var(VarDecl {
												span: DUMMY_SP,
												kind: VarDeclKind::Const,
												declare: false,
												decls: vec![create_aleph_pack_var_decl(fixed_url.as_ref(), name)],
											}),
										}))
									} else if names.len() > 0 {
										ModuleItem::ModuleDecl(ModuleDecl::ExportDecl(ExportDecl {
											span: DUMMY_SP,
											decl: Decl::Var(VarDecl {
												span: DUMMY_SP,
												kind: VarDeclKind::Const,
												declare: false,
												decls: vec![create_aleph_pack_var_decl_member(fixed_url.as_ref(), names)],
											}),
										}))
									} else {
										ModuleItem::Stmt(Stmt::Empty(EmptyStmt { span: DUMMY_SP }))
									}
								} else {
									ModuleItem::ModuleDecl(ModuleDecl::ExportNamed(NamedExport {
										span: DUMMY_SP,
										specifiers,
										src: Some(new_str(fixed_url)),
										type_only: false,
										asserts: None,
									}))
								}
							}
						}
						// match: export * from "https://esm.sh/react"
						ModuleDecl::ExportAll(ExportAll { src, .. }) => {
							let mut resolver = self.resolver.borrow_mut();
							let fixed_url = resolver.resolve(src.value.as_ref(), false);
							if resolver.bundle_mode && resolver.bundle_externals.contains(fixed_url.as_str()) {
								resolver.star_exports.push(fixed_url.clone());
								ModuleItem::ModuleDecl(ModuleDecl::ExportDecl(ExportDecl {
									span: DUMMY_SP,
									decl: Decl::Var(VarDecl {
										span: DUMMY_SP,
										kind: VarDeclKind::Const,
										declare: false,
										decls: vec![create_aleph_pack_var_decl(
											fixed_url.as_ref(),
											quote_ident!(format!("$$star_{}", resolver.star_exports.len() - 1)),
										)],
									}),
								}))
							} else {
								if self.is_dev {
									ModuleItem::ModuleDecl(ModuleDecl::ExportAll(ExportAll {
										span: DUMMY_SP,
										src: new_str(fixed_url.into()),
										asserts: None,
									}))
								} else {
									let mut src = "".to_owned();
									src.push('[');
									src.push_str(fixed_url.as_str());
									src.push(']');
									src.push(':');
									src.push_str(fixed_url.as_str());
									resolver.star_exports.push(fixed_url.clone());
									ModuleItem::ModuleDecl(ModuleDecl::ExportAll(ExportAll {
										span: DUMMY_SP,
										src: new_str(src.into()),
										asserts: None,
									}))
								}
							}
						}
						_ => ModuleItem::ModuleDecl(decl),
					};
					items.push(item.fold_children_with(self));
				}
				_ => {
					items.push(item.fold_children_with(self));
				}
			};
		}

		items
	}

	// resolve dynamic import url
	fn fold_call_expr(&mut self, mut call: CallExpr) -> CallExpr {
		if is_call_expr_by_name(&call, "import") {
			let url = match call.args.first() {
				Some(ExprOrSpread { expr, .. }) => match expr.as_ref() {
					Expr::Lit(lit) => match lit {
						Lit::Str(s) => s.value.as_ref(),
						_ => return call,
					},
					_ => return call,
				},
				_ => return call,
			};
			let mut resolver = self.resolver.borrow_mut();
			if resolver.bundle_mode {
				call.callee = ExprOrSuper::Expr(Box::new(Expr::MetaProp(MetaPropExpr {
					meta: quote_ident!("__ALEPH__"),
					prop: quote_ident!("import"),
				})))
			}
			let fixed_url = resolver.resolve(url, true);
			call.args = vec![ExprOrSpread {
				spread: None,
				expr: Box::new(Expr::Lit(Lit::Str(new_str(fixed_url)))),
			}];
		}

		call.fold_children_with(self)
	}
}

pub fn is_call_expr_by_name(call: &CallExpr, name: &str) -> bool {
	let callee = match &call.callee {
		ExprOrSuper::Super(_) => return false,
		ExprOrSuper::Expr(callee) => callee.as_ref(),
	};

	match callee {
		Expr::Ident(id) => id.sym.as_ref().eq(name),
		_ => false,
	}
}

fn create_aleph_pack_member_expr(url: &str) -> MemberExpr {
	MemberExpr {
		span: DUMMY_SP,
		obj: ExprOrSuper::Expr(Box::new(Expr::Ident(quote_ident!("__ALEPH__")))),
		prop: Box::new(Expr::Member(MemberExpr {
			span: DUMMY_SP,
			obj: ExprOrSuper::Expr(Box::new(Expr::Ident(quote_ident!("pack")))),
			prop: Box::new(Expr::Lit(Lit::Str(new_str(url.into())))),
			computed: true,
		})),
		computed: false,
	}
}

fn create_aleph_pack_var_decl(url: &str, name: Ident) -> VarDeclarator {
	VarDeclarator {
		span: DUMMY_SP,
		name: Pat::Ident(BindingIdent {
			id: name,
			type_ann: None,
		}),
		init: Some(Box::new(Expr::Member(create_aleph_pack_member_expr(url)))),
		definite: false,
	}
}

pub fn create_aleph_pack_var_decl_member(
	url: &str,
	names: Vec<(Ident, Option<String>)>,
) -> VarDeclarator {
	VarDeclarator {
		span: DUMMY_SP,
		name: Pat::Object(ObjectPat {
			span: DUMMY_SP,
			props: names
				.into_iter()
				.map(|(name, rename)| {
					if let Some(rename) = rename {
						ObjectPatProp::KeyValue(KeyValuePatProp {
							key: PropName::Ident(quote_ident!(rename)),
							value: Box::new(Pat::Ident(BindingIdent {
								id: name,
								type_ann: None,
							})),
						})
					} else {
						ObjectPatProp::Assign(AssignPatProp {
							span: DUMMY_SP,
							key: name,
							value: None,
						})
					}
				})
				.collect(),
			optional: false,
			type_ann: None,
		}),
		init: Some(Box::new(Expr::Member(create_aleph_pack_member_expr(url)))),
		definite: false,
	}
}

fn new_str(str: String) -> Str {
	Str {
		span: DUMMY_SP,
		value: str.into(),
		has_escape: false,
		kind: Default::default(),
	}
}
