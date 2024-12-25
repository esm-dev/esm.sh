export default (...rest) => structuredClone(Object.assign({}, ...rest));
