package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/npm"
	esbuild "github.com/ije/esbuild-internal/api"
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
	importMap     *importmap.ImportMap
	globalVersion string
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map"`
}

// transform transforms the given code with the given options.
func transform(options *ResolvedTransformOptions) (out *TransformOutput, err error) {
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
	case "js":
		loader = esbuild.LoaderJS
	case "jsx":
		loader = esbuild.LoaderJSX
	case "ts":
		loader = esbuild.LoaderTS
	case "tsx":
		loader = esbuild.LoaderTSX
	case "css":
		loader = esbuild.LoaderCSS
	default:
		err = errors.New("unsupported language:" + options.Lang)
		return
	}

	if jsxImportSource == "" && (loader == esbuild.LoaderJSX || loader == esbuild.LoaderTSX) && options.importMap != nil {
		for _, key := range options.importMap.Imports.Keys() {
			if before, ok := strings.CutSuffix(key, "/jsx-runtime"); ok {
				jsxImportSource = before
				break
			}
		}
		if jsxImportSource == "" {
			for _, key := range []string{"react/", "preact/", "solid-js/", "mono-jsx/dom/", "mono-jsx/", "vue/"} {
				if options.importMap.Imports.Has(key) {
					jsxImportSource = strings.TrimSuffix(key, "/")
					break
				}
			}
		}
		if jsxImportSource == "" {
			jsxImportSource = "react"
		}
	}

	sourceMap := esbuild.SourceMapNone
	switch options.SourceMap {
	case "external":
		sourceMap = esbuild.SourceMapExternal
	case "inline":
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
						importerUrl, _ := url.Parse(args.Importer)
						if options.importMap != nil {
							path, ok := options.importMap.Resolve(args.Path, importerUrl)
							if ok && isHttpSpecifier(path) {
								return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
							}
						}
						return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
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
	out = &TransformOutput{}
	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") || strings.HasSuffix(file.Path, ".css") {
			out.Code = string(file.Contents)
		} else if strings.HasSuffix(file.Path, ".map") {
			out.Map = string(file.Contents)
		}
	}
	return
}

// treeShake tree-shakes the given javascript code with the given exports.
func treeShake(npmrc *NpmRC, pkg npm.Package, code []byte, exports []string, target esbuild.Target) ([]byte, error) {
	input := &esbuild.StdinOptions{
		Contents: fmt.Sprintf(`export { %s } from '.';`, strings.Join(exports, ", ")),
		Loader:   esbuild.LoaderJS,
	}
	plugins := []esbuild.Plugin{
		{
			Name: "tree-shaking",
			Setup: func(build esbuild.PluginBuild) {
				build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if args.Path == "." {
						return esbuild.OnResolveResult{Path: ".", Namespace: "memory", PluginData: code}, nil
					}
					sideEffects := esbuild.SideEffectsTrue
					if isRelPathSpecifier(args.Path) {
						pkgJson, err := npmrc.installPackage(pkg)
						if err != nil {
							return esbuild.OnResolveResult{}, err
						}
						if pkgJson.SideEffectsFalse {
							sideEffects = esbuild.SideEffectsFalse
						}
					}
					return esbuild.OnResolveResult{SideEffects: sideEffects, External: true}, nil
				})
				build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "memory"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					contents := string(args.PluginData.([]byte))
					return esbuild.OnLoadResult{Contents: &contents}, nil
				})
			},
		},
	}
	ret := esbuild.Build(esbuild.BuildOptions{
		Stdin:             input,
		Bundle:            true,
		Format:            esbuild.FormatESModule,
		Target:            target,
		Platform:          esbuild.PlatformBrowser,
		MinifyWhitespace:  config.Minify,
		MinifyIdentifiers: config.Minify,
		MinifySyntax:      config.Minify,
		Outdir:            "/esbuild",
		Write:             false,
		Plugins:           plugins,
	})
	if len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.OutputFiles[0].Contents, nil
}

// minify minifies the given javascript code.
func minify(code string, loader esbuild.Loader, target esbuild.Target) ([]byte, error) {
	ret := esbuild.Transform(code, esbuild.TransformOptions{
		Target:            target,
		Format:            esbuild.FormatESModule,
		Platform:          esbuild.PlatformBrowser,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     esbuild.LegalCommentsExternal,
		Loader:            loader,
	})
	if len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return concatBytes(ret.LegalComments, ret.Code), nil
}
