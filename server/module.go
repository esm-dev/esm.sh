package server

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/esm-dev/esm.sh/server/common"
	esbuild "github.com/evanw/esbuild/pkg/api"
	esbuild_config "github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
	"github.com/ije/gox/utils"
)

var moduleExts = []string{".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".cjs", ".cts"}

// stripModuleExt strips the module extension from the given string.
func stripModuleExt(s string) string {
	for _, ext := range moduleExts {
		if strings.HasSuffix(s, ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

// stripEntryModuleExt strips the entry module extension from the given string.
func stripEntryModuleExt(s string) string {
	if s != "" {
		if strings.HasSuffix(s, ".mjs") || strings.HasSuffix(s, ".cjs") {
			return s[:len(s)-4]
		}
		return strings.TrimSuffix(s, ".js")
	}
	return ""
}

// validateModuleFile validates javascript/typescript module from the given file.
func validateModuleFile(filename string) (isESM bool, namedExports []string, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	parserOpts := js_parser.OptionsFromConfig(&esbuild_config.Options{
		JSX: esbuild_config.JSXOptions{
			Parse: endsWith(filename, ".jsx", ".tsx"),
		},
		TS: esbuild_config.TSOptions{
			Parse: endsWith(filename, ".ts", ".mts", ".cts", ".tsx"),
		},
	})
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPath:     "<stdin>",
		IdentifierName: "stdin",
		Contents:       string(data),
	}, parserOpts)
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	isESM = ast.ExportsKind == js_ast.ExportsESM || ast.ExportsKind == js_ast.ExportsESMWithDynamicFallback
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
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

// bundleHttpModule bundles the http module and it's submodules.
func bundleHttpModule(npmrc *NpmRC, entry string, importMap common.ImportMap, collectDependencies bool, fetchClient *HttpClient) (js []byte, jsx bool, css []byte, dependencyTree map[string][]byte, err error) {
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
									} else {
										return esbuild.OnResolveResult{Path: path, Namespace: "url"}, nil
									}
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
							return esbuild.OnLoadResult{}, errors.New("failed to fetch module " + args.Path + ": " + res.Status)
						}
						data, err := io.ReadAll(res.Body)
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
								jsxCode, err := common.RenderMarkdown([]byte(code), common.MarkdownRenderKindJSX)
								if err != nil {
									return esbuild.OnLoadResult{}, err
								}
								code = string(jsxCode)
								loader = esbuild.LoaderJSX
								jsx = true
							} else if query.Has("svelte") {
								svelteCode, err := common.RenderMarkdown([]byte(code), common.MarkdownRenderKindSvelte)
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
								vueCode, err := common.RenderMarkdown([]byte(code), common.MarkdownRenderKindVue)
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
								js, err := common.RenderMarkdown([]byte(code), common.MarkdownRenderKindJS)
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
