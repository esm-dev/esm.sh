export default (v, cb) => {
  const i = v?.[Symbol.iterator]?.();
  if (cb) {
    for (const v of i) {
      cb(v);
    }
  } else {
    return [...i];
  }
};
