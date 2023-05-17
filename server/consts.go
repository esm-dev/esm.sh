package server

const (
	// esm.sh build version
	VERSION = 122
	// esm.sh stable build version, used for UI libraries like react, to make sure the runtime is single copy
	// change this carefully!
	STABLE_VERSION = 118
)

const (
	nodejsMinVersion = 16
	nodejsLatestLTS  = "18.16.0"
	nodeTypesVersion = "18.16.9"
	denoStdVersion   = "0.177.0"
)

// fix some npm package versions
var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.59",
	"@types/react@18": "18.2.6",
	"isomorphic-ws@4": "5.0.0",
	"resolve@1.22":    "1.22.2", // 1.22.3+ will read package.json from disk
}

// css packages
var cssPackages = map[string]string{
	"normalize.css": "normalize.css",
	"@unocss/reset": "tailwind.css",
	"reset-css":     "reset.css",
}

// stable build for UI libraries like react, to make sure the runtime is single copy
var stableBuild = map[string]bool{
	"preact":   true,
	"react":    true,
	"solid-js": true,
	"svelte":   true,
	"vue":      true,
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

// native node packages, for `deno` target use `npm:package` to import (skip build)
var nativeNodePackages = []string{
	"@achingbrain/ssdp",
	"default-gateway",
	"fsevent",
	"re2",
	"zlib-sync",
}

var denoNextUnspportedNodeModules = map[string]bool{
	"inspector": true,
}
