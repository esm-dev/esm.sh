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
 * check if the given value is null or undefined.
 * @param {unknown} v
 * @returns {v is null | undefined}
 */
export function isNullish(v) {
  return v === null || v === undefined;
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
 * check if the given response is a JSON response.
 * @param {Response} response
 * @returns {boolean}
 */
export function isJSONResponse(response) {
  const cType = response.headers.get("content-type");
  return /^(application|text)\/json(;|$)/.test(cType);
}

/**
 * check if the given url is a local host.
 * @param {URL} url
 * @returns {boolean}
 */
export function isLocalHost({ hostname }) {
  return hostname === "localhost" || hostname === "127.0.0.1" ||
    hostname === "[::1]";
}

/**
 * read text from the given readable stream.
 * @param {ReadableStream<Uint8Array>} readable
 * @returns {Promise<string>}
 */
export function readTextFromStream(readable) {
  const decoder = new TextDecoder();
  const reader = readable.getReader();
  let buf = "";
  return reader.read().then(function process({ done, value }) {
    if (done) {
      return buf;
    }
    buf += decoder.decode(value, { stream: true });
    return reader.read().then(process);
  });
}

/**
 * lookup value from the given object by the given path.
 * @param {Record<string, unknown>} obj
 * @param {string} expr
 * @returns {unknown}
 */
export function lookupValue(obj, expr) {
  let value = obj;
  if (value === undefined || value === null) {
    return value;
  }
  const path = expr.split(".").map((p) =>
    p.split("[").map((expr) => {
      if (expr.endsWith("]")) {
        const key = expr.slice(0, -1);
        if (/^\d+$/.test(key)) {
          return parseInt(key);
        }
        return key.replace(/^['"]|['"]$/g, "");
      }
      return expr;
    })
  ).flat();
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
  const r = glob
    .replace(/^\.*\//g, "")
    .replace(/[-+?.^$\[\]\(\)]/g, "\\$&")
    .replace(/\{/g, "(").replace(/\}/g, ")").replace(/\s*,\s*/g, "|")
    .replace(/\*\*(\/\*+)?/g, "++").replace(/\*/g, "[^/]+")
    .replace(/\+\+/g, ".*?")
    .replace(/\//g, "\\/");
  cache.set(glob, reg = new RegExp("^" + r + "$", "i"));
  return reg;
}
globToRegExp.cache = new Map();
