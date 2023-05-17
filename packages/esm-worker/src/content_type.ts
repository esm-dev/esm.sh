import { splitBy } from "./utils.ts";

// MIME types for web
const mimeTypes: Record<string, string[]> = {
  // application
  "application/javascript": ["js", "mjs"],
  "application/typescript": ["ts", "mts", "tsx"],
  "application/wasm": ["wasm"],
  "application/json": ["json", "map"],
  "application/jsonc": ["jsonc"],
  "application/pdf": ["pdf"],
  "application/xml": ["xml", "plist", "tmLanguage", "tmTheme"],
  "application/zip": ["zip"],
  "application/gzip": ["gz"],
  "application/tar": ["tar"],
  "application/tar+gzip": ["tar.gz", "tgz"],
  // text
  "text/html": ["html", "htm"],
  "text/markdown": ["md", "markdown"],
  "text/mdx": ["mdx"],
  "text/jsx": ["jsx"],
  "text/vue": ["vue"],
  "text/svelte": ["svelte"],
  "text/css": ["css"],
  "text/less": ["less"],
  "text/sass": ["sass", "scss"],
  "text/stylus": ["stylus", "styl"],
  "text/csv": ["csv"],
  "text/yaml": ["yaml", "yml"],
  "text/plain": ["txt", "glsl"],
  // font
  "font/ttf": ["ttf"],
  "font/otf": ["otf"],
  "font/woff": ["woff"],
  "font/woff2": ["woff2"],
  "font/collection": ["ttc"],
  // image
  "image/jpeg": ["jpg", "jpeg"],
  "image/png": ["png"],
  "image/gif": ["gif"],
  "image/webp": ["webp"],
  "image/avif": ["avif"],
  "image/svg+xml": ["svg", "svgz"],
  "image/x-icon": ["ico"],
  // audio
  "audio/mp4": ["m4a"],
  "audio/mpeg": ["mp3", "m3a"],
  "audio/ogg": ["ogg", "oga"],
  "audio/wav": ["wav"],
  "audio/webm": ["weba"],
  // video
  "video/mp4": ["mp4", "m4v"],
  "video/ogg": ["ogv"],
  "video/webm": ["webm"],
  // shader
  "x-shader/x-fragment": ["frag"],
  "x-shader/x-vertex": ["vert"],
};

const typesMap = new Map<string, string>();
for (const contentType in mimeTypes) {
  for (const ext of mimeTypes[contentType]) {
    typesMap.set(ext, contentType);
  }
}

/** get the content type by file name */
export function getContentType(path: string): string {
  const [pathname] = splitBy(path, "?");
  let [, ext] = splitBy(pathname, ".", true);
  if (ext === "gz" && pathname.endsWith(".tar.gz")) {
    ext = "tar.gz";
  }
  return typesMap.get(ext) ?? "application/octet-stream";
}
