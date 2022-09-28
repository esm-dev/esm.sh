package server

const (
	// esm.sh build version
	VERSION          = 95
	nodejsMinVersion = 16
	denoStdVersion   = "0.153.0"
	nodejsLatestLTS  = "16.16.0"
	nodeTypesVersion = "16.11.49"
)

var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.49",
	"@types/react@18": "18.0.18",
}

// stable build for UI libraries like react, to make sure the runtime is single copy
var stableBuild = map[string]bool{
	"react":  true,
	"preact": true,
	"vue":    true,
}
