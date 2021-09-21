use swc_ecma_ast::*;
use swc_ecma_visit::{noop_fold_type, Fold};

pub struct CJSLexer {
  pub exports: Vec<String>,
  pub reexports: Vec<String>,
}

impl Fold for CJSLexer {
  noop_fold_type!();

	fn fold_call_expr(&mut self, mut call: CallExpr) -> CallExpr {
		call
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
