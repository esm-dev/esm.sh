export const enc = new TextEncoder();

export const fsFilter = (filename) => {
  return !/(^|\/)(\.|node_modules\/)/.test(filename) &&
    !filename.endsWith(".log");
};

const regexpCache = new Map();
export const globToRegExp = (glob) => {
  let reg = regexpCache.get(glob);
  if (reg) return reg
  regexpCache.set(glob, reg = new RegExp(
    "^" + glob.replace(/[-+?.^$[\]()]/g, "\\$&")
      .replace(/\{/g, "(").replace(/\}/g, ")").replace(/,\s*/g, "|")
      .replace(/\*\*(\/\*+)?/g, "++").replace(/\*/g, "[^/]+").replace(/\+\+/g,".*?")
      .replace(/\//g, "\\/") +
      "$",
    "i",
  ));
  return reg
};
