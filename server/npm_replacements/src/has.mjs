// Object.prototype.hasOwnProperty.call(obj, prop) (or in later versions of node, "Object.hasOwn(obj, prop)")
export default (obj, prop) => Object.hasOwn ? Object.hasOwn(obj, prop) : Object.prototype.hasOwnProperty.call(obj, prop);
