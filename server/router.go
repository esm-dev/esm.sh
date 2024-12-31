package server

import (
	"bytes"
	"crypto/sha1"
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
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/esm-dev/esm.sh/server/common"
	"github.com/esm-dev/esm.sh/server/storage"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/esbuild-internal/xxhash"
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

var (
	regexpVersion       = regexp.MustCompile(`^[\w\+\-\.]+$`)
	regexpVersionStrict = regexp.MustCompile(`^\d+\.\d+\.\d+(-[\w\+\-\.]+)?$`)
	regexpJSIdent       = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
)

func esmRouter() rex.Handle {
	var (
		startTime  = time.Now()
		globalETag = fmt.Sprintf(`W/"%s"`, VERSION)
	)

	return func(ctx *rex.Context) any {
		pathname := ctx.R.URL.Path

		// ban malicious requests
		if strings.HasPrefix(pathname, "/.") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// handle POST API requests
		switch ctx.R.Method {
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
				h.Write([]byte(fmt.Sprintf("%v", options.Minify)))
				hash := hex.EncodeToString(h.Sum(nil))

				// if previous build exists, return it directly
				savePath := normalizeSavePath(ctx.R.Header.Get("X-Zone-Id"), fmt.Sprintf("modules/transform/%s.mjs", hash))
				if file, _, err := buildStorage.Get(savePath); err == nil {
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					output := TransformOutput{
						Code: string(data),
					}
					file, _, err = buildStorage.Get(savePath + ".map")
					if err == nil {
						data, err = io.ReadAll(file)
						file.Close()
						if err == nil {
							output.Map = string(data)
						}
					}
					return output
				}

				importMap := common.ImportMap{Imports: map[string]string{}}
				if len(options.ImportMap) > 0 {
					err = json.Unmarshal(options.ImportMap, &importMap)
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
					go buildStorage.Put(savePath+".map", strings.NewReader(output.Map))
				}
				go buildStorage.Put(savePath, strings.NewReader(output.Code))
				ctx.Header.Set("Cache-Control", ccMustRevalidate)
				return output

			case "/purge":
				zoneId := ctx.FormValue("zoneId")
				packageName := ctx.FormValue("package")
				version := ctx.FormValue("version")
				if packageName == "" {
					return rex.Err(400, "param `package` is required")
				}
				if version != "" && !regexpVersion.MatchString(version) {
					return rex.Err(400, "invalid version")
				}
				prefix := ""
				if zoneId != "" {
					prefix = zoneId + "/"
				}
				deletedBuildFiles, err := buildStorage.DeleteAll(prefix + "esm/" + packageName + "@" + version)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				deletedDTSFiles, err := buildStorage.DeleteAll(prefix + "types/" + packageName + "@" + version)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				deleteKeys := make([]string, len(deletedBuildFiles)+len(deletedDTSFiles))
				copy(deleteKeys, deletedBuildFiles)
				copy(deleteKeys[len(deletedBuildFiles):], deletedDTSFiles)
				log.Infof("Purged %d files for %s@%s (ip: %s)", len(deleteKeys), packageName, version, ctx.RemoteIP())
				return map[string]any{"deleted": deleteKeys}

			default:
				return rex.Status(404, "not found")
			}
		case "GET", "HEAD":
			// continue
		default:
			return rex.Status(405, "Method Not Allowed")
		}

		// strip trailing slash
		if last := len(pathname) - 1; pathname != "/" && pathname[last] == '/' {
			pathname = pathname[:last]
		}

		// strip loc suffix
		// e.g. https://esm.sh/react/es2022/react.mjs:2:3
		i := len(pathname) - 1
		j := 0
		for {
			if i < 0 || pathname[i] == '/' {
				break
			}
			if pathname[i] == ':' {
				j = i
			}
			i--
		}
		if j > 0 {
			pathname = pathname[:j]
		}

		// static routes
		switch pathname {
		case "/favicon.ico":
			favicon, err := embedFS.ReadFile("embed/favicon.ico")
			if err != nil {
				return err
			}
			ctx.Header.Set("Content-Type", "image/x-icon")
			ctx.Header.Set("Cache-Control", ccImmutable)
			return favicon

		case "/robots.txt":
			return "User-agent: *\nAllow: /\n"

		case "/":
			if strings.HasPrefix(ctx.UserAgent(), "Deno/") {
				ctx.Header.Set("Content-Type", ctJavaScript)
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
					fetchClient, recycle := NewFetchClient(15, ctx.UserAgent())
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
				readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(getCdnOrigin(ctx)))
				readmeHtml, err := common.RenderMarkdown(readme, common.MarkdownRenderKindHTML)
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
			ctx.Header.Set("Content-Type", ctHTML)
			ctx.Header.Set("Cache-Control", ccMustRevalidate)
			ctx.Header.Set("Etag", globalETag)
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

			disk := "ok"
			var stat syscall.Statfs_t
			err := syscall.Statfs(config.WorkDir, &stat)
			if err == nil {
				avail := stat.Bavail * uint64(stat.Bsize)
				if avail < 100*MB {
					disk = "full"
				} else if avail < 1024*MB {
					disk = "low"
				}
			} else {
				disk = "error"
			}

			ctx.Header.Set("Cache-Control", ccMustRevalidate)
			return map[string]any{
				"buildQueue": q[:i],
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
				"disk":       disk,
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
				return rex.Status(500, "Unknown error")
			}

		// builtin scripts
		case "/x", "/run":
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
				ctx.Header.Set("Cache-Control", ccMustRevalidate)
			} else {
				ctx.Header.Set("Cache-Control", ccOneDay)
			}
			ctx.Header.Set("Etag", globalETag)
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			ctx.Header.Set("Content-Type", ctJavaScript)
			return js
		}

		// module generated by the `/transform` API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByFirstByte(pathname[2:], '.')
			if len(hash) != 40 || !valid.IsHexString(hash) {
				return rex.Status(404, "Not Found")
			}
			savePath := normalizeSavePath(ctx.R.Header.Get("X-Zone-Id"), fmt.Sprintf("modules/transform/%s.%s", hash, ext))
			f, fi, err := buildStorage.Get(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if strings.HasSuffix(pathname, ".map") {
				ctx.Header.Set("Content-Type", ctJSON)
			} else {
				ctx.Header.Set("Content-Type", ctJavaScript)
			}
			ctx.Header.Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			ctx.Header.Set("Cache-Control", ccImmutable)
			return f // auto closed
		}

		// node libs
		if strings.HasPrefix(pathname, "/node/") {
			if !strings.HasSuffix(pathname, ".mjs") {
				return rex.Status(404, "Not Found")
			}
			name := pathname[6:]
			code, ok := unenvNodeRuntimeBulid[name]
			if !ok {
				if !nodeBuiltinModules[name] {
					return rex.Status(404, "Not Found")
				}
				code = []byte("export default {}")
			}
			if strings.HasPrefix(name, "chunk-") {
				ctx.Header.Set("Cache-Control", ccImmutable)
			} else {
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch == globalETag && !DEBUG {
					return rex.Status(http.StatusNotModified, nil)
				}
				ctx.Header.Set("Cache-Control", ccOneDay)
				ctx.Header.Set("Etag", globalETag)
			}
			ctx.Header.Set("Content-Type", ctJavaScript)
			return code
		}

		// embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile(pathname)
			if err != nil {
				return rex.Status(404, "not found")
			}
			if !DEBUG {
				ctx.Header.Set("Cache-Control", ccMustRevalidate)
			} else {
				etag := fmt.Sprintf(`W/"%d%d"`, startTime.Unix(), len(data))
				if ifNoneMatch := ctx.R.Header.Get("If-None-Match"); ifNoneMatch == etag {
					return rex.Status(http.StatusNotModified, nil)
				}
				ctx.Header.Set("Etag", etag)
				ctx.Header.Set("Cache-Control", ccOneDay)
			}
			contentType := common.ContentType(pathname)
			if contentType != "" {
				ctx.Header.Set("Content-Type", contentType)
			}
			return data
		}

		var npmrc *NpmRC
		if v := ctx.R.Header.Get("X-Npmrc"); v != "" {
			rc, err := NewNpmRcFromJSON([]byte(v))
			if err != nil {
				return rex.Status(400, "Invalid Npmrc Header")
			}
			npmrc = rc
		} else {
			npmrc = DefaultNpmRC()
		}

		zoneIdHeader := ctx.R.Header.Get("X-Zone-Id")
		if zoneIdHeader != "" {
			if !valid.IsDomain(zoneIdHeader) {
				zoneIdHeader = ""
			} else {
				var scopeName string
				if pkgName := toPackageName(pathname[1:]); strings.HasPrefix(pkgName, "@") {
					scopeName = pkgName[:strings.Index(pkgName, "/")]
				}
				if scopeName != "" {
					reg, ok := npmrc.ScopedRegistries[scopeName]
					if !ok || (reg.Registry == jsrRegistry && reg.Token == "" && (reg.User == "" || reg.Password == "")) {
						zoneIdHeader = ""
					}
				} else if npmrc.Registry == npmRegistry && npmrc.Token == "" && (npmrc.User == "" || npmrc.Password == "") {
					zoneIdHeader = ""
				}
			}
		}
		if zoneIdHeader != "" {
			npmrc.zoneId = zoneIdHeader
		}

		if strings.HasPrefix(pathname, "/http://") || strings.HasPrefix(pathname, "/https://") {
			query := ctx.Query()
			modUrl, err := url.Parse(pathname[1:])
			if err != nil {
				return rex.Status(400, "Invalid URL")
			}
			if modUrl.Scheme != "http" && modUrl.Scheme != "https" {
				return rex.Status(400, "Invalid URL")
			}
			modUrlRaw := modUrl.String()
			// disallow localhost or ip address for production
			if !DEBUG {
				hostname := modUrl.Hostname()
				if isLocalhost(hostname) || !valid.IsDomain(hostname) || modUrl.Host == ctx.R.Host {
					return rex.Status(400, "Invalid URL")
				}
			}
			extname := path.Ext(modUrl.Path)
			if !(stringInSlice(moduleExts, extname) || extname == ".vue" || extname == ".svelte" || extname == ".md" || extname == ".css") {
				return redirect(ctx, modUrl.String(), true)
			}
			target := strings.ToLower(query.Get("target"))
			if targets[target] == 0 {
				target = "es2022"
			}
			v := query.Get("v")
			if v != "" && (!regexpVersion.MatchString(v) || len(v) > 32) {
				return rex.Status(400, "Invalid Version Param")
			}
			fetchClient, recycle := NewFetchClient(15, ctx.UserAgent())
			defer recycle()
			if strings.HasSuffix(modUrl.Path, "/uno.css") {
				ctxParam := query.Get("ctx")
				if ctxParam == "" {
					return rex.Status(400, "Missing `ctx` Param")
				}
				ctxPath, err := atobUrl(ctxParam)
				if err != nil {
					return rex.Status(400, "Invalid `ctx` Param")
				}
				ctxUrlRaw := modUrl.Scheme + "://" + modUrl.Host + ctxPath
				ctxUrl, err := url.Parse(ctxUrlRaw)
				if err != nil {
					return rex.Status(400, "Invalid `ctx` Param")
				}
				h := sha1.New()
				h.Write([]byte(modUrlRaw))
				h.Write([]byte(ctxParam))
				h.Write([]byte(target))
				h.Write([]byte(v))
				savePath := normalizeSavePath(zoneIdHeader, path.Join("modules/x", hex.EncodeToString(h.Sum(nil))+".css"))
				r, _, err := buildStorage.Get(savePath)
				if err != nil && err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
				if err == nil {
					ctx.Header.Set("Cache-Control", ccImmutable)
					ctx.Header.Set("Content-Type", ctCSS)
					return r // auto closed
				}
				res, err := fetchClient.Fetch(ctxUrl, nil)
				if err != nil {
					return rex.Status(500, "Failed to fetch unocss context page content")
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					if res.StatusCode == 404 {
						return rex.Status(404, "Unocss context page not found")
					}
					return rex.Status(500, "Failed to fetch unocss context page content")
				}
				tokenizer := html.NewTokenizer(io.LimitReader(res.Body, 5*MB))
				content := []string{}
				jsEntries := map[string]struct{}{}
				importMap := common.ImportMap{}
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
								typeAttr string
								srcAttr  string
								hrefAttr string
							)
							for moreAttr {
								var key, val []byte
								key, val, moreAttr = tokenizer.TagAttr()
								if len(val) > 0 {
									if bytes.Equal(key, []byte("type")) {
										typeAttr = string(val)
									} else if bytes.Equal(key, []byte("src")) {
										srcAttr = string(val)
									} else if bytes.Equal(key, []byte("href")) {
										hrefAttr = string(val)
									}
								}
							}
							if typeAttr == "importmap" {
								tokenizer.Next()
								innerText := bytes.TrimSpace(tokenizer.Text())
								if len(innerText) > 0 {
									err := json.Unmarshal(innerText, &importMap)
									if err == nil {
										importMap.Src = ctxUrl.Path
									}
								}
							} else if srcAttr == "" {
								// inline script content
								tokenizer.Next()
								content = append(content, string(tokenizer.Text()))
							} else {
								if hrefAttr != "" && isHttpSepcifier(srcAttr) {
									if !isHttpSepcifier(hrefAttr) && endsWith(hrefAttr, moduleExts...) {
										jsEntries[hrefAttr] = struct{}{}
									}
								} else if !isHttpSepcifier(srcAttr) && endsWith(srcAttr, ".js", ".mjs") {
									jsEntries[srcAttr] = struct{}{}
								}
							}
						case "link", "meta", "title", "base", "head", "noscript", "slot", "template", "option":
							// ignore
						default:
							content = append(content, string(tokenizer.Raw()))
						}
					}
				}
				res, err = fetchClient.Fetch(modUrl, nil)
				if err != nil {
					return rex.Status(500, "Failed to fetch uno.css")
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					if res.StatusCode == 404 {
						return rex.Status(404, "uno.css not found")
					}
					return rex.Status(500, "Failed to fetch uno.css: "+res.Status)
				}
				configCSS, err := io.ReadAll(io.LimitReader(res.Body, MB))
				if err != nil {
					return rex.Status(500, "Failed to fetch uno.css")
				}
				for src := range jsEntries {
					url := ctxUrl.ResolveReference(&url.URL{Path: src})
					_, _, _, tree, err := bundleHttpModule(npmrc, url.String(), importMap, true, fetchClient)
					if err == nil {
						for _, code := range tree {
							content = append(content, string(code))
						}
					}
				}
				out, err := generateUnoCSS(npmrc, string(configCSS), strings.Join(content, "\n"))
				if err != nil {
					return rex.Status(500, "Failed to generate uno.css: "+err.Error())
				}
				ret := esbuild.Build(esbuild.BuildOptions{
					Stdin: &esbuild.StdinOptions{
						Sourcefile: "uno.css",
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
				go buildStorage.Put(savePath, bytes.NewReader(minifiedCSS))
				ctx.Header.Set("Cache-Control", ccImmutable)
				ctx.Header.Set("Content-Type", ctCSS)
				return minifiedCSS
			} else {
				im := query.Get("im")
				h := sha1.New()
				h.Write([]byte(modUrlRaw))
				h.Write([]byte(im))
				h.Write([]byte(target))
				h.Write([]byte(v))
				savePath := normalizeSavePath(zoneIdHeader, path.Join("modules/x", hex.EncodeToString(h.Sum(nil))+".mjs"))
				content, _, err := buildStorage.Get(savePath)
				if err != nil && err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
				var body io.Reader = content
				if err == storage.ErrNotFound {
					importMap := common.ImportMap{}
					if len(im) > 0 {
						imPath, err := atobUrl(im)
						if err != nil {
							return rex.Status(400, "Invalid `im` Param")
						}
						imUrl, err := url.Parse(modUrl.Scheme + "://" + modUrl.Host + imPath)
						if err != nil {
							return rex.Status(400, "Invalid `im` Param")
						}
						res, err := fetchClient.Fetch(imUrl, nil)
						if err != nil {
							return rex.Status(500, "Failed to fetch import map")
						}
						defer res.Body.Close()
						if res.StatusCode != 200 {
							return rex.Status(500, "Failed to fetch import map")
						}
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
										err := json.Unmarshal(innerText, &importMap)
										if err != nil {
											return rex.Status(400, "Invalid import map")
										}
										importMap.Src, _ = atobUrl(im)
									}
									break
								}
							}
						}
					}
					if extname == ".md" {
						for _, kind := range []string{"jsx", "svelte", "vue"} {
							if query.Has(kind) {
								modUrl.RawQuery = kind
								modUrlRaw = modUrl.String()
								break
							}
						}
					}
					js, jsx, css, _, err := bundleHttpModule(npmrc, modUrlRaw, importMap, false, fetchClient)
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
							Filename: modUrlRaw,
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
					go buildStorage.Put(savePath, strings.NewReader(out.Code))
				}
				if extname == ".css" && query.Has("module") {
					css, err := io.ReadAll(body)
					if closer, ok := body.(io.Closer); ok {
						closer.Close()
					}
					if err != nil {
						return rex.Status(500, "Failed to read css")
					}
					body = strings.NewReader(fmt.Sprintf("var style = document.createElement('style');\nstyle.textContent = %s;\ndocument.head.appendChild(style);\nexport default null;", utils.MustEncodeJSON(string(css))))
				}
				ctx.Header.Set("Cache-Control", ccImmutable)
				if extname == ".css" {
					ctx.Header.Set("Content-Type", ctCSS)
				} else {
					ctx.Header.Set("Content-Type", ctJavaScript)
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

		esm, extraQuery, isFixedVersion, isBuildDist, err := praseEsmPath(npmrc, pathname)
		if err != nil {
			status := 500
			message := err.Error()
			if strings.HasPrefix(message, "invalid") {
				status = 400
			} else if strings.HasSuffix(message, " not found") {
				status = 404
			}
			return rex.Status(status, message)
		}

		pkgAllowed := config.AllowList.IsPackageAllowed(esm.PkgName)
		pkgBanned := config.BanList.IsPackageBanned(esm.PkgName)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		cdnOrigin := getCdnOrigin(ctx)

		registryPrefix := ""
		if esm.GhPrefix {
			registryPrefix = "/gh"
		} else if esm.PrPrefix {
			registryPrefix = "/pr"
		}

		// redirect `/@types/PKG` to it's main dts file
		if strings.HasPrefix(esm.PkgName, "@types/") && esm.SubPath == "" {
			info, err := npmrc.getPackageInfo(esm.PkgName, esm.PkgVersion)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			types := "index.d.ts"
			if info.Types != "" {
				types = info.Types
			} else if info.Typings != "" {
				types = info.Typings
			} else if info.Main != "" && strings.HasSuffix(info.Main, ".d.ts") {
				types = info.Main
			}
			return redirect(ctx, fmt.Sprintf("%s/%s@%s%s", cdnOrigin, info.Name, info.Version, utils.NormalizePathname(types)), isFixedVersion)
		}

		// redirect to the main css path for CSS packages
		if css := cssPackages[esm.PkgName]; css != "" && esm.SubModuleName == "" {
			url := fmt.Sprintf("%s/%s/%s", cdnOrigin, esm.Specifier(), css)
			return redirect(ctx, url, isFixedVersion)
		}

		// store the raw query
		rawQuery := ctx.R.URL.RawQuery

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"/jsx-runtime", "/jsx-dev-runtime"} {
			if strings.HasSuffix(rawQuery, jsxRuntime) {
				if esm.SubPath == "" {
					esm.SubPath = jsxRuntime[1:]
				} else {
					esm.SubPath = esm.SubPath + jsxRuntime
				}
				esm.SubModuleName = esm.SubPath
				pathname = fmt.Sprintf("/%s/%s", esm.PkgName, esm.SubPath)
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
		query := ctx.Query()

		// use `?path=$PATH` query to override the pathname
		if v := query.Get("path"); v != "" {
			esm.SubPath = utils.NormalizePathname(v)[1:]
			esm.SubModuleName = stripEntryModuleExt(esm.SubPath)
		}

		// check the path kind
		pathKind := EsmEntry
		if esm.SubPath != "" {
			ext := path.Ext(esm.SubPath)
			switch ext {
			case ".mjs":
				if isBuildDist {
					pathKind = EsmBuild
				}
			case ".ts", ".mts":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					pathKind = EsmDts
				}
			case ".css":
				if isBuildDist {
					pathKind = EsmBuild
				} else {
					pathKind = RawFile
				}
			case ".map":
				if isBuildDist {
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
		if query.Has("raw") {
			pathKind = RawFile
		}

		// redirect to the url with fixed package version
		if !isFixedVersion {
			if isBuildDist {
				pkgName := esm.Name()
				subPath := ""
				query := ""
				if asteriskPrefix {
					if esm.GhPrefix || esm.PrPrefix {
						pkgName = pkgName[0:3] + "*" + pkgName[3:]
					} else {
						pkgName = "*" + pkgName
					}
				}
				if extraQuery != "" {
					pkgName += "&" + extraQuery
				}
				if esm.SubPath != "" {
					subPath = "/" + esm.SubPath
				}
				if rawQuery != "" {
					query = "?" + rawQuery
				}
				ctx.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				return redirect(ctx, fmt.Sprintf("%s/%s%s%s", cdnOrigin, pkgName, subPath, query), false)
			}
			if pathKind != EsmEntry {
				pkgName := esm.PkgName
				pkgVersion := esm.PkgVersion
				subPath := ""
				qs := ""
				if strings.HasPrefix(pkgName, "@jsr/") {
					pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
				}
				if asteriskPrefix {
					if esm.GhPrefix || esm.PrPrefix {
						pkgName = pkgName[0:3] + "*" + pkgName[3:]
					} else {
						pkgName = "*" + pkgName
					}
				}
				if esm.SubPath != "" {
					subPath = "/" + esm.SubPath
					// workaround for es5-ext "../#/.." path
					if esm.PkgName == "es5-ext" {
						subPath = strings.ReplaceAll(subPath, "/#/", "/%23/")
					}
				}
				if extraQuery != "" {
					pkgVersion += "&" + extraQuery
				}
				if rawQuery != "" {
					qs = "?" + rawQuery
				}
				ctx.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				return redirect(ctx, fmt.Sprintf("%s%s/%s@%s%s%s", cdnOrigin, registryPrefix, pkgName, pkgVersion, subPath, qs), false)
			}
		} else {
			// `*.wasm` as an es6 module when `?module` query is set (requires `top-level-await` support)
			if pathKind == RawFile && strings.HasSuffix(esm.SubPath, ".wasm") && query.Has("module") {
				buf := &bytes.Buffer{}
				wasmUrl := cdnOrigin + pathname
				fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
				fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
				ctx.Header.Set("Content-Type", ctJavaScript)
				ctx.Header.Set("Cache-Control", ccImmutable)
				return buf
			}

			// fix url that is related to `import.meta.url`
			if pathKind == RawFile && isBuildDist && !query.Has("raw") {
				extname := path.Ext(esm.SubPath)
				dir := path.Join(npmrc.StoreDir(), esm.Name())
				if !existsDir(dir) {
					_, err := npmrc.installPackage(esm.Package())
					if err != nil {
						return rex.Status(500, err.Error())
					}
				}
				pkgRoot := path.Join(dir, "node_modules", esm.PkgName)
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
						if strings.HasSuffix(esm.SubPath, f) {
							file = f
							break
						}
					}
					if file == "" {
						for _, f := range files {
							if path.Base(esm.SubPath) == path.Base(f) {
								file = f
								break
							}
						}
					}
				}
				if file == "" {
					return rex.Status(404, "File not found")
				}
				url := fmt.Sprintf("%s/%s@%s/%s", cdnOrigin, esm.PkgName, esm.PkgVersion, file)
				return redirect(ctx, url, true)
			}

			// package raw files
			if pathKind == RawFile {
				var stat storage.Stat
				var content io.ReadCloser
				var etag string
				var cachePath string
				var cacheHit bool
				if config.CacheRawFile {
					cachePath = path.Join("raw", esm.Name(), esm.SubPath)
					content, stat, err = buildStorage.Get(cachePath)
					if err != nil && err != storage.ErrNotFound {
						return rex.Status(500, "storage error")
					}
					if err == nil {
						etag = fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
						if ifNoneMatch := ctx.R.Header.Get("If-None-Match"); ifNoneMatch == etag {
							defer content.Close()
							return rex.Status(http.StatusNotModified, nil)
						}
						cacheHit = true
					}
				}
				if !cacheHit {
					filename := path.Join(npmrc.StoreDir(), esm.Name(), "node_modules", esm.PkgName, esm.SubPath)
					stat, err = os.Lstat(filename)
					if err != nil && os.IsNotExist(err) {
						// if the file not found, try to install the package and retry
						_, err = npmrc.installPackage(esm.Package())
						if err != nil {
							return rex.Status(500, err.Error())
						}
						stat, err = os.Lstat(filename)
					}
					if err != nil {
						if os.IsNotExist(err) {
							return rex.Status(404, "File Not Found")
						}
						return rex.Status(500, err.Error())
					}
					// limit the file size up to 50MB
					if stat.Size() > maxAssetFileSize {
						return rex.Status(403, "File Too Large")
					}
					etag = fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
					if ifNoneMatch := ctx.R.Header.Get("If-None-Match"); ifNoneMatch == etag {
						return rex.Status(http.StatusNotModified, nil)
					}
					content, err = os.Open(filename)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					if config.CacheRawFile {
						go func() {
							f, err := os.Open(filename)
							if err != nil {
								return
							}
							defer f.Close()
							buildStorage.Put(cachePath, f)
						}()
					}
				}
				if endsWith(esm.SubPath, ".js", ".mjs", ".jsx") {
					ctx.Header.Set("Content-Type", ctJavaScript)
				} else if endsWith(esm.SubPath, ".ts", ".mts", ".tsx") {
					ctx.Header.Set("Content-Type", ctTypeScript)
				} else {
					contentType := common.ContentType(esm.SubPath)
					if contentType != "" {
						ctx.Header.Set("Content-Type", contentType)
					}
				}
				if cacheHit {
					ctx.Header.Set("X-Raw-File-Cache-Status", "HIT")
				}
				ctx.Header.Set("Etag", etag)
				ctx.Header.Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
				ctx.Header.Set("Cache-Control", ccImmutable)
				if strings.HasSuffix(esm.SubPath, ".json") && query.Has("module") {
					jsonData, err := io.ReadAll(content)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ctx.Header.Set("Content-Type", ctJavaScript)
					return concatBytes([]byte("export default "), jsonData)
				}
				return content // auto closed
			}

			// build/dts files
			if pathKind == EsmBuild || pathKind == EsmSourceMap || pathKind == EsmDts {
				var savePath string
				if pathKind == EsmDts {
					savePath = path.Join("types", pathname)
				} else {
					savePath = path.Join("modules", pathname)
				}
				savePath = normalizeSavePath(npmrc.zoneId, savePath)
				f, stat, err := buildStorage.Get(savePath)
				if err != nil {
					if err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					} else if pathKind == EsmSourceMap {
						return rex.Status(404, "Not found")
					}
				}
				if err == nil {
					ctx.Header.Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
					ctx.Header.Set("Cache-Control", ccImmutable)
					if pathKind == EsmDts {
						ctx.Header.Set("Content-Type", ctTypeScript)
					} else if pathKind == EsmSourceMap {
						ctx.Header.Set("Content-Type", ctJSON)
					} else if strings.HasSuffix(pathname, ".css") {
						ctx.Header.Set("Content-Type", ctCSS)
					} else {
						ctx.Header.Set("Content-Type", ctJavaScript)
						// check `?exports` query
						jsIndentSet := set.New[string]()
						if query.Has("exports") {
							for _, p := range strings.Split(query.Get("exports"), ",") {
								p = strings.TrimSpace(p)
								if regexpJSIdent.MatchString(p) {
									jsIndentSet.Add(p)
								}
							}
						}
						exports := jsIndentSet.Values()
						sort.Strings(exports)
						if query.Has("worker") {
							defer f.Close()
							moduleUrl := cdnOrigin + pathname
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
							f2, _, err := buildStorage.Get(savePath)
							if err == nil {
								return f2 // auto closed
							}
							if err != storage.ErrNotFound {
								return rex.Status(500, err.Error())
							}
							code, err := io.ReadAll(f)
							if err != nil {
								return rex.Status(500, err.Error())
							}
							target := "es2022"
							// check target in the pathname
							for _, seg := range strings.Split(pathname, "/") {
								if targets[seg] > 0 {
									target = seg
									break
								}
							}
							ret, err := treeShake(code, exports, targets[target])
							if err != nil {
								return rex.Status(500, err.Error())
							}
							go buildStorage.Put(savePath, bytes.NewReader(ret))
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
						return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))
					}
					return f // auto closed
				}
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		target := strings.ToLower(query.Get("target"))
		targetFromUA := targets[target] == 0
		if targetFromUA {
			target = getBuildTargetByUA(ctx.UserAgent())
		}

		// redirect to the url with fixed package version for `deno` and `denonext` target
		if !isFixedVersion && (target == "denonext" || target == "deno") {
			pkgName := esm.PkgName
			pkgVersion := esm.PkgVersion
			subPath := ""
			qs := ""
			if strings.HasPrefix(pkgName, "@jsr/") {
				pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
			}
			if asteriskPrefix {
				if esm.GhPrefix || esm.PrPrefix {
					pkgName = pkgName[0:3] + "*" + pkgName[3:]
				} else {
					pkgName = "*" + pkgName
				}
			}
			if esm.SubPath != "" {
				subPath = "/" + esm.SubPath
				// workaround for es5-ext "../#/.." path
				if esm.PkgName == "es5-ext" {
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
			return redirect(ctx, fmt.Sprintf("%s%s/%s@%s%s%s", cdnOrigin, registryPrefix, pkgName, pkgVersion, subPath, qs), false)
		}

		// check `?alias` query
		alias := map[string]string{}
		if query.Has("alias") {
			for _, p := range strings.Split(query.Get("alias"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" && name != esm.PkgName {
						alias[name] = to
					}
				}
			}
		}

		// check `?deps` query
		deps := map[string]string{}
		if query.Has("deps") {
			for _, v := range strings.Split(query.Get("deps"), ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					m, _, _, _, err := praseEsmPath(npmrc, v)
					if err != nil {
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", v))
					}
					if esm.PkgName == "react-dom" && m.PkgName == "react" {
						// make sure react-dom and react are in the same version
						continue
					}
					if m.PkgName != esm.PkgName {
						deps[m.PkgName] = m.PkgVersion
					}
				}
			}
		}

		// check `?conditions` query
		var conditions []string
		conditionsSet := set.New[string]()
		if query.Has("conditions") {
			for _, p := range strings.Split(query.Get("conditions"), ",") {
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
			for _, p := range strings.Split(query.Get("external"), ",") {
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
			alias:      alias,
			conditions: conditions,
			deps:       deps,
		}
		if !externalAll && external.Len() > 0 {
			buildArgs.external = *external.ReadOnly()
		}

		// match path `PKG@VERSION/X-${args}/esnext/SUBPATH`
		xArgs := false
		if pathKind == EsmBuild || pathKind == EsmDts {
			a := strings.Split(esm.SubModuleName, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				args, err := decodeBuildArgs(strings.TrimPrefix(a[0], "X-"))
				if err != nil {
					return rex.Status(500, "Invalid build args: "+a[0])
				}
				esm.SubPath = strings.Join(strings.Split(esm.SubPath, "/")[1:], "/")
				esm.SubModuleName = stripEntryModuleExt(esm.SubPath)
				buildArgs = args
				xArgs = true
			}
		}

		// resolve `alias`, `deps`, `external` of the build args
		if !xArgs {
			err := resolveBuildArgs(npmrc, path.Join(npmrc.StoreDir(), esm.Name()), &buildArgs, esm)
			if err != nil {
				return rex.Status(500, err.Error())
			}
		}

		// build and return the types(.d.ts) file
		if pathKind == EsmDts {
			readDts := func() (content io.ReadCloser, stat storage.Stat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, true); a != "" {
					args = "X-" + a
				}
				savePath := normalizeSavePath(npmrc.zoneId, path.Join(fmt.Sprintf(
					"types/%s/%s",
					esm.Name(),
					args,
				), esm.SubPath))
				content, stat, err = buildStorage.Get(savePath)
				return
			}
			content, _, err := readDts()
			if err != nil {
				if err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
				buildCtx := &BuildContext{
					esmPath:     esm,
					npmrc:       npmrc,
					args:        buildArgs,
					externalAll: externalAll,
					target:      "types",
					zoneId:      zoneIdHeader,
				}
				ch := buildQueue.Add(buildCtx)
				select {
				case output := <-ch:
					if output.err != nil {
						if output.err.Error() == "types not found" {
							return rex.Status(404, "Types Not Found")
						}
						return rex.Status(500, "Failed to build types: "+output.err.Error())
					}
				case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
					ctx.Header.Set("Cache-Control", ccMustRevalidate)
					return rex.Status(http.StatusRequestTimeout, "timeout, the types is waiting to be built, please try refreshing the page.")
				}
				content, _, err = readDts()
			}
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Types Not Found")
				}
				return rex.Status(500, err.Error())
			}
			defer content.Close()
			buffer, err := io.ReadAll(content)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.Header.Set("Content-Type", ctTypeScript)
			ctx.Header.Set("Cache-Control", ccImmutable)
			return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))
		}

		if !xArgs {
			externalRequire := query.Has("external-require")
			// workaround: force "unocss/preset-icons" to external `require` calls
			if !externalRequire && esm.PkgName == "@unocss/preset-icons" {
				externalRequire = true
			}
			buildArgs.externalRequire = externalRequire
			buildArgs.keepNames = query.Has("keep-names")
			buildArgs.ignoreAnnotations = query.Has("ignore-annotations")
		}

		bundleMode := BundleDefault
		if (query.Has("bundle") && query.Get("bundle") != "false") || query.Has("bundle-all") || query.Has("bundle-deps") || query.Has("standalone") {
			bundleMode = BundleAll
		} else if query.Has("no-bundle") || query.Get("bundle") == "false" {
			bundleMode = BundleFalse
		}

		isDev := query.Has("dev")
		isPkgCss := query.Has("css")
		isWorker := query.Has("worker")
		noDts := query.Has("no-dts") || query.Has("no-check")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((esm.PkgName == "react" && esm.SubModuleName == "jsx-dev-runtime") || esm.PkgName == "react-refresh") {
			isDev = true
		}

		// get build args from the pathname
		if pathKind == EsmBuild {
			a := strings.Split(esm.SubModuleName, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
					if strings.HasSuffix(submodule, ".bundle") {
						submodule = strings.TrimSuffix(submodule, ".bundle")
						bundleMode = BundleAll
					} else if strings.HasSuffix(submodule, ".nobundle") {
						submodule = strings.TrimSuffix(submodule, ".nobundle")
						bundleMode = BundleFalse
					}
					if strings.HasSuffix(submodule, ".development") {
						submodule = strings.TrimSuffix(submodule, ".development")
						isDev = true
					}
					basename := strings.TrimSuffix(path.Base(esm.PkgName), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(esm.SubPath, ".mjs") {
						if submodule == basename+".css" {
							esm.SubModuleName = ""
							target = maybeTarget
						} else {
							url := fmt.Sprintf("%s/%s", cdnOrigin, esm.Specifier())
							return redirect(ctx, url, isFixedVersion)
						}
					} else {
						if submodule == basename {
							submodule = ""
						} else if submodule == "__"+basename {
							// the sub-module name is same as the package name
							submodule = basename
						}
						esm.SubModuleName = submodule
						target = maybeTarget
					}
				}
			}
		}

		buildCtx := &BuildContext{
			esmPath:     esm,
			npmrc:       npmrc,
			args:        buildArgs,
			bundleMode:  bundleMode,
			externalAll: externalAll,
			target:      target,
			pinedTarget: !targetFromUA,
			dev:         isDev,
			zoneId:      zoneIdHeader,
		}
		ret, ok, err := buildCtx.Exists()
		if err != nil {
			return rex.Status(500, err.Error())
		}
		if !ok {
			ch := buildQueue.Add(buildCtx)
			select {
			case output := <-ch:
				if output.err != nil {
					msg := output.err.Error()
					if strings.Contains(msg, "no such file or directory") ||
						strings.Contains(msg, "is not exported from package") ||
						strings.Contains(msg, "could not resolve build entry") {
						ctx.Header.Set("Cache-Control", ccImmutable)
						return rex.Status(404, "module not found")
					}
					if strings.HasSuffix(msg, " not found") {
						return rex.Status(404, msg)
					}
					return rex.Status(500, msg)
				}
				ret = output.meta
			case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
				ctx.Header.Set("Cache-Control", ccMustRevalidate)
				return rex.Status(http.StatusRequestTimeout, "timeout, the module is waiting to be built, please try refreshing the page.")
			}
		}

		// redirect to `*.d.ts` file
		if ret.TypesOnly {
			dtsUrl := cdnOrigin + ret.Dts
			ctx.Header.Set("X-TypeScript-Types", dtsUrl)
			ctx.Header.Set("Content-Type", ctJavaScript)
			ctx.Header.Set("Cache-Control", ccImmutable)
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if isPkgCss && esm.SubModuleName == "" {
			if !ret.HasCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s.css", cdnOrigin, strings.TrimSuffix(buildCtx.Path(), ".mjs"))
			return redirect(ctx, url, isFixedVersion)
		}

		// check `?exports` query
		jsIdentSet := set.New[string]()
		if query.Has("exports") {
			for _, p := range strings.Split(query.Get("exports"), ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					jsIdentSet.Add(p)
				}
			}
		}
		exports := jsIdentSet.Values()
		sort.Strings(exports)

		// if the path is `ESMBuild`, return the built js/css content
		if pathKind == EsmBuild {
			savePath := buildCtx.getSavepath()
			if strings.HasSuffix(esm.SubPath, ".css") {
				path, _ := utils.SplitByLastByte(savePath, '.')
				savePath = path + ".css"
			}
			f, fi, err := buildStorage.Get(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "File not found")
				}
				return rex.Status(500, err.Error())
			}
			ctx.Header.Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			ctx.Header.Set("Cache-Control", ccImmutable)
			if endsWith(savePath, ".css") {
				ctx.Header.Set("Content-Type", ctCSS)
			} else if endsWith(savePath, ".map") {
				ctx.Header.Set("Content-Type", ctJSON)
			} else {
				ctx.Header.Set("Content-Type", ctJavaScript)
				if isWorker {
					defer f.Close()
					moduleUrl := cdnOrigin + buildCtx.Path()
					if !ret.CJS && len(exports) > 0 {
						moduleUrl += "?exports=" + strings.Join(exports, ",")
					}
					return fmt.Sprintf(
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					)
				}
				if !ret.CJS && len(exports) > 0 {
					defer f.Close()
					xxh := xxhash.New()
					xxh.Write([]byte(strings.Join(exports, ",")))
					savePath = strings.TrimSuffix(savePath, ".mjs") + "_" + base64.RawURLEncoding.EncodeToString(xxh.Sum(nil)) + ".mjs"
					f2, _, err := buildStorage.Get(savePath)
					if err == nil {
						return f2 // auto closed
					}
					if err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					}
					code, err := io.ReadAll(f)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ret, err := treeShake(code, exports, targets[target])
					if err != nil {
						return rex.Status(500, err.Error())
					}
					go buildStorage.Put(savePath, bytes.NewReader(ret))
					// note: the source map is dropped
					return ret
				}
			}
			return f // auto closed
		}

		buf, recycle := NewBuffer()
		defer recycle()
		fmt.Fprintf(buf, "/* esm.sh - %s */\n", esm.Specifier())

		if isWorker {
			moduleUrl := cdnOrigin + buildCtx.Path()
			if !ret.CJS && len(exports) > 0 {
				moduleUrl += "?exports=" + strings.Join(exports, ",")
			}
			fmt.Fprintf(buf,
				`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
				moduleUrl,
				moduleUrl,
			)
		} else {
			if len(ret.Imports) > 0 {
				for _, dep := range ret.Imports {
					fmt.Fprintf(buf, "import \"%s\";\n", dep)
				}
			}
			esmPath := buildCtx.Path()
			if !ret.CJS && len(exports) > 0 {
				esmPath += "?exports=" + strings.Join(exports, ",")
			}
			ctx.Header.Set("X-ESM-Path", esmPath)
			fmt.Fprintf(buf, "export * from \"%s\";\n", esmPath)
			if ret.ExportDefault && (len(exports) == 0 || stringInSlice(exports, "default")) {
				fmt.Fprintf(buf, "export { default } from \"%s\";\n", esmPath)
			}
			if ret.CJS && len(exports) > 0 {
				fmt.Fprintf(buf, "import _ from \"%s\";\n", esmPath)
				fmt.Fprintf(buf, "export const { %s } = _;\n", strings.Join(exports, ", "))
			}
			if !noDts && ret.Dts != "" {
				ctx.Header.Set("X-TypeScript-Types", cdnOrigin+ret.Dts)
				ctx.Header.Set("Access-Control-Expose-Headers", "X-ESM-Path, X-TypeScript-Types")
			} else {
				ctx.Header.Set("Access-Control-Expose-Headers", "X-ESM-Path")
			}
		}

		if targetFromUA {
			appendVaryHeader(ctx.W.Header(), "User-Agent")
		}
		if isFixedVersion {
			ctx.Header.Set("Cache-Control", ccImmutable)
		} else {
			ctx.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
		}
		ctx.Header.Set("Content-Type", ctJavaScript)
		if ctx.R.Method == http.MethodHead {
			return rex.NoContent()
		}
		return buf.Bytes()
	}
}

func getCdnOrigin(ctx *rex.Context) string {
	cdnOrigin := ctx.R.Header.Get("X-Real-Origin")
	if cdnOrigin == "" {
		proto := "http"
		if cfVisitor := ctx.R.Header.Get("CF-Visitor"); cfVisitor != "" {
			if strings.Contains(cfVisitor, "\"scheme\":\"https\"") {
				proto = "https"
			}
		} else if ctx.R.TLS != nil {
			proto = "https"
		}
		cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
	}
	return cdnOrigin
}

func redirect(ctx *rex.Context, url string, isMovedPermanently bool) any {
	code := http.StatusFound
	if isMovedPermanently {
		code = http.StatusMovedPermanently
		ctx.Header.Set("Cache-Control", ccImmutable)
	} else {
		ctx.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
	}
	ctx.Header.Set("Location", url)
	return rex.Status(code, nil)
}

func errorJS(ctx *rex.Context, message string) any {
	buf, recycle := NewBuffer()
	defer recycle()
	buf.WriteString("/* esm.sh - error */\n")
	buf.WriteString("throw new Error(")
	buf.Write(utils.MustEncodeJSON(message))
	buf.WriteString(");\n")
	buf.WriteString("export default null;\n")
	ctx.Header.Set("Content-Type", ctJavaScript)
	ctx.Header.Set("Cache-Control", ccImmutable)
	return buf.Bytes()
}
