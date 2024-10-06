package server

import (
	"errors"
	"os"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	C "github.com/ije/esbuild-internal/config"
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
	parserOpts := js_parser.OptionsFromConfig(&C.Options{
		JSX: C.JSXOptions{
			Parse: endsWith(filename, ".jsx", ".tsx"),
		},
		TS: C.TSOptions{
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
