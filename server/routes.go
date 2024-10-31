package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/server/storage"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/crypto/rs"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
	"golang.org/x/net/html"
)

const (
	ccMustRevalidate = "public, max-age=0, must-revalidate"
	cc10mins         = "public, max-age=600"
	cc1day           = "public, max-age=86400"
	ccImmutable      = "public, max-age=31536000, immutable"
	ctJavaScript     = "application/javascript; charset=utf-8"
	ctTypeScript     = "application/typescript; charset=utf-8"
	ctJSON           = "application/json; charset=utf-8"
	ctCSS            = "text/css; charset=utf-8"
	ctHtml           = "text/html; charset=utf-8"
)

type ESMPathKind uint8

const (
	// module entry
	ESMEntry ESMPathKind = iota
	// js/css build
	ESMBuild
	// source map
	ESMSourceMap
	// *.d.ts
	ESMDts
	// package raw file
	RawFile
)

type ESMPath struct {
	GhPrefix    bool
	PrPrefix    bool
	PkgName     string
	PkgVersion  string
	SubPath     string
	SubBareName string
}

func (path ESMPath) PackageName() string {
	s := path.PkgName
	if path.PkgVersion != "" && path.PkgVersion != "*" && path.PkgVersion != "latest" {
		s += "@" + path.PkgVersion
	}
	if path.GhPrefix {
		return "gh/" + s
	}
	if path.PrPrefix {
		return "pr/" + s
	}
	return s
}

func (path ESMPath) String() string {
	s := path.PackageName()
	if path.SubBareName != "" {
		s += "/" + path.SubBareName
	}
	return s
}

