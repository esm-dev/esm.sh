use super::*;
use lol_html::html_content::Comment as NativeComment;

#[wasm_bindgen]
pub struct Comment(NativeRefWrap<NativeComment<'static>>);

impl_from_native!(NativeComment --> Comment);
impl_mutations!(Comment);

#[wasm_bindgen]
impl Comment {
    #[wasm_bindgen(method, getter)]
    pub fn text(&self) -> JsResult<String> {
        self.0.get().map(|c| c.text())
    }
}
