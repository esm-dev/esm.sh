package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type ImportMap struct {
	Support bool                         `json:"$support,omitempty"`
	Imports map[string]string            `json:"imports,omitempty"`
	Scopes  map[string]map[string]string `json:"scopes,omitempty"`
}

type TransformInput struct {
	Filename         string          `json:"filename"`
	Lang             string          `json:"lang"`
	Code             string          `json:"code"`
	ImportMap        json.RawMessage `json:"importMap"`
	JsxImportSource  string          `json:"jsxImportSource"`
	Target           string          `json:"target"`
	SourceMap        string          `json:"sourceMap"`
	Minify           bool            `json:"minify"`
	vArg             string          `json:"-"`
	supportImportMap bool            `json:"-"`
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map,omitempty"`
}

func transform(npmrc *NpmRC, input TransformInput) (out TransformOutput, err error) {
	target := api.ESNext
	if input.Target != "" {
		if t, ok := targets[input.Target]; ok {
			target = t
		} else {
			err = errors.New("invalid target")
			return
		}
	}

	var importMap ImportMap
	if len(input.ImportMap) > 0 {
		if json.Unmarshal(input.ImportMap, &importMap) != nil {
			err = errors.New("invalid import map")
		}
	}
	if input.supportImportMap {
		importMap.Support = true
	}

	imports := map[string]string{}
	trailingSlashImports := map[string]string{}
	jsxImportSource := input.JsxImportSource

	for key, value := range importMap.Imports {
		if value != "" {
			if strings.HasSuffix(key, "/") {
				trailingSlashImports[key] = value
			} else {
				imports[key] = value
			}
		}
	}

	loader := api.LoaderJS
	sourceCode := input.Code

	if input.Lang == "" && input.Filename != "" {
		_, input.Lang = utils.SplitByLastByte(input.Filename, '.')
	}
	switch input.Lang {
	case "jsx":
		loader = api.LoaderJSX
	case "ts":
		loader = api.LoaderTS
	case "tsx":
		loader = api.LoaderTSX
	case "css":
		loader = api.LoaderCSS
	case "vue":
		vueVersion := "3"
		vueRuntimeModuleName, ok := imports["vue"]
		if ok {
			a := regexpVuePath.FindAllStringSubmatch(vueRuntimeModuleName, 1)
			if len(a) > 0 {
				vueVersion = a[0][1]
			}
		}
		if !regexpVersionStrict.MatchString(vueVersion) {
			info, e := npmrc.getPackageInfo("vue", vueVersion)
			if e != nil {
				err = e
				return
			}
			vueVersion = info.Version
		}
		if semverLessThan(vueVersion, "3.0.0") {
			err = errors.New("unsupported vue version, only 3.0.0+ is supported")
			return
		}
		// pre-process Vue SFC
		o, e := preTransform(npmrc, "vue", vueVersion, input.Filename, sourceCode)
		if e != nil {
			log.Error("failed to transform vue:", e)
			err = errors.New("failed to transform vue")
			return
		}
		sourceCode = o.Code
	case "svelte":
		svelteVersion := "4"
		sveltePath, ok := imports["svelte"]
		if ok {
			a := regexpSveltePath.FindAllStringSubmatch(sveltePath, 1)
			if len(a) > 0 {
				svelteVersion = a[0][1]
			}
		}
		if !regexpVersionStrict.MatchString(svelteVersion) {
			info, e := npmrc.getPackageInfo("svelte", svelteVersion)
			if e != nil {
				err = e
				return
			}
			svelteVersion = info.Version
		}
		if semverLessThan(svelteVersion, "4.0.0") {
			err = errors.New("unsupported svelte version, only 4.0.0+ is supported")
			return
		}
		// pre-process svelte component
		o, e := preTransform(npmrc, "svelte", svelteVersion, input.Filename, sourceCode)
		if e != nil {
			log.Error("failed to transform svelte:", e)
			err = errors.New("failed to transform svelte")
			return
		}
		sourceCode = o.Code
	}

	if jsxImportSource == "" && (loader == api.LoaderJSX || loader == api.LoaderTSX) {
		var ok bool
		for _, key := range []string{"@jsxRuntime", "@jsxImportSource", "react", "preact"} {
			jsxImportSource, ok = imports[key]
			if ok {
				break
			}
		}
		if !ok {
			jsxImportSource = "react"
		}
	}

	sourceMap := api.SourceMapNone
	if input.SourceMap == "external" {
		sourceMap = api.SourceMapExternal
	} else if input.SourceMap == "inline" {
		sourceMap = api.SourceMapInline
	}

	filename := input.Filename
	if filename == "" {
		filename = "source." + input.Lang
	}
	stdin := &api.StdinOptions{
		Sourcefile: filename,
		Contents:   sourceCode,
		Loader:     loader,
	}
	opts := api.BuildOptions{
		Outdir:            "/esbuild",
		Stdin:             stdin,
		Platform:          api.PlatformBrowser,
		Format:            api.FormatESModule,
		Target:            target,
		JSX:               api.JSXAutomatic,
		JSXImportSource:   strings.TrimSuffix(jsxImportSource, "/"),
		MinifyWhitespace:  input.Minify,
		MinifySyntax:      input.Minify,
		MinifyIdentifiers: input.Minify,
		Sourcemap:         sourceMap,
		Write:             false,
		Bundle:            true,
		Plugins: []api.Plugin{
			{
				Name: "resolver",
				Setup: func(build api.PluginBuild) {
					build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						path := args.Path
						if loader != api.LoaderCSS {
							if value, ok := imports[path]; ok {
								if !importMap.Support {
									path = value
								}
							} else {
								matched := false
								for key, value := range trailingSlashImports {
									if strings.HasPrefix(path, key) {
										if !importMap.Support {
											path = value + path[len(key):]
										}
										matched = true
										break
									}
								}
								// if not match leading slash imports, try to match regular imports
								if !matched {
									for key, value := range imports {
										if strings.HasPrefix(path, key+"/") {
											path = value + "/" + path[len(key+"/"):]
											break
										}
									}
								}
							}
							if strings.HasSuffix(path, ".css") {
								path += "?module"
							}
							if input.vArg != "" && isRelativeSpecifier(path) {
								q := "?"
								if strings.Contains(path, "?") {
									q = "&"
								}
								path += q + "v=" + input.vArg
							}
						}
						return api.OnResolveResult{
							Path:     path,
							External: true,
						}, nil
					})
				},
			},
		},
	}
	ret := api.Build(opts)
	if len(ret.Errors) > 0 {
		err = errors.New("failed to validate code: " + ret.Errors[0].Text)
		return
	}
	if len(ret.OutputFiles) == 0 {
		err = errors.New("failed to validate code: no output files")
		return
	}
	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") || strings.HasSuffix(file.Path, ".css") {
			out.Code = string(file.Contents)
		} else if strings.HasSuffix(file.Path, ".map") {
			out.Map = string(file.Contents)
		}
	}
	return
}