func routes(debug bool) rex.Handle {
	startTime := time.Now()
	globalETag := fmt.Sprintf(`W/"v%d"`, VERSION)

	return func(ctx *rex.Context) any {
		pathname := ctx.Pathname()

		// ban malicious requests
		if strings.HasPrefix(pathname, "/.") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// handle POST requests
		if ctx.R.Method == "POST" {
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
				savePath := fmt.Sprintf("modules/%s.mjs", hash)
				if file, err := buildStorage.Get(savePath); err == nil {
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					output := TransformOutput{
						Code: string(data),
					}
					file, err = buildStorage.Get(savePath + ".map")
					if err == nil {
						data, err = io.ReadAll(file)
						file.Close()
						if err == nil {
							output.Map = string(data)
						}
					}
					return output
				}

				var npmrc *NpmRC
				if rc := ctx.R.Header.Get("X-Npmrc"); rc != "" {
					rc, err := NewNpmRcFromJSON([]byte(rc))
					if err != nil {
						return rex.Status(400, "Invalid Npmrc Header")
					}
					npmrc = rc
				} else {
					npmrc = NewNpmRcFromConfig()
				}

				importMap := ImportMap{Imports: map[string]string{}}
				if len(options.ImportMap) > 0 {
					err = json.Unmarshal(options.ImportMap, &importMap)
					if err != nil {
						return rex.Err(400, "Invalid ImportMap")
					}
				}

				output, err := transform(npmrc, &ResolvedTransformOptions{
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
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
				return output

			case "/purge":
				zoneId := ctx.FormValue("zoneId")
				packageName := ctx.FormValue("package")
				version := ctx.FormValue("version")
				if packageName == "" {
					return rex.Err(400, "param `package` is required")
				}
				prefix := "/" + packageName + "@"
				if version != "" {
					prefix += version
				}
				if zoneId != "" {
					prefix = zoneId + prefix
				}
				deletedKeys, err := db.DeleteAll(prefix)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				deletedPkgs := NewStringSet()
				for _, key := range deletedKeys {
					pathname := key
					if zoneId != "" {
						pathname = pathname[len(zoneId):]
					}
					fromGithub := strings.HasPrefix(pathname, "/gh/")
					if fromGithub {
						pathname = pathname[3:]
					}
					pkgName, version, _, _ := splitPkgPath(pathname)
					pkgId := pkgName + "@" + version
					if fromGithub {
						pkgId = "gh/" + pkgId
					}
					deletedPkgs.Add(pkgId)
				}
				deletedFiles := []string{}
				for _, pkgId := range deletedPkgs.Values() {
					buildPrefix := fmt.Sprintf("builds/%s", pkgId)
					buildFiles, err := buildStorage.List(buildPrefix)
					if err == nil && len(buildFiles) > 0 {
						err = buildStorage.RemoveAll(buildPrefix)
						if err != nil {
							return rex.Err(500, "FS error")
						}
						for i, filepath := range buildFiles {
							buildFiles[i] = fmt.Sprintf("%s/%s", pkgId, filepath)
						}
						deletedFiles = append(deletedFiles, buildFiles...)
					}
					dtsPrefix := fmt.Sprintf("types/%s", pkgId)
					dtsFiles, err := buildStorage.List(dtsPrefix)
					if err == nil && len(dtsFiles) > 0 {
						err = buildStorage.RemoveAll(dtsPrefix)
						if err != nil {
							return rex.Err(500, "FS error")
						}
						for i, filepath := range dtsFiles {
							dtsFiles[i] = fmt.Sprintf("%s/%s", pkgId, filepath)
						}
						deletedFiles = append(deletedFiles, dtsFiles...)
					}
					log.Info("purged", pkgId)
				}
				ret := map[string]any{
					"deletedPkgs":  deletedPkgs.Values(),
					"deletedFiles": deletedFiles,
				}
				if zoneId != "" {
					ret["zoneId"] = zoneId
				}
				return ret
			default:
				return rex.Err(404, "not found")
			}
		}

		// strip trailing slash
		if pathname != "/" && strings.HasSuffix(pathname, "/") {
			pathname = strings.TrimRight(pathname, "/")
		}

		cdnOrigin := ctx.R.Header.Get("X-Real-Origin")
		// use current host as cdn origin if not set
		if cdnOrigin == "" {
			proto := "http"
			if ctx.R.TLS != nil {
				proto = "https"
			}
			cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
		}

		// static routes
		switch pathname {
		case "/":
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == globalETag {
				return rex.Status(http.StatusNotModified, nil)
			}
			indexHTML, err := embedFS.ReadFile("server/embed/index.html")
			if err != nil {
				return err
			}
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte("/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/main/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(cdnOrigin))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			ctx.SetHeader("Cache-Control", ccMustRevalidate)
			ctx.SetHeader("Etag", globalETag)
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			q := make([]map[string]any, buildQueue.queue.Len())
			i := 0

			buildQueue.lock.RLock()
			for el := buildQueue.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*BuildTask)
				clientIps := make([]string, len(t.clients))
				for idx, c := range t.clients {
					clientIps[idx] = c.IP
				}
				if ok {
					m := map[string]any{
						"clients":   clientIps,
						"createdAt": t.createdAt.Format(http.TimeFormat),
						"inProcess": t.inProcess,
						"path":      t.Pathname(),
						"stage":     t.stage,
					}
					if !t.startedAt.IsZero() {
						m["startedAt"] = t.startedAt.Format(http.TimeFormat)
					}
					q[i] = m
					i++
				}
			}
			buildQueue.lock.RUnlock()

			disk := "ok"
			tmpFilepath := path.Join(os.TempDir(), rs.Hex.String(32))
			err := os.WriteFile(tmpFilepath, make([]byte, MB), 0644)
			if err != nil {
				if errors.Is(err, syscall.ENOSPC) {
					disk = "full"
				} else {
					disk = "error"
				}
			}
			os.Remove(tmpFilepath)

			ctx.SetHeader("Cache-Control", ccMustRevalidate)
			return map[string]any{
				"buildQueue": q[:i],
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
				"disk":       disk,
			}

		case "/error.js":
			switch query := ctx.Query(); query.Get("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Could not resolve "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-builtin-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported Node builtin module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-native-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported node native module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported NPM package "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported file dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			default:
				return throwErrorJS(ctx, "Unknown error", true)
			}

		case "/favicon.ico":
			favicon, err := embedFS.ReadFile("server/embed/assets/favicon.ico")
			if err != nil {
				return err
			}
			ctx.SetHeader("Cache-Control", ccImmutable)
			return rex.Content("favicon.ico", startTime, bytes.NewReader(favicon))
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
		}

		// serve internal scripts
		if pathname == "/run" || pathname == "/tsx" || pathname == "/uno" {
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch == globalETag && !debug {
				return rex.Status(http.StatusNotModified, nil)
			}

			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(ctx.Query().Get("target"))
			targetFromUA := targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(ctx.UserAgent())
			}

			js, err := buildEmbedTS(pathname[1:]+".ts", target, debug)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}

			if debug {
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
			} else {
				ctx.SetHeader("Cache-Control", cc1day)
				ctx.SetHeader("Etag", globalETag)
			}
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			ctx.SetHeader("Content-Type", ctJavaScript)
			return js
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			modTime := startTime
			if fs, ok := embedFS.(*MockEmbedFS); ok {
				if fi, err := fs.Lstat("server" + pathname); err == nil {
					modTime = fi.ModTime()
				}
			}
			data, err := embedFS.ReadFile("server" + pathname)
			if err != nil {
				return rex.Status(404, "not found")
			}
			ctx.SetHeader("Cache-Control", cc1day)
			return rex.Content(pathname, modTime, bytes.NewReader(data))
		}

		// serve modules are generated by the /transform API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByFirstByte(pathname[2:], '.')
			if len(hash) != 40 {
				return rex.Status(404, "Not Found")
			}
			savePath := fmt.Sprintf("modules/%s.%s", hash, ext)
			fi, err := buildStorage.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Not Found")
				}
				return rex.Status(500, err.Error())
			}
			f, err := buildStorage.Get(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if strings.HasSuffix(pathname, ".map") {
				ctx.SetHeader("Content-Type", ctJSON)
			} else {
				ctx.SetHeader("Content-Type", ctJavaScript)
			}
			ctx.SetHeader("Cache-Control", ccImmutable)
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		// serve node libs
		if strings.HasPrefix(pathname, "/node/") && strings.HasSuffix(pathname, ".js") {
			lib, ok := nodeLibs[pathname[1:]]
			if !ok {
				// empty module
				lib = "export default {}"
			}
			if strings.HasPrefix(pathname, "/node/chunk-") {
				ctx.SetHeader("Cache-Control", ccImmutable)
			} else {
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch == globalETag && !debug {
					return rex.Status(http.StatusNotModified, nil)
				}
				if debug {
					ctx.SetHeader("Cache-Control", ccMustRevalidate)
				} else {
					ctx.SetHeader("Cache-Control", cc1day)
					ctx.SetHeader("Etag", globalETag)
				}
			}
			target := getBuildTargetByUA(ctx.UserAgent())
			code, err := minify(lib, targets[target], esbuild.LoaderJS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}
			ctx.SetHeader("Content-Type", ctJavaScript)
			appendVaryHeader(ctx.W.Header(), "User-Agent")
			return rex.Content(pathname, startTime, bytes.NewReader(code))
		}

		var npmrc *NpmRC
		if rc := ctx.R.Header.Get("X-Npmrc"); rc != "" {
			rc, err := NewNpmRcFromJSON([]byte(rc))
			if err != nil {
				return rex.Status(400, "Invalid Npmrc Header")
			}
			npmrc = rc
		} else {
			npmrc = NewNpmRcFromConfig()
		}

		zoneId := ctx.R.Header.Get("X-Zone-Id")
		if zoneId != "" {
			if !valid.IsDomain(zoneId) {
				zoneId = ""
			} else {
				var scopeName string
				if pkgName := getPkgName(pathname[1:]); strings.HasPrefix(pkgName, "@") {
					scopeName = pkgName[:strings.Index(pkgName, "/")]
				}
				if scopeName != "" {
					reg, ok := npmrc.Registries[scopeName]
					if !ok || (reg.Registry == jsrRegistry && reg.Token == "" && (reg.User == "" || reg.Password == "")) {
						zoneId = ""
					}
				} else if npmrc.Registry == npmRegistry && npmrc.Token == "" && (npmrc.User == "" || npmrc.Password == "") {
					zoneId = ""
				}
			}
		}
		if zoneId != "" {
			npmrc.zoneId = zoneId
			cdnOrigin = fmt.Sprintf("https://%s", zoneId)
		}

		if pathname == "/uno.css" {
			query := ctx.Query()
			ctxUrlRaw, err := atobUrl(query.Get("ctx"))
			if err != nil {
				return rex.Status(400, "Invalid context url")
			}
			ctxUrl, err := url.Parse(ctxUrlRaw)
			if err != nil {
				return rex.Status(400, "Invalid context url")
			}
			hostname := ctxUrl.Hostname()
			if isLocalhost(hostname) {
				ctx.SetHeader("Cache-Control", ccImmutable)
				ctx.SetHeader("Content-Type", ctCSS)
				return "body:after{position:fixed;top:0;left:0;z-index:9999;padding:18px 32px;width:100vw;content:'esm.sh/uno doesn't support local development, try serving your app with `esm.sh run`.';font-size:14px;background:rgba(255,232,232,.9);color:#f00;backdrop-filter:blur(8px)}"
			}
			if !regexpDomain.MatchString(hostname) {
				return rex.Status(400, "Invalid context url")
			}
			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(query.Get("target"))
			targetFromUA := targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(ctx.UserAgent())
			}
			h := sha1.New()
			h.Write([]byte(ctxUrlRaw))
			h.Write([]byte(query.Get("v")))
			h.Write([]byte(target))
			savePath := normalizeSavePath(zoneId, path.Join("modules", hex.EncodeToString(h.Sum(nil))+".css"))
			_, err = buildStorage.Stat(savePath)
			if err != nil && err != storage.ErrNotFound {
				return rex.Status(500, err.Error())
			}
			var resp any
			if err == nil {
				f, err := buildStorage.Get(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				resp = f // auto closed
			} else {
				fetcher := newFetcher(ctx.UserAgent(), 15*time.Second)
				res, err := fetcher.Fetch(ctxUrl)
				if err != nil {
					return rex.Status(500, "Failed to fetch page html")
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					if res.StatusCode == 404 {
						return rex.Status(404, "Page html not found")
					}
					return rex.Status(500, "Failed to fetch page html")
				}
				tokenizer := html.NewTokenizer(io.LimitReader(res.Body, 2*MB))
				configCSS := ""
				input := []string{}
				jsEntries := map[string]struct{}{}
				importMap := ImportMap{}
				for {
					tt := tokenizer.Next()
					if tt == html.ErrorToken {
						break
					}
					if tt == html.StartTagToken {
						name, moreAttr := tokenizer.TagName()
						switch string(name) {
						case "style":
							for moreAttr {
								var key, val []byte
								key, val, moreAttr = tokenizer.TagAttr()
								if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("uno/css")) {
									tokenizer.Next()
									innerText := bytes.TrimSpace(tokenizer.Text())
									if len(innerText) > 0 {
										configCSS += string(innerText)
									}
									break
								}
							}
						case "script":
							srcAttr := ""
							mainAttr := ""
							typeAttr := ""
							for moreAttr {
								var key, val []byte
								key, val, moreAttr = tokenizer.TagAttr()
								if bytes.Equal(key, []byte("src")) {
									srcAttr = string(val)
								} else if bytes.Equal(key, []byte("main")) {
									mainAttr = string(val)
								} else if bytes.Equal(key, []byte("type")) {
									typeAttr = string(val)
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
								input = append(input, string(tokenizer.Text()))
							} else {
								if mainAttr != "" && isHttpSepcifier(srcAttr) {
									if !isHttpSepcifier(mainAttr) && endsWith(mainAttr, esExts...) {
										jsEntries[mainAttr] = struct{}{}
									}
								} else if !isHttpSepcifier(srcAttr) && endsWith(srcAttr, esExts...) {
									jsEntries[srcAttr] = struct{}{}
								}
							}
						case "link", "meta", "title", "base", "head", "noscript", "slot", "template", "option":
							// ignore
						default:
							input = append(input, string(tokenizer.Raw()))
						}
					}
				}
				if configCSS == "" {
					res, err := fetcher.Fetch(ctxUrl.ResolveReference(&url.URL{Path: "./uno.css"}))
					if err != nil {
						return rex.Status(500, "Failed to lookup config css")
					}
					if res.StatusCode == 404 {
						res.Body.Close()
						res, err = fetcher.Fetch(ctxUrl.ResolveReference(&url.URL{Path: "/uno.css"}))
						if err != nil {
							return rex.Status(500, "Failed to lookup config css")
						}
					}
					defer res.Body.Close()
					// ignore non-exist config css
					if res.StatusCode != 404 {
						if res.StatusCode != 200 {
							return rex.Status(500, "Failed to fetch config css")
						}
						css, err := io.ReadAll(res.Body)
						if err != nil {
							return rex.Status(500, "Failed to fetch config css")
						}
						configCSS = string(css)
					}
				}
				for src := range jsEntries {
					url := ctxUrl.ResolveReference(&url.URL{Path: src})
					_, _, tree, err := bundleRemoteModule(npmrc, url.String(), importMap, fetcher)
					if err == nil {
						for _, code := range tree {
							input = append(input, string(code))
						}
					}
				}
				out, err := transform(npmrc, &ResolvedTransformOptions{
					TransformOptions: TransformOptions{
						Lang:   "css",
						Target: target,
						Minify: true,
					},
					unocss: UnoCSSGenerateOptions{
						generate:  true,
						configCSS: configCSS,
						content:   input,
					},
				})
				if err != nil {
					return rex.Status(500, "Failed to generate uno.css")
				}
				go buildStorage.Put(savePath, strings.NewReader(out.Code))
				resp = out.Code
			}
			ctx.SetHeader("Cache-Control", ccImmutable)
			ctx.SetHeader("Content-Type", ctCSS)
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			return resp
		}

		if strings.HasPrefix(pathname, "/http://") || strings.HasPrefix(pathname, "/https://") {
			query := ctx.Query()
			urlRaw := pathname[1:]
			u, err := url.Parse(urlRaw)
			if err != nil {
				return rex.Status(400, "Invalid URL")
			}
			hostname := u.Hostname()
			extname := path.Ext(u.Path)
			isCss := extname == ".css"
			if (u.Scheme != "http" && u.Scheme != "https") || isLocalhost(hostname) || !regexpDomain.MatchString(hostname) {
				return rex.Status(400, "Invalid URL")
			}
			if !(isCss || includes(esExts, extname) || extname == ".vue" || extname == ".svelte") {
				return rex.Redirect(urlRaw, http.StatusMovedPermanently)
			}
			im := query.Get("im")
			v := query.Get("v")
			if v == "" {
				v = "0"
			} else if !regexpVersion.MatchString(v) || len(v) > 32 {
				return rex.Status(400, "Invalid Version Param")
			}
			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(query.Get("target"))
			targetFromUA := targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(ctx.UserAgent())
			}
			h := sha1.New()
			h.Write([]byte(urlRaw))
			h.Write([]byte(im))
			h.Write([]byte(v))
			h.Write([]byte(target))
			savePath := normalizeSavePath(zoneId, path.Join("modules", hex.EncodeToString(h.Sum(nil))+".mjs"))
			_, err = buildStorage.Stat(savePath)
			if err != nil && err != storage.ErrNotFound {
				return rex.Status(500, err.Error())
			}
			var resp any
			if err == nil {
				f, err := buildStorage.Get(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				resp = f // auto closed
			} else {
				fetcher := newFetcher(ctx.UserAgent(), 15*time.Second)
				importMap := ImportMap{}
				if len(im) > 1 {
					imPath, err := atobUrl(im[1:])
					if err != nil {
						return rex.Status(400, "Invalid `im` Param")
					}
					imUrl, err := url.Parse(u.Scheme + "://" + u.Host + imPath)
					if err != nil {
						return rex.Status(400, "Invalid `im` Param")
					}
					res, err := fetcher.Fetch(imUrl)
					if err != nil {
						return rex.Status(500, "Failed to fetch import map")
					}
					defer res.Body.Close()
					if res.StatusCode != 200 {
						return rex.Status(500, "Failed to fetch import map")
					}
					tokenizer := html.NewTokenizer(io.LimitReader(res.Body, 2*MB))
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
									importMap.Src, _ = atobUrl(im[1:])
									importMap.Support = im[0] == 'y'
								}
								break
							}
						}
					}
				}
				js, css, _, err := bundleRemoteModule(npmrc, urlRaw, importMap, fetcher)
				if err != nil {
					return rex.Status(500, "Failed to build module:"+err.Error())
				}
				code := string(js)
				if len(css) > 0 {
					code += fmt.Sprintf("\nvar style=document.createElement('style');style.textContent=%s;document.head.appendChild(style);", utils.MustEncodeJSON(css))
				}
				out, err := transform(npmrc, &ResolvedTransformOptions{
					TransformOptions: TransformOptions{
						Filename: u.Path,
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
				go buildStorage.Put(savePath, strings.NewReader(out.Code))
				resp = out.Code
			}
			if isCss && query.Has("module") {
				resp = fmt.Sprintf("var style = document.createElement('style');\nstyle.textContent = %s;\ndocument.head.appendChild(style);\nexport default null;", utils.MustEncodeJSON(resp))
			}
			ctx.SetHeader("Cache-Control", ccImmutable)
			if isCss && !query.Has("module") {
				ctx.SetHeader("Content-Type", ctCSS)
			} else {
				ctx.SetHeader("Content-Type", ctJavaScript)
			}
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			return resp
		}

		// check `/*pathname` pattern
		asteriskPrefix := ""
		if strings.HasPrefix(pathname, "/*") {
			asteriskPrefix = "*"
			pathname = "/" + pathname[2:]
		} else if strings.HasPrefix(pathname, "/gh/*") {
			asteriskPrefix = "*"
			pathname = "/gh/" + pathname[5:]
		} else if strings.HasPrefix(pathname, "/pr/*") {
			asteriskPrefix = "*"
			pathname = "/pr/" + pathname[5:]
		}

		esm, extraQuery, isFixedVersion, isModuleFullpath, err := praseESMPath(npmrc, pathname)
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

		ghPrefix := ""
		if esm.GhPrefix {
			ghPrefix = "/gh"
		}

		// redirect `/@types/PKG` to it's main dts file
		if strings.HasPrefix(esm.PkgName, "@types/") && esm.SubBareName == "" {
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
			return rex.Redirect(fmt.Sprintf("%s/%s@%s%s", cdnOrigin, info.Name, info.Version, utils.CleanPath(types)), http.StatusFound)
		}

		// redirect to main css path for CSS packages
		if css := cssPackages[esm.PkgName]; css != "" && esm.SubBareName == "" {
			url := fmt.Sprintf("%s/%s/%s", cdnOrigin, esm.String(), css)
			return rex.Redirect(url, http.StatusFound)
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
				esm.SubBareName = esm.SubPath
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
			esm.SubPath = utils.CleanPath(v)[1:]
			esm.SubBareName = toModuleBareName(esm.SubPath, true)
		}

		// check the response type
		resType := ESMEntry
		if esm.SubPath != "" {
			ext := path.Ext(esm.SubPath)
			switch ext {
			case ".js", ".mjs":
				if isModuleFullpath {
					resType = ESMBuild
				}
			case ".ts", ".mts":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					resType = ESMDts
				}
			case ".css":
				if isModuleFullpath {
					resType = ESMBuild
				} else {
					resType = RawFile
				}
			case ".map":
				if isModuleFullpath {
					resType = ESMSourceMap
				} else {
					resType = RawFile
				}
			default:
				if ext != "" && assetExts[ext[1:]] {
					resType = RawFile
				}
			}
		}
		if query.Has("raw") {
			resType = RawFile
		}

		// redirect to the url with fixed package version
		if !isFixedVersion {
			if isModuleFullpath {
				subPath := ""
				query := ""
				if esm.SubPath != "" {
					subPath = "/" + esm.SubPath
				}
				if rawQuery != "" {
					query = "?" + rawQuery
				}
				ctx.SetHeader("Cache-Control", cc10mins)
				return rex.Redirect(fmt.Sprintf("%s/%s%s%s", cdnOrigin, esm.PackageName(), subPath, query), http.StatusFound)
			}
			if resType != ESMEntry {
				pkgName := esm.PkgName
				pkgVersion := esm.PkgVersion
				subPath := ""
				qs := ""
				if strings.HasPrefix(pkgName, "@jsr/") {
					pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
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
				ctx.SetHeader("Cache-Control", cc10mins)
				return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, asteriskPrefix, pkgName, pkgVersion, subPath, qs), http.StatusFound)
			}
		} else {
			// serve `*.wasm` as an es6 module when `?module` query is set (requires `top-level-await` support)
			if resType == RawFile && strings.HasSuffix(esm.SubPath, ".wasm") && query.Has("module") {
				buf := &bytes.Buffer{}
				wasmUrl := cdnOrigin + pathname
				fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
				fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Cache-Control", ccImmutable)
				return buf
			}

			// fix url that is related to `import.meta.url`
			if resType == RawFile && isModuleFullpath && !query.Has("raw") {
				extname := path.Ext(esm.SubPath)
				dir := path.Join(npmrc.StoreDir(), esm.PackageName())
				if !existsDir(dir) {
					_, err := npmrc.installPackage(esm)
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
					sort.Sort(sort.Reverse(SortablePaths(files)))
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
				return rex.Redirect(url, http.StatusMovedPermanently)
			}

			// serve package raw files
			if resType == RawFile {
				savePath := path.Join(npmrc.StoreDir(), esm.PackageName(), "node_modules", esm.PkgName, esm.SubPath)
				fi, err := os.Lstat(savePath)
				if err != nil {
					if os.IsExist(err) {
						return rex.Status(500, err.Error())
					}
					// if the file not found, try to install the package
					_, err = npmrc.installPackage(esm)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					fi, err = os.Lstat(savePath)
					if err != nil {
						if os.IsExist(err) {
							return rex.Status(500, err.Error())
						}
						return rex.Status(404, "File Not Found")
					}
				}
				// limit the file size up to 50MB
				if fi.Size() > assetMaxSize {
					return rex.Status(403, "File Too Large")
				}
				f, err := os.Open(savePath)
				if err != nil {
					if os.IsExist(err) {
						return rex.Status(500, err.Error())
					}
					return rex.Status(404, "File Not Found")
				}
				ctx.SetHeader("Cache-Control", ccImmutable)
				if strings.HasSuffix(savePath, ".json") && query.Has("module") {
					defer f.Close()
					data, err := io.ReadAll(f)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ctx.SetHeader("Content-Type", ctJavaScript)
					return concatBytes([]byte("export default "), data)
				}
				if endsWith(savePath, ".js", ".mjs", ".jsx") {
					ctx.SetHeader("Content-Type", ctJavaScript)
				} else if endsWith(savePath, ".ts", ".mts", ".tsx") {
					ctx.SetHeader("Content-Type", ctTypeScript)
				}
				return rex.Content(savePath, fi.ModTime(), f) // auto closed
			}

			// serve build/dts files
			if resType == ESMBuild || resType == ESMSourceMap || resType == ESMDts {
				var savePath string
				if resType == ESMDts {
					savePath = path.Join("types", pathname)
				} else {
					savePath = path.Join("builds", pathname)
				}
				savePath = normalizeSavePath(zoneId, savePath)
				fi, err := buildStorage.Stat(savePath)
				if err != nil {
					if err == storage.ErrNotFound && resType == ESMSourceMap {
						return rex.Status(404, "Not found")
					}
					if err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					}
				}
				if err == nil {
					if query.Has("worker") && resType == ESMBuild {
						moduleUrl := cdnOrigin + pathname
						ctx.SetHeader("Content-Type", ctJavaScript)
						ctx.SetHeader("Cache-Control", ccImmutable)
						return fmt.Sprintf(
							`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
							moduleUrl,
							moduleUrl,
						)
					}
					r, err := buildStorage.Get(savePath)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					if resType == ESMDts {
						ctx.SetHeader("Content-Type", ctTypeScript)
					} else if resType == ESMSourceMap {
						ctx.SetHeader("Content-Type", ctJSON)
					} else if strings.HasSuffix(pathname, ".css") {
						ctx.SetHeader("Content-Type", ctCSS)
					} else {
						ctx.SetHeader("Content-Type", ctJavaScript)
					}
					ctx.SetHeader("Cache-Control", ccImmutable)
					if resType == ESMDts {
						buffer, err := io.ReadAll(r)
						r.Close()
						if err != nil {
							return rex.Status(500, err.Error())
						}
						return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))
					}
					return rex.Content(savePath, fi.ModTime(), r) // auto closed
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
			if rawQuery == "target="+target {
				rawQuery = ""
			} else if p := "&target=" + target; strings.Contains(rawQuery, p) {
				rawQuery = strings.ReplaceAll(rawQuery, p, "")
			} else if p := "target=" + target + "&"; strings.Contains(rawQuery, p) {
				rawQuery = strings.ReplaceAll(rawQuery, p, "")
			}
			if rawQuery != "" {
				qs = "?" + rawQuery
			}
			ctx.SetHeader("Cache-Control", cc10mins)
			if targetFromUA {
				appendVaryHeader(ctx.W.Header(), "User-Agent")
			}
			return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, asteriskPrefix, pkgName, pkgVersion, subPath, qs), http.StatusFound)
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
					m, _, _, _, err := praseESMPath(npmrc, v)
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

		// check `?exports` query
		exports := NewStringSet()
		if query.Has("exports") {
			value := query.Get("exports")
			for _, p := range strings.Split(value, ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					exports.Add(p)
				}
			}
		}

		// check `?conditions` query
		var conditions []string
		conditionsSet := NewStringSet()
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
		external := NewStringSet()
		externalAll := asteriskPrefix == "*"
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

		buildArgs := BuildArgs{
			alias:       alias,
			conditions:  conditions,
			deps:        deps,
			exports:     exports,
			externalAll: externalAll,
			external:    external,
		}

		// match path `PKG@VERSION/X-${args}/esnext/SUBPATH`
		argsX := false
		if resType == ESMBuild || resType == ESMDts {
			a := strings.Split(esm.SubBareName, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				args, err := decodeBuildArgs(npmrc, strings.TrimPrefix(a[0], "X-"))
				if err != nil {
					return throwErrorJS(ctx, "Invalid build args: "+a[0], false)
				}
				esm.SubPath = strings.Join(strings.Split(esm.SubPath, "/")[1:], "/")
				esm.SubBareName = toModuleBareName(esm.SubPath, true)
				buildArgs = args
				argsX = true
			}
		}

		// fix the build args that are from the query
		if !argsX {
			err := normalizeBuildArgs(npmrc, path.Join(npmrc.StoreDir(), esm.PackageName()), &buildArgs, esm)
			if err != nil {
				return throwErrorJS(ctx, err.Error(), false)
			}
		}

		// build and return `.d.ts`
		if resType == ESMDts {
			findDts := func() (savePath string, stat storage.Stat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, true); a != "" {
					args = "X-" + a
				}
				savePath = normalizeSavePath(zoneId, path.Join(fmt.Sprintf(
					"types/%s/%s",
					esm.PackageName(),
					args,
				), esm.SubPath))
				stat, err = buildStorage.Stat(savePath)
				return savePath, stat, err
			}
			_, _, err := findDts()
			if err == storage.ErrNotFound {
				buildCtx := NewBuildContext(zoneId, npmrc, esm, buildArgs, "types", BundleDefault, false, false)
				c := buildQueue.Add(buildCtx, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						if output.err.Error() == "types not found" {
							return rex.Status(404, "Types Not Found")
						}
						return rex.Status(500, "types: "+output.err.Error())
					}
				case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
					ctx.SetHeader("Cache-Control", ccMustRevalidate)
					return rex.Status(http.StatusRequestTimeout, "timeout, we are transforming the types hardly, please try again later!")
				}
			}
			savePath, _, err := findDts()
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Types Not Found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := buildStorage.Get(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			buffer, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Content-Type", ctTypeScript)
			ctx.SetHeader("Cache-Control", ccImmutable)
			return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))

		}

		if !argsX {
			// check `?jsx-rutnime` query
			var jsxRuntime *ESMPath = nil
			if v := query.Get("jsx-runtime"); v != "" {
				m, _, _, _, err := praseESMPath(npmrc, v)
				if err != nil {
					return rex.Status(400, fmt.Sprintf("Invalid jsx-runtime query: %v not found", v))
				}
				jsxRuntime = &m
			}

			externalRequire := query.Has("external-require")
			// workaround: force "unocss/preset-icons" to external `require` calls
			if !externalRequire && esm.PkgName == "@unocss/preset-icons" {
				externalRequire = true
			}

			buildArgs.externalRequire = externalRequire
			buildArgs.jsxRuntime = jsxRuntime
			buildArgs.keepNames = query.Has("keep-names")
			buildArgs.ignoreAnnotations = query.Has("ignore-annotations")
		}

		bundleMode := BundleDefault
		if (query.Has("bundle") && query.Get("bundle") != "false") || query.Has("bundle-all") || query.Has("bundle-deps") || query.Has("standalone") {
			bundleMode = BundleAll
		} else if query.Get("bundle") == "false" || query.Has("no-bundle") {
			bundleMode = BundleFalse
		}

		isDev := query.Has("dev")
		isPkgCss := query.Has("css")
		isWorker := query.Has("worker")
		noDts := query.Has("no-dts") || query.Has("no-check")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((esm.PkgName == "react" && esm.SubBareName == "jsx-dev-runtime") || esm.PkgName == "react-refresh") {
			isDev = true
		}

		if resType == ESMBuild {
			a := strings.Split(esm.SubBareName, "/")
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
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(esm.SubPath, ".js") {
						if submodule == basename+".css" {
							esm.SubBareName = ""
							target = maybeTarget
						} else {
							url := fmt.Sprintf("%s/%s", cdnOrigin, esm.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						isMjs := strings.HasSuffix(esm.SubPath, ".mjs")
						if isMjs && submodule == basename {
							submodule = ""
						}
						esm.SubBareName = submodule
						target = maybeTarget
					}
				}
			}
		}

		buildCtx := NewBuildContext(zoneId, npmrc, esm, buildArgs, target, bundleMode, isDev, !config.DisableSourceMap)
		ret, hasBuild := buildCtx.Query()
		if !hasBuild {
			c := buildQueue.Add(buildCtx, ctx.RemoteIP())
			select {
			case output := <-c.C:
				if output.err != nil {
					msg := output.err.Error()
					if strings.Contains(msg, "no such file or directory") ||
						strings.Contains(msg, "is not exported from package") {
						// redirect old build path (.js) to new build path (.mjs)
						if strings.HasSuffix(esm.SubPath, "/"+esm.PkgName+".js") {
							url := strings.TrimSuffix(ctx.R.URL.String(), ".js") + ".mjs"
							return rex.Redirect(url, http.StatusFound)
						}
						ctx.SetHeader("Cache-Control", ccImmutable)
						return rex.Status(404, "Module not found")
					}
					if strings.HasSuffix(msg, " not found") {
						return rex.Status(404, msg)
					}
					return throwErrorJS(ctx, output.err.Error(), false)
				}
				ret = output.result
			case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
				ctx.SetHeader("Cache-Control", ccMustRevalidate)
				return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
			}
		}

		// redirect to `*.d.ts` file
		if ret.TypesOnly {
			dtsUrl := cdnOrigin + ret.Dts
			ctx.SetHeader("X-TypeScript-Types", dtsUrl)
			ctx.SetHeader("Content-Type", ctJavaScript)
			ctx.SetHeader("Cache-Control", ccImmutable)
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if isPkgCss && esm.SubBareName == "" {
			if !ret.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s.css", cdnOrigin, strings.TrimSuffix(buildCtx.Pathname(), ".mjs"))
			return rex.Redirect(url, 301)
		}

		// if the response type is `ResBuild`, return the build js/css content
		if resType == ESMBuild {
			savePath := buildCtx.getSavepath()
			if strings.HasSuffix(esm.SubPath, ".css") {
				path, _ := utils.SplitByLastByte(savePath, '.')
				savePath = path + ".css"
			}
			fi, err := buildStorage.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "File not found")
				}
				return rex.Status(500, err.Error())
			}
			f, err := buildStorage.Get(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Cache-Control", ccImmutable)
			if endsWith(savePath, ".css") {
				ctx.SetHeader("Content-Type", ctCSS)
			} else if endsWith(savePath, ".mjs", ".js") {
				ctx.SetHeader("Content-Type", ctJavaScript)
				if isWorker {
					f.Close()
					moduleUrl := cdnOrigin + buildCtx.Pathname()
					return fmt.Sprintf(
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					)
				}
			}
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, esm, EOL)

		if isWorker {
			moduleUrl := cdnOrigin + buildCtx.Pathname()
			fmt.Fprintf(buf,
				`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
				moduleUrl,
				moduleUrl,
			)
		} else {
			if len(ret.Deps) > 0 {
				for _, dep := range ret.Deps {
					fmt.Fprintf(buf, `import "%s";%s`, dep, EOL)
				}
			}
			ctx.SetHeader("X-ESM-Path", buildCtx.Pathname())
			fmt.Fprintf(buf, `export * from "%s";%s`, buildCtx.Pathname(), EOL)
			if (ret.FromCJS || ret.HasDefaultExport) && (exports.Len() == 0 || exports.Has("default")) {
				fmt.Fprintf(buf, `export { default } from "%s";%s`, buildCtx.Pathname(), EOL)
			}
			if ret.FromCJS && exports.Len() > 0 {
				fmt.Fprintf(buf, `import __cjs_exports$ from "%s";%s`, buildCtx.Pathname(), EOL)
				fmt.Fprintf(buf, `export const { %s } = __cjs_exports$;%s`, strings.Join(exports.Values(), ", "), EOL)
			}
		}

		if ret.Dts != "" && !noDts && !isWorker {
			dtsUrl := cdnOrigin + ret.Dts
			ctx.SetHeader("X-TypeScript-Types", dtsUrl)
		}
		if targetFromUA {
			appendVaryHeader(ctx.W.Header(), "User-Agent")
		}
		if isFixedVersion {
			ctx.SetHeader("Cache-Control", ccImmutable)
		} else {
			ctx.SetHeader("Cache-Control", cc10mins)
		}
		ctx.SetHeader("Content-Type", ctJavaScript)
		if ctx.R.Method == http.MethodHead {
			return []byte{}
		}
		return buf.Bytes()
	}
}

func auth(secret string) rex.Handle {
	return func(ctx *rex.Context) any {
		if secret != "" && ctx.R.Header.Get("Authorization") != "Bearer "+secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
	}
}

func praseESMPath(npmrc *NpmRC, pathname string) (esm ESMPath, extraQuery string, isFixedVersion bool, hasTargetSegment bool, err error) {
	// see https://pkg.pr.new
	if strings.HasPrefix(pathname, "/pr/") {
		pkgName, rest := utils.SplitByFirstByte(pathname[4:], '@')
		if rest == "" {
			err = errors.New("invalid path")
			return
		}
		version, subPath := utils.SplitByFirstByte(rest, '/')
		if !valid.IsHexString(version) || len(version) < 7 {
			err = errors.New("invalid path")
			return
		}
		esm = ESMPath{
			PkgName:     pkgName,
			PkgVersion:  version,
			SubPath:     subPath,
			SubBareName: toModuleBareName(subPath, true),
			PrPrefix:    true,
		}
		isFixedVersion = true
		return
	}

	ghPrefix := strings.HasPrefix(pathname, "/gh/")
	if ghPrefix {
		if len(pathname) == 4 {
			err = errors.New("invalid path")
			return
		}
		// add a leading `@` to the package name
		pathname = "/@" + pathname[4:]
	} else if strings.HasPrefix(pathname, "/jsr/") {
		segs := strings.Split(pathname[5:], "/")
		if len(segs) < 2 || !strings.HasPrefix(segs[0], "@") {
			err = errors.New("invalid path")
			return
		}
		pathname = "/@jsr/" + segs[0][1:] + "__" + segs[1]
		if len(segs) > 2 {
			pathname += "/" + strings.Join(segs[2:], "/")
		}
	}

	pkgName, maybeVersion, subPath, hasTargetSegment := splitPkgPath(pathname)
	if !validatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	// strip the leading `@` added before
	if ghPrefix {
		pkgName = pkgName[1:]
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}

	esm = ESMPath{
		PkgName:     pkgName,
		PkgVersion:  version,
		SubPath:     subPath,
		SubBareName: toModuleBareName(subPath, true),
		GhPrefix:    ghPrefix,
	}

	// workaround for es5-ext "../#/.." path
	if esm.SubBareName != "" && esm.PkgName == "es5-ext" {
		esm.SubBareName = strings.ReplaceAll(esm.SubBareName, "/%23/", "/#/")
	}

	if ghPrefix {
		if (valid.IsHexString(esm.PkgVersion) && len(esm.PkgVersion) >= 7) || regexpVersionStrict.MatchString(strings.TrimPrefix(esm.PkgVersion, "v")) {
			isFixedVersion = true
			return
		}
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", esm.PkgName))
		if err != nil {
			return
		}
		if esm.PkgVersion == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					esm.PkgVersion = ref.Sha[:7]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+esm.PkgVersion || ref.Ref == "refs/heads/"+esm.PkgVersion {
					esm.PkgVersion = ref.Sha[:7]
					return
				}
			}
			// try to find the semver tag
			var c *semver.Constraints
			c, err = semver.NewConstraint(strings.TrimPrefix(esm.PkgVersion, "semver:"))
			if err == nil {
				vs := make([]*semver.Version, len(refs))
				i := 0
				for _, ref := range refs {
					if strings.HasPrefix(ref.Ref, "refs/tags/") {
						v, e := semver.NewVersion(strings.TrimPrefix(ref.Ref, "refs/tags/"))
						if e == nil && c.Check(v) {
							vs[i] = v
							i++
						}
					}
				}
				if i > 0 {
					vs = vs[:i]
					if i > 1 {
						sort.Sort(semver.Collection(vs))
					}
					esm.PkgVersion = vs[i-1].String()
					return
				}
			}
		}
		err = errors.New("tag or branch not found")
		return
	}

	isFixedVersion = regexpVersionStrict.MatchString(esm.PkgVersion)
	if !isFixedVersion {
		var p *PackageJSON
		p, err = npmrc.fetchPackageInfo(pkgName, esm.PkgVersion)
		if err == nil {
			esm.PkgVersion = p.Version
		}
	}
	return
}

