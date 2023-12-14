export const enc = new TextEncoder();

export const globToRegExp = (glob) => {
  const cache = globToRegExp.cache;
  let reg = cache.get(glob);
  if (reg) {
    return reg;
  }
  const r = glob.replace(/[-+?.^$[\]]/g, "\\$&")
    .replace(/\{/g, "(").replace(/\}/g, ")").replace(/,\s*/g, "|")
    .replace(/\*\*(\/\*+)?/g, "++").replace(/\*/g, "[^/]+")
    .replace(/\+\+/g, ".*?").replace(/\//g, "\\/");
  cache.set(glob, reg = new RegExp("^" + r + "$", "i"));
  return reg;
};
globToRegExp.cache = new Map();
