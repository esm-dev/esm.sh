package server

import "github.com/evanw/esbuild/pkg/api"

var targets = map[string]api.Target{
	"deno":   api.ESNext,
	"es2015": api.ES2015,
	"es2016": api.ES2016,
	"es2017": api.ES2017,
	"es2018": api.ES2018,
	"es2019": api.ES2019,
	"es2020": api.ES2020,
}

// ESMeta defines the ES Module meta
type ESMeta struct {
	*NpmPackage
	Exports []string `json:"exports"`
	Dts     string   `json:"dts"`
}
