package server

const (
	// esm.sh build version
	VERSION          = 96
	nodejsMinVersion = 16
	denoStdVersion   = "0.153.0"
	nodejsLatestLTS  = "16.17.1"
	nodeTypesVersion = "16.11.62"
)

var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.50",
	"@types/react@18": "18.0.21",
}

// stable build for UI libraries like react, to make sure the runtime is single copy
var stableBuild = map[string]bool{
	"react":  true,
	"preact": true,
	"vue":    true,
}
