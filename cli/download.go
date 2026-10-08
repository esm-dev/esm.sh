package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/ije/esbuild-internal/ast"
	"github.com/ije/esbuild-internal/js_parser"
	"github.com/ije/esbuild-internal/logger"
)

func vendorPath(cdn, module *url.URL, directory bool) (string, error) {
	pathname := strings.TrimPrefix(module.EscapedPath(), "/")
	if module.Scheme == cdn.Scheme && module.Host == cdn.Host && strings.HasPrefix(module.EscapedPath(), strings.TrimRight(cdn.EscapedPath(), "/")+"/") {
		pathname = strings.TrimPrefix(module.EscapedPath(), strings.TrimRight(cdn.EscapedPath(), "/")+"/")
	} else {
		pathname = "_remote/" + url.PathEscape(module.Scheme+"_"+module.Host) + "/" + pathname
	}
	segments := strings.Split(strings.TrimRight(pathname, "/"), "/")
	for i, segment := range segments {
		if segment == "." || segment == ".." || strings.ContainsAny(segment, "\\\x00") {
			return "", fmt.Errorf("invalid module path: %s", module)
		}
		if at := strings.LastIndexByte(segment, '@'); at > 0 {
			segment = segment[:at] + "_" + segment[at+1:]
		}
		segments[i] = strings.ReplaceAll(segment, "*", "_external_")
	}
	filename := strings.Join(segments, "/")
	if !directory {
		if filename == "" || strings.HasSuffix(pathname, "/") || strings.Contains(path.Base(pathname), "@") {
			filename = path.Join(filename, "index.mjs")
		} else if path.Ext(filename) == "" {
			filename += ".mjs"
		}
		if module.RawQuery != "" {
			hash := sha256.Sum256([]byte(module.RawQuery))
			ext := path.Ext(filename)
			filename = strings.TrimSuffix(filename, ext) + fmt.Sprintf("_%x", hash[:6]) + ext
		}
	}
	if filename != "" && !filepath.IsLocal(filepath.FromSlash(filename)) {
		return "", fmt.Errorf("invalid module path: %s", module)
	}
	return filename, nil
}

