export default (target, value) => Object.defineProperty(target, Symbol.toStringTag, { value, configurable: true });
