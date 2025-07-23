package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/gfm"
	"github.com/esm-dev/esm.sh/internal/importmap"
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
	importMap     importmap.ImportMap
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
						path, _ := options.importMap.Resolve(args.Path)
						return esbuild.OnResolveResult{Path: path, External: true}, nil
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

// bundleHttpModule bundles the http module and it's submodules.
func bundleHttpModule(npmrc *NpmRC, entry string, importMap importmap.ImportMap, collectDependencies bool, fetchClient *fetch.FetchClient) (js []byte, jsx bool, css []byte, dependencyTree map[string][]byte, err error) {
	if !isHttpSepcifier(entry) {
		err = errors.New("require a http module")
		return
	}
	entryUrl, err := url.Parse(entry)
	if err != nil {
		err = errors.New("invalid enrtry, require a valid url")
		return
	}
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{entry},
		Target:           esbuild.ESNext,
		Format:           esbuild.FormatESModule,
		Platform:         esbuild.PlatformBrowser,
		JSX:              esbuild.JSXPreserve,
		Bundle:           true,
		MinifyWhitespace: true,
		Outdir:           "/esbuild",
		Write:            false,
		Plugins: []esbuild.Plugin{
			{
				Name: "http-loader",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						path, _ := importMap.Resolve(args.Path)
						if isHttpSepcifier(args.Importer) && (isRelPathSpecifier(path) || isAbsPathSpecifier(path)) {
							u, e := url.Parse(args.Importer)
							if e == nil {
								var query string
								path, query = utils.SplitByFirstByte(path, '?')
								if query != "" {
									query = "?" + query
								}
								u = u.ResolveReference(&url.URL{Path: path})
								path = u.Scheme + "://" + u.Host + u.Path + query
							}
						}
						if isHttpSepcifier(path) && (args.Kind != esbuild.ResolveJSDynamicImport || collectDependencies) {
							u, e := url.Parse(path)
							if e == nil {
								if u.Scheme == entryUrl.Scheme && u.Host == entryUrl.Host {
									if (endsWith(u.Path, moduleExts...) || endsWith(u.Path, ".css", ".json", ".vue", ".svelte", ".md")) && !u.Query().Has("url") {
										return esbuild.OnResolveResult{Path: path, Namespace: "http"}, nil
									}
									return esbuild.OnResolveResult{Path: path, Namespace: "url"}, nil
								}
							}
						}
						return esbuild.OnResolveResult{Path: path, External: true}, nil
					})
					build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "url"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
						js := `export default ` + "`" + args.Path + "`"
						return esbuild.OnLoadResult{Contents: &js, Loader: esbuild.LoaderJS}, nil
					})
					build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "http"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
						url, err := url.Parse(args.Path)
						if err != nil {
							return esbuild.OnLoadResult{}, err
						}
						res, err := fetchClient.Fetch(url, nil)
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to fetch module " + args.Path + ": " + err.Error())
						}
						defer res.Body.Close()
						if res.StatusCode != 200 {
							if res.StatusCode == 404 {
								return esbuild.OnLoadResult{}, errors.New("module not found: " + args.Path)
							}
							if res.StatusCode == 301 || res.StatusCode == 302 || res.StatusCode == 307 || res.StatusCode == 308 {
								return esbuild.OnLoadResult{}, errors.New("failed to fetch module " + args.Path + ": redirect not allowed")
							}
							return esbuild.OnLoadResult{}, errors.New("failed to fetch module " + args.Path + ": " + res.Status)
						}
						data, err := io.ReadAll(io.LimitReader(res.Body, 5*MB))
						if err != nil {
							return esbuild.OnLoadResult{}, errors.New("failed to fetch module " + args.Path + ": " + err.Error())
						}
						if collectDependencies {
							if dependencyTree == nil {
								dependencyTree = make(map[string][]byte)
							}
							dependencyTree[args.Path] = data
						}
						code := string(data)
						loader := esbuild.LoaderJS
						switch path.Ext(url.Path) {
						case ".ts", ".mts", ".cts":
							loader = esbuild.LoaderTS
						case ".jsx":
							loader = esbuild.LoaderJSX
							jsx = true
						case ".tsx":
							loader = esbuild.LoaderTSX
							jsx = true
						case ".css":
							loader = esbuild.LoaderCSS
						case ".json":
							loader = esbuild.LoaderJSON
						case ".svelte":
							svelteVersion, err := resolveSvelteVersion(npmrc, importMap)
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							ret, err := transformSvelte(npmrc, svelteVersion, args.Path, code)
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							code = ret.Code
						case ".vue":
							vueVersion, err := resolveVueVersion(npmrc, importMap)
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							ret, err := transformVue(npmrc, vueVersion, args.Path, code)
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							code = ret.Code
							if ret.Lang == "ts" {
								loader = esbuild.LoaderTS
							}
						case ".md":
							query := url.Query()
							if query.Has("jsx") {
								jsxCode, err := gfm.Render([]byte(code), gfm.RenderFormatJSX)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								code = string(jsxCode)
								loader = esbuild.LoaderJSX
								jsx = true
							} else if query.Has("svelte") {
								svelteCode, err := gfm.Render([]byte(code), gfm.RenderFormatSvelte)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								svelteVersion, err := resolveSvelteVersion(npmrc, importMap)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								ret, err := transformSvelte(npmrc, svelteVersion, args.Path, string(svelteCode))
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								code = ret.Code
							} else if query.Has("vue") {
								vueCode, err := gfm.Render([]byte(code), gfm.RenderFormatVue)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								vueVersion, err := resolveVueVersion(npmrc, importMap)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								ret, err := transformVue(npmrc, vueVersion, args.Path, string(vueCode))
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								code = ret.Code
							} else {
								js, err := gfm.Render([]byte(code), gfm.RenderFormatJS)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								code = string(js)
							}
						}
						return esbuild.OnLoadResult{Contents: &code, Loader: loader}, nil
					})
				},
			},
		},
	})
	if len(ret.Errors) > 0 {
		err = errors.New(ret.Errors[0].Text)
		return
	}
	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") {
			js = file.Contents
		} else if strings.HasSuffix(file.Path, ".css") {
			css = file.Contents
		}
	}
	return
}

// treeShake tree-shakes the given javascript code with the given exports.
func treeShake(code []byte, exports []string, target esbuild.Target) ([]byte, error) {
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
					return esbuild.OnResolveResult{External: true}, nil
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
