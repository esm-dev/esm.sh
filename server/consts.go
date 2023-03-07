package server

const (
	// esm.sh build version
	VERSION = 111
	// esm.sh stable build version, used for UI libraries like react, to make sure the runtime is single copy
	// change this carefully
	STABLE_VERSION = 110
)

const (
	nodejsMinVersion = 16
	denoStdVersion   = "0.177.0"
	nodejsLatestLTS  = "16.19.1"
	nodeTypesVersion = "16.18.12"
)

// fix some package versions
var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.53",
	"@types/react@18": "18.0.28",
	"isomorphic-ws@4": "5.0.0",
}

// css packages
var cssPackages = map[string]string{
	"normalize.css": "normalize.css",
	"@unocss/reset": "tailwind.css",
	"reset-css":     "reset.css",
}

// stable build for UI libraries like react, to make sure the runtime is single copy
var stableBuild = map[string]bool{
	"react":  true,
	"preact": true,
	"vue":    true,
}

// allowlist for require mode when parsing cjs exports fails
var requireModeAllowList = []string{
	"@babel/types",
	"domhandler",
	"he",
	"keycode",
	"lru_map",
	"lz-string",
	"maplibre-gl",
	"postcss-selector-parser",
	"react-draggable",
	"resolve",
	"safe-buffer",
	"seedrandom",
	"stream-browserify",
	"stream-http",
	"typescript",
	"vscode-oniguruma",
	"web-streams-ponyfill",
}
