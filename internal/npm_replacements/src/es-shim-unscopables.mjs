// Array.prototype[Symbol.unscopables]

export default (m) => {
  if (Object.hasOwn(Array.prototype, m)) Array.prototype[Symbol.unscopables][m] = true;
};
