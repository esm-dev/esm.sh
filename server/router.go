package server

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/gfm"
	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/mime"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
	"golang.org/x/net/html"
)

type RouteKind uint8

const (
	// module entry
	EsmEntry RouteKind = iota
	// js/css build
	EsmBuild
	// source map
	EsmSourceMap
	// *.d.ts
	EsmDts
	// package raw file
	RawFile
)

const (
	ccMustRevalidate = "public, max-age=0, must-revalidate"
	ccOneDay         = "public, max-age=86400"
	ccImmutable      = "public, max-age=31536000, immutable"
	ctHTML           = "text/html; charset=utf-8"
	ctCSS            = "text/css; charset=utf-8"
	ctJSON           = "application/json; charset=utf-8"
	ctJavaScript     = "application/javascript; charset=utf-8"
	ctTypeScript     = "application/typescript; charset=utf-8"
)

func esmRouter(esmStorage storage.Storage, logger *log.Logger) rex.Handle {
	var (
		startTime  = time.Now()
		globalETag = fmt.Sprintf(`W/"%s"`, VERSION)
		buildQueue = NewBuildQueue(int(config.BuildConcurrency))
		npmrc      = DefaultNpmRC()
		metaDB     = NewBuildMetaDB(esmStorage)
	)

	// todo: remove old db code after migration is complete
	{
		oldDbFile := path.Join(config.WorkDir, "esm.db")
		if existsFile(oldDbFile) {
			var err error
			metaDB.oldDB, err = OpenBoltDB(oldDbFile)
			if err != nil {
				logger.Errorf("failed to open old db: %v", err)
			}
		}
	}

	return func(ctx *rex.Context) any {
		pathname := ctx.R.URL.Path

		// ban malicious requests
		if strings.HasSuffix(pathname, ".env") || strings.HasSuffix(pathname, ".php") || strings.Contains(pathname, "/.") {
			ctx.SetHeader("Cache-Control", ccImmutable)
			return rex.Status(404, "not found")
		}

		// handle POST API requests
		switch ctx.R.Method {
		case "HEAD", "GET":
			// continue
		case "POST":
			switch pathname {
			case "/transform":
				var options TransformOptions
				err := json.NewDecoder(io.LimitReader(ctx.R.Body, 2*MB)).Decode(&options)
				ctx.R.Body.Close()
				if err != nil {
					return rex.Err(400, "require valid json body")
				}
				if options.Code == "" {
					return rex.Err(400, "Code is required")
				}
				if len(options.Code) > MB {
					return rex.Err(429, "Code is too large")
				}
				if targets[options.Target] == 0 {
					options.Target = "esnext"
				}
				if options.Lang == "" && options.Filename != "" {
					_, options.Lang = utils.SplitByLastByte(options.Filename, '.')
				}

				h := sha1.New()
				h.Write([]byte(options.Lang))
				h.Write([]byte(options.Code))
				h.Write([]byte(options.Target))
				h.Write(options.ImportMap)
				h.Write([]byte(options.JsxImportSource))
				h.Write([]byte(options.SourceMap))
				fmt.Fprintf(h, "%v", options.Minify)
				hash := hex.EncodeToString(h.Sum(nil))
				savePath := normalizeSavePath(fmt.Sprintf("modules/transform/%s.mjs", hash))

				// if previous build exists, return it directly
				if file, _, err := esmStorage.Get(savePath); err == nil {
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					output := TransformOutput{
						Code: string(data),
					}
					file, _, err = esmStorage.Get(savePath + ".map")
					if err == nil {
						data, err = io.ReadAll(file)
						file.Close()
						if err == nil {
							output.Map = string(data)
						}
					}
					return output
				}

				var importMap *importmap.ImportMap
				if len(options.ImportMap) > 0 {
					importMap, err = importmap.Parse(nil, options.ImportMap)
					if err != nil {
						return rex.Err(400, "Invalid ImportMap")
					}
				}

				output, err := transform(&ResolvedTransformOptions{
					TransformOptions: options,
					importMap:        importMap,
				})
				if err != nil {
					return rex.Err(400, err.Error())
				}
				if len(output.Map) > 0 {
					output.Code = fmt.Sprintf("%s//# sourceMappingURL=+%s", output.Code, path.Base(savePath)+".map")
					go esmStorage.Put(savePath+".map", strings.NewReader(output.Map))
				}
				go esmStorage.Put(savePath, strings.NewReader(output.Code))
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
				return output

			default:
				return rex.Status(404, "not found")
			}
		default:
			return rex.Status(405, "Method Not Allowed")
		}

		// strip trailing slash
		if pl := len(pathname); pl > 1 && pathname[pl-1] == '/' {
			pathname = pathname[:pl-1]
		}

		// strip loc suffix
		// e.g. https://esm.sh/react@19.0.0/es2022/react.mjs:2:3
		{
			p, loc := utils.SplitByLastByte(pathname, ':')
			if loc != "" && valid.IsDigtalOnlyString(loc) {
				p2, loc2 := utils.SplitByLastByte(p, ':')
				if loc2 != "" && valid.IsDigtalOnlyString(loc2) {
					pathname = p2
				} else {
					pathname = p
				}
			}
		}

		// static routes
		switch pathname {
		case "/favicon.ico":
			favicon, err := embedFS.ReadFile("embed/favicon.ico")
			if err != nil {
				return err
			}
			ctx.SetHeader("Content-Type", "image/x-icon")
			ctx.SetHeader("Cache-Control", ccImmutable)
			return favicon

		case "/robots.txt":
			return "User-agent: *\nAllow: /\n"

		case "/":
			if strings.HasPrefix(ctx.UserAgent(), "Deno/") {
				ctx.SetHeader("Content-Type", ctJavaScript)
				return `throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`
			}
			if ctx.R.Header.Get("If-None-Match") == globalETag {
				return rex.Status(http.StatusNotModified, nil)
			}
			cacheTtl := 31536000
			if DEBUG {
				cacheTtl = 0
			}
			indexHTML, err := withCache("index.html", time.Duration(cacheTtl)*time.Second, func() (indexHTML []byte, _ string, err error) {
				readme, err := os.ReadFile("README.md")
				if err != nil {
					fetchClient, recycle := fetch.NewClient(ctx.UserAgent(), 15, false, nil)
					defer recycle()
					readmeUrl, _ := url.Parse("https://raw.githubusercontent.com/esm-dev/esm.sh/refs/heads/main/README.md")
					var res *http.Response
					res, err = fetchClient.Fetch(readmeUrl, nil)
					if err != nil {
						err = errors.New("failed to fetch README.md from GitHub")
						return
					}
					defer res.Body.Close()
					if res.StatusCode != 200 {
						err = errors.New("failed to fetch README.md from GitHub: " + res.Status)
						return
					}
					readme, err = io.ReadAll(res.Body)
				}
				if err != nil {
					err = errors.New("failed to read readme: " + err.Error())
					return
				}
				readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte("/embed/"))
				readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/main/HOSTING.md"))
				readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(getOrigin(ctx)))
				readmeHtml, err := gfm.Render(readme, gfm.RenderFormatHTML)
				if err != nil {
					err = errors.New("failed to render readme: " + err.Error())
					return
				}
				indexHTML, err = embedFS.ReadFile("embed/index.html")
				if err != nil {
					return
				}
				indexHTML = bytes.ReplaceAll(indexHTML, []byte("{README}"), readmeHtml)
				return
			})
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Content-Type", ctHTML)
			ctx.SetHeader("Cache-Control", ccMustRevalidate)
			ctx.SetHeader("Etag", globalETag)
			return indexHTML

		case "/status.json":
			q := make([]map[string]any, buildQueue.queue.Len())
			i := 0

			for el := buildQueue.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*BuildTask)
				if ok {
					m := map[string]any{
						"waitClients": len(t.waitChans),
						"createdAt":   t.createdAt.Format(http.TimeFormat),
						"path":        t.ctx.Path(),
						"status":      t.ctx.status,
					}
					q[i] = m
					i++
				}
			}

			diskStatus := "ok"
			var stat syscall.Statfs_t
			err := syscall.Statfs(config.WorkDir, &stat)
			if err == nil {
				avail := stat.Bavail * uint64(stat.Bsize)
				if avail < 100*MB {
					diskStatus = "full"
				} else if avail < 1024*MB {
					diskStatus = "low"
				}
			} else {
				diskStatus = "error"
			}

			ctx.SetHeader("Cache-Control", ccMustRevalidate)
			return map[string]any{
				"buildQueue": q[:i],
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
				"disk":       diskStatus,
			}

		case "/error.js":
			switch query := ctx.Query(); query.Get("type") {
			case "resolve":
				return errorJS(ctx, fmt.Sprintf(
					`Could not resolve "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-node-builtin-module":
				return errorJS(ctx, fmt.Sprintf(
					`Unsupported Node builtin module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-node-native-module":
				return errorJS(ctx, fmt.Sprintf(
					`Unsupported node native module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-npm-package":
				return errorJS(ctx, fmt.Sprintf(
					`Unsupported NPM package "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-file-dependency":
				return errorJS(ctx, fmt.Sprintf(
					`Unsupported file dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-git-dependency":
				return errorJS(ctx, fmt.Sprintf(
					`Unsupported git dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "invalid-jsr-dependency":
				return errorJS(ctx, fmt.Sprintf(
					`Invalid jsr dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "invalid-http-dependency":
				return errorJS(ctx, fmt.Sprintf(
					`Invalid http dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			default:
				ctx.SetHeader("Cache-Control", ccOneDay)
				return rex.Status(500, "Unknown error")
			}

		// builtin scripts
		case "/x", "/tsx", "/run":
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch == globalETag && !DEBUG {
				return rex.Status(http.StatusNotModified, nil)
			}

			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(ctx.Query().Get("target"))
			targetFromUA := targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(ctx.UserAgent())
			}

			cacheTtl := 31536000
			if DEBUG {
				cacheTtl = 0
			}
			filename := "embed/" + pathname[1:] + ".ts"
			if pathname == "/run" {
				filename = "embed/tsx.ts"
			}
			js, err := withCache(filename+"?"+target, time.Duration(cacheTtl)*time.Second, func() (js []byte, _ string, err error) {
				data, err := embedFS.ReadFile(filename)
				if err != nil {
					return
				}
				// replace `$TARGET` with the target
				data = bytes.ReplaceAll(data, []byte("$TARGET"), []byte(target))
				js, err = minify(string(data), esbuild.LoaderTS, targets[target])
				return
			})
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if DEBUG {
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
			} else {
				ctx.SetHeader("Cache-Control", ccOneDay)
			}
			ctx.SetHeader("Etag", globalETag)
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			ctx.SetHeader("Content-Type", ctJavaScript)
			return js

		case "/install":
			data, err := embedFS.ReadFile("embed/install.sh")
			if err != nil {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(404, "not found")
			}
			ctx.SetHeader("Content-Type", "text/plain; charset=utf-8")
			ctx.SetHeader("Cache-Control", ccMustRevalidate)
			return data
		}

		// for testing
		if DEBUG {
			switch pathname {
			case "/$test/a.tsx":
				return "import { HelloWorld } from \"./b.ts\"; console.log(<HelloWorld />);"
			case "/$test/b.ts":
				return "export * from \"./c.tsx\";"
			case "/$test/c.tsx":
				return "export function HelloWorld(props: {}) { return <h1 class=\"text-3xl font-bold\">Hello world!</h1> }"
			case "/$test/readme.md":
				return "# esm.sh"
			case "/$test/tailwind.css":
				return "@import \"tailwindcss\";"
			case "/$test":
				return `<DOCTYPE html>
					<script type="importmap">
						{
							"imports": {
								"react/": "/react@19.2.0/"
							}
						}
					</script>
					<script src="/x" href="/$test/a.tsx"></script>
					<a class="text-blue-500 underline" href="https://esm.sh">esm.sh</a>
				`
			}
		}

		// module generated by the `/transform` API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByFirstByte(pathname[2:], '.')
			if len(hash) != 40 || !valid.IsHexString(hash) {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(404, "Not Found")
			}
			savePath := normalizeSavePath(fmt.Sprintf("modules/transform/%s.%s", hash, ext))
			f, fi, err := esmStorage.Get(savePath)
			if err != nil {
				logger.Errorf("storage.get(%s): %v", savePath, err)
				return rex.Status(500, "Storage error, please try again")
			}
			if strings.HasSuffix(pathname, ".map") {
				ctx.SetHeader("Content-Type", ctJSON)
			} else {
				ctx.SetHeader("Content-Type", ctJavaScript)
			}
			ctx.SetHeader("Content-Length", fmt.Sprintf("%d", fi.Size()))
			ctx.SetHeader("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			ctx.SetHeader("Cache-Control", ccImmutable)
			return f // auto closed
		}

		// node libs
		if strings.HasPrefix(pathname, "/node/") {
			if !strings.HasSuffix(pathname, ".mjs") {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(404, "Not Found")
			}
			name := pathname[6:]
			js, ok := getNodeRuntimeJS(name)
			if !ok {
				if !nodeBuiltinModules[name] {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(404, "Not Found")
				}
				js = []byte("export default {}")
			}
			if strings.HasPrefix(name, "chunk-") {
				ctx.SetHeader("Cache-Control", ccImmutable)
			} else {
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch == globalETag && !DEBUG {
					return rex.Status(http.StatusNotModified, nil)
				}
				ctx.SetHeader("Cache-Control", ccOneDay)
				ctx.SetHeader("Etag", globalETag)
			}
			ctx.SetHeader("Content-Type", ctJavaScript)
			return js
		}

		// embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile(pathname[1:])
			if err != nil {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(404, "not found")
			}
			if !DEBUG {
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
			} else {
				etag := fmt.Sprintf(`W/"%d%d"`, startTime.Unix(), len(data))
				if ifNoneMatch := ctx.R.Header.Get("If-None-Match"); ifNoneMatch == etag {
					return rex.Status(http.StatusNotModified, nil)
				}
				ctx.SetHeader("Etag", etag)
				ctx.SetHeader("Cache-Control", ccOneDay)
			}
			contentType := mime.GetContentType(pathname)
			if contentType != "" {
				ctx.SetHeader("Content-Type", contentType)
			}
			return data
		}

		if strings.HasPrefix(pathname, "/http://") || strings.HasPrefix(pathname, "/https://") {
			query := ctx.Query()
			modUrl, err := url.Parse(pathname[1:])
			if err != nil {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(400, "Invalid URL")
			}
			if modUrl.Scheme != "http" && modUrl.Scheme != "https" {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(400, "Invalid URL")
			}
			modUrlStr := modUrl.String()

			// disallow localhost or ip address for production
			if !DEBUG {
				hostname := modUrl.Hostname()
				if isLocalhost(hostname) || !valid.IsDomain(hostname) || modUrl.Host == ctx.R.Host {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(400, "Invalid URL")
				}
			}

			extname := path.Ext(modUrl.Path)
			if !(slices.Contains(moduleExts, extname) || extname == ".vue" || extname == ".svelte" || extname == ".md" || extname == ".css") {
				return redirect(ctx, modUrl.String(), true)
			}

			target := "es2022"
			if v := query.Get("target"); v != "" {
				if targets[v] == 0 {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(400, "Invalid target param")
				}
				target = v
			}

			v := query.Get("v")
			if v != "" && (!npm.Versioning.Match(v) || len(v) > 32) {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(400, "Invalid version param")
			}

			var basePath string
			if v := query.Get("b"); v != "" {
				var err error
				basePath, err = atobUrl(v)
				if err != nil {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(400, "Invalid base param")
				}
				if !strings.HasPrefix(basePath, "/") {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(400, "Invalid base param")
				}
				basePath = utils.NormalizePathname(basePath)
			}
			baseUrl, err := url.Parse(modUrl.Scheme + "://" + modUrl.Host + basePath)
			if err != nil {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(400, "Invalid base param")
			}

			allowedHosts := map[string]struct{}{}
			allowedHosts[modUrl.Host] = struct{}{}
			fetchClient, recycle := fetch.NewClient(ctx.UserAgent(), 15, false, allowedHosts)
			defer recycle()

			if strings.HasSuffix(modUrl.Path, "/tailwind.css") || strings.HasSuffix(modUrl.Path, "/uno.css") {
				h := sha1.New()
				h.Write([]byte(modUrlStr))
				h.Write([]byte(basePath))
				h.Write([]byte(target))
				h.Write([]byte(v))
				savePath := normalizeSavePath(path.Join("modules/x", hex.EncodeToString(h.Sum(nil))+".css"))
				r, fi, err := esmStorage.Get(savePath)
				if err != nil && err != storage.ErrNotFound {
					logger.Errorf("storage.get(%s): %v", savePath, err)
					return rex.Status(500, "Storage error, please try again")
				}
				if err == nil {
					ctx.SetHeader("Cache-Control", ccImmutable)
					ctx.SetHeader("Content-Type", ctCSS)
					ctx.SetHeader("Content-Length", fmt.Sprintf("%d", fi.Size()))
					return r // auto closed
				}
				res, err := fetchClient.Fetch(baseUrl, nil)
				if err != nil {
					return rex.Status(500, "Failed to fetch page content: "+err.Error())
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					if res.StatusCode == 404 {
						return rex.Status(404, "Page not found")
					}
					return rex.Status(500, "Failed to fetch page content: "+res.Status)
				}
				tokenizer := html.NewTokenizer(io.LimitReader(res.Body, 5*MB))
				content := []string{}
				jsEntries := map[string]struct{}{}
				var importMap *importmap.ImportMap
				for {
					tt := tokenizer.Next()
					if tt == html.ErrorToken {
						break
					}
					if tt == html.StartTagToken {
						name, moreAttr := tokenizer.TagName()
						switch string(name) {
						case "script":
							var (
								srcAttr  string
								hrefAttr string
								typeAttr string
							)
							for moreAttr {
								var key, val []byte
								key, val, moreAttr = tokenizer.TagAttr()
								if len(val) > 0 {
									switch string(key) {
									case "src":
										srcAttr = string(val)
									case "href":
										hrefAttr = string(val)
									case "type":
										typeAttr = string(val)
									}
								}
							}
							if typeAttr == "importmap" {
								tokenizer.Next()
								innerText := bytes.TrimSpace(tokenizer.Text())
								if len(innerText) > 0 {
									importMap, _ = importmap.Parse(baseUrl, innerText)
								}
							} else if srcAttr == "" {
								// inline script content
								tokenizer.Next()
								content = append(content, string(tokenizer.Text()))
							} else {
								if hrefAttr != "" && strings.HasSuffix(srcAttr, "/x") {
									if !isHttpSpecifier(hrefAttr) && endsWith(hrefAttr, moduleExts...) {
										jsEntries[hrefAttr] = struct{}{}
									}
								}
							}
						case "link", "meta", "title", "base", "head", "noscript":
							// ignore
						default:
							content = append(content, string(tokenizer.Raw()))
						}
					}
				}
				res, err = fetchClient.Fetch(modUrl, nil)
				if err != nil {
					return rex.Status(500, "Failed to fetch "+modUrl.String()+": "+err.Error())
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					if res.StatusCode == 404 {
						return rex.Status(404, "Not found: "+modUrl.String())
					}
					return rex.Status(500, "Failed to fetch "+modUrl.String()+": "+res.Status)
				}
				configCSS, err := io.ReadAll(io.LimitReader(res.Body, MB))
				if err != nil {
					return rex.Status(500, "Failed to fetch "+modUrl.String()+": "+err.Error())
				}
				for src := range jsEntries {
					url := baseUrl.ResolveReference(&url.URL{Path: src})
					_, _, _, tree, err := bundleHttpModule(npmrc, url.String(), importMap, true, fetchClient)
					if err == nil {
						for _, code := range tree {
							content = append(content, string(code))
						}
					}
				}
				baseName := path.Base(modUrl.Path)
				var out *LoaderOutput
				if baseName == "uno.css" {
					out, err = generateUnoCSS(npmrc, string(configCSS), strings.Join(content, "\n"))
				} else {
					out, err = generateTailwindCSS(npmrc, string(configCSS), strings.Join(content, "\n"))
				}
				if err != nil {
					return rex.Status(500, "Failed to generate "+baseName+": "+err.Error())
				}
				ret := esbuild.Build(esbuild.BuildOptions{
					Stdin: &esbuild.StdinOptions{
						Sourcefile: baseName,
						Contents:   out.Code,
						Loader:     esbuild.LoaderCSS,
					},
					Write:            false,
					MinifyWhitespace: config.Minify,
					MinifySyntax:     config.Minify,
					Target:           targets[target],
				})
				if len(ret.Errors) > 0 {
					return rex.Status(500, ret.Errors[0].Text)
				}
				minifiedCSS := ret.OutputFiles[0].Contents
				go esmStorage.Put(savePath, bytes.NewReader(minifiedCSS))
				ctx.SetHeader("Cache-Control", ccImmutable)
				ctx.SetHeader("Content-Type", ctCSS)
				return minifiedCSS
			} else {
				if extname == ".md" {
					for _, kind := range []string{"jsx", "svelte", "vue"} {
						if query.Has(kind) {
							modUrlStr += "?" + kind
							break
						}
					}
				}
				h := sha1.New()
				h.Write([]byte(modUrlStr))
				h.Write([]byte(basePath))
				h.Write([]byte(target))
				h.Write([]byte(v))
				savePath := normalizeSavePath(path.Join("modules/x", hex.EncodeToString(h.Sum(nil))+".mjs"))
				content, fi, err := esmStorage.Get(savePath)
				if err != nil && err != storage.ErrNotFound {
					logger.Errorf("storage.get(%s): %v", savePath, err)
					return rex.Status(500, "Storage error, please try again")
				}
				var body io.Reader = content
				if err == storage.ErrNotFound {
					var importMap *importmap.ImportMap
					res, err := fetchClient.Fetch(baseUrl, nil)
					if err != nil {
						return rex.Status(500, "Failed to fetch import map")
					}
					defer res.Body.Close()
					if res.StatusCode == 200 {
						tokenizer := html.NewTokenizer(io.LimitReader(res.Body, 5*MB))
						for {
							tt := tokenizer.Next()
							if tt == html.ErrorToken {
								break
							}
							if tt == html.StartTagToken {
								name, moreAttr := tokenizer.TagName()
								isImportMapScript := false
								if bytes.Equal(name, []byte("script")) {
									for moreAttr {
										var key, val []byte
										key, val, moreAttr = tokenizer.TagAttr()
										if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("importmap")) {
											isImportMapScript = true
											break
										}
									}
								}
								if isImportMapScript {
									tokenizer.Next()
									innerText := bytes.TrimSpace(tokenizer.Text())
									if len(innerText) > 0 {
										var err error
										importMap, err = importmap.Parse(baseUrl, innerText)
										if err != nil {
											return rex.Status(400, "Invalid import map")
										}
									}
									break
								}
							}
						}
					} else if res.StatusCode != 404 {
						if res.StatusCode == 301 || res.StatusCode == 302 || res.StatusCode == 307 || res.StatusCode == 308 {
							return rex.Status(400, "Failed to fetch import map: redirects are not allowed")
						}
						return rex.Status(500, "Failed to fetch import map: "+res.Status)
					}
					js, jsx, css, _, err := bundleHttpModule(npmrc, modUrlStr, importMap, false, fetchClient)
					if err != nil {
						return rex.Status(500, "Failed to build module: "+err.Error())
					}
					code := string(js)
					if len(css) > 0 {
						code += fmt.Sprintf(`globalThis.document.head.insertAdjacentHTML("beforeend","<style>"+%s+"</style>")`, utils.MustEncodeJSON(string(css)))
					}
					lang := "js"
					if jsx {
						lang = "jsx"
					}
					out, err := transform(&ResolvedTransformOptions{
						TransformOptions: TransformOptions{
							Filename: modUrlStr,
							Lang:     lang,
							Code:     code,
							Target:   target,
							Minify:   true,
						},
						importMap:     importMap,
						globalVersion: v,
					})
					if err != nil {
						return rex.Status(500, err.Error())
					}
					body = bytes.NewReader([]byte(out.Code))
					fi = nil
					go esmStorage.Put(savePath, strings.NewReader(out.Code))
				}
				if extname == ".css" && query.Has("module") {
					css, err := io.ReadAll(body)
					if closer, ok := body.(io.Closer); ok {
						closer.Close()
					}
					if err != nil {
						return rex.Status(500, "Failed to read css")
					}
					body = strings.NewReader(fmt.Sprintf("var style=document.createElement('style');\nstyle.textContent=%s;\ndocument.head.appendChild(style);\nexport default null;", utils.MustEncodeJSON(string(css))))
					fi = nil
				}
				ctx.SetHeader("Cache-Control", ccImmutable)
				if extname == ".css" {
					ctx.SetHeader("Content-Type", ctCSS)
				} else {
					ctx.SetHeader("Content-Type", ctJavaScript)
				}
				if fi != nil {
					ctx.SetHeader("Content-Length", fmt.Sprintf("%d", fi.Size()))
				}
				return body // auto closed
			}
		}

		// check `/*pathname` pattern
		asteriskPrefix := false
		if strings.HasPrefix(pathname, "/*") {
			asteriskPrefix = true
			pathname = "/" + pathname[2:]
		} else if strings.HasPrefix(pathname, "/gh/*") {
			asteriskPrefix = true
			pathname = "/gh/" + pathname[5:]
		} else if strings.HasPrefix(pathname, "/github.com/*") {
			asteriskPrefix = true
			pathname = "/gh/" + pathname[13:]
		} else if strings.HasPrefix(pathname, "/pr/*") {
			asteriskPrefix = true
			pathname = "/pr/" + pathname[5:]
		} else if strings.HasPrefix(pathname, "/pkg.pr.new/*") {
			asteriskPrefix = true
			pathname = "/pr/" + pathname[13:]
		}

		esmPath, extraQuery, isExactVersion, target, xArgs, err := parseEsmPath(npmrc, pathname)
		if err != nil {
			status := 500
			message := err.Error()
			if strings.HasPrefix(message, "invalid") {
				status = 400
				ctx.SetHeader("Cache-Control", ccImmutable)
			} else if strings.HasSuffix(message, " not found") {
				status = 404
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
			}
			return rex.Status(status, message)
		}

		if !config.AllowList.IsEmpty() && !config.AllowList.IsPackageAllowed(esmPath.ID()) {
			ctx.SetHeader("Cache-Control", "public, max-age=3600")
			return rex.Status(403, "forbidden")
		}

		if !config.BanList.IsEmpty() && config.BanList.IsPackageBanned(esmPath.ID()) {
			ctx.SetHeader("Cache-Control", "public, max-age=3600")
			return rex.Status(403, "forbidden")
		}

		origin := getOrigin(ctx)

		registryPrefix := ""
		if esmPath.GhPrefix {
			registryPrefix = "/gh"
		} else if esmPath.PrPrefix {
			registryPrefix = "/pr"
		}

		// redirect `/@types/PKG` to it's main dts file
		if strings.HasPrefix(esmPath.PkgName, "@types/") && esmPath.SubPath == "" {
			info, err := npmrc.getPackageInfo(esmPath.PkgName, esmPath.PkgVersion)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			types := "index.d.ts"
			if info.Types != "" {
				types = info.Types
			} else if info.Typings != "" {
				types = info.Typings
			} else if info.Main != "" && endsWith(info.Main, ".d.ts", ".d.mts", ".d.cts") {
				types = info.Main
			}
			if strings.HasSuffix(types, ".d") {
				types += ".ts"
			} else if !endsWith(types, ".d.ts", ".d.mts", ".d.cts") {
				types += ".d.ts"
			}
			return redirect(ctx, fmt.Sprintf("%s/%s@%s%s", origin, info.Name, info.Version, utils.NormalizePathname(types)), isExactVersion)
		}

		// redirect to the main css path for CSS packages
		if css := cssPackages[esmPath.PkgName]; css != "" && esmPath.SubPath == "" {
			url := fmt.Sprintf("%s/%s/%s", origin, esmPath.ID(), css)
			return redirect(ctx, url, isExactVersion)
		}

		// store the raw query
		rawQuery := ctx.R.URL.RawQuery

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"/jsx-runtime", "/jsx-dev-runtime"} {
			if strings.HasSuffix(rawQuery, jsxRuntime) {
				if esmPath.SubPath == "" {
					esmPath.SubPath = jsxRuntime[1:]
				} else {
					esmPath.SubPath = esmPath.SubPath + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", esmPath.PkgName, esmPath.SubPath)
				ctx.R.URL.RawQuery = strings.TrimSuffix(rawQuery, jsxRuntime)
				break
			}
		}

		// apply the extra query if exists
		if extraQuery != "" {
			qs := []string{extraQuery}
			if rawQuery != "" {
				qs = append(qs, rawQuery)
			}
			ctx.R.URL.RawQuery = strings.Join(qs, "&")
		}

		// parse the query
		// todo: validate query
		query := ctx.Query()

		// use `?path=$PATH` query to override the pathname
		if v := query.Get("path"); v != "" {
			esmPath.SubPath = stripEntryModuleExt(utils.NormalizePathname(v)[1:])
		}

		// check the path kind
		pathKind := EsmEntry
		hasTargetSegment := target != ""
		if esmPath.SubPath != "" {
			ext := path.Ext(pathname)
			switch ext {
			case ".mjs":
				if hasTargetSegment {
					pathKind = EsmBuild
				}
			case ".ts", ".mts", ".cts", ".tsx":
				if strings.HasSuffix(pathname, ".d"+ext) || query.Has("dts") {
					pathKind = EsmDts
				}
			case ".css":
				if hasTargetSegment {
					pathKind = EsmBuild
				} else {
					pathKind = RawFile
				}
			case ".map":
				if hasTargetSegment {
					pathKind = EsmSourceMap
				} else {
					pathKind = RawFile
				}
			default:
				if ext != "" && assetExts[ext[1:]] {
					pathKind = RawFile
				}
			}
		}

		rawFlag := query.Has("raw") || strings.HasPrefix(ctx.R.Host, "raw.")
		if rawFlag {
			pathKind = RawFile
			if esmPath.SubPath != "" {
				extname := path.Ext(pathname)
				if !strings.HasSuffix(esmPath.SubPath, extname) {
					esmPath.SubPath += extname
				}
			}
		}

		// redirect to the url with exact package version
		if !isExactVersion {
			if hasTargetSegment {
				pkgName := esmPath.ID()
				subPath := ""
				query := ""
				if asteriskPrefix {
					if esmPath.GhPrefix || esmPath.PrPrefix {
						pkgName = pkgName[0:3] + "*" + pkgName[3:]
					} else {
						pkgName = "*" + pkgName
					}
				}
				if extraQuery != "" {
					pkgName += "&" + extraQuery
				}
				if esmPath.SubPath != "" {
					subPath = "/" + esmPath.SubPath
				}
				if rawQuery != "" {
					query = "?" + rawQuery
				}
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				return redirect(ctx, fmt.Sprintf("%s/%s%s%s", origin, pkgName, subPath, query), false)
			}
			if pathKind != EsmEntry {
				pkgName := esmPath.PkgName
				pkgVersion := esmPath.PkgVersion
				subPath := ""
				query := ""
				if strings.HasPrefix(pkgName, "@jsr/") {
					pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
				}
				if asteriskPrefix {
					if esmPath.GhPrefix || esmPath.PrPrefix {
						pkgName = pkgName[0:3] + "*" + pkgName[3:]
					} else {
						pkgName = "*" + pkgName
					}
				}
				if esmPath.SubPath != "" {
					subPath = "/" + esmPath.SubPath
					// workaround for es5-ext "../#/.." path
					if esmPath.PkgName == "es5-ext" {
						subPath = strings.ReplaceAll(subPath, "/#/", "/%23/")
					}
				}
				if extraQuery != "" {
					pkgVersion += "&" + extraQuery
				}
				if rawQuery != "" {
					query = "?" + rawQuery
				}
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				return redirect(ctx, fmt.Sprintf("%s%s/%s@%s%s%s", origin, registryPrefix, pkgName, pkgVersion, subPath, query), false)
			}
		}

		// fix url that is related to `import.meta.url`
		if hasTargetSegment && isExactVersion && pathKind == RawFile && !rawFlag {
			extname := path.Ext(esmPath.SubPath)
			dir := path.Join(npmrc.StoreDir(), esmPath.ID())
			if !existsDir(dir) {
				_, err := npmrc.installPackage(esmPath.Package())
				if err != nil {
					return rex.Status(500, err.Error())
				}
			}
			pkgRoot := path.Join(dir, "node_modules", esmPath.PkgName)
			files, err := findFiles(pkgRoot, "", func(fp string) bool {
				return strings.HasSuffix(fp, extname)
			})
			if err != nil {
				return rex.Status(500, err.Error())
			}
			var file string
			if l := len(files); l == 1 {
				file = files[0]
			} else if l > 1 {
				for _, f := range files {
					if strings.HasSuffix(esmPath.SubPath, f) {
						file = f
						break
					}
				}
				if file == "" {
					for _, f := range files {
						if path.Base(esmPath.SubPath) == path.Base(f) {
							file = f
							break
						}
					}
				}
			}
			if file == "" {
				ctx.SetHeader("Cache-Control", ccImmutable)
				return rex.Status(404, "File not found")
			}
			url := fmt.Sprintf("%s%s/%s@%s/%s", origin, registryPrefix, esmPath.PkgName, esmPath.PkgVersion, file)
			return redirect(ctx, url, true)
		}

		// try to serve package static files if the version is exact
		if isExactVersion {
			// return wasm file as an es6 module when `?module` query is present (requires `top-level-await` support)
			if pathKind == RawFile && strings.HasSuffix(esmPath.SubPath, ".wasm") && query.Has("module") {
				wasmUrl := origin + pathname
				buf := bytes.NewBufferString("/* esm.sh - wasm module */\n")
				buf.WriteString("const data = await fetch(")
				buf.WriteString(strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
				buf.WriteString(").then(r => r.arrayBuffer());\n")
				buf.WriteString("export default new WebAssembly.Module(data);")
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Content-Length", fmt.Sprintf("%d", buf.Len()))
				ctx.SetHeader("Cache-Control", ccImmutable)
				return buf
			}

			// return css file as a `CSSStyleSheet` object when `?module` query is present
			if pathKind == RawFile && strings.HasSuffix(esmPath.SubPath, ".css") && query.Has("module") {
				filename := path.Join(npmrc.StoreDir(), esmPath.ID(), "node_modules", esmPath.PkgName, esmPath.SubPath)
				css, err := os.ReadFile(filename)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				css, err = minify(string(css), esbuild.LoaderCSS, esbuild.ES2022)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				buf := bytes.NewBufferString("/* esm.sh - css module */\n")
				buf.WriteString("const stylesheet = new CSSStyleSheet();\n")
				buf.WriteString("stylesheet.replaceSync(")
				buf.WriteString(strings.TrimSuffix(string(utils.MustEncodeJSON(strings.TrimSuffix(string(css), "\n"))), "\n"))
				buf.WriteString(");\n")
				buf.WriteString("export default stylesheet;\n")
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Content-Length", fmt.Sprintf("%d", buf.Len()))
				ctx.SetHeader("Cache-Control", ccImmutable)
				return buf
			}

			// serve package raw files
			if pathKind == RawFile {
				if esmPath.SubPath == "" {
					b := &BuildContext{
						npmrc:   npmrc,
						esmPath: esmPath,
					}
					err = b.install()
					if err != nil {
						return rex.Status(500, err.Error())
					}
					entry := b.resolveEntry(esmPath)
					if entry.main == "" {
						ctx.SetHeader("Cache-Control", ccImmutable)
						return rex.Status(404, "File Not Found")
					}
					query := ""
					if rawQuery != "" {
						query = "?" + rawQuery
					}
					return redirect(ctx, fmt.Sprintf("%s/%s%s%s", origin, esmPath.ID(), utils.NormalizePathname(entry.main), query), true)
				}
				filename := path.Join(npmrc.StoreDir(), esmPath.ID(), "node_modules", esmPath.PkgName, esmPath.SubPath)
				stat, err := os.Lstat(filename)
				if err != nil && os.IsNotExist(err) {
					// if the file does not exist, try to install the package
					_, err = npmrc.installPackage(esmPath.Package())
					if err != nil {
						return rex.Status(500, err.Error())
					}
					stat, err = os.Lstat(filename)
				}
				if err != nil {
					if os.IsNotExist(err) {
						// try to resolve the file through package.json exports
						b := &BuildContext{
							npmrc:   npmrc,
							esmPath: esmPath,
						}
						err = b.install()
						if err != nil {
							return rex.Status(500, err.Error())
						}
						entry := b.resolveEntry(esmPath)
						if entry.main != "" && entry.main != "./"+esmPath.SubPath {
							// redirect to the resolved path
							query := ""
							if rawQuery != "" {
								query = "?" + rawQuery
							}
							return redirect(ctx, fmt.Sprintf("%s/%s%s%s", origin, esmPath.ID(), utils.NormalizePathname(entry.main), query), true)
						}
						ctx.SetHeader("Cache-Control", ccImmutable)
						return rex.Status(404, "File Not Found")
					}
					return rex.Status(500, err.Error())
				}
				if stat.IsDir() {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(404, "File Not Found")
				}
				// limit the file size up to 50MB
				if stat.Size() > maxAssetFileSize {
					ctx.SetHeader("Cache-Control", ccImmutable)
					return rex.Status(403, "File Too Large")
				}
				etag := fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
				if ifNoneMatch := ctx.R.Header.Get("If-None-Match"); ifNoneMatch == etag {
					return rex.Status(http.StatusNotModified, nil)
				}
				content, err := os.Open(filename)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if endsWith(esmPath.SubPath, ".js", ".mjs", ".cjs") {
					ctx.SetHeader("Content-Type", ctJavaScript)
				} else if endsWith(esmPath.SubPath, ".ts", ".mts", ".cts", ".tsx") {
					ctx.SetHeader("Content-Type", ctTypeScript)
				} else if strings.HasSuffix(esmPath.SubPath, ".jsx") {
					ctx.SetHeader("Content-Type", "text/jsx; charset=utf-8")
				} else {
					contentType := mime.GetContentType(esmPath.SubPath)
					if contentType != "" {
						ctx.SetHeader("Content-Type", contentType)
					}
				}
				ctx.SetHeader("Content-Length", fmt.Sprintf("%d", stat.Size()))
				ctx.SetHeader("Etag", etag)
				ctx.SetHeader("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
				ctx.SetHeader("Cache-Control", ccImmutable)
				if strings.HasSuffix(esmPath.SubPath, ".json") && query.Has("module") {
					defer content.Close()
					jsonData, err := io.ReadAll(content)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ctx.SetHeader("Content-Type", ctJavaScript)
					return concatBytes([]byte("export default "), jsonData)
				}
				return content // auto closed
			}

			// serve build/dts files
			if pathKind == EsmBuild || pathKind == EsmSourceMap || pathKind == EsmDts {
				var savePath string
				if asteriskPrefix {
					pathname = "/*" + pathname[1:]
				}
				if pathKind == EsmDts {
					savePath = path.Join("types", pathname)
				} else {
					savePath = path.Join("modules", pathname)
				}
				savePath = normalizeSavePath(savePath)
				f, stat, err := esmStorage.Get(savePath)
				if err != nil {
					if err != storage.ErrNotFound {
						logger.Errorf("storage.get(%s): %v", savePath, err)
						return rex.Status(500, "Storage error, please try again")
					} else if pathKind == EsmSourceMap {
						ctx.SetHeader("Cache-Control", ccImmutable)
						return rex.Status(404, "Not found")
					}
				}
				if err == nil {
					ctx.SetHeader("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
					ctx.SetHeader("Cache-Control", ccImmutable)
					if pathKind == EsmDts {
						ctx.SetHeader("Content-Type", ctTypeScript)
					} else if pathKind == EsmSourceMap {
						ctx.SetHeader("Content-Type", ctJSON)
					} else if strings.HasSuffix(pathname, ".css") {
						ctx.SetHeader("Content-Type", ctCSS)
					} else {
						ctx.SetHeader("Content-Type", ctJavaScript)
						// check `?exports` query
						jsIndentSet := set.New[string]()
						if query.Has("exports") {
							for p := range strings.SplitSeq(query.Get("exports"), ",") {
								p = strings.TrimSpace(p)
								if isJsIdentifier(p) {
									jsIndentSet.Add(p)
								}
							}
						}
						exports := jsIndentSet.Values()
						sort.Strings(exports)
						if query.Has("worker") {
							defer f.Close()
							moduleUrl := origin + pathname
							if len(exports) > 0 {
								moduleUrl += "?exports=" + strings.Join(exports, ",")
							}
							return fmt.Sprintf(
								`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
								moduleUrl,
								moduleUrl,
							)
						}
						if len(exports) > 0 {
							defer f.Close()
							xxh := xxhash.New()
							xxh.Write([]byte(strings.Join(exports, ",")))
							savePath = strings.TrimSuffix(savePath, ".mjs") + "_" + base64.RawURLEncoding.EncodeToString(xxh.Sum(nil)) + ".mjs"
							f2, stat, err := esmStorage.Get(savePath)
							if err == nil {
								ctx.SetHeader("Content-Length", fmt.Sprintf("%d", stat.Size()))
								return f2 // auto closed
							}
							if err != storage.ErrNotFound {
								logger.Errorf("storage.get(%s): %v", savePath, err)
								return rex.Status(500, "Storage error, please try again")
							}
							code, err := io.ReadAll(f)
							if err != nil {
								return rex.Status(500, err.Error())
							}
							target := "es2022"
							// check target in the pathname
							for seg := range strings.SplitSeq(pathname, "/") {
								if targets[seg] > 0 {
									target = seg
									break
								}
							}
							ret, err := treeShake(code, exports, targets[target])
							if err != nil {
								return rex.Status(500, err.Error())
							}
							go esmStorage.Put(savePath, bytes.NewReader(ret))
							// note: the source map is dropped
							return ret
						}
					}
					if pathKind == EsmDts {
						defer f.Close()
						buffer, err := io.ReadAll(f)
						if err != nil {
							return rex.Status(500, err.Error())
						}
						return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(origin))
					}
					ctx.SetHeader("Content-Length", fmt.Sprintf("%d", stat.Size()))
					return f // auto closed
				}
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		var targetFromUA bool
		if target == "" {
			target = strings.ToLower(query.Get("target"))
			targetFromUA = targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(ctx.UserAgent())
			}
		}

		// redirect to the url with exact package version for `deno` and `denonext` target
		if !isExactVersion && (target == "denonext" || target == "deno") {
			pkgName := esmPath.PkgName
			pkgVersion := esmPath.PkgVersion
			subPath := ""
			qs := ""
			if strings.HasPrefix(pkgName, "@jsr/") {
				pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
			}
			if asteriskPrefix {
				if esmPath.GhPrefix || esmPath.PrPrefix {
					pkgName = pkgName[0:3] + "*" + pkgName[3:]
				} else {
					pkgName = "*" + pkgName
				}
			}
			if esmPath.SubPath != "" {
				subPath = "/" + esmPath.SubPath
				// workaround for es5-ext "../#/.." path
				if esmPath.PkgName == "es5-ext" {
					subPath = strings.ReplaceAll(subPath, "/#/", "/%23/")
				}
			}
			if extraQuery != "" {
				pkgVersion += "&" + extraQuery
			}
			if rawQuery != "" {
				qs = "?" + rawQuery
			}
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			return redirect(ctx, fmt.Sprintf("%s%s/%s@%s%s%s", origin, registryPrefix, pkgName, pkgVersion, subPath, qs), false)
		}

		// check `?alias` query
		alias := map[string]string{}
		if query.Has("alias") {
			for p := range strings.SplitSeq(query.Get("alias"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" && name != esmPath.PkgName {
						alias[name] = to
					}
				}
			}
		}

		// check `?deps` query
		deps := map[string]string{}
		if query.Has("deps") {
			for v := range strings.SplitSeq(query.Get("deps"), ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					esm, _, _, _, _, err := parseEsmPath(npmrc, v)
					if err != nil {
						ctx.SetHeader("Cache-Control", ccImmutable)
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", v))
					}
					if esm.PkgName != esmPath.PkgName {
						deps[esm.PkgName] = esm.PkgVersion
					}
				}
			}
		}

		// check `?conditions` query
		var conditions []string
		conditionsSet := set.New[string]()
		if query.Has("conditions") {
			for p := range strings.SplitSeq(query.Get("conditions"), ",") {
				p = strings.TrimSpace(p)
				if p != "" && !strings.ContainsRune(p, ' ') && !conditionsSet.Has(p) {
					conditionsSet.Add(p)
					conditions = append(conditions, p)
				}
			}
		}

		// check `?external` query
		external := set.New[string]()
		externalAll := asteriskPrefix
		if !asteriskPrefix && query.Has("external") {
			for p := range strings.SplitSeq(query.Get("external"), ",") {
				p = strings.TrimSpace(p)
				if p == "*" {
					external.Reset()
					externalAll = true
					break
				}
				if p != "" {
					external.Add(p)
				}
			}
		}

		buildArgs := BuildArgs{
			Alias:      alias,
			Conditions: conditions,
			Deps:       deps,
		}
		if !externalAll && external.Len() > 0 {
			buildArgs.External = *external.ReadOnly()
		}

		if xArgs != nil {
			buildArgs = *xArgs
		}

		// build and return the types(.d.ts) file
		if pathKind == EsmDts {
			readDts := func() (content io.ReadCloser, stat storage.Stat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, true); a != "" {
					args = "X-" + a
				}
				savePath := normalizeSavePath(path.Join(fmt.Sprintf(
					"types/%s/%s",
					esmPath.ID(),
					args,
				), esmPath.SubPath))
				content, stat, err = esmStorage.Get(savePath)
				return
			}
			content, _, err := readDts()
			if err != nil {
				if err != storage.ErrNotFound {
					return rex.Status(500, "Storage error, please try again")
				}
				buildCtx := &BuildContext{
					npmrc:       npmrc,
					logger:      logger,
					metaDB:      metaDB,
					storage:     esmStorage,
					esmPath:     esmPath,
					args:        buildArgs,
					externalAll: externalAll,
					target:      "types",
				}
				ch := buildQueue.Add(buildCtx)
				select {
				case output := <-ch:
					if output.err != nil {
						if output.err.Error() == "types not found" {
							if isExactVersion {
								ctx.SetHeader("Cache-Control", ccImmutable)
							} else {
								ctx.SetHeader("Cache-Control", ccOneDay)
							}
							return rex.Status(404, "Types Not Found")
						}
						return rex.Status(500, "Failed to build types: "+output.err.Error())
					}
				case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
					ctx.SetHeader("Cache-Control", ccMustRevalidate)
					return rex.Status(http.StatusRequestTimeout, "timeout, the types is waiting to be built, please try refreshing the page.")
				}
				content, _, err = readDts()
			}
			if err != nil {
				if err == storage.ErrNotFound {
					if isExactVersion {
						ctx.SetHeader("Cache-Control", ccImmutable)
					} else {
						ctx.SetHeader("Cache-Control", ccOneDay)
					}
					return rex.Status(404, "Types Not Found")
				}
				return rex.Status(500, err.Error())
			}
			defer content.Close()
			buffer, err := io.ReadAll(content)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Content-Type", ctTypeScript)
			ctx.SetHeader("Cache-Control", ccImmutable)
			return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(origin))
		}

		if xArgs == nil {
			externalRequire := query.Has("external-require")
			// workaround: force "unocss/preset-icons" to external `require` calls
			if !externalRequire && esmPath.PkgName == "@unocss/preset-icons" {
				externalRequire = true
			}
			buildArgs.ExternalRequire = externalRequire
			buildArgs.KeepNames = query.Has("keep-names")
			buildArgs.IgnoreAnnotations = query.Has("ignore-annotations")
		}

		bundleMode := BundleDefault
		if (query.Has("bundle") && query.Get("bundle") != "false") || query.Has("bundle-all") || query.Has("bundle-deps") || query.Has("standalone") {
			bundleMode = BundleDeps
		} else if query.Has("no-bundle") || query.Get("bundle") == "false" {
			bundleMode = BundleFalse
		}

		dev := query.Has("dev")
		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !dev && (((esmPath.PkgName == "react" || esmPath.PkgName == "vue") && esmPath.SubPath == "jsx-dev-runtime") || esmPath.PkgName == "react-refresh") {
			dev = true
		}

		// get build args from the pathname
		if pathKind == EsmBuild {
			if strings.HasSuffix(esmPath.SubPath, ".bundle") {
				esmPath.SubPath = strings.TrimSuffix(esmPath.SubPath, ".bundle")
				bundleMode = BundleDeps
			} else if strings.HasSuffix(esmPath.SubPath, ".nobundle") {
				esmPath.SubPath = strings.TrimSuffix(esmPath.SubPath, ".nobundle")
				bundleMode = BundleFalse
			}
			if strings.HasSuffix(esmPath.SubPath, ".development") {
				esmPath.SubPath = strings.TrimSuffix(esmPath.SubPath, ".development")
				dev = true
			}
			basename := strings.TrimSuffix(path.Base(esmPath.PkgName), ".js")
			switch esmPath.SubPath {
			case basename:
				esmPath.SubPath = ""
			case "__" + basename:
				// the sub-module name is same as the package name
				esmPath.SubPath = basename
			}
			// a := strings.Split(esmPath.SubPath, "/")
			// if len(a) > 0 {
			// 	maybeTarget := a[0]
			// 	if _, ok := targets[maybeTarget]; ok {
			// 		subPath := strings.Join(a[1:], "/")

			// 		basename := strings.TrimSuffix(path.Base(esmPath.PkgName), ".js")
			// 		if strings.HasSuffix(subPath, ".css") && !strings.HasSuffix(esmPath.SubPath, ".mjs") {
			// 			if subPath == basename+".css" {
			// 				esmPath.SubPath = ""
			// 				target = maybeTarget
			// 			} else {
			// 				url := fmt.Sprintf("%s/%s", origin, esmPath.String())
			// 				return redirect(ctx, url, isExactVersion)
			// 			}
			// 		} else {
			// 			switch subPath {
			// 			case basename:
			// 				subPath = ""
			// 			case "__" + basename:
			// 				// the sub-module name is same as the package name
			// 				subPath = basename
			// 			}
			// 			esmPath.SubPath = subPath
			// 			target = maybeTarget
			// 		}
			// 	}
			// }
		}

		build := &BuildContext{
			npmrc:       npmrc,
			logger:      logger,
			metaDB:      metaDB,
			storage:     esmStorage,
			esmPath:     esmPath,
			args:        buildArgs,
			bundleMode:  bundleMode,
			externalAll: externalAll,
			target:      target,
			dev:         dev,
		}
		buildMeta, ok, err := build.Exists()
		if err != nil {
			return rex.Status(500, err.Error())
		}
		if !ok {
			ch := buildQueue.Add(build)
			select {
			case output := <-ch:
				if output.err != nil {
					msg := output.err.Error()
					if msg == "could not resolve build entry" || strings.HasSuffix(msg, " not found") || strings.Contains(msg, "is not exported from package") || strings.Contains(msg, "no such file or directory") {
						ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
						return rex.Status(404, msg)
					}
					return rex.Status(500, msg)
				}
				buildMeta = output.meta
			case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
				return rex.Status(http.StatusRequestTimeout, "timeout, the module is waiting to be built, please try refreshing the page.")
			}
		}

		if buildMeta.CSSEntry != "" {
			url := strings.Join([]string{origin, esmPath.ID(), buildMeta.CSSEntry[2:]}, "/")
			return redirect(ctx, url, isExactVersion)
		}

		// redirect to `*.d.ts` file
		if buildMeta.TypesOnly {
			dtsUrl := origin + buildMeta.Dts
			ctx.SetHeader("X-TypeScript-Types", dtsUrl)
			ctx.SetHeader("Content-Type", ctJavaScript)
			ctx.SetHeader("Cache-Control", ccImmutable)
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if query.Has("css") && esmPath.SubPath == "" {
			if !buildMeta.CSSInJS {
				if isExactVersion {
					ctx.SetHeader("Cache-Control", ccImmutable)
				} else {
					ctx.SetHeader("Cache-Control", ccOneDay)
				}
				return rex.Status(404, "Package CSS not found")
			}
			url := origin + strings.TrimSuffix(build.Path(), ".mjs") + ".css"
			return redirect(ctx, url, isExactVersion)
		}

		if query.Has("meta") {
			metaJson := map[string]any{
				"name":    esmPath.PkgName,
				"version": esmPath.PkgVersion,
			}
			if esmPath.GhPrefix {
				metaJson["gh"] = true
			}
			if esmPath.PrPrefix {
				metaJson["pr"] = true
			}
			if esmPath.SubPath != "" {
				metaJson["subpath"] = esmPath.SubPath
			} else {
				packageJson, err := npmrc.getPackageInfo(esmPath.PkgName, esmPath.PkgVersion)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				var exports []string
				for _, key := range packageJson.Exports.Keys() {
					if strings.HasPrefix(key, "./") && key != "./package.json" {
						exports = append(exports, key)
					}
				}
				metaJson["exports"] = exports
			}
			if buildMeta.Dts != "" {
				metaJson["dts"] = buildMeta.Dts
			}
			if buildMeta.Imports != nil {
				packageJson, err := npmrc.getPackageInfo(esmPath.PkgName, esmPath.PkgVersion)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				var imports []string
				var peerImports []string
				for _, p := range buildMeta.Imports {
					pkgName := toPackageName(p)
					if _, ok := packageJson.PeerDependencies[pkgName]; ok {
						peerImports = append(peerImports, p)
					} else {
						imports = append(imports, p)
					}
				}
				if len(imports) > 0 {
					metaJson["imports"] = imports
				}
				if len(peerImports) > 0 {
					metaJson["peerImports"] = peerImports
				}
			}
			if buildMeta.CSSInJS {
				metaJson["cssInJS"] = true
			}
			if buildMeta.TypesOnly {
				metaJson["typesOnly"] = true
			}
			integrity := buildMeta.Integrity
			// compute the integrity from the original js if it's not set in the build meta
			if len(buildMeta.Integrity) == 0 || !strings.HasPrefix(buildMeta.Integrity, "sha384-") {
				savePath := build.getSavePath()
				f, _, err := esmStorage.Get(savePath)
				if err != nil {
					logger.Errorf("storage.get(%s): %v", savePath, err)
					return rex.Status(500, "Storage error, please try again")
				}
				defer f.Close()
				sha := sha512.New384()
				_, err = io.Copy(sha, f)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				integrity = "sha384-" + base64.StdEncoding.EncodeToString(sha.Sum(nil))
				buildMeta.Integrity = integrity
				err = metaDB.Put(build.Path(), encodeBuildMeta(buildMeta))
				if err != nil {
					return rex.Status(500, err.Error())
				}
			}
			metaJson["integrity"] = integrity
			ctx.SetHeader("Content-Type", ctJSON)
			if isExactVersion {
				ctx.SetHeader("Cache-Control", ccImmutable)
			} else {
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
			}
			return metaJson
		}

		// check `?exports` query
		jsIdentSet := set.New[string]()
		if query.Has("exports") {
			for p := range strings.SplitSeq(query.Get("exports"), ",") {
				p = strings.TrimSpace(p)
				if isJsIdentifier(p) {
					jsIdentSet.Add(p)
				}
			}
		}
		exports := jsIdentSet.Values()
		sort.Strings(exports)

		// if the path is `ESMBuild`, return the built js/css content
		if pathKind == EsmBuild {
			if esmPath.SubPath != build.esmPath.SubPath {
				buf, recycle := newBuffer()
				defer recycle()
				fmt.Fprintf(buf, "export * from \"%s\";\n", build.Path())
				if buildMeta.ExportDefault {
					fmt.Fprintf(buf, "export { default } from \"%s\";\n", build.Path())
				}
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Cache-Control", ccImmutable)
				return buf.Bytes()
			}
			savePath := build.getSavePath()
			if strings.HasSuffix(esmPath.SubPath, ".css") && buildMeta.CSSInJS {
				path, _ := utils.SplitByLastByte(savePath, '.')
				savePath = path + ".css"
			}
			f, fi, err := esmStorage.Get(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					// seems the build output file is not found in the storage
					// let's remove the build meta from the database and clear the cache
					// then re-build the module
					key := build.Path()
					metaDB.Delete(key)
					cacheLRU.Remove(key)
				} else {
					logger.Errorf("storage.get(%s): %v", savePath, err)
				}
				return rex.Status(500, "Storage error, please try again")
			}
			ctx.SetHeader("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			ctx.SetHeader("Cache-Control", ccImmutable)
			if endsWith(savePath, ".css") {
				ctx.SetHeader("Content-Type", ctCSS)
			} else if endsWith(savePath, ".map") {
				ctx.SetHeader("Content-Type", ctJSON)
			} else {
				ctx.SetHeader("Content-Type", ctJavaScript)
				if query.Has("worker") {
					defer f.Close()
					moduleUrl := origin + build.Path()
					if !buildMeta.CJS && len(exports) > 0 {
						moduleUrl += "?exports=" + strings.Join(exports, ",")
					}
					return fmt.Sprintf(
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					)
				}
				if !buildMeta.CJS && len(exports) > 0 {
					defer f.Close()
					xxh := xxhash.New()
					xxh.Write([]byte(strings.Join(exports, ",")))
					savePath = strings.TrimSuffix(savePath, ".mjs") + "_" + base64.RawURLEncoding.EncodeToString(xxh.Sum(nil)) + ".mjs"
					f2, stat, err := esmStorage.Get(savePath)
					if err == nil {
						ctx.SetHeader("Content-Length", fmt.Sprintf("%d", stat.Size()))
						return f2 // auto closed
					}
					if err != storage.ErrNotFound {
						logger.Errorf("storage.get(%s): %v", savePath, err)
						return rex.Status(500, "Storage error, please try again")
					}
					code, err := io.ReadAll(f)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ret, err := treeShake(code, exports, targets[target])
					if err != nil {
						return rex.Status(500, err.Error())
					}
					go esmStorage.Put(savePath, bytes.NewReader(ret))
					// note: the source map is dropped
					return ret
				}
			}
			ctx.SetHeader("Content-Length", fmt.Sprintf("%d", fi.Size()))
			return f // auto closed
		}

		buf, recycle := newBuffer()
		defer recycle()
		fmt.Fprintf(buf, "/* esm.sh - %s */\n", esmPath.String())

		if query.Has("worker") {
			moduleUrl := origin + build.Path()
			if !buildMeta.CJS && len(exports) > 0 {
				moduleUrl += "?exports=" + strings.Join(exports, ",")
			}
			fmt.Fprintf(buf,
				`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
				moduleUrl,
				moduleUrl,
			)
		} else {
			if len(buildMeta.Imports) > 0 && len(exports) == 0 {
				for _, dep := range buildMeta.Imports {
					fmt.Fprintf(buf, "import \"%s\";\n", dep)
				}
			}
			esm := build.Path()
			if !buildMeta.CJS && len(exports) > 0 {
				esm += "?exports=" + strings.Join(exports, ",")
			}
			fmt.Fprintf(buf, "export * from \"%s\";\n", esm)
			if buildMeta.ExportDefault && (len(exports) == 0 || slices.Contains(exports, "default")) {
				fmt.Fprintf(buf, "export { default } from \"%s\";\n", esm)
			}
			if buildMeta.CJS && len(exports) > 0 {
				fmt.Fprintf(buf, "import _ from \"%s\";\n", esm)
				fmt.Fprintf(buf, "export const { %s } = _;\n", strings.Join(exports, ", "))
			}
			ctx.SetHeader("X-ESM-Path", esm)
			if noDts := query.Has("no-dts") || query.Has("no-check"); !noDts && buildMeta.Dts != "" {
				ctx.SetHeader("X-TypeScript-Types", origin+buildMeta.Dts)
				ctx.SetHeader("Access-Control-Expose-Headers", "X-ESM-Path, X-TypeScript-Types")
			} else {
				ctx.SetHeader("Access-Control-Expose-Headers", "X-ESM-Path")
			}
		}

		if targetFromUA {
			appendVaryHeader(ctx.W.Header(), "User-Agent")
		}
		if isExactVersion {
			ctx.SetHeader("Cache-Control", ccImmutable)
		} else {
			ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
		}
		ctx.SetHeader("Content-Type", ctJavaScript)
		if ctx.R.Method == http.MethodHead {
			return rex.NoContent()
		}
		return buf.Bytes()
	}
}

func getOrigin(ctx *rex.Context) string {
	origin := ctx.R.Header.Get("X-Real-Origin")
	if origin != "" {
		return origin
	}
	proto := "http:"
	if cfVisitor := ctx.R.Header.Get("CF-Visitor"); cfVisitor != "" {
		if strings.Contains(cfVisitor, "\"https\"") {
			proto = "https:"
		}
	} else if ctx.R.TLS != nil {
		proto = "https:"
	}
	return proto + "//" + ctx.R.Host
}

func redirect(ctx *rex.Context, url string, isMovedPermanently bool) any {
	code := http.StatusFound
	if isMovedPermanently {
		code = http.StatusMovedPermanently
		ctx.SetHeader("Cache-Control", ccImmutable)
	} else {
		ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
	}
	ctx.SetHeader("Location", url)
	return rex.Status(code, nil)
}

func errorJS(ctx *rex.Context, message string) any {
	buf, recycle := newBuffer()
	defer recycle()
	buf.WriteString("/* esm.sh - error */\n")
	buf.WriteString("throw new Error(")
	buf.Write(utils.MustEncodeJSON(message))
	buf.WriteString(");\n")
	buf.WriteString("export default null;\n")
	ctx.SetHeader("Content-Type", ctJavaScript)
	ctx.SetHeader("Cache-Control", ccImmutable)
	return buf.Bytes()
}
