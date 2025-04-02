export default (str, count = 1, { indent = " ", includeEmptyLines } = {}) =>
  count === 0 ? str : str.replace(includeEmptyLines ? /^/gm : /^(?!\s*$)/gm, indent.repeat(count));
