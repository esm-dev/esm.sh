export default (path) => path.startsWith("\\\\?\\") ? path : path.replace(/\\/g, "/");
