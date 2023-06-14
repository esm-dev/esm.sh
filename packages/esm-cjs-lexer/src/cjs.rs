use indexmap::{IndexMap, IndexSet};
use std::vec;
use swc_common::DUMMY_SP;
use swc_ecma_ast::*;
use swc_ecma_visit::{noop_fold_type, Fold};

#[derive(Clone, Debug)]
pub enum IdentKind {
  Lit(Lit),
  Alias(String),
  Object(Vec<PropOrSpread>),
  Class(Class),
  Fn(FnDesc),
  Reexport(String),
  Unkonwn,
}

#[derive(Clone, Debug)]
pub struct FnDesc {
  stmts: Vec<Stmt>,
  extends: Vec<String>,
}

pub struct ExportsParser {
  pub node_env: String,
  pub call_mode: bool,
  pub fn_returned: bool,
  pub exports_alias: IndexSet<String>,
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
    if let Expr::Paren(ParenExpr { expr, .. }) = expr {
      self.reset(expr);
      return;
    }
    if let Some(reexport) = self.as_reexport(expr) {
      self.clear();
      self.reexports.insert(reexport);
    } else if let Some(props) = self.as_obj(expr) {
      self.clear();
      self.use_object_as_exports(props);
    } else if let Some(class) = self.as_class(expr) {
      self.clear();
      for name in get_class_static_names(&class) {
        self.exports.insert(name);
      }
    } else if let Some(FnDesc { stmts, extends }) = self.as_function(expr) {
      self.clear();
      if self.call_mode {
        self.dep_parse(stmts, true);
      } else {
        for name in extends {
          self.exports.insert(name);
        }
      }
    } else if let Expr::Call(call) = expr {
      if let Some(callee) = with_expr_callee(call) {
        if let Some(reexport) = self.as_reexport(callee) {
          self.clear();
          self.reexports.insert(format!("{}()", reexport));
        } else if let Some(FnDesc { stmts, .. }) = self.as_function(callee) {
          self.dep_parse(stmts, true);
        }
      }
    }
  }

  fn mark_ident(&mut self, name: &str, expr: &Expr) {
    if let Expr::Paren(ParenExpr { expr, .. }) = expr {
      self.mark_ident(name, expr);
      return;
    }
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
        self.idents.insert(name.into(), IdentKind::Object(obj.props.clone()));
      }
      Expr::Class(ClassExpr { class, .. }) => {
        self
          .idents
          .insert(name.into(), IdentKind::Class(class.as_ref().clone()));
      }
      Expr::Arrow(arrow) => {
        self.idents.insert(
          name.into(),
          IdentKind::Fn(FnDesc {
            stmts: get_arrow_body_as_stmts(&arrow),
            extends: vec![],
          }),
        );
      }
      Expr::Fn(FnExpr { function, .. }) => {
        if let Function { body: Some(body), .. } = function.as_ref() {
          self.idents.insert(
            name.into(),
            IdentKind::Fn(FnDesc {
              stmts: body.stmts.clone(),
              extends: vec![],
            }),
          );
        };
      }
      Expr::Member(_) => {
        if is_member_member(expr, "process", "env", "NODE_ENV") {
          self
            .idents
            .insert(name.into(), IdentKind::Lit(Lit::Str(quote_str(self.node_env.as_str()))));
        }
      }
      _ => {
        self.idents.insert(name.into(), IdentKind::Unkonwn);
      }
    };
  }

  fn as_str(&self, expr: &Expr) -> Option<String> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_str(expr),
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
      Expr::Member(_) => {
        if is_member_member(expr, "process", "env", "NODE_ENV") {
          return Some(self.node_env.to_owned());
        }
      }
      _ => {}
    };
    None
  }

  fn as_num(&self, expr: &Expr) -> Option<f64> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_num(expr),
      Expr::Lit(Lit::Num(Number { value, .. })) => return Some(*value),
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Lit(Lit::Num(Number { value, .. })) => return Some(*value),
            IdentKind::Alias(id) => return self.as_num(&Expr::Ident(quote_ident(id))),
            _ => {}
          }
        }
      }
      _ => {}
    };
    None
  }

  fn as_bool(&self, expr: &Expr) -> Option<bool> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_bool(expr),
      Expr::Lit(Lit::Bool(Bool { value, .. })) => return Some(*value),
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Lit(Lit::Bool(Bool { value, .. })) => return Some(*value),
            IdentKind::Alias(id) => return self.as_bool(&Expr::Ident(quote_ident(id))),
            _ => {}
          }
        }
      }
      _ => {}
    };
    None
  }

  fn as_null(&self, expr: &Expr) -> Option<bool> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_null(expr),
      Expr::Lit(Lit::Null(_)) => return Some(true),
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Lit(Lit::Null(_)) => return Some(true),
            IdentKind::Alias(id) => return self.as_null(&Expr::Ident(quote_ident(id))),
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
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_obj(expr),
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
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_reexport(expr),
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

  fn as_class(&self, expr: &Expr) -> Option<Class> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_class(expr),
      Expr::Class(ClassExpr { class, .. }) => Some(class.as_ref().clone()),
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Class(class) => return Some(class.clone()),
            IdentKind::Alias(id) => return self.as_class(&Expr::Ident(quote_ident(id))),
            _ => {}
          }
        }
        None
      }
      _ => None,
    }
  }

  fn as_function(&self, expr: &Expr) -> Option<FnDesc> {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.as_function(expr),
      Expr::Fn(FnExpr { function, .. }) => {
        if let Function { body: Some(body), .. } = function.as_ref() {
          Some(FnDesc {
            stmts: body.stmts.clone(),
            extends: vec![],
          })
        } else {
          None
        }
      }
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Fn(desc) => return Some(desc.clone()),
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

  fn eqeq(&self, left: &Expr, right: &Expr) -> bool {
    if let Some(left) = self.as_str(left) {
      if let Some(right) = self.as_str(right) {
        return left == right;
      }
    } else if let Some(left) = self.as_num(left) {
      if let Some(right) = self.as_num(right) {
        return left == right;
      }
    } else if let Some(left) = self.as_bool(left) {
      if let Some(right) = self.as_bool(right) {
        return left == right;
      }
    } else if let Some(left) = self.as_null(left) {
      if let Some(right) = self.as_null(right) {
        return left == right;
      }
    }
    false
  }

  fn is_true(&self, expr: &Expr) -> bool {
    match expr {
      Expr::Paren(ParenExpr { expr, .. }) => return self.is_true(expr),
      Expr::Ident(id) => {
        if let Some(value) = self.idents.get(id.sym.as_ref().into()) {
          match value {
            IdentKind::Lit(lit) => return self.is_true(&Expr::Lit(lit.clone())),
            IdentKind::Alias(id) => return self.is_true(&Expr::Ident(quote_ident(id))),
            _ => {}
          }
        } else {
          return false; // undefined
        }
      }
      Expr::Lit(lit) => {
        return match lit {
          Lit::Bool(Bool { value, .. }) => *value,
          Lit::Str(Str { value, .. }) => !value.as_ref().is_empty(),
          Lit::Null(_) => false,
          Lit::Num(Number { value, .. }) => *value != 0.0,
          _ => false,
        }
      }
      Expr::Bin(BinExpr { op, left, right, .. }) => {
        if matches!(op, BinaryOp::LogicalAnd) {
          return self.is_true(left) && self.is_true(right);
        }
        if matches!(op, BinaryOp::LogicalOr) {
          return self.is_true(left) || self.is_true(right);
        }
        if matches!(op, BinaryOp::EqEq | BinaryOp::EqEqEq) {
          return self.eqeq(left, right);
        }
        if matches!(op, BinaryOp::NotEq | BinaryOp::NotEqEq) {
          return !self.eqeq(left, right);
        }
      }
      _ => {}
    }
    true
  }

  // var foo = module.exports = {};
  // foo === module.exports;
  fn try_to_mark_exports_alias(&mut self, decl: &VarDeclarator) {
    if let Pat::Ident(id) = &decl.name {
      if let Some(init) = &decl.init {
        if is_member(init, "module", "exports") {
          self.exports_alias.insert(id.id.sym.as_ref().to_owned());
        } else if let Expr::Assign(assign) = init.as_ref() {
          if let Some(expr) = get_expr_from_pat_or_expr(&assign.left) {
            if is_member(expr, "module", "exports") {
              self.exports_alias.insert(id.id.sym.as_ref().to_owned());
            }
          }
        }
      }
    }
  }

  fn is_exports_ident(&self, id: &str) -> bool {
    return id.eq("exports") || self.exports_alias.contains(id);
  }

  fn is_exports_expr(&self, expr: &Expr) -> bool {
    match expr {
      Expr::Ident(id) => {
        let id = id.sym.as_ref();
        self.is_exports_ident(id)
      }
      Expr::Member(_) => is_member(expr, "module", "exports"),
      _ => false,
    }
  }

  fn get_exports_prop_name(&self, expr: &Expr) -> Option<String> {
    if let Expr::Member(MemberExpr { obj, prop, .. }) = expr {
      if let Expr::Ident(obj) = obj.as_ref() {
        if self.is_exports_ident(obj.sym.as_ref()) {
          return get_prop_name(prop);
        }
      }
    }
    None
  }

  // exports.foo || (exports.foo = {})
  // foo = exports.foo || (exports.foo = {})
  fn get_bare_export_names(&mut self, expr: &Expr) -> Option<String> {
    if let Expr::Assign(assign) = expr {
      return self.get_bare_export_names(assign.right.as_ref());
    }

    if let Expr::Bin(bin) = expr {
      if bin.op == BinaryOp::LogicalOr {
        if let Some(member_prop_name) = self.get_exports_prop_name(bin.left.as_ref()) {
          if let Expr::Paren(ParenExpr { expr, .. }) = bin.right.as_ref() {
            if let Expr::Assign(assign) = expr.as_ref() {
              let left_expr = match &assign.left {
                PatOrExpr::Expr(expr) => Some(expr.as_ref()),
                PatOrExpr::Pat(pat) => match pat.as_ref() {
                  Pat::Expr(expr) => Some(expr.as_ref()),
                  _ => None,
                },
              };
              if let Some(left_expr) = left_expr {
                if assign.op == AssignOp::Assign {
                  if let Some(prop_name) = self.get_exports_prop_name(left_expr) {
                    if prop_name.eq(&member_prop_name) {
                      return Some(member_prop_name);
                    }
                  }
                }
              }
            }
          }
        }
      }
    }

    None
  }

  fn get_exports_from_assign(&mut self, assign: &AssignExpr) {
    if assign.op == AssignOp::Assign {
      let left_expr = match &assign.left {
        PatOrExpr::Expr(expr) => Some(expr.as_ref()),
        PatOrExpr::Pat(pat) => match pat.as_ref() {
          Pat::Expr(expr) => Some(expr.as_ref()),
          _ => None,
        },
      };
      if let Some(Expr::Member(MemberExpr { obj, prop, .. })) = left_expr {
        let prop = get_prop_name(prop);
        if let Some(prop) = prop {
          match obj.as_ref() {
            Expr::Ident(obj) => {
              let obj_name = obj.sym.as_ref();
              if self.is_exports_ident(obj_name) {
                // exports.foo = 'bar'
                self.exports.insert(prop);
                if let Expr::Assign(dep_assign) = assign.right.as_ref() {
                  self.get_exports_from_assign(dep_assign);
                }
              } else if obj_name.eq("module") && self.is_exports_ident(&prop) {
                // module.exports = ??
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
      } else {
        if let Some(bare_export_name) = self.get_bare_export_names(assign.right.as_ref()) {
          self.exports.insert(bare_export_name);
        }
      }
    }
  }

  // walk and mark idents
  fn walk_stmts(&mut self, stmts: &Vec<Stmt>) -> bool {
    for stmt in stmts {
      match stmt {
        Stmt::Decl(decl) => match decl {
          Decl::Var(var) => {
            for decl in &var.decls {
              self.try_to_mark_exports_alias(decl);
              match &decl.name {
                Pat::Ident(BindingIdent { id, .. }) => {
                  let id = id.sym.as_ref();
                  if let Some(init) = &decl.init {
                    self.mark_ident(id, init);
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
          Decl::Fn(FnDecl { ident, function, .. }) => {
            self.mark_ident(
              ident.sym.as_ref(),
              &Expr::Fn(FnExpr {
                ident: Some(ident.clone()),
                function: function.clone(),
              }),
            );
          }
          Decl::Class(ClassDecl { ident, class, .. }) => {
            self.mark_ident(
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
                        self.mark_ident(id, &assign.right.as_ref())
                      }
                    }
                    // var foo = {}
                    // foo.bar = 'bar'
                    Expr::Member(MemberExpr { obj, prop, .. }) => {
                      let key = get_prop_name(prop);
                      if let Some(key) = key {
                        if let Expr::Ident(obj_id) = obj.as_ref() {
                          let obj_name = obj_id.sym.as_ref();
                          if let Some(mut props) = self.as_obj(obj) {
                            props.push(PropOrSpread::Prop(Box::new(Prop::KeyValue(KeyValueProp {
                              key: PropName::Ident(quote_ident(&key)),
                              value: Box::new(Expr::Lit(Lit::Bool(Bool {
                                span: DUMMY_SP,
                                value: true,
                              }))),
                            }))));
                            self.idents.insert(obj_name.into(), IdentKind::Object(props));
                          } else if let Some(FnDesc { stmts, mut extends }) = self.as_function(obj) {
                            extends.push(key.to_owned());
                            self
                              .idents
                              .insert(obj_name.into(), IdentKind::Fn(FnDesc { stmts, extends }));
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
        Stmt::Block(BlockStmt { stmts, .. }) => {
          let returned = self.walk_stmts(&stmts);
          if returned {
            return true;
          }
        }
        Stmt::If(IfStmt { test, cons, alt, .. }) => {
          let mut returned = false;
          if self.is_true(test) {
            returned = self.walk_stmts(&vec![cons.as_ref().clone()])
          } else if let Some(alt) = alt {
            returned = self.walk_stmts(&vec![alt.as_ref().clone()])
          }
          if returned {
            return true;
          }
        }
        Stmt::Return(_) => return true,
        _ => {}
      }
    }
    false
  }

  fn parse(&mut self, stmts: Vec<Stmt>, as_fn: bool) {
    self.walk_stmts(&stmts);

    // parse exports (as function)
    if as_fn {
      for stmt in &stmts {
        if self.fn_returned {
          break;
        }
        match stmt {
          Stmt::Block(BlockStmt { stmts, .. }) => {
            self.dep_parse(stmts.clone(), true);
          }
          Stmt::If(IfStmt { test, cons, alt, .. }) => {
            if self.is_true(test) {
              self.dep_parse(vec![cons.as_ref().clone()], true);
            } else if let Some(alt) = alt {
              self.dep_parse(vec![alt.as_ref().clone()], true);
            }
          }
          Stmt::Return(ReturnStmt { arg, .. }) => {
            self.fn_returned = true;
            if let Some(arg) = arg {
              self.reset(arg);
            }
          }
          _ => {}
        }
      }
      return;
    }

    // parse exports
    for stmt in &stmts {
      match stmt {
        // var foo = exports.foo || (exports.foo = {})
        Stmt::Decl(Decl::Var(var)) => {
          for decl in var.as_ref().decls.iter() {
            self.try_to_mark_exports_alias(decl);
            if let Some(init_expr) = &decl.init {
              if let Some(bare_export_name) = self.get_bare_export_names(init_expr) {
                self.exports.insert(bare_export_name);
              }
            }
          }
        }
        Stmt::Expr(ExprStmt { expr, .. }) => match expr.as_ref() {
          // exports.foo = 'bar'
          // module.exports.foo = 'bar'
          // module.exports = { foo: 'bar' }
          // module.exports = { ...require('a'), ...require('b') }
          // module.exports = require('lib')
          // foo = exports.foo || (exports.foo = {})
          Expr::Assign(assign) => {
            self.get_exports_from_assign(assign);
          }
          // Object.defineProperty(exports, 'foo', { value: 'bar' })
          // Object.defineProperty(module.exports, 'foo', { value: 'bar' })
          // Object.defineProperty(module, 'exports', { value: { foo: 'bar' }})
          // Object.assign(exports, { foo: 'bar' })
          // Object.assign(module.exports, { foo: 'bar' }, { ...require('a') }, require('b'))
          // Object.assign(module, { exports: { foo: 'bar' } })
          // Object.assign(module, { exports: require('lib') })
          // (function() { ... })()
          // require("tslib").__exportStar(..., exports)
          // tslib.__exportStar(..., exports)
          // __exportStar(..., exports)
          Expr::Call(call) => {
            if is_object_static_mothod_call(&call, "defineProperty") && call.args.len() >= 3 {
              let arg0 = &call.args[0];
              let arg1 = &call.args[1];
              let arg2 = &call.args[2];
              let is_module = is_module_ident(arg0.expr.as_ref());
              let is_exports = self.is_exports_expr(arg0.expr.as_ref());

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
              let is_module = is_module_ident(call.args[0].expr.as_ref());
              let is_exports = self.is_exports_expr(call.args[0].expr.as_ref());
              for arg in &call.args[1..] {
                if let Some(props) = self.as_obj(arg.expr.as_ref()) {
                  if is_module {
                    let mut with_exports: Option<Expr> = None;
                    for prop in props {
                      if let PropOrSpread::Prop(prop) = prop {
                        if let Prop::KeyValue(KeyValueProp { key, value, .. }) = prop.as_ref() {
                          let key = stringify_prop_name(key);
                          if let Some(key) = &key {
                            if self.is_exports_ident(key) {
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
            } else if is_tslib_export_star_call(&call) && call.args.len() >= 2 {
              let is_exports = self.is_exports_expr(call.args[1].expr.as_ref());
              if is_exports {
                if let Some(props) = self.as_obj(call.args[0].expr.as_ref()) {
                  self.use_object_as_exports(props);
                } else if let Some(reexport) = self.as_reexport(call.args[0].expr.as_ref()) {
                  self.reexports.insert(reexport);
                }
              }
            } else if is_export_call(&call) && call.args.len() > 0 {
              if let Some(props) = self.as_obj(call.args[0].expr.as_ref()) {
                self.use_object_as_exports(props);
              } else if let Some(reexport) = self.as_reexport(call.args[0].expr.as_ref()) {
                self.reexports.insert(reexport);
              }
            } else if let Some(body) = is_umd_iife_call(&call) {
              self.dep_parse(body, false);
            } else if let Some(body) = is_iife_call(&call) {
              for arg in &call.args {
                if arg.spread.is_none() {
                  // (function() { ... })(exports.foo || (exports.foo = {}))
                  if let Some(bare_export_name) = self.get_bare_export_names(arg.expr.as_ref()) {
                    self.exports.insert(bare_export_name);
                  }
                }
              }
              self.dep_parse(body, false);
            }
          }
          // ~function(){ ... }()
          Expr::Unary(UnaryExpr { op, arg, .. }) => {
            if let UnaryOp::Minus | UnaryOp::Plus | UnaryOp::Bang | UnaryOp::Tilde | UnaryOp::Void = op {
              if let Expr::Call(call) = arg.as_ref() {
                if let Some(body) = is_iife_call(&call) {
                  // (function() { ... })(exports.foo || (exports.foo = {}))
                  for arg in &call.args {
                    if arg.spread.is_none() {
                      if let Some(bare_export_name) = self.get_bare_export_names(arg.expr.as_ref()) {
                        self.exports.insert(bare_export_name);
                      }
                    }
                  }
                  self.dep_parse(body, false);
                }
              }
            }
          }
          // (function(){ ... }())
          Expr::Paren(ParenExpr { expr, .. }) => {
            self.parse(
              vec![Stmt::Expr(ExprStmt {
                span: DUMMY_SP,
                expr: expr.clone(),
              })],
              false,
            );
          }
          // 0 && (module.exports = { foo })
          Expr::Bin(BinExpr { op, right, .. }) => {
            if matches!(op, BinaryOp::LogicalAnd) {
              if let Expr::Assign(assign) = right.as_ref() {
                self.get_exports_from_assign(assign);
              }  else   if let Expr::Paren(paren) = right.as_ref() {
                if let Expr::Assign(assign) = paren.expr.as_ref() {
                  self.get_exports_from_assign(assign);
                }
              }
            }
          }
          _ => {}
        },
        Stmt::Block(BlockStmt { stmts, .. }) => {
          self.dep_parse(stmts.clone(), false);
        }
        Stmt::If(IfStmt { test, cons, alt, .. }) => {
          if self.is_true(test) {
            self.dep_parse(vec![cons.as_ref().clone()], false);
          } else if let Some(alt) = alt {
            self.dep_parse(vec![alt.as_ref().clone()], false);
          }
        }
        _ => {}
      }
    }
  }

  fn dep_parse(&mut self, body: Vec<Stmt>, as_fn: bool) {
    let mut dep_parser = ExportsParser {
      node_env: self.node_env.to_owned(),
      call_mode: false,
      fn_returned: false,
      idents: self.idents.clone(),
      exports_alias: self.exports_alias.clone(),
      exports: self.exports.clone(),
      reexports: self.reexports.clone(),
    };
    dep_parser.parse(body, as_fn);
    self.fn_returned = dep_parser.fn_returned;
    self.exports = dep_parser.exports;
    self.reexports = dep_parser.reexports;
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
      .filter(|item| !item.is_empty())
      .collect::<Vec<Stmt>>();
    self.parse(stmts, false);
    items
  }
}

fn is_module_ident(expr: &Expr) -> bool {
  match expr {
    Expr::Ident(id) => {
      let id = id.sym.as_ref();
      id.eq("module")
    }
    _ => false,
  }
}

fn is_member(expr: &Expr, obj_name: &str, prop_name: &str) -> bool {
  if let Some(member_prop_name) = get_member_prop_name(expr, obj_name) {
    return member_prop_name.eq(prop_name);
  }
  false
}

fn is_member_member(expr: &Expr, obj_name: &str, middle_obj_name: &str, prop_name: &str) -> bool {
  if let Expr::Member(MemberExpr { obj, prop, .. }) = expr {
    if is_member(obj, obj_name, middle_obj_name) {
      if let Some(name) = get_prop_name(prop) {
        return name.eq(prop_name);
      }
    }
  }
  false
}

fn with_expr_callee(call: &CallExpr) -> Option<&Expr> {
  match &call.callee {
    Callee::Expr(callee) => Some(callee.as_ref()),
    _ => None,
  }
}

// require('lib')
fn is_require_call(call: &CallExpr) -> Option<String> {
  if let Some(Expr::Ident(id)) = with_expr_callee(call) {
    if id.sym.as_ref().eq("require") && call.args.len() > 0 {
      return match call.args[0].expr.as_ref() {
        Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref().to_owned()),
        _ => None,
      };
    }
  };
  None
}

// match:
// Object.defineProperty()
// Object.assgin()
fn is_object_static_mothod_call(call: &CallExpr, method: &str) -> bool {
  if let Some(callee) = with_expr_callee(call) {
    return is_member(callee, "Object", method);
  }
  false
}

fn is_umd_params(params: &Vec<Pat>) -> bool {
  if params.len() == 2 {
    if let Pat::Ident(bid) = &params.get(0).unwrap() {
      if bid.id.sym.eq("global") {
        if let Pat::Ident(bid) = &params.get(1).unwrap() {
          if bid.id.sym.eq("factory") {
            return true;
          }
        }
      }
    }
  }
  false
}

fn is_umd_iife_call(call: &CallExpr) -> Option<Vec<Stmt>> {
  if call.args.len() == 2 {
    let mut arg1 = call.args.get(1).unwrap().expr.as_ref();
    if let Expr::Paren(ParenExpr { expr, .. }) = arg1 {
      arg1 = expr.as_ref();
    }
    let stmts = match arg1 {
      Expr::Fn(func) => {
        if let Some(BlockStmt { stmts, .. }) = &func.function.body {
          Some(stmts.clone())
        } else {
          None
        }
      }
      Expr::Arrow(arrow) => Some(get_arrow_body_as_stmts(&arrow)),
      _ => None,
    };
    let expr = if let Some(callee) = with_expr_callee(call) {
      match callee {
        Expr::Paren(ParenExpr { expr, .. }) => expr.as_ref(),
        _ => callee,
      }
    } else {
      return None;
    };
    match expr {
      Expr::Fn(func) => {
        if is_umd_params(&func.function.params.iter().map(|p| p.pat.clone()).collect()) {
          return stmts;
        }
      }
      Expr::Arrow(arrow) => {
        if is_umd_params(&arrow.params) {
          return stmts;
        }
      }
      _ => {}
    }
  }
  None
}

fn is_iife_call(call: &CallExpr) -> Option<Vec<Stmt>> {
  let expr = if let Some(callee) = with_expr_callee(call) {
    match callee {
      Expr::Paren(ParenExpr { expr, .. }) => expr.as_ref(),
      _ => callee,
    }
  } else {
    return None;
  };
  match expr {
    Expr::Fn(func) => {
      if let Some(BlockStmt { stmts, .. }) = &func.function.body {
        return Some(stmts.clone());
      }
    }
    Expr::Arrow(arrow) => return Some(get_arrow_body_as_stmts(arrow)),
    _ => {}
  }
  None
}

fn is_export_call(call: &CallExpr) -> bool {
  if let Some(callee) = with_expr_callee(call) {
    match callee {
      Expr::Ident(id) => {
        return id.sym.as_ref().eq("__export");
      }
      _ => {}
    }
  }
  false
}

// match:
// require("tslib").__exportStar(..., exports)
// (0, require("tslib").__exportStar)(..., exports)
// const tslib = require("tslib"); (0, tslib.__exportStar)(..., exports)
// const {__exportStar} = require("tslib"); (0, __exportStar)(..., exports)
// const __exportStar = () => {}; __exportStar(..., exports)
fn is_tslib_export_star_call(call: &CallExpr) -> bool {
  if let Some(callee) = with_expr_callee(call) {
    match callee {
      Expr::Member(MemberExpr { prop, .. }) => {
        if let MemberProp::Ident(prop) = prop {
          return prop.sym.as_ref().eq("__exportStar");
        }
      }
      Expr::Ident(id) => {
        return id.sym.as_ref().eq("__exportStar");
      }
      Expr::Paren(ParenExpr { expr, .. }) => match expr.as_ref() {
        Expr::Member(MemberExpr { prop, .. }) => {
          if let MemberProp::Ident(prop) = prop {
            return prop.sym.as_ref().eq("__exportStar");
          }
        }
        Expr::Ident(id) => {
          return id.sym.as_ref().eq("__exportStar");
        }
        Expr::Seq(SeqExpr { exprs, .. }) => {
          if let Some(last) = exprs.last() {
            match last.as_ref() {
              Expr::Member(MemberExpr { prop, .. }) => {
                if let MemberProp::Ident(prop) = prop {
                  return prop.sym.as_ref().eq("__exportStar");
                }
              }
              Expr::Ident(id) => {
                return id.sym.as_ref().eq("__exportStar");
              }
              _ => {}
            }
          }
        }
        _ => {}
      },
      _ => {}
    }
  }
  false
}

fn get_expr_from_pat_or_expr(v: &PatOrExpr) -> Option<&Expr> {
  match v {
    PatOrExpr::Expr(left_expr) => Some(left_expr),
    PatOrExpr::Pat(pat) => {
      if let Pat::Expr(left_expr) = pat.as_ref() {
        Some(left_expr)
      } else {
        None
      }
    }
  }
}

fn get_arrow_body_as_stmts(arrow: &ArrowExpr) -> Vec<Stmt> {
  match &arrow.body {
    BlockStmtOrExpr::BlockStmt(BlockStmt { stmts, .. }) => stmts.clone(),
    BlockStmtOrExpr::Expr(expr) => vec![Stmt::Return(ReturnStmt {
      span: DUMMY_SP,
      arg: Some(expr.clone()),
    })],
  }
}

fn get_member_prop_name(expr: &Expr, obj_name: &str) -> Option<String> {
  if let Expr::Member(MemberExpr { obj, prop, .. }) = expr {
    if let Expr::Ident(obj) = obj.as_ref() {
      if obj.sym.as_ref().eq(obj_name) {
        return get_prop_name(prop);
      }
    }
  }
  None
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
          if let PropName::Ident(id) = &prop.key {
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

fn get_prop_name(prop: &MemberProp) -> Option<String> {
  match prop {
    MemberProp::Ident(prop) => Some(prop.sym.as_ref().into()),
    MemberProp::Computed(ComputedPropName { expr, .. }) => match expr.as_ref() {
      Expr::Ident(prop) => Some(prop.sym.as_ref().into()),
      Expr::Lit(Lit::Str(Str { value, .. })) => Some(value.as_ref().into()),
      _ => None,
    },
    _ => None,
  }
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
    raw: None,
  }
}
