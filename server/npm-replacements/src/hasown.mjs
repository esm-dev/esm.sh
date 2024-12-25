export default Object.hasOwn ?? ((o, p) => Object.prototype.hasOwnProperty.call(o, p));