func throwErrorJS(ctx *rex.Context, message string, static bool) any {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(buf, "throw new Error(%s);\n", strings.TrimSpace(string(utils.MustEncodeJSON(strings.TrimSpace("[esm.sh] "+message)))))
	fmt.Fprintf(buf, "export default null;\n")
	if static {
		ctx.SetHeader("Cache-Control", ccImmutable)
	} else {
		ctx.SetHeader("Cache-Control", ccMustRevalidate)
	}
	ctx.SetHeader("Content-Type", ctJavaScript)
	return rex.Status(500, buf)
}

func toModuleBareName(path string, stripIndexSuffier bool) string {
	if path != "" {
		subModule := path
		if strings.HasSuffix(subModule, ".mjs") {
			subModule = strings.TrimSuffix(subModule, ".mjs")
		} else if strings.HasSuffix(subModule, ".cjs") {
			subModule = strings.TrimSuffix(subModule, ".cjs")
		} else {
			subModule = strings.TrimSuffix(subModule, ".js")
		}
		if stripIndexSuffier {
			subModule = strings.TrimSuffix(subModule, "/index")
		}
		return subModule
	}
	return ""
}

func splitPkgPath(pathname string) (pkgName string, version string, subPath string, hasTargetSegment bool) {
	a := strings.Split(strings.TrimPrefix(pathname, "/"), "/")
	nameAndVersion := ""
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		nameAndVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
		hasTargetSegment = checkTargetSegment(a[2:])
	} else {
		nameAndVersion = a[0]
		subPath = strings.Join(a[1:], "/")
		hasTargetSegment = checkTargetSegment(a[1:])
	}
	if len(nameAndVersion) > 0 && nameAndVersion[0] == '@' {
		pkgName, version = utils.SplitByFirstByte(nameAndVersion[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, version = utils.SplitByFirstByte(nameAndVersion, '@')
	}
	if version != "" {
		version = strings.TrimSpace(version)
	}
	return
}

func checkTargetSegment(segments []string) bool {
	if len(segments) < 2 {
		return false
	}
	if strings.HasPrefix(segments[0], "X-") && len(segments) > 2 {
		_, ok := targets[segments[1]]
		return ok
	}
	_, ok := targets[segments[0]]
	return ok
}

func getPkgName(specifier string) string {
	name, _, _, _ := splitPkgPath(specifier)
	return name
}
