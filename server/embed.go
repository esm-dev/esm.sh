package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

var (
	embedFS      EmbedFS
	nodeLibs     map[string]string
	npmPolyfills map[string][]byte
	buildCache   sync.Map
)

type EmbedFS interface {
	ReadDir(name string) ([]os.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

type MockEmbedFS struct {
	cwd string
}

func (fs MockEmbedFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(path.Join(fs.cwd, name))
}

func (fs MockEmbedFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(path.Join(fs.cwd, name))
}

func (fs MockEmbedFS) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(path.Join(fs.cwd, name))
}

func loadNodeLibs(fs EmbedFS) (err error) {
	data, err := fs.ReadFile("server/embed/node-libs.tar.gz")
	if err != nil {
		return
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return
	}
	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			nodeLibs[h.Name] = string(data)
		}
	}
	// override some libs
	entries, err := fs.ReadDir("server/embed/polyfills")
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, "node_") && strings.HasSuffix(name, ".js") {
			data, err := fs.ReadFile("server/embed/polyfills/" + name)
			if err != nil {
				return err
			}
			nodeLibs["node/"+name[5:]] = string(data)
		}
	}
	return nil
}

func loadNpmPolyfills(fs EmbedFS) (err error) {
	entries, err := fs.ReadDir("server/embed/polyfills/npm")
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".mjs") {
			data, err := fs.ReadFile("server/embed/polyfills/npm/" + name)
			if err != nil {
				return err
			}
			data = bytes.ReplaceAll(data, []byte{';', '\n'}, []byte{';'})
			data = bytes.TrimSuffix(data, []byte{';'})
			npmPolyfills[strings.TrimSuffix(name, ".mjs")] = data
		}
	}
	return nil
}

func buildEmbedTS(filename string, target string, debug bool) (js []byte, err error) {
	cacheKey := filename + "?" + target
	if data, ok := buildCache.Load(cacheKey); ok {
		return data.([]byte), nil
	}

	data, err := embedFS.ReadFile("server/embed/" + filename)
	if err != nil {
		return
	}

	// replace `$TARGET` with the target
	data = bytes.ReplaceAll(data, []byte("$TARGET"), []byte(target))

	js, err = minify(string(data), targets[target], api.LoaderTS)
	if err == nil && !debug {
		buildCache.Store(cacheKey, js)
	}
	return
}

func init() {
	embedFS = &embed.FS{}
	nodeLibs = make(map[string]string)
	npmPolyfills = make(map[string][]byte)
}
