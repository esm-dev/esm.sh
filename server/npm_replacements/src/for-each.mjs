export default (obj, cb) => {
  if (Array.isArray(obj)) {
    obj.forEach(cb);
  } else if (typeof obj === "object" && obj !== null) {
    for (const [k, v] of Object.entries(obj)) {
      cb(v, k, obj);
    }
  }
};
