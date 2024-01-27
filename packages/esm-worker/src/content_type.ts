// MIME types for web
const mimeTypes = {
  // application
  "a/javascript;": ["js", "mjs"],
  "a/wasm": ["wasm"],
  "a/json;": ["json", "map"],
  "a/jsonc;": ["jsonc"],
  "a/json5;": ["json5"],
  "a/pdf": ["pdf"],
  "a/xml;": ["xml", "plist", "tmLanguage", "tmTheme"],
  "a/zip": ["zip"],
  "a/gzip": ["gz"],
  "a/tar": ["tar"],
  "a/tar+gzip": ["tar.gz", "tgz"],
  // text
  "t/html": ["html", "htm"],
  "t/markdown": ["md", "markdown"],
  "t/mdx": ["mdx"],
  "t/jsx": ["jsx"],
  "t/typescript": ["ts", "mts"],
  "t/tsx": ["tsx"],
  "t/vue": ["vue"],
  "t/svelte": ["svelte"],
  "t/css": ["css"],
  "t/less": ["less"],
  "t/sass": ["sass", "scss"],
  "t/stylus": ["stylus", "styl"],
  "t/csv": ["csv"],
  "t/yaml": ["yaml", "yml"],
  "t/plain": ["txt", "glsl"],
  "t/x-fragment": ["frag"],
  "t/x-vertex": ["vert"],
  // font
  "f/ttf": ["ttf"],
  "f/otf": ["otf"],
  "f/woff": ["woff"],
  "f/woff2": ["woff2"],
  "f/collection": ["ttc"],
  // image
  "i/jpeg": ["jpg", "jpeg"],
  "i/png": ["png"],
  "i/apng": ["apng"],
  "i/gif": ["gif"],
  "i/webp": ["webp"],
  "i/avif": ["avif"],
  "i/svg+xml;": ["svg", "svgz"],
  "i/x-icon": ["ico"],
  // audio
  "u/mp4": ["m4a"],
  "u/mpeg": ["mp3", "m3a"],
  "u/ogg": ["ogg", "oga"],
  "u/wav": ["wav"],
  "u/webm": ["weba"],
  // video
  "v/mp4": ["mp4", "m4v"],
  "v/ogg": ["ogv"],
  "v/webm": ["webm"],
  "v/x-matroska": ["mkv"],
};
const alias = {
  a: "application",
  t: "text",
  f: "font",
  i: "image",
  u: "audio",
  v: "video",
};
const defaultType = "binary/octet-stream";
const typesMap = Object.entries(mimeTypes).reduce(
  (map, [mimeType, exts]) => {
    const type = alias[mimeType.charAt(0)];
    const endsWithSemicolon = mimeType.endsWith(";");
    let suffix = mimeType.slice(1);
    if (type === "text" || endsWithSemicolon) {
      if (endsWithSemicolon) {
        suffix = suffix.slice(0, -1);
      }
      suffix += "; charset=utf-8";
    }
    exts.forEach((ext) => map.set(ext, type + suffix));
    return map;
  },
  new Map(),
);

export function getMimeType(filename: string): string {
  const idx = filename.lastIndexOf(".");
  if (idx < 0) return defaultType;
  let ext = filename.slice(idx + 1);
  if (ext === "gz" && filename.endsWith(".tar.gz")) {
    ext = "tar.gz";
  }
  return typesMap.get(ext) ?? defaultType;
}
