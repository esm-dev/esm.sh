export default (v, cb) => {
  if (Array.isArray(v)) {
    v.forEach(cb);
  } else if (v && typeof v === "object") {
    for (const k in v) {
      cb(v[k], k, v);
    }
  }
};
