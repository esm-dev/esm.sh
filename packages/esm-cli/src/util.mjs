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
 * lookup value from the given object by the given path.
 * @param {Record<string, unknown>} obj
 * @param {(string | number)[]} path
 * @returns {unknown}
 */
export function lookupValue(obj, path) {
  let value = obj;
  if (value === undefined || value === null) {
    return value;
  }
  for (const key of path) {
    const v = value[key];
    if (v === undefined) {
      return;
    }
    if (typeof v === "function") {
      return v.call(value);
    }
    value = v;
  }
  return value;
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
