const hasOwn = Object.prototype.hasOwnProperty;
export default Object.hasOwn ?? ((o, p) => hasOwn.call(o, p));
