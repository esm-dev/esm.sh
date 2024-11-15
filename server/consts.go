package server

// esm.sh version
const VERSION = 136
const assetMaxSize = 50 * 1024 * 1024 // limit asset size to 50mb

// asset extensions
var assetExts = map[string]bool{
	"node":       true,
	"wasm":       true,
	"less":       true,
	"sass":       true,
	"scss":       true,
	"stylus":     true,
	"styl":       true,
	"json":       true,
	"jsonc":      true,
	"csv":        true,
	"xml":        true,
	"plist":      true,
	"tmLanguage": true,
	"tmTheme":    true,
	"yml":        true,
	"yaml":       true,
	"txt":        true,
	"glsl":       true,
	"frag":       true,
	"vert":       true,
	"md":         true,
	"mdx":        true,
	"markdown":   true,
	"html":       true,
	"htm":        true,
	"svg":        true,
	"png":        true,
	"jpg":        true,
	"jpeg":       true,
	"webp":       true,
	"gif":        true,
	"ico":        true,
	"eot":        true,
	"ttf":        true,
	"otf":        true,
	"woff":       true,
	"woff2":      true,
	"m4a":        true,
	"mp3":        true,
	"m3a":        true,
	"ogg":        true,
	"oga":        true,
	"wav":        true,
	"weba":       true,
	"gz":         true,
	"tgz":        true,
}

// css packages
var cssPackages = map[string]string{
	"@unocss/reset":    "tailwind.css",
	"inter-ui":         "inter.css",
	"normalize.css":    "normalize.css",
	"modern-normalize": "modern-normalize.css",
	"reset-css":        "reset.css",
}
