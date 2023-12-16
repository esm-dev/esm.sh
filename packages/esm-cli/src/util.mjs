export const enc = new TextEncoder();

/**
 * check if the given value is a non-empty string.
 * @param {unknown} v
 * @returns {v is string}
 */
export function isNEString(v) {
  return typeof v === "string" && v.length > 0;
}

/**
 * check if the given value is an object.
 * @param {unknown} v
 * @returns {v is Record<string, unknown>}
 */
export function isObject(v) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/**
 * covert a glob pattern to a RegExp object.
 * @param {string} glob
 * @returns {RegExp}
 */
export function globToRegExp(glob) {
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
}
globToRegExp.cache = new Map();
