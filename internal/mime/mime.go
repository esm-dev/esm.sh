package mime

import (
	"path"
	"strings"
)

var mimeExts = map[string][]string{
	"application/gzip":        {"gz"},
	"application/javascript;": {"js", "mjs", "cjs"},
	"application/json;":       {"json", "map"},
	"application/json5;":      {"json5"},
	"application/jsonc;":      {"jsonc"},
	"application/pdf":         {"pdf"},
	"application/tar":         {"tar"},
	"application/tar+gzip":    {"tgz", "tar.gz"},
	"application/wasm":        {"wasm"},
	"application/xml;":        {"xml", "plist", "tmLanguage", "tmTheme"},
	"application/zip":         {"zip"},
	"audio/mp4":               {"m4a"},
	"audio/mpeg":              {"mp3", "m3a"},
	"audio/ogg":               {"ogg", "oga"},
	"audio/wav":               {"wav"},
	"audio/webm":              {"weba"},
	"font/collection":         {"ttc"},
	"font/otf":                {"otf"},
	"font/ttf":                {"ttf"},
	"font/woff":               {"woff"},
	"font/woff2":              {"woff2"},
	"image/apng":              {"apng"},
	"image/avif":              {"avif"},
	"image/gif":               {"gif"},
	"image/jpeg":              {"jpg", "jpeg"},
	"image/png":               {"png"},
	"image/svg+xml;":          {"svg", "svgz"},
	"image/webp":              {"webp"},
	"image/x-icon":            {"ico"},
	"text/css":                {"css"},
	"text/csv":                {"csv"},
	"text/html":               {"html", "htm"},
	"text/jsx":                {"jsx"},
	"text/less":               {"less"},
	"text/markdown":           {"md", "markdown"},
	"text/mdx":                {"mdx"},
	"text/plain":              {"txt", "glsl"},
	"text/sass":               {"sass", "scss"},
	"text/stylus":             {"stylus", "styl"},
	"text/svelte":             {"svelte"},
	"text/tsx":                {"tsx"},
	"text/typescript":         {"ts", "mts", "cts"},
	"text/vue":                {"vue"},
	"text/x-fragment":         {"frag"},
	"text/x-vertex":           {"vert"},
	"text/yaml":               {"yaml", "yml"},
	"video/mp4":               {"mp4", "m4v"},
	"video/ogg":               {"ogv"},
	"video/webm":              {"webm"},
}
var mineMap = map[string]string{}

func init() {
	for k, v := range mimeExts {
		if strings.HasSuffix(k, ";") || strings.HasPrefix(k, "text/") {
			k = strings.TrimSuffix(k, ";") + "; charset=utf-8"
		}
		for _, ext := range v {
			mineMap["."+ext] = k
		}
	}
	mimeExts = nil
}

// GetContentType returns the MIME type of the file with the given filename.
func GetContentType(filename string) string {
	extname := path.Ext(filename)
	if extname == ".gz" && strings.HasSuffix(filename, ".tar.gz") {
		extname = ".tar.gz"
	}
	return mineMap[extname]
}
