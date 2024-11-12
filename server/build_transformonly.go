package server

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/esm-dev/esm.sh/server/common"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type TransformOptions struct {
	Filename        string          `json:"filename"`
	Lang            string          `json:"lang"`
	Code            string          `json:"code"`
	ImportMap       json.RawMessage `json:"importMap"`
	JsxImportSource string          `json:"jsxImportSource"`
	Target          string          `json:"target"`
	SourceMap       string          `json:"sourceMap"`
	Minify          bool            `json:"minify"`
}

type ResolvedTransformOptions struct {
	TransformOptions
	importMap     common.ImportMap
	globalVersion string
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map"`
}

func transform(npmrc *NpmRC, options *ResolvedTransformOptions) (out TransformOutput, err error) {
	target := esbuild.ESNext
	if options.Target != "" {
		if t, ok := targets[options.Target]; ok {
			target = t
		} else {
			err = errors.New("invalid target")
			return
		}
	}

	loader := esbuild.LoaderJS
	sourceCode := options.Code
	jsxImportSource := options.JsxImportSource

	if options.Lang == "" && options.Filename != "" {
		filename, _ := utils.SplitByFirstByte(options.Filename, '?')
		_, basename := utils.SplitByLastByte(filename, '/')
		_, options.Lang = utils.SplitByLastByte(basename, '.')
	}
	switch options.Lang {
	case "jsx":
		loader = esbuild.LoaderJSX
	case "ts":
		loader = esbuild.LoaderTS
	case "tsx":
		loader = esbuild.LoaderTSX
	case "css":
		loader = esbuild.LoaderCSS
	case "vue":
		o, e := transformVue(npmrc, options.importMap, []string{options.Filename, options.Code})
		if e != nil {
			log.Error("failed to transform vue:", e)
			err = errors.New("failed to transform vue")
			return
		}
		sourceCode = o.Code
		if o.Lang == "ts" {
			loader = esbuild.LoaderTS
		}
	case "svelte":
		o, e := transformSvelte(npmrc, options.importMap, []string{options.Filename, options.Code})
		if e != nil {
			log.Error("failed to transform svelte:", e)
			err = errors.New("failed to transform svelte")
			return
		}
		sourceCode = o.Code
	case "md":
	}

	if jsxImportSource == "" && (loader == esbuild.LoaderJSX || loader == esbuild.LoaderTSX) {
		var ok bool
		for _, key := range []string{"@jsxRuntime", "@jsxImportSource", "preact", "react"} {
			jsxImportSource, ok = options.importMap.Resolve(key)
			if ok {
				break
			}
		}
		if !ok {
			jsxImportSource = "react"
		}
	}

	sourceMap := esbuild.SourceMapNone
	if options.SourceMap == "external" {
		sourceMap = esbuild.SourceMapExternal
	} else if options.SourceMap == "inline" {
		sourceMap = esbuild.SourceMapInline
	}

	filename := options.Filename
	if filename == "" {
		filename = "source." + options.Lang
	}
	stdin := &esbuild.StdinOptions{
		Sourcefile: filename,
		Contents:   sourceCode,
		Loader:     loader,
	}
	opts := esbuild.BuildOptions{
		Stdin:             stdin,
		Platform:          esbuild.PlatformBrowser,
		Format:            esbuild.FormatESModule,
		Target:            target,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   strings.TrimSuffix(jsxImportSource, "/"),
		MinifyWhitespace:  options.Minify,
		MinifySyntax:      options.Minify,
		MinifyIdentifiers: options.Minify,
		Sourcemap:         sourceMap,
		Bundle:            true,
		Outdir:            "/esbuild",
		Write:             false,
		Plugins: []esbuild.Plugin{
			{
				Name: "resolver",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						path, _ := options.importMap.Resolve(args.Path)
						if strings.HasSuffix(path, ".css") {
							path += "?module"
						}
						if isHttpSepcifier(path) && isHttpSepcifier(options.Filename) {
							orig, e1 := url.Parse(options.Filename)
							u, e2 := url.Parse(path)
							if e1 == nil && e2 == nil && u.Scheme == orig.Scheme && u.Host == orig.Host {
								if strings.HasPrefix(u.Path, ".css") {
									path = appendQueryString(path, "module", "")
								} else if endsWith(u.Path, esExts...) || endsWith(u.Path, ".vue", ".svelet", ".md") {
									if !strings.HasPrefix(u.Path, ".md") || endsWith(path, "?jsx", "?svelte", "?vue") {
										if options.importMap.Src != "" {
											path = appendQueryString(path, "im", btoaUrl(options.importMap.Src))
										}
										if options.globalVersion != "" {
											path = appendQueryString(path, "v", options.globalVersion)
										}
									}
								}
							}
						}
						return esbuild.OnResolveResult{
							Path:     path,
							External: true,
						}, nil
					})
				},
			},
		},
	}
	ret := esbuild.Build(opts)
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
