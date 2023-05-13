package server

const (
	// esm.sh build version
	VERSION = 120
	// esm.sh stable build version, used for UI libraries like react, to make sure the runtime is single copy
	// change this carefully!
	STABLE_VERSION = 118
)

const (
	nodejsMinVersion = 16
	nodejsLatestLTS  = "18.16.0"
	nodeTypesVersion = "18.16.0"
	denoStdVersion   = "0.177.0"
)

// fix some npm package versions
var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.58",
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
	"wasm":  true,
	"css":   true,
	"less":  true,
	"sass":  true,
	"scss":  true,
	"json":  true,
	"xml":   true,
	"yml":   true,
	"yaml":  true,
	"txt":   true,
	"md":    true,
	"html":  true,
	"htm":   true,
	"svg":   true,
	"png":   true,
	"jpg":   true,
	"webp":  true,
	"gif":   true,
	"eot":   true,
	"ttf":   true,
	"otf":   true,
	"woff":  true,
	"woff2": true,
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
