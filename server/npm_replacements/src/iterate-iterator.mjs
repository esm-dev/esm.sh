export default (i, cb) => {
  if (i) {
    if (cb) {
      for (const v of i) {
        cb(v);
      }
    } else {
      return [...i];
    }
  }
};
