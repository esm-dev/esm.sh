package server

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

var (
	npmReplacements = map[string]npmReplacement{}
	exportAsExpr    = regexp.MustCompile(`([\w$]+) as ([\w$]+)`)
)

type npmReplacement struct {
	esm []byte
	cjs []byte
}

func buildNpmReplacements(fs EmbedFS) (err error) {
	return walkEmbedFS(fs, "server/embed/npm-replacements", []string{".mjs"}, func(path string) error {
		esm, err := fs.ReadFile(path)
		if err != nil {
			return err
		}
		code, err := minify(string(esm), esbuild.LoaderJS, esbuild.ES2022)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		npmReplacements[strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "server/embed/npm-replacements/"), ".mjs"), "/index")] = npmReplacement{
			esm: esm,
			cjs: exportAsExpr.ReplaceAll(bytes.ReplaceAll(bytes.TrimSuffix(bytes.TrimSpace(code), []byte{';'}), []byte("export{"), []byte("return{")), []byte("$2:$1")),
		}
		return nil
	})
}

func walkEmbedFS(fs EmbedFS, dir string, exts []string, fn func(path string) error) error {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			if err := walkEmbedFS(fs, path, exts, fn); err != nil {
				return err
			}
		} else if endsWith(path, exts...) {
			if err := fn(path); err != nil {
				return err
			}
		}
	}
	return nil
}
