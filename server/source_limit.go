package server

import (
	"errors"
	"os"
	"path/filepath"

	esbuild "github.com/ije/esbuild-internal/api"
)

const maxSourceComplexity = 64 * 1024

func checkSourceComplexity(code string) error {
	if len(code) <= maxSourceComplexity {
		return nil
	}
	complexity := 0
	// Count text inside literals and comments too: guessing lexer context can hide nesting.
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '$' {
			start := i
			for i+1 < len(code) {
				c = code[i+1]
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$') {
					break
				}
				i++
			}
			switch code[start : i+1] {
			case "new", "void", "typeof", "delete", "await", "yield", "do", "declare", "keyof", "unique", "infer", "is", "abstract", "readonly", "public", "private", "protected", "override":
				complexity++
			}
		} else if c >= 0x80 {
			complexity++
		} else {
			switch c {
			case '(', '[', '{', '<', '>', '!', '~', '?', ':', '=', '.', ',', '+', '-', '*', '/', '%', '&', '|', '^', '`', '\\', '@':
				complexity++
			}
		}
		if complexity > maxSourceComplexity {
			return errors.New("Source is too complex")
		}
	}
	return nil
}

var sourceLimitPlugin = esbuild.Plugin{
	Name: "source-limit",
	Setup: func(build esbuild.PluginBuild) {
		build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "file"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
			loader := loaders[filepath.Ext(args.Path)]
			if args.With["type"] == "json" {
				loader = esbuild.LoaderJSON
			}
			switch loader {
			case esbuild.LoaderJS, esbuild.LoaderJSX, esbuild.LoaderTS, esbuild.LoaderTSX, esbuild.LoaderJSON, esbuild.LoaderCSS:
			default:
				return esbuild.OnLoadResult{}, nil
			}
			data, err := os.ReadFile(args.Path)
			if err != nil {
				return esbuild.OnLoadResult{}, err
			}
			code := string(data)
			if err := checkSourceComplexity(code); err != nil {
				return esbuild.OnLoadResult{}, err
			}
			return esbuild.OnLoadResult{Contents: &code, ResolveDir: filepath.Dir(args.Path), Loader: loader}, nil
		})
	},
}
