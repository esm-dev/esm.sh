package server

import (
	"encoding/json"
	"errors"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type TransformInput struct {
	Filename        string          `json:"filename"`
	Lang            string          `json:"lang"`
	Code            string          `json:"code"`
	ImportMap       json.RawMessage `json:"importMap"`
	JsxImportSource string          `json:"jsxImportSource"`
	Target          string          `json:"target"`
	SourceMap       string          `json:"sourceMap"`
	Minify          bool            `json:"minify"`
}

type TransformOptions struct {
	TransformInput
	unocssInput   []string
	importMap     ImportMap
	globalVersion string
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map,omitempty"`
}

func transform(npmrc *NpmRC, options TransformOptions) (out TransformOutput, err error) {
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
		_, options.Lang = utils.SplitByLastByte(options.Filename, '.')
	}
	switch options.Lang {
	case "jsx":
		loader = esbuild.LoaderJSX
	case "ts":
		loader = esbuild.LoaderTS
	case "tsx":
		loader = esbuild.LoaderTSX
	case "css":
		if options.unocssInput != nil {
			// pre-process uno.css
			o, e := npmrc.preTransform("unocss", "", strings.Join(options.unocssInput, "\n"), sourceCode, "esm-unocss@0.8.0", "@iconify/json@2.2.260")
			if e != nil {
				log.Error("failed to generate uno.css:", e)
				err = errors.New("failed to generate uno.css")
				return
			}
			sourceCode = o.Code
		}
		loader = esbuild.LoaderCSS
	case "vue":
		var vueVersion string
		vueVersion, err = npmrc.getVueLoaderVersion(options.importMap)
		if err != nil {
			return
		}
		// pre-process Vue SFC
		o, e := npmrc.preTransform("vue", vueVersion, options.Filename, sourceCode)
		if e != nil {
			log.Error("failed to transform vue:", e)
			err = errors.New("failed to transform vue")
			return
		}
		sourceCode = o.Code
	case "svelte":
		var svelteVersion string
		svelteVersion, err = npmrc.getSvelteLoaderVersion(options.importMap)
		if err != nil {
			return
		}
		// pre-process svelte component
		o, e := npmrc.preTransform("svelte", svelteVersion, options.Filename, sourceCode)
		if e != nil {
			log.Error("failed to transform svelte:", e)
			err = errors.New("failed to transform svelte")
			return
		}
		sourceCode = o.Code
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
						if isRelativeSpecifier(path) {
							if options.importMap.Src != "" {
								suffix := "N"
								if options.importMap.Support {
									suffix = "y"
								}
								path = appendQueryString(path, "im", suffix+btoaUrl(options.importMap.Src))
							}
							if options.globalVersion != "" {
								path = appendQueryString(path, "v", options.globalVersion)
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
