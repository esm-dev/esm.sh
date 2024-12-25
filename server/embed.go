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
	if err == nil && !DEBUG {
		embedBuildCache.Store(cacheKey, js)
	}
	return
}

func init() {
	embedFS = &embed.FS{}
}
