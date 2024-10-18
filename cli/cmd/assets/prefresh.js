// react-refresh
// @link https://github.com/facebook/react/issues/16604#issuecomment-528663101

import "https://esm.sh/@prefresh/core@1.5.2";
import { flush as __REFRESH__ } from "https://esm.sh/@prefresh/@prefresh/utils@1.2.0";

const __PREFRESH__ = globalThis.__PREFRESH__;
const __REFRESH_RUNTIME__ = {
  register: (specifier) => {
    return (type, id) => {
      __PREFRESH__.register(type, specifier + " " + id);
    };
  },
  sign: () => {
    let status = "begin";
    let savedType;
    return (type, key, forceReset, getCustomHooks) => {
      if (!savedType) {
        savedType = type;
      }
      status = __PREFRESH__.sign(type || savedType, key, forceReset, getCustomHooks, status);
      return type;
    };
  },
};

globalThis.$RefreshReg$ = () => {};
globalThis.$RefreshSig$ = () => type => type;

export { __REFRESH__, __REFRESH_RUNTIME__ };
