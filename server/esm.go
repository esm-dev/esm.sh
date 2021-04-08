package server

import (
	"encoding/json"
	"path"

	"github.com/postui/postdb/q"
)

// ESMeta defines the ES Module meta
type ESMeta struct {
	*NpmPackage
	Exports []string `json:"exports"`
	Dts     string   `json:"dts"`
}

func findESM(id string) (esm *ESMeta, pkgCSS bool, ok bool) {
	post, err := db.Get(q.Alias(id), q.Select("esmeta", "css"))
	if err == nil {
		err = json.Unmarshal(post.KV["esmeta"], &esm)
		if err != nil {
			db.Delete(q.Alias(id))
			return
		}

		if !fileExists(path.Join(config.storageDir, "builds", id+".js")) {
			db.Delete(q.Alias(id))
			return
		}

		if val := post.KV["css"]; len(val) == 1 && val[0] == 1 {
			pkgCSS = fileExists(path.Join(config.storageDir, "builds", id+".css"))
		}
		ok = true
	}
	return
}
