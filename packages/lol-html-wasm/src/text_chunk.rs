use super::*;
use lol_html::html_content::TextChunk as NativeTextChunk;

#[wasm_bindgen]
pub struct TextChunk(NativeRefWrap<NativeTextChunk<'static>>);

impl_from_native!(NativeTextChunk --> TextChunk);
impl_mutations!(TextChunk);

#[wasm_bindgen]
impl TextChunk {
    #[wasm_bindgen(method, getter)]
    pub fn text(&self) -> JsResult<String> {
        self.0.get().map(|c| c.as_str().into())
    }

    #[wasm_bindgen(method, getter=lastInTextNode)]
    pub fn last_in_text_node(&self) -> JsResult<bool> {
        self.0.get().map(|c| c.last_in_text_node())
    }
}
