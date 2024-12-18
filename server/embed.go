package server

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

var (
	embedFS         EmbedFS
	embedBuildCache sync.Map
	npmReplacements map[string]npmReplacement
)

type EmbedFS interface {
	ReadDir(name string) ([]os.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

type MockEmbedFS struct {
	root string
}

func (fs MockEmbedFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(path.Join(fs.root, name))
}

func (fs MockEmbedFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(path.Join(fs.root, name))
}

type npmReplacement struct {
	esm []byte
	cjs []byte
}

func buildNpmReplacements(fs EmbedFS) (err error) {
	npmReplacements = make(map[string]npmReplacement)
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
			cjs: regexpExportAsExpr.ReplaceAll(bytes.ReplaceAll(bytes.TrimSuffix(bytes.TrimSpace(code), []byte{';'}), []byte("export{"), []byte("return{")), []byte("$2:$1")),
		}
		return nil
	})
}

func buildEmbedTS(filename string, target string, debug bool) (js []byte, err error) {
	cacheKey := filename + "?" + target
	if data, ok := embedBuildCache.Load(cacheKey); ok {
		return data.([]byte), nil
	}

	data, err := embedFS.ReadFile("server/embed/" + filename)
	if err != nil {
		return
	}

	// replace `$TARGET` with the target
	data = bytes.ReplaceAll(data, []byte("$TARGET"), []byte(target))

	js, err = minify(string(data), esbuild.LoaderTS, targets[target])
	if err == nil && !debug {
		embedBuildCache.Store(cacheKey, js)
	}
	return
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

func init() {
	embedFS = &embed.FS{}
}
