// react-refresh
// @link https://github.com/facebook/react/issues/16604#issuecomment-528663101

import "https://esm.sh/@prefresh/core@1.5.2?external=*";

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
const __REFRESH__ = () => {
  const pending = [...__PREFRESH__.getPendingUpdates()];
  __PREFRESH__.flush();

  if (pending.length > 0) {
    pending.forEach(([prev, next]) => {
      compareSignatures(prev, next);
    });
  }
};

function compareSignatures(prev, next) {
  const prevSignature = __PREFRESH__.getSignature(prev) || {};
  const nextSignature = __PREFRESH__.getSignature(next) || {};
  if (
    prevSignature.key !== nextSignature.key
    || __PREFRESH__.computeKey(prevSignature) !== __PREFRESH__.computeKey(nextSignature)
    || nextSignature.forceReset
  ) {
    __PREFRESH__.replaceComponent(prev, next, true);
  } else {
    __PREFRESH__.replaceComponent(prev, next, false);
  }
}

globalThis.$RefreshReg$ = () => {};
globalThis.$RefreshSig$ = () => type => type;

export { __REFRESH__, __REFRESH_RUNTIME__ };
