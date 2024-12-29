// Object.defineProperty
export default (o, p, value, nonEnumerable, nonWritable, nonConfigurable) =>
  Object.defineProperty(o, p, { value, enumerable: !nonEnumerable, writable: !nonWritable, configurable: !nonConfigurable });
