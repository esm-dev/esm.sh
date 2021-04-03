package server

import (
	"encoding/json"
	"path"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/postui/postdb/q"
)

var targets = map[string]api.Target{
	"deno":   api.ESNext,
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

func findESM(id string) (esm *ESMeta, packageCSS bool, ok bool) {
	post, err := db.Get(q.Alias(id), q.K("esmeta", "css"))
	if err == nil {
		err = json.Unmarshal(post.KV.Get("esmeta"), &esm)
		if err != nil {
			db.Delete(q.Alias(id))
			return
		}

		if !fileExists(path.Join(config.storageDir, "builds", id+".js")) {
			db.Delete(q.Alias(id))
			return
		}

		if val := post.KV.Get("css"); len(val) == 1 && val[0] == 1 {
			packageCSS = fileExists(path.Join(config.storageDir, "builds", id+".css"))
		}
		ok = true
	}
	return
}
