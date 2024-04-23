package server

const VERSION = 136

// css packages
var cssPackages = map[string]string{
	"@unocss/reset":    "tailwind.css",
	"inter-ui":         "inter.css",
	"normalize.css":    "normalize.css",
	"modern-normalize": "modern-normalize.css",
	"reset-css":        "reset.css",
}

var assetExts = map[string]bool{
	"wasm":       true,
	"css":        true,
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
	"pdf":        true,
	"txt":        true,
	"glsl":       true,
	"frag":       true,
	"vert":       true,
	"md":         true,
	"mdx":        true,
	"markdown":   true,
	"html":       true,
	"htm":        true,
	"vue":        true,
	"svelte":     true,
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
	"mp4":        true,
	"m4v":        true,
	"ogv":        true,
	"webm":       true,
	"zip":        true,
	"gz":         true,
	"tar":        true,
	"tgz":        true,
}

// native node packages, for `denonext` target use `npm:package` instead of url
var nativeNodePackages = []string{
	"@achingbrain/ssdp",
	"default-gateway",
	"fsevent",
	"lightningcss",
	"re2",
	"zlib-sync",
}

// force to use `npm:` specifier for `denonext` target to fix `createRequire` issue
var forceNpmSpecifiers = map[string]bool{
	"css-tree": true,
}
