use super::*;
use lol_html::html_content::DocumentEnd as NativeDocumentEnd;

#[wasm_bindgen]
pub struct DocumentEnd(NativeRefWrap<NativeDocumentEnd<'static>>);

impl_from_native!(NativeDocumentEnd --> DocumentEnd);

#[wasm_bindgen]
impl DocumentEnd {
    pub fn append(
        &mut self,
        content: &str,
        content_type: Option<ContentTypeOptions>,
    ) -> Result<(), JsValue> {
        self.0
            .get_mut()
            .map(|e| e.append(content, content_type.into_native()))
    }
}
