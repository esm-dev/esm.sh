// querystring.ts
var hexTable = new Array(256);
for (let i = 0; i < 256; ++i) {
  hexTable[i] = "%" + ((i < 16 ? "0" : "") + i.toString(16)).toUpperCase();
}
function parse(str, sep = "&", eq = "=", { decodeURIComponent: decodeURIComponent2 = unescape, maxKeys = 1e3 } = {}) {
  const entries = str.split(sep).map((entry) => entry.split(eq).map(decodeURIComponent2));
  const final = {};
  let i = 0;
  while (true) {
    if (Object.keys(final).length === maxKeys && !!maxKeys || !entries[i]) {
      break;
    }
    const [key, val] = entries[i];
    if (final[key]) {
      if (Array.isArray(final[key])) {
        final[key].push(val);
      } else {
        final[key] = [final[key], val];
      }
    } else {
      final[key] = val;
    }
    i++;
  }
  return final;
}
function encodeStr(str, noEscapeTable, hexTable2) {
  const len = str.length;
  if (len === 0)
    return "";
  let out = "";
  let lastPos = 0;
  for (let i = 0; i < len; i++) {
    let c = str.charCodeAt(i);
    if (c < 128) {
      if (noEscapeTable[c] === 1)
        continue;
      if (lastPos < i)
        out += str.slice(lastPos, i);
      lastPos = i + 1;
      out += hexTable2[c];
      continue;
    }
    if (lastPos < i)
      out += str.slice(lastPos, i);
    if (c < 2048) {
      lastPos = i + 1;
      out += hexTable2[192 | c >> 6] + hexTable2[128 | c & 63];
      continue;
    }
    if (c < 55296 || c >= 57344) {
      lastPos = i + 1;
      out += hexTable2[224 | c >> 12] + hexTable2[128 | c >> 6 & 63] + hexTable2[128 | c & 63];
      continue;
    }
    ++i;
    if (i >= len)
      throw new Deno.errors.InvalidData("invalid URI");
    const c2 = str.charCodeAt(i) & 1023;
    lastPos = i + 1;
    c = 65536 + ((c & 1023) << 10 | c2);
    out += hexTable2[240 | c >> 18] + hexTable2[128 | c >> 12 & 63] + hexTable2[128 | c >> 6 & 63] + hexTable2[128 | c & 63];
  }
  if (lastPos === 0)
    return str;
  if (lastPos < len)
    return out + str.slice(lastPos);
  return out;
}
function stringify(obj, sep = "&", eq = "=", { encodeURIComponent: encodeURIComponent2 = escape } = {}) {
  const final = [];
  for (const entry of Object.entries(obj)) {
    if (Array.isArray(entry[1])) {
      for (const val of entry[1]) {
        final.push(encodeURIComponent2(entry[0]) + eq + encodeURIComponent2(val));
      }
    } else if (typeof entry[1] !== "object" && entry[1] !== void 0) {
      final.push(entry.map(encodeURIComponent2).join(eq));
    } else {
      final.push(encodeURIComponent2(entry[0]) + eq);
    }
  }
  return final.join(sep);
}
var decode = parse;
var encode = stringify;
var unescape = decodeURIComponent;
var escape = encodeURIComponent;
var querystring_default = {
  parse,
  encodeStr,
  stringify,
  hexTable,
  decode,
  encode,
  unescape,
  escape
};
export {
  decode,
  querystring_default as default,
  encode,
  encodeStr,
  escape,
  hexTable,
  parse,
  stringify,
  unescape
};
