// Object.assign, or if deep clones are needed, use structuredClone
export default (o, d) => Object.assign({}, d, o);