func preTransform(npmrc *NpmRC, loaderName string, loaderVersion string, filename, sourceCode string) (output *TransformOutput, err error) {
	pkgInfo, e := npmrc.installPackage(EsmURL{PkgName: loaderName, PkgVersion: loaderVersion})
	if e != nil {
		err = errors.New("failed to install " + loaderName + "@" + loaderVersion)
		return
	}
	dirname := path.Join(npmrc.StoreDir(), loaderName+"@"+pkgInfo.Version)
	loaderJsFp := path.Join(dirname, "loader.mjs")
	if !existsFile(loaderJsFp) {
		var loaderJS []byte
		major, _ := utils.SplitByFirstByte(pkgInfo.Version, '.')
		loaderJS, err = embedFS.ReadFile(fmt.Sprintf("server/embed/internal/%s_loader@%s.js", loaderName, major))
		if err != nil {
			err = fmt.Errorf("could not find %s loader", loaderName)
			return
		}
		err = os.WriteFile(loaderJsFp, loaderJS, 0644)
		if err != nil {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd := exec.CommandContext(
		ctx,
		"node",
		"--experimental-permission",
		"--allow-fs-read="+npmrc.StoreDir(),
		"loader.mjs",
	)
	cmd.Dir = dirname
	cmd.Stdin = bytes.NewReader(utils.MustEncodeJSON([]string{filename, sourceCode}))
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = fmt.Errorf("preTransform: %s", errBuf.String())
		}
		return
	}

	var ret TransformOutput
	err = json.Unmarshal(outBuf.Bytes(), &ret)
	if err != nil {
		return
	}

	output = &ret
	return
}
