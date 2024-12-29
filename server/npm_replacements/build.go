package npm_replacements

import (
	"bytes"
	"embed"
	"errors"
	"regexp"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

//go:embed src
var efs embed.FS

var npmReplacements = map[string]NpmReplacement{}

type NpmReplacement struct {
	ESM  []byte
	IIFE []byte
}

// Get returns the npm replacement by the given name.
func Get(name string) (NpmReplacement, bool) {
	ret, ok := npmReplacements[name]
	return ret, ok
}

// Build builds the npm replacements.
func Build() (n int, err error) {
	regexpExportAsExpr := regexp.MustCompile(`([\w$]+) as ([\w$]+)`)
	err = walkEmbedFS("src", func(path string) error {
		sourceCode, err := efs.ReadFile(path)
		if err != nil {
			return err
		}
		ret := esbuild.Transform(string(sourceCode), esbuild.TransformOptions{
			Target:            esbuild.ES2022,
			Format:            esbuild.FormatESModule,
			Platform:          esbuild.PlatformBrowser,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			MinifySyntax:      true,
			Loader:            esbuild.LoaderJS,
		})
		if len(ret.Errors) > 0 {
			return errors.New(ret.Errors[0].Text)
		}
		specifier := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "src/"), ".mjs"), "/index")
		npmReplacements[specifier] = NpmReplacement{
			ESM:  ret.Code,
			IIFE: concatBytes(concatBytes([]byte("(()=>{"), regexpExportAsExpr.ReplaceAll(bytes.ReplaceAll(bytes.TrimSpace(ret.Code), []byte("export{"), []byte("return{")), []byte("$2:$1"))), []byte("})()")),
		}
		return nil
	})
	if err != nil {
		return
	}
	return len(npmReplacements), nil
}

// concatBytes concatenates two byte slices.
func concatBytes(a, b []byte) []byte {
	al, bl := len(a), len(b)
	c := make([]byte, al+bl)
	copy(c, a)
	copy(c[al:], b)
	return c
}

func walkEmbedFS(dir string, fn func(path string) error) error {
	entries, err := efs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			if err := walkEmbedFS(path, fn); err != nil {
				return err
			}
		} else if strings.HasSuffix(path, ".mjs") {
			if err := fn(path); err != nil {
				return err
			}
		}
	}
	return nil
}
