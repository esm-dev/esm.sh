package server

import (
	"bytes"
	"embed"
	"os"
	"path"
	"sync"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

var (
	embedFS         EmbedFS
	embedBuildCache sync.Map
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

func buildEmbedTSModule(filename string, target string) (js []byte, err error) {
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
