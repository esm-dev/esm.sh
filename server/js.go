package server

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	esbuild_config "github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
)

var jsExts = []string{".mjs", ".js", ".jsx", ".mts", ".ts", ".tsx", ".cjs"}

// stripModuleExt strips the module extension from the given string.
func stripModuleExt(s string, exts ...string) string {
	if len(exts) == 0 {
		exts = jsExts
	}
	for _, ext := range exts {
		if strings.HasSuffix(s, ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

// validateJSFile validates the given javascript file.
func validateJSFile(filename string) (isESM bool, namedExports []string, err error) {
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
		Contents:       string(data),
		IdentifierName: "stdin",
	}, parserOpts)
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	isESM = ast.ExportsKind == js_ast.ExportsESM
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
}

// minify minifies the given javascript code.
func minify(code string, target api.Target, loader api.Loader) ([]byte, error) {
	ret := api.Transform(code, api.TransformOptions{
		Target:            target,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsExternal,
		Loader:            loader,
	})
	if len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}

	return concatBytes(ret.LegalComments, ret.Code), nil
}

// bundleRemoteModules builds the remote module and it's submodules.
func bundleRemoteModules(entry string, ua string) ([]byte, error) {
	if !isHttpSepcifier(entry) {
		return nil, errors.New("require a remote module")
	}
	entryUrl, err := url.Parse(entry)
	if err != nil {
		return nil, errors.New("invalid enrtry, require a valid url")
	}
	httpClient := &http.Client{
		Timeout: time.Minute,
	}
	ret := api.Build(api.BuildOptions{
		EntryPoints:      []string{entry},
		Bundle:           true,
		Format:           api.FormatESModule,
		Target:           api.ESNext,
		Platform:         api.PlatformBrowser,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		JSX:              api.JSXPreserve,
		LegalComments:    api.LegalCommentsNone,
		Plugins: []api.Plugin{
			{
				Name: "http-loader",
				Setup: func(build api.PluginBuild) {
					build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
						path := args.Path
						if isRelativeSpecifier(args.Path) && isHttpSepcifier(args.Importer) {
							u, e := url.Parse(args.Importer)
							if e == nil {
								path = u.ResolveReference(&url.URL{Path: args.Path}).String()
							}
						}
						if isHttpSepcifier(path) {
							u, e := url.Parse(path)
							if e == nil {
								if u.Host == entryUrl.Host && u.Scheme == entryUrl.Scheme {
									return api.OnResolveResult{Path: path, Namespace: "http"}, nil
								}
							}
						}
						return api.OnResolveResult{Path: path, External: true}, nil
					})
					build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "http"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
						url, err := url.Parse(args.Path)
						if err != nil {
							return api.OnLoadResult{}, err
						}
						req := &http.Request{
							Method: "GET",
							URL:    url,
							Header: map[string][]string{
								"User-Agent": {ua},
							},
						}
						resp, err := httpClient.Do(req)
						if err != nil {
							return api.OnLoadResult{}, errors.New("failed to read remote module " + args.Path + ": " + err.Error())
						}
						defer resp.Body.Close()
						data, err := io.ReadAll(resp.Body)
						if err != nil {
							return api.OnLoadResult{}, errors.New("failed to read remote module " + args.Path)
						}
						loader := api.LoaderJS
						switch path.Ext(url.Path) {
						case ".js", ".cjs", ".mjs":
							loader = api.LoaderJS
						case ".ts", ".cts", ".mts":
							loader = api.LoaderTS
						case ".jsx":
							loader = api.LoaderJSX
						case ".tsx":
							loader = api.LoaderTSX
						case ".css":
							loader = api.LoaderCSS
						case ".json":
							loader = api.LoaderJSON
						}
						code := string(data)
						return api.OnLoadResult{Contents: &code, Loader: loader}, nil
					})
				},
			},
		},
	})
	if len(ret.Errors) > 0 {
		return nil, errors.New(ret.Errors[0].Text)
	}
	return ret.OutputFiles[0].Contents, nil
}
