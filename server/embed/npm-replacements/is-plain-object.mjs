export const isPlainObject = v =>
  v && typeof v === "object" && (Object.getPrototypeOf(v) === null || Object.getPrototypeOf(v) === Object.prototype);
export default isPlainObject;
isPlainObject.isPlainObject = isPlainObject;
