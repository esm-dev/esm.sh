// Object.assign, or if deep clones are needed, use structuredClone
export default (target, ...rest) =>
  typeof target === "boolean"
    ? (target ? structuredClone(Object.assign(...rest)) : Object.assign(...rest))
    : Object.assign(target, ...rest);
