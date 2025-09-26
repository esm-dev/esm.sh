package web

import (
	"errors"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/ije/esbuild-internal/config"
	"github.com/ije/esbuild-internal/js_ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
)

// isModulePath checks if the given string is a module path.
func isModulePath(s string) bool {
	switch path.Ext(s) {
	case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".svelte", ".vue":
		return true
	default:
		return false
	}
}

// isHttpSepcifier returns true if the specifier is a remote URL.
func isHttpSepcifier(specifier string) bool {
	return strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "http://")
}

// isRelPathSpecifier returns true if the specifier is a local path.
func isRelPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}

// isAbsPathSpecifier returns true if the specifier is an absolute path.
func isAbsPathSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "file://")
}

// validateModule validates javascript/typescript module from the given file.
func validateModule(filename string) (namedExports []string, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	ext := path.Ext(filename)
	parserOpts := js_parser.OptionsFromConfig(&config.Options{
		JSX: config.JSXOptions{
			Parse: ext == ".jsx" || ext == ".tsx",
		},
		TS: config.TSOptions{
			Parse: ext == ".ts" || ext == ".tsx" || ext == ".mts",
		},
	})
	ast, pass := js_parser.Parse(log, logger.Source{
		Index:          0,
		KeyPath:        logger.Path{Text: "<stdin>"},
		PrettyPaths:    logger.PrettyPaths{Rel: "<stdin>"},
		IdentifierName: "stdin",
		Contents:       string(data),
	}, parserOpts)
	if !pass {
		err = errors.New("invalid syntax, require javascript/typescript")
		return
	}
	if ast.ExportsKind == js_ast.ExportsCommonJS {
		err = errors.New("not a module")
		return
	}
	if ast.ExportsKind == js_ast.ExportsESMWithDynamicFallback {
		err = errors.New("\"export * from\" syntax is no allowed")
		return
	}
	namedExports = make([]string, len(ast.NamedExports))
	i := 0
	for name := range ast.NamedExports {
		namedExports[i] = name
		i++
	}
	return
}

// dummyResponseWriter is a dummy http.ResponseWriter that does nothing.
type dummyResponseWriter struct {
	header http.Header
}

func (w *dummyResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *dummyResponseWriter) WriteHeader(statusCode int) {
}

func (w *dummyResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}
