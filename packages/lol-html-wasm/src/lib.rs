use lol_html::html_content::ContentType as NativeContentType;
use std::cell::Cell;
use std::convert::Into;
use std::marker::PhantomData;
use std::mem;
use std::ops::Drop;
use std::rc::Rc;
use wasm_bindgen::prelude::*;

type JsResult<T> = Result<T, JsValue>;

struct Anchor<'r> {
    poisoned: Rc<Cell<bool>>,
    lifetime: PhantomData<&'r mut ()>,
}

impl<'r> Anchor<'r> {
    pub fn new(poisoned: Rc<Cell<bool>>) -> Self {
        Anchor {
            poisoned,
            lifetime: PhantomData,
        }
    }
}

impl Drop for Anchor<'_> {
    fn drop(&mut self) {
        self.poisoned.replace(true);
    }
}

// NOTE: wasm_bindgen doesn't allow structures with lifetimes. To workaround that
// we create a wrapper that erases all the lifetime information from the inner reference
// and provides an anchor object that keeps track of the lifetime in the runtime.
//
// When anchor goes out of scope, wrapper becomes poisoned and any attempt to get inner
// object results in exception.
struct NativeRefWrap<R> {
    inner_ptr: *mut R,
    poisoned: Rc<Cell<bool>>,
}

impl<R> NativeRefWrap<R> {
    pub fn wrap<I>(inner: &mut I) -> (Self, Anchor) {
        let wrap = NativeRefWrap {
            inner_ptr: unsafe { mem::transmute(inner) },
            poisoned: Rc::new(Cell::new(false)),
        };

        let anchor = Anchor::new(Rc::clone(&wrap.poisoned));

        (wrap, anchor)
    }

    fn assert_not_poisoned(&self) -> JsResult<()> {
        if self.poisoned.get() {
            Err("The object has been freed and can't be used anymore.".into())
        } else {
            Ok(())
        }
    }

    pub fn get(&self) -> JsResult<&R> {
        self.assert_not_poisoned()?;

        Ok(unsafe { self.inner_ptr.as_ref() }.unwrap())
    }

    pub fn get_mut(&mut self) -> JsResult<&mut R> {
        self.assert_not_poisoned()?;

        Ok(unsafe { self.inner_ptr.as_mut() }.unwrap())
    }
}

trait IntoJsResult<T> {
    fn into_js_result(self) -> JsResult<T>;
}

impl<T, E: ToString> IntoJsResult<T> for Result<T, E> {
    #[inline]
    fn into_js_result(self) -> JsResult<T> {
        self.map_err(|e| JsValue::from(e.to_string()))
    }
}

trait IntoNative<T> {
    fn into_native(self) -> T;
}

#[wasm_bindgen]
extern "C" {
    pub type ContentTypeOptions;

    #[wasm_bindgen(method, getter)]
    fn html(this: &ContentTypeOptions) -> Option<bool>;
}

impl IntoNative<NativeContentType> for Option<ContentTypeOptions> {
    fn into_native(self) -> NativeContentType {
        match self {
            Some(opts) => match opts.html() {
                Some(true) => NativeContentType::Html,
                Some(false) => NativeContentType::Text,
                None => NativeContentType::Text,
            },
            None => NativeContentType::Text,
        }
    }
}

macro_rules! impl_mutations {
    ($Ty:ident) => {
        #[wasm_bindgen]
        impl $Ty {
            pub fn before(
                &mut self,
                content: &str,
                content_type: Option<ContentTypeOptions>,
            ) -> Result<(), JsValue> {
                self.0
                    .get_mut()
                    .map(|o| o.before(content, content_type.into_native()))
            }

            pub fn after(
                &mut self,
                content: &str,
                content_type: Option<ContentTypeOptions>,
            ) -> Result<(), JsValue> {
                self.0
                    .get_mut()
                    .map(|o| o.after(content, content_type.into_native()))
            }

            pub fn replace(
                &mut self,
                content: &str,
                content_type: Option<ContentTypeOptions>,
            ) -> Result<(), JsValue> {
                self.0
                    .get_mut()
                    .map(|o| o.replace(content, content_type.into_native()))
            }

            pub fn remove(&mut self) -> Result<(), JsValue> {
                self.0.get_mut().map(|o| o.remove())
            }

            #[wasm_bindgen(method, getter)]
            pub fn removed(&self) -> JsResult<bool> {
                self.0.get().map(|o| o.removed())
            }
        }
    };
}

macro_rules! impl_from_native {
    ($Ty:ident --> $JsTy:ident) => {
        impl $JsTy {
            pub(crate) fn from_native<'r>(inner: &'r mut $Ty) -> (Self, Anchor<'r>) {
                let (ref_wrap, anchor) = NativeRefWrap::wrap(inner);

                ($JsTy(ref_wrap), anchor)
            }
        }
    };
}

mod comment;
mod doctype;
mod document_end;
mod element;
mod handlers;
mod html_rewriter;
mod text_chunk;
