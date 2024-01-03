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
	Code          string            `json:"code"`
	Loader        string            `json:"loader,omitempty"`
	Deps          map[string]string `json:"dependencies,omitempty"`
	Types         string            `json:"types,omitempty"`
	TransformOnly bool              `json:"transformOnly,omitempty"`
	Target        string            `json:"target,omitempty"`
	ImportMap     string            `json:"importMap,omitempty"`
	Hash          string            `json:"hash,omitempty"`
}

func apiHandler() rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if ctx.R.Method == "POST" {
			switch ctx.Path.String() {
			case "/build", "/transform":
				var input BuildInput
				err := json.NewDecoder(io.LimitReader(ctx.R.Body, 2*1024*1024)).Decode(&input)
				ctx.R.Body.Close()
				if err != nil {
					return rex.Err(400, "require valid json body")
				}
				if input.Code == "" {
					return rex.Err(400, "code is required")
				}
				if len(input.Code) > 1024*1024 {
					return rex.Err(429, "code is too large")
				}
				if !input.TransformOnly {
					input.TransformOnly = ctx.Path.String() == "/transform"
				}
				if input.TransformOnly {
					if targets[input.Target] == 0 {
						input.Target = getBuildTargetByUA(ctx.R.UserAgent())
					}
					if input.Hash != "" {
						if len(input.Hash) != 40 {
							return rex.Err(400, "invalid hash")
						}
						h := sha1.New()
						h.Write([]byte(input.Loader))
						h.Write([]byte(input.Code))
						h.Write([]byte(input.ImportMap))
						if hex.EncodeToString(h.Sum(nil)) != input.Hash {
							return rex.Err(400, "invalid hash")
						}
						savePath := fmt.Sprintf("publish/+%s.%s.mjs", input.Hash, input.Target)
						_, err := fs.Stat(savePath)
						if err == nil {
							r, err := fs.OpenFile(savePath)
							if err != nil {
								return rex.Err(500, "failed to read code")
							}
							code, err := io.ReadAll(r)
							r.Close()
							if err != nil {
								return rex.Err(500, "failed to read code")
							}
							return map[string]interface{}{
								"code": string(code),
							}
						}
					}
				}
				cdnOrigin := getCdnOrign(ctx)
				id, err := build(input, cdnOrigin)
				if err != nil {
					if strings.HasPrefix(err.Error(), "<400> ") {
						return rex.Err(400, err.Error()[6:])
					}
					return rex.Err(500, "failed to save code")
				}
				ctx.W.Header().Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
				if input.TransformOnly {
					if input.Hash != "" {
						go fs.WriteFile(fmt.Sprintf("publish/+%s.%s.mjs", input.Hash, input.Target), strings.NewReader(id))
					}
					return map[string]interface{}{
						"code": id,
					}
				}
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

func build(input BuildInput, cdnOrigin string) (id string, err error) {
	loader := "tsx"
	switch input.Loader {
	case "js", "jsx", "ts", "tsx":
		loader = input.Loader
	case "babel":
		loader = "tsx"
	default:
		if input.Loader != "" {
			return "", errors.New("<400> invalid loader")
		}
	}
	target := api.ESNext
	if input.Target != "" {
		if t, ok := targets[input.Target]; ok {
			target = t
		} else {
			return "", errors.New("<400> invalid target")
		}
	}
	if input.Deps == nil {
		input.Deps = map[string]string{}
	}

	imports := map[string]string{}
	trailingSlashImports := map[string]string{}
	jsxImportSource := ""

	var im map[string]interface{}
	if json.Unmarshal([]byte(input.ImportMap), &im) == nil {
		v, ok := im["imports"]
		if ok {
			m, ok := v.(map[string]interface{})
			if ok {
				for key, v := range m {
					if value, ok := v.(string); ok && value != "" {
						if strings.HasSuffix(key, "/") {
							trailingSlashImports[key] = value
						} else {
							if key == "@jsxImportSource" {
								jsxImportSource = value
							}
							imports[key] = value
						}
					}
				}
			}
		}
	}

	onResolver := func(args api.OnResolveArgs) (api.OnResolveResult, error) {
		path := args.Path
		if input.TransformOnly {
			if value, ok := imports[path]; ok {
				path = value
			} else {
				for key, value := range trailingSlashImports {
					if strings.HasPrefix(path, key) {
						path = value + path[len(key):]
						break
					}
				}
			}
		} else {
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
		}
		return api.OnResolveResult{
			Path:     path,
			External: true,
		}, nil
	}
	stdin := &api.StdinOptions{
		Contents:   input.Code,
		ResolveDir: "/",
		Sourcefile: "index." + loader,
		Loader:     api.LoaderTSX,
	}
	jsx := api.JSXTransform
	if jsxImportSource != "" {
		jsx = api.JSXAutomatic
	}
	opts := api.BuildOptions{
		Outdir:           "/esbuild",
		Stdin:            stdin,
		Platform:         api.PlatformBrowser,
		Format:           api.FormatESModule,
		Target:           target,
		JSX:              jsx,
		JSXImportSource:  jsxImportSource,
		Bundle:           true,
		TreeShaking:      api.TreeShakingFalse,
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
	}
	ret := api.Build(opts)
	if len(ret.Errors) > 0 {
		return "", errors.New("<400> failed to validate code: " + ret.Errors[0].Text)
	}
	if len(ret.OutputFiles) == 0 {
		return "", errors.New("<400> failed to validate code: no output files")
	}
	code := ret.OutputFiles[0].Contents
	if input.TransformOnly {
		return string(code), nil
	}
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
