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
)

var (
	embedFS      EmbedFS
	nodeLibs     map[string]string
	npmPolyfills map[string][]byte
)

type EmbedFS interface {
	ReadFile(name string) ([]byte, error)
}

type MockEmbedFS struct {
	cwd string
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
	node_async_hooks_js, err := fs.ReadFile("server/embed/polyfills/node_async_hooks.js")
	if err != nil {
		return
	}
	nodeLibs["node/async_hooks.js"] = string(node_async_hooks_js)
	// extra libs
	node_filename_resolver_js, err := fs.ReadFile("server/embed/polyfills/node_filename_resolver.js")
	if err != nil {
		return
	}
	nodeLibs["node/filename_resolver.js"] = string(node_filename_resolver_js)
	return nil
}

func loadNpmPolyfills(fs EmbedFS) (err error) {
	data, err := fs.ReadFile("server/embed/npm-polyfills.tar.gz")
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
			if strings.HasSuffix(h.Name, ".mjs") {
				name := strings.TrimSuffix(path.Base(h.Name), ".mjs")
				data = bytes.ReplaceAll(data, []byte{';', '\n'}, []byte{';'})
				data = bytes.TrimSuffix(data, []byte{';'})
				npmPolyfills[name] = data
			}
		}
	}
	return nil
}

func init() {
	embedFS = &embed.FS{}
	nodeLibs = make(map[string]string)
	npmPolyfills = make(map[string][]byte)
}
