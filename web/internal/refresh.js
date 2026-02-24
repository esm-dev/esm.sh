// Fast-refresh for React!
// @see https://github.com/facebook/react/issues/16604#issuecomment-528663101

import Refresh from "https://esm.sh/react-refresh@0.18.0/es2022/react-refresh.development.mjs";

let timer;

export const __REFRESH_RUNTIME__ = {
  register: (specifier) => (type, id) => Refresh.register(type, specifier + " " + id),
  sign: Refresh.createSignatureFunctionForTransform,
};

export const __REFRESH__ = (module) => {
  console.log(module)
  if (timer !== null) {
    clearTimeout(timer);
  }
  timer = setTimeout(() => {
    timer = null;
    Refresh.performReactRefresh();
  }, 30);
};

Refresh.injectIntoGlobalHook(globalThis);
globalThis.$RefreshReg$ = () => {};
globalThis.$RefreshSig$ = () => type => type;
