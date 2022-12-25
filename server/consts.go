package server

const (
	// esm.sh build version
	VERSION          = 99
	nodejsMinVersion = 16
	denoStdVersion   = "0.170.0"
	nodejsLatestLTS  = "16.18.1"
	nodeTypesVersion = "16.18.10"
)

// fix some package versions
var fixedPkgVersions = map[string]string{
	"@types/react@17": "17.0.52",
	"@types/react@18": "18.0.26",
	"isomorphic-ws@4": "5.0.0",
}

// stable build for UI libraries like react, to make sure the runtime is single copy
var stableBuild = map[string]bool{
	"react":  true,
	"preact": true,
	"vue":    true,
}