func downloadImports(im *importmap.ImportMap, directory string, noSRI bool) error {
	cdnOrigin := im.Config().CDN
	if !strings.HasPrefix(cdnOrigin, "https://") && !strings.HasPrefix(cdnOrigin, "http://") {
		cdnOrigin = "https://esm.sh"
	}
	cdn, err := url.Parse(strings.TrimRight(cdnOrigin, "/"))
	if err != nil {
		return err
	}
	var queue []*url.URL
	files := make(map[string]string)
	owners := make(map[string]string)
	addURL := func(module *url.URL) (string, error) {
		moduleURL := *module
		moduleURL.Fragment, moduleURL.RawFragment = "", ""
		key := moduleURL.String()
		if filename, ok := files[key]; ok {
			return filename, nil
		}
		filename, err := vendorPath(cdn, &moduleURL, false)
		if err != nil {
			return "", err
		}
		if owner, ok := owners[filename]; ok && owner != key {
			return "", fmt.Errorf("modules %s and %s have the same vendor path", owner, key)
		}
		owners[filename], files[key] = key, filename
		queue = append(queue, &moduleURL)
		return filename, nil
	}
	maps := []*importmap.Imports{im.Imports}
	var scopes []string
	im.RangeScopes(func(scope string, imports *importmap.Imports) bool {
		scopes = append(scopes, scope)
		maps = append(maps, imports)
		return true
	})
	for _, imports := range maps {
		for _, key := range imports.Keys() {
			value, _ := imports.Get(key)
			if strings.HasPrefix(value, cdn.String()+"/") && !strings.HasSuffix(value, "/") {
				module, err := url.Parse(value)
				if err != nil {
					return err
				}
				if _, err := addURL(module); err != nil {
					return err
				}
			}
		}
	}
	if len(queue) == 0 {
		return nil
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll("vendor", 0755); err != nil {
		return err
	}
	vendor, err := root.OpenRoot("vendor")
	if err != nil {
		return err
	}
	defer vendor.Close()
	stage := ".download-" + rand.Text()
	if err := vendor.Mkdir(stage, 0700); err != nil {
		return err
	}
	defer vendor.RemoveAll(stage)
	client := &http.Client{Timeout: 30 * time.Second}
	integrity := make(map[string]string)
	for cursor := 0; cursor < len(queue); {
		batch := queue[cursor:min(cursor+8, len(queue))]
		results := make([]struct {
			data      []byte
			base      *url.URL
			mediaType string
			err       error
		}, len(batch))
		var wg sync.WaitGroup
		for i, module := range batch {
			wg.Go(func() {
				result := &results[i]
				resp, err := client.Get(module.String())
				if err != nil {
					result.err = err
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					result.err = fmt.Errorf("download %s: HTTP %d", module, resp.StatusCode)
					return
				}
				result.data, result.err = io.ReadAll(resp.Body)
				result.base = resp.Request.URL
				result.mediaType, _, _ = strings.Cut(resp.Header.Get("Content-Type"), ";")
			})
		}
		wg.Wait()
		for i, module := range batch {
			result := &results[i]
			if result.err != nil {
				return result.err
			}
			filename := files[module.String()]
			data := result.data
			if result.mediaType != "application/json" && result.mediaType != "application/wasm" && result.mediaType != "text/css" {
				log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
				parsed, ok := js_parser.Parse(log, logger.Source{KeyPath: logger.Path{Text: module.String()}, Contents: string(data)}, js_parser.Options{})
				if !ok {
					return fmt.Errorf("invalid JavaScript module: %s", module)
				}
				slices.SortFunc(parsed.ImportRecords, func(a, b ast.ImportRecord) int { return int(a.Range.Loc.Start - b.Range.Loc.Start) })
				buf := bytes.NewBuffer(nil)
				offset := 0
				for _, record := range parsed.ImportRecords {
					if record.Kind != ast.ImportStmt && record.Kind != ast.ImportDynamic || record.Flags.Has(ast.IsUnused) {
						continue
					}
					specifier := record.Path.Text
					resolved, mapped := im.Resolve(specifier, result.base)
					if !mapped {
						if !strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "/") && !strings.Contains(specifier, ":") {
							return fmt.Errorf("unmapped import %q in %s", specifier, module)
						}
						ref, err := url.Parse(specifier)
						if err != nil {
							return err
						}
						resolved = result.base.ResolveReference(ref).String()
					}
					dependency, err := url.Parse(resolved)
					if err != nil {
						return err
					}
					if dependency.Scheme != "https" && dependency.Scheme != "http" {
						continue
					}
					target, err := addURL(dependency)
					if err != nil {
						return err
					}
					relative, err := filepath.Rel(filepath.Dir(filepath.FromSlash(filename)), filepath.FromSlash(target))
					if err != nil {
						return err
					}
					relative = filepath.ToSlash(relative)
					if !strings.HasPrefix(relative, ".") {
						relative = "./" + relative
					}
					localURL := (&url.URL{Path: relative, Fragment: dependency.Fragment}).String()
					quoted, _ := json.Marshal(localURL)
					buf.Write(data[offset:record.Range.Loc.Start])
					buf.Write(quoted)
					offset = int(record.Range.End())
				}
				buf.Write(data[offset:])
				data = buf.Bytes()
			}
			stagedPath := filepath.Join(stage, filepath.FromSlash(filename))
			if err := vendor.MkdirAll(filepath.Dir(stagedPath), 0755); err != nil {
				return err
			}
			if err := vendor.WriteFile(stagedPath, data, 0644); err != nil {
				return err
			}
			if !noSRI {
				hash := sha512.Sum384(data)
				integrity[filename] = "sha384-" + base64.StdEncoding.EncodeToString(hash[:])
			}
		}
		cursor += len(batch)
	}
	for _, module := range queue {
		filename := filepath.FromSlash(files[module.String()])
		if err := vendor.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return err
		}
		if err := vendor.Rename(filepath.Join(stage, filename), filename); err != nil {
			return err
		}
	}
	for _, imports := range maps {
		for _, key := range imports.Keys() {
			value, _ := imports.Get(key)
			module, err := url.Parse(value)
			if err != nil {
				continue
			}
			fragment := module.Fragment
			module.Fragment, module.RawFragment = "", ""
			if filename, ok := files[module.String()]; ok {
				imports.Set(key, (&url.URL{Path: "./vendor/" + filename, Fragment: fragment}).String())
			}
		}
	}
	for _, scope := range scopes {
		if strings.HasPrefix(scope, cdn.String()+"/") {
			module, err := url.Parse(scope)
			if err != nil {
				return err
			}
			filename, err := vendorPath(cdn, module, strings.HasSuffix(scope, "/"))
			if err != nil {
				return err
			}
			localScope := "./vendor/" + filename
			if strings.HasSuffix(scope, "/") && filename != "" {
				localScope += "/"
			}
			imports, _ := im.GetScopeImports(scope)
			im.SetScopeImports((&url.URL{Path: localScope}).String(), imports)
			im.SetScopeImports(scope, nil)
		}
	}
	for remote, filename := range files {
		localURL := (&url.URL{Path: "./vendor/" + filename}).String()
		im.Integrity().Delete(remote)
		im.Integrity().Delete(localURL)
		if hash, ok := integrity[filename]; ok {
			im.Integrity().Set(localURL, hash)
		}
	}
	return nil
}
