use super::comment::Comment;
use super::doctype::Doctype;
use super::document_end::DocumentEnd;
use super::element::Element;
use super::text_chunk::TextChunk;
use super::*;
use js_sys::Function as JsFunction;
use lol_html::{
    DocumentContentHandlers as NativeDocumentContentHandlers,
    ElementContentHandlers as NativeElementContentHandlers,
};
use std::mem;
use thiserror::Error;

#[derive(Error, Debug)]
#[error("JS handler error")]
pub struct HandlerJsErrorWrap(pub JsValue);

// SAFETY: The exposed js-api only supports single-threaded usage.
unsafe impl Send for HandlerJsErrorWrap {}
unsafe impl Sync for HandlerJsErrorWrap {}

macro_rules! make_handler {
    ($handler:ident, $JsArgType:ident) => {
        move |arg: &mut _| {
            let (js_arg, anchor) = $JsArgType::from_native(arg);

            let res = match $handler.call1(&JsValue::NULL, &JsValue::from(js_arg)) {
                Ok(_) => Ok(()),
                Err(e) => Err(HandlerJsErrorWrap(e).into()),
            };

            mem::drop(anchor);

            res
        }
    };
}

#[wasm_bindgen]
extern "C" {
    pub type ElementContentHandlers;

    #[wasm_bindgen(method, getter)]
    fn element(this: &ElementContentHandlers) -> Option<JsFunction>;

    #[wasm_bindgen(method, getter)]
    fn comments(this: &ElementContentHandlers) -> Option<JsFunction>;

    #[wasm_bindgen(method, getter)]
    fn text(this: &ElementContentHandlers) -> Option<JsFunction>;
}

impl IntoNative<NativeElementContentHandlers<'static>> for ElementContentHandlers {
    fn into_native(self) -> NativeElementContentHandlers<'static> {
        let mut native = NativeElementContentHandlers::default();

        if let Some(handler) = self.element() {
            native = native.element(make_handler!(handler, Element));
        }

        if let Some(handler) = self.comments() {
            native = native.comments(make_handler!(handler, Comment));
        }

        if let Some(handler) = self.text() {
            native = native.text(make_handler!(handler, TextChunk));
        }

        native
    }
}

#[wasm_bindgen]
extern "C" {
    pub type DocumentContentHandlers;

    #[wasm_bindgen(method, getter)]
    fn doctype(this: &DocumentContentHandlers) -> Option<JsFunction>;

    #[wasm_bindgen(method, getter)]
    fn comments(this: &DocumentContentHandlers) -> Option<JsFunction>;

    #[wasm_bindgen(method, getter)]
    fn text(this: &DocumentContentHandlers) -> Option<JsFunction>;

    #[wasm_bindgen(method, getter)]
    fn end(this: &DocumentContentHandlers) -> Option<JsFunction>;
}

impl IntoNative<NativeDocumentContentHandlers<'static>> for DocumentContentHandlers {
    fn into_native(self) -> NativeDocumentContentHandlers<'static> {
        let mut native = NativeDocumentContentHandlers::default();

        if let Some(handler) = self.doctype() {
            native = native.doctype(make_handler!(handler, Doctype));
        }

        if let Some(handler) = self.comments() {
            native = native.comments(make_handler!(handler, Comment));
        }

        if let Some(handler) = self.text() {
            native = native.text(make_handler!(handler, TextChunk));
        }

        if let Some(handler) = self.end() {
            native = native.end(make_handler!(handler, DocumentEnd));
        }

        native
    }
}
