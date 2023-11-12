package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

type BuildInput struct {
	Code   string            `json:"code"`
	Loader string            `json:"loader,omitempty"`
	Deps   map[string]string `json:"dependencies,omitempty"`
	Types  string            `json:"types,omitempty"`
}

func apiHandler() rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if ctx.R.Method == "POST" || ctx.R.Method == "PUT" {
			switch ctx.Path.String() {
			case "/build":
				var input BuildInput
				defer ctx.R.Body.Close()
				switch ct := ctx.R.Header.Get("Content-Type"); ct {
				case "application/json":
					err := json.NewDecoder(ctx.R.Body).Decode(&input)
					if err != nil {
						return rex.Err(400, "failed to parse input config: "+err.Error())
					}
				case "application/javascript", "text/javascript", "application/typescript", "text/typescript":
					code, err := io.ReadAll(ctx.R.Body)
					if err != nil {
						return rex.Err(400, "failed to read code: "+err.Error())
					}
					input.Code = string(code)
					if strings.Contains(ct, "javascript") {
						input.Loader = "jsx"
					} else {
						input.Loader = "tsx"
					}
				default:
					return rex.Err(400, "invalid content type")
				}
				if input.Code == "" {
					return rex.Err(400, "code is required")
				}
				id, err := build(input)
				if err != nil {
					if strings.HasPrefix(err.Error(), "<400> ") {
						return rex.Err(400, err.Error()[6:])
					}
					return rex.Err(500, "failed to save code")
				}
				cdnOrigin := getCdnOrign(ctx)
				ctx.W.Header().Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
				return map[string]interface{}{
					"id":        id,
					"url":       fmt.Sprintf("%s/~%s", cdnOrigin, id),
					"bundleUrl": fmt.Sprintf("%s/~%s?bundle", cdnOrigin, id),
				}
			default:
				return rex.Err(404, "not found")
			}
		}
		return nil
	}
}

func build(input BuildInput) (id string, err error) {
	loader := "tsx"
	switch input.Loader {
	case "js", "jsx", "ts", "tsx":
		loader = input.Loader
	}
	stdin := &api.StdinOptions{
		Contents:   input.Code,
		ResolveDir: "/",
		Sourcefile: "index." + loader,
		Loader:     api.LoaderTSX,
	}
	if input.Deps == nil {
		input.Deps = map[string]string{}
	}
	onResolver := func(args api.OnResolveArgs) (api.OnResolveResult, error) {
		path := args.Path
		if isLocalSpecifier(path) {
			return api.OnResolveResult{}, errors.New("local specifier is not allowed")
		}
		if !isHttpSepcifier(path) {
			pkgName, version, subPath := splitPkgPath(strings.TrimPrefix(path, "npm:"))
			path = pkgName
			if subPath != "" {
				path += "/" + subPath
			}
			if version != "" {
				input.Deps[pkgName] = version
			} else if _, ok := input.Deps[pkgName]; !ok {
				input.Deps[pkgName] = "*"
			}
		}
		return api.OnResolveResult{
			Path:     path,
			External: true,
		}, nil
	}
	ret := api.Build(api.BuildOptions{
		Outdir:           "/esbuild",
		Stdin:            stdin,
		Platform:         api.PlatformBrowser,
		Format:           api.FormatESModule,
		TreeShaking:      api.TreeShakingTrue,
		Target:           api.ESNext,
		Bundle:           true,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		Write:            false,
		Plugins: []api.Plugin{
			{
				Name: "resolver",
				Setup: func(build api.PluginBuild) {
					build.OnResolve(api.OnResolveOptions{Filter: ".*"}, onResolver)
				},
			},
		},
	})
	if len(ret.Errors) > 0 {
		return "", errors.New("<400> failed to validate code: " + ret.Errors[0].Text)
	}
	if len(ret.OutputFiles) == 0 {
		return "", errors.New("<400> failed to validate code: no output files")
	}
	code := ret.OutputFiles[0].Contents
	if len(code) == 0 {
		return "", errors.New("<400> code is empty")
	}
	h := sha1.New()
	h.Write(code)
	if len(input.Deps) > 0 {
		keys := make(sort.StringSlice, len(input.Deps))
		i := 0
		for key := range input.Deps {
			keys[i] = key
			i++
		}
		keys.Sort()
		for _, key := range keys {
			h.Write([]byte(key))
			h.Write([]byte(input.Deps[key]))
		}
	}
	if input.Types != "" {
		h.Write([]byte(input.Types))
	}
	id = hex.EncodeToString(h.Sum(nil))
	record, err := db.Get("publish-" + id)
	if err != nil {
		return
	}
	if record == nil {
		_, err = fs.WriteFile(path.Join("publish", id, "index.mjs"), bytes.NewReader(code))
		if err == nil {
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			pkgJson := map[string]interface{}{
				"name":         "~" + id,
				"version":      "0.0.0",
				"dependencies": input.Deps,
				"type":         "module",
				"module":       "index.mjs",
			}
			if input.Types != "" {
				pkgJson["types"] = "index.d.ts"
				_, err = fs.WriteFile(path.Join("publish", id, "index.d.ts"), strings.NewReader(input.Types))
			}
			if err == nil {
				err = enc.Encode(pkgJson)
				if err == nil {
					_, err = fs.WriteFile(path.Join("publish", id, "package.json"), buf)
				}
			}
		}
		if err == nil {
			err = db.Put("publish-"+id, utils.MustEncodeJSON(map[string]interface{}{
				"createdAt": time.Now().Unix(),
			}))
		}
	}
	return
}

func auth(secret string) rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if secret != "" && ctx.R.Header.Get("Authorization") != "Bearer "+secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
	}
}
