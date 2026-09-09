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
	"time"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/mime"
	"github.com/esm-dev/esm.sh/internal/storage"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
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

func esmRouter(esmStorage storage.Storage, logger *log.Logger) http.Handler {
	var (
		startTime  = time.Now()
		globalETag = fmt.Sprintf(`W/"%s"`, VERSION)
		buildQueue = NewBuildQueue(int(config.BuildConcurrency))
		npmrc      = DefaultNpmRC()
		metaDB     = NewBuildMetaDB(esmStorage)
	)

	// purge npm cache when disk is low or full
	go func() {
		// run an initial check before waiting for the first ticker event
		purgeNPMCacheWhenDiskIsLowOrFull(npmrc, logger)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			go purgeNPMCacheWhenDiskIsLowOrFull(npmrc, logger)
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathname := r.URL.Path
		header := w.Header()

		// ban malicious requests
		if strings.HasSuffix(pathname, ".env") || strings.HasSuffix(pathname, ".php") || strings.Contains(pathname, "/.") {
			header.Set("Cache-Control", ccImmutable)
			writeStatus(w, 404, "not found")
			return
		}

		// handle POST API requests
		switch r.Method {
		case "HEAD", "GET":
			// continue
		case "POST":
			switch pathname {
			case "/transform":
				var options TransformOptions
				err := json.NewDecoder(io.LimitReader(r.Body, 2*MB)).Decode(&options)
				r.Body.Close()
				if err != nil {
					writeJSONError(w, 400, "require valid json body")
					return
				}
				if options.Code == "" {
					writeJSONError(w, 400, "Code is required")
					return
				}
				if len(options.Code) > MB {
					writeJSONError(w, 429, "Code is too large")
					return
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
				h.Write(options.ImportMapRaw)
				h.Write([]byte(options.JSXImportSource))
				h.Write([]byte(options.SourceMap))
				fmt.Fprintf(h, "%v", options.Minify)
				hash := hex.EncodeToString(h.Sum(nil))
				savePath := normalizeSavePath(fmt.Sprintf("modules/transform/%s.mjs", hash))

				// if previous build exists, return it directly
				if file, _, err := esmStorage.Get(savePath); err == nil {
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						writeJSONError(w, 500, "failed to read code")
						return
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
					writeJSON(w, 200, output)
					return
				}

				var importMap *importmap.ImportMap
				if len(options.ImportMapRaw) > 0 {
					importMap, err = importmap.Parse(nil, options.ImportMapRaw)
					if err != nil {
						writeJSONError(w, 400, "Invalid ImportMap")
						return
					}
				}

				output, err := transform(&ResolvedTransformOptions{
					TransformOptions: options,
					importMap:        importMap,
				})
				if err != nil {
					writeJSONError(w, 400, err.Error())
					return
				}
				if len(output.Map) > 0 {
					output.Code = fmt.Sprintf("%s//# sourceMappingURL=+%s", output.Code, path.Base(savePath)+".map")
					err = esmStorage.Put(savePath+".map", strings.NewReader(output.Map))
					if err != nil {
						logger.Errorf("storage.put(%s): %v", savePath+".map", err)
						writeJSONError(w, 500, "failed to store source map")
						return
					}
				}
				err = esmStorage.Put(savePath, strings.NewReader(output.Code))
				if err != nil {
					logger.Errorf("storage.put(%s): %v", savePath, err)
					writeJSONError(w, 500, "failed to store transformed code")
					return
				}
				header.Set("Cache-Control", ccMustRevalidate)
				writeJSON(w, 200, output)
				return

			default:
				writeStatus(w, 404, "not found")
				return
			}
		default:
			writeStatus(w, 405, "Method Not Allowed")
			return
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
				writeStatus(w, 500, err.Error())
				return
			}
			header.Set("Content-Type", "image/x-icon")
			header.Set("Cache-Control", ccImmutable)
			writeBody(w, favicon)
			return

		case "/robots.txt":
			header.Set("Content-Type", "text/plain; charset=utf-8")
			writeBody(w, []byte("User-agent: *\nAllow: /\n"))
			return

		case "/":
			if strings.HasPrefix(r.UserAgent(), "Deno/") {
				header.Set("Content-Type", ctJavaScript)
				writeBody(w, []byte(`throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`))
				return
			}
			if r.Header.Get("If-None-Match") == globalETag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			cacheTtl := 31536000
			if DEBUG {
				cacheTtl = 0
			}
			indexHTML, err := withCache("index.html", time.Duration(cacheTtl)*time.Second, func() (indexHTML []byte, _ string, err error) {
				readme, err := os.ReadFile("README.md")
				if err != nil {
					fetchClient := fetch.NewClient(r.UserAgent(), 15, false)
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
				readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(getOrigin(r)))
				indexHTML, err = embedFS.ReadFile("embed/index.html")
				if err != nil {
					err = errors.New("failed to read index.html: " + err.Error())
					return
				}
				readmeStrLit, err := json.Marshal(string(readme))
				if err != nil {
					err = errors.New("failed to marshal README: " + err.Error())
					return
				}
				indexHTML = bytes.ReplaceAll(indexHTML, []byte("README"), readmeStrLit)
				return
			})
			if err != nil {
				writeStatus(w, 500, err.Error())
				return
			}
			header.Set("Content-Type", ctHTML)
			header.Set("Cache-Control", ccMustRevalidate)
			header.Set("Etag", globalETag)
			writeBody(w, indexHTML)
			return

		case "/status.json":
			diskStatus := "ok"
			switch checkDiskStatus() {
			case DiskStatusFull:
				diskStatus = "full"
			case DiskStatusLow:
				diskStatus = "low"
			case DiskStatusError:
				diskStatus = "error"
			}

			header.Set("Cache-Control", ccMustRevalidate)
			writeJSON(w, 200, map[string]any{
				"buildQueue": buildQueue.Snapshot(),
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
				"disk":       diskStatus,
			})
			return

		case "/error.js":
			switch query := r.URL.Query(); query.Get("type") {
			case "resolve":
				errorJS(w, fmt.Sprintf(
					`Could not resolve "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-node-builtin-module":
				errorJS(w, fmt.Sprintf(
					`Unsupported Node builtin module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-node-native-module":
				errorJS(w, fmt.Sprintf(
					`Unsupported node native module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-npm-package":
				errorJS(w, fmt.Sprintf(
					`Unsupported NPM package "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-file-dependency":
				errorJS(w, fmt.Sprintf(
					`Unsupported file dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "unsupported-git-dependency":
				errorJS(w, fmt.Sprintf(
					`Unsupported git dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "invalid-jsr-dependency":
				errorJS(w, fmt.Sprintf(
					`Invalid jsr dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			case "invalid-http-dependency":
				errorJS(w, fmt.Sprintf(
					`Invalid http dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				))
			default:
				header.Set("Cache-Control", ccOneDay)
				writeStatus(w, 500, "Unknown error")
			}
			return

		// builtin scripts
		case "/tsx", "/run":
			ifNoneMatch := r.Header.Get("If-None-Match")
			if ifNoneMatch == globalETag && !DEBUG {
				w.WriteHeader(http.StatusNotModified)
				return
			}

			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(r.URL.Query().Get("target"))
			targetFromUA := targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(r.UserAgent())
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
				writeStatus(w, 500, err.Error())
				return
			}
			if DEBUG {
				header.Set("Cache-Control", ccMustRevalidate)
			} else {
				header.Set("Cache-Control", ccOneDay)
			}
			header.Set("Etag", globalETag)
			if targetFromUA {
				appendVaryHeader(header, "User-Agent")
			}
			header.Set("Content-Type", ctJavaScript)
			writeBody(w, js)
			return

		case "/install":
			data, err := embedFS.ReadFile("embed/install.sh")
			if err != nil {
				header.Set("Cache-Control", ccImmutable)
				writeStatus(w, 404, "not found")
				return
			}
			header.Set("Content-Type", "text/plain; charset=utf-8")
			header.Set("Cache-Control", ccMustRevalidate)
			writeBody(w, data)
			return
		}

		// module generated by the `/transform` API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByFirstByte(pathname[2:], '.')
			if len(hash) != 40 || !valid.IsHexString(hash) {
				header.Set("Cache-Control", ccImmutable)
				writeStatus(w, 404, "Not Found")
				return
			}
			savePath := normalizeSavePath(fmt.Sprintf("modules/transform/%s.%s", hash, ext))
			f, fi, err := esmStorage.Get(savePath)
			if err != nil {
				logger.Errorf("storage.get(%s): %v", savePath, err)
				writeStatus(w, 500, "Storage error, please try again")
				return
			}
			if strings.HasSuffix(pathname, ".map") {
				header.Set("Content-Type", ctJSON)
			} else {
				header.Set("Content-Type", ctJavaScript)
			}
			header.Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
			header.Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			header.Set("Cache-Control", ccImmutable)
			writeReader(w, f)
			return
		}

		// node libs
		if strings.HasPrefix(pathname, "/node/") {
			if !strings.HasSuffix(pathname, ".mjs") {
				header.Set("Cache-Control", ccImmutable)
				writeStatus(w, 404, "Not Found")
				return
			}
			name := pathname[6:]
			js, ok := getNodeRuntimeJS(name)
			if !ok {
				if !nodeBuiltinModules[name] {
					header.Set("Cache-Control", ccImmutable)
					writeStatus(w, 404, "Not Found")
					return
				}
				js = []byte("export default {}")
			}
			if strings.HasPrefix(name, "chunk-") {
				header.Set("Cache-Control", ccImmutable)
			} else {
				ifNoneMatch := r.Header.Get("If-None-Match")
				if ifNoneMatch == globalETag && !DEBUG {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				header.Set("Cache-Control", ccOneDay)
				header.Set("Etag", globalETag)
			}
			header.Set("Content-Type", ctJavaScript)
			writeBody(w, js)
			return
		}

		// embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile(pathname[1:])
			if err != nil {
				header.Set("Cache-Control", ccImmutable)
				writeStatus(w, 404, "not found")
				return
			}
			if !DEBUG {
				header.Set("Cache-Control", ccMustRevalidate)
			} else {
				etag := fmt.Sprintf(`W/"%d%d"`, startTime.Unix(), len(data))
				if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch == etag {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				header.Set("Etag", etag)
				header.Set("Cache-Control", ccOneDay)
			}
			contentType := mime.GetContentType(pathname)
			if contentType != "" {
				header.Set("Content-Type", contentType)
			}
			writeBody(w, data)
			return
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
				header.Set("Cache-Control", ccImmutable)
			} else if strings.HasSuffix(message, " not found") {
				status = 404
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
			}
			writeStatus(w, status, message)
			return
		}

		if !config.AllowList.IsEmpty() && !config.AllowList.IsPackageAllowed(esmPath.PackageId()) {
			header.Set("Cache-Control", "public, max-age=3600")
			writeStatus(w, 403, "forbidden")
			return
		}

		if !config.BanList.IsEmpty() && config.BanList.IsPackageBanned(esmPath.PackageId()) {
			header.Set("Cache-Control", "public, max-age=3600")
			writeStatus(w, 403, "forbidden")
			return
		}

		origin := getOrigin(r)

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
				writeStatus(w, 500, err.Error())
				return
			}
			types := "index.d.ts"
			if info.Types != "" {
				types = info.Types
			} else if info.Main != "" && endsWith(info.Main, ".d.ts", ".d.mts", ".d.cts") {
				types = info.Main
			}
			if strings.HasSuffix(types, ".d") {
				types += ".ts"
			} else if !endsWith(types, ".d.ts", ".d.mts", ".d.cts") {
				types += ".d.ts"
			}
			redirect(w, fmt.Sprintf("%s/%s@%s%s", origin, info.Name, info.Version, utils.NormalizePathname(types)), isExactVersion)
			return
		}

		// redirect to the main css path for CSS packages
		if css := cssPackages[esmPath.PkgName]; css != "" && esmPath.SubPath == "" {
			url := fmt.Sprintf("%s/%s/%s", origin, esmPath.PackageId(), css)
			redirect(w, url, isExactVersion)
			return
		}

		// store the raw query
		rawQuery := r.URL.RawQuery

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"/jsx-runtime", "/jsx-dev-runtime"} {
			if strings.HasSuffix(rawQuery, jsxRuntime) {
				if esmPath.SubPath == "" {
					esmPath.SubPath = jsxRuntime[1:]
				} else {
					esmPath.SubPath = esmPath.SubPath + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", esmPath.PkgName, esmPath.SubPath)
				r.URL.RawQuery = strings.TrimSuffix(rawQuery, jsxRuntime)
				break
			}
		}

		// apply the extra query if exists
		if extraQuery != "" {
			qs := []string{extraQuery}
			if rawQuery != "" {
				qs = append(qs, rawQuery)
			}
			r.URL.RawQuery = strings.Join(qs, "&")
		}

		// parse the query
		// todo: validate query
		query := r.URL.Query()

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

		rawFlag := query.Has("raw") || strings.HasPrefix(r.Host, "raw.")
		if rawFlag {
			pathKind = RawFile
		}

		// restore the original path extension
		if pathKind == RawFile && esmPath.SubPath != "" {
			extname := path.Ext(pathname)
			if !strings.HasSuffix(esmPath.SubPath, extname) {
				esmPath.SubPath += extname
			}
		}

		if pathKind == RawFile && !rawFlag && esmPath.SubPath != "" && strings.HasSuffix(esmPath.SubPath, ".map") {
			pkgJson, err := npmrc.installPackage(esmPath.Package())
			if err != nil {
				writeStatus(w, 500, err.Error())
				return
			}
			filename := path.Join(npmrc.StoreDir(), esmPath.PackageId(), "node_modules", esmPath.PkgName, esmPath.SubPath)
			stat, err := os.Lstat(filename)
			if err != nil {
				if os.IsNotExist(err) {
					if _, ok := pkgJson.Exports.Get("./" + esmPath.SubPath); ok {
						pathKind = EsmEntry
					}
				} else {
					writeStatus(w, 500, err.Error())
					return
				}
			} else if stat.IsDir() {
				if _, ok := pkgJson.Exports.Get("./" + esmPath.SubPath); ok {
					pathKind = EsmEntry
				}
			}
		}

		// redirect to the url with exact package version
		if !isExactVersion {
			if hasTargetSegment {
				pkgName := esmPath.PackageId()
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
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				redirect(w, fmt.Sprintf("%s/%s%s%s", origin, pkgName, subPath, query), false)
				return
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
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
				redirect(w, fmt.Sprintf("%s%s/%s@%s%s%s", origin, registryPrefix, pkgName, pkgVersion, subPath, query), false)
				return
			}
		}

		// fix url that is related to `import.meta.url`
		if hasTargetSegment && isExactVersion && pathKind == RawFile && !rawFlag {
			extname := path.Ext(esmPath.SubPath)
			dir := path.Join(npmrc.StoreDir(), esmPath.PackageId())
			if !existsDir(dir) {
				_, err := npmrc.installPackage(esmPath.Package())
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
			}
			pkgRoot := path.Join(dir, "node_modules", esmPath.PkgName)
			files, err := findFiles(pkgRoot, "", func(fp string) bool {
				return strings.HasSuffix(fp, extname)
			})
			if err != nil {
				writeStatus(w, 500, err.Error())
				return
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
				header.Set("Cache-Control", ccImmutable)
				writeStatus(w, 404, "File not found")
				return
			}
			url := fmt.Sprintf("%s%s/%s@%s/%s", origin, registryPrefix, esmPath.PkgName, esmPath.PkgVersion, file)
			redirect(w, url, true)
			return
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
				header.Set("Content-Type", ctJavaScript)
				header.Set("Cache-Control", ccImmutable)
				writeBody(w, buf.Bytes())
				return
			}

			// return css file as a `CSSStyleSheet` object when `?module` query is present
			if pathKind == RawFile && strings.HasSuffix(esmPath.SubPath, ".css") && query.Has("module") {
				filename := path.Join(npmrc.StoreDir(), esmPath.PackageId(), "node_modules", esmPath.PkgName, esmPath.SubPath)
				css, err := os.ReadFile(filename)
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
				css, err = minify(string(css), esbuild.LoaderCSS, esbuild.ES2022)
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
				buf := bytes.NewBufferString("/* esm.sh - css module */\n")
				buf.WriteString("const stylesheet = new CSSStyleSheet();\n")
				buf.WriteString("stylesheet.replaceSync(")
				buf.WriteString(strings.TrimSuffix(string(utils.MustEncodeJSON(strings.TrimSuffix(string(css), "\n"))), "\n"))
				buf.WriteString(");\n")
				buf.WriteString("export default stylesheet;\n")
				header.Set("Content-Type", ctJavaScript)
				header.Set("Cache-Control", ccImmutable)
				writeBody(w, buf.Bytes())
				return
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
						writeStatus(w, 500, err.Error())
						return
					}
					entry := b.resolveEntry(esmPath)
					if entry.main == "" {
						header.Set("Cache-Control", ccImmutable)
						writeStatus(w, 404, "File Not Found")
						return
					}
					query := ""
					if rawQuery != "" {
						query = "?" + rawQuery
					}
					// redirect to the 'main' JS file
					redirect(w, fmt.Sprintf("%s/%s%s%s", origin, esmPath.PackageId(), utils.NormalizePathname(entry.main), query), true)
					return
				}

				filename := path.Join(npmrc.StoreDir(), esmPath.PackageId(), "node_modules", esmPath.PkgName, esmPath.SubPath)
				stat, err := os.Lstat(filename)
				if err != nil && os.IsNotExist(err) {
					// if the file does not exist, try to install the package
					_, err = npmrc.installPackage(esmPath.Package())
					if err != nil {
						writeStatus(w, 500, err.Error())
						return
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
							writeStatus(w, 500, err.Error())
							return
						}
						entry := b.resolveEntry(esmPath)
						if entry.main != "" && entry.main != "./"+esmPath.SubPath {
							query := ""
							if rawQuery != "" {
								query = "?" + rawQuery
							}
							// redirect to the resolved path
							redirect(w, fmt.Sprintf("%s/%s%s%s", origin, esmPath.PackageId(), utils.NormalizePathname(entry.main), query), true)
							return
						}
						header.Set("Cache-Control", ccImmutable)
						writeStatus(w, 404, "File Not Found")
						return
					}
					writeStatus(w, 500, err.Error())
					return
				}
				if stat.IsDir() {
					header.Set("Cache-Control", ccImmutable)
					writeStatus(w, 404, "File Not Found")
					return
				}
				// limit the file size up to 50MB
				if stat.Size() > maxAssetFileSize {
					header.Set("Cache-Control", ccImmutable)
					writeStatus(w, 403, "File Too Large")
					return
				}
				etag := fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
				if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch == etag {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				content, err := os.Open(filename)
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
				if endsWith(esmPath.SubPath, ".js", ".mjs", ".cjs") {
					header.Set("Content-Type", ctJavaScript)
				} else if endsWith(esmPath.SubPath, ".ts", ".mts", ".cts", ".tsx") {
					header.Set("Content-Type", ctTypeScript)
				} else if strings.HasSuffix(esmPath.SubPath, ".jsx") {
					header.Set("Content-Type", "text/jsx; charset=utf-8")
				} else {
					contentType := mime.GetContentType(esmPath.SubPath)
					if contentType != "" {
						header.Set("Content-Type", contentType)
					}
				}
				header.Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
				header.Set("Etag", etag)
				header.Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
				header.Set("Cache-Control", ccImmutable)
				if strings.HasSuffix(esmPath.SubPath, ".json") && query.Has("module") {
					defer content.Close()
					jsonData, err := io.ReadAll(content)
					if err != nil {
						writeStatus(w, 500, err.Error())
						return
					}
					header.Set("Content-Type", ctJavaScript)
					writeBody(w, concatBytes([]byte("export default "), jsonData))
					return
				}
				writeReader(w, content)
				return
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
						writeStatus(w, 500, "Storage error, please try again")
						return
					} else if pathKind == EsmSourceMap {
						header.Set("Cache-Control", ccImmutable)
						writeStatus(w, 404, "Not found")
						return
					}
				}
				if err == nil {
					header.Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
					header.Set("Cache-Control", ccImmutable)
					if pathKind == EsmDts {
						header.Set("Content-Type", ctTypeScript)
					} else if pathKind == EsmSourceMap {
						header.Set("Content-Type", ctJSON)
					} else if strings.HasSuffix(pathname, ".css") {
						header.Set("Content-Type", ctCSS)
					} else {
						header.Set("Content-Type", ctJavaScript)
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
							writeBody(w, fmt.Appendf(nil,
								`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
								moduleUrl,
								moduleUrl,
							))
							return
						}
						if len(exports) > 0 {
							defer f.Close()
							xxh := xxhash.New()
							xxh.Write([]byte(strings.Join(exports, ",")))
							savePath = strings.TrimSuffix(savePath, ".mjs") + "_" + base64.RawURLEncoding.EncodeToString(xxh.Sum(nil)) + ".mjs"
							f2, stat, err := esmStorage.Get(savePath)
							if err == nil {
								header.Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
								writeReader(w, f2)
								return
							}
							if err != storage.ErrNotFound {
								logger.Errorf("storage.get(%s): %v", savePath, err)
								writeStatus(w, 500, "Storage error, please try again")
								return
							}
							code, err := io.ReadAll(f)
							if err != nil {
								writeStatus(w, 500, err.Error())
								return
							}
							target := esbuild.ES2022
							// check target in the pathname
							for seg := range strings.SplitSeq(pathname, "/") {
								if t, ok := targets[seg]; ok {
									target = t
									break
								}
							}
							ret, err := treeShake(npmrc, esmPath.Package(), code, exports, target)
							if err != nil {
								writeStatus(w, 500, err.Error())
								return
							}
							// note: the source map is dropped
							go esmStorage.Put(savePath, bytes.NewReader(ret))
							writeBody(w, ret)
							return
						}
					}
					if pathKind == EsmDts {
						defer f.Close()
						buffer, err := io.ReadAll(f)
						if err != nil {
							writeStatus(w, 500, err.Error())
							return
						}
						writeBody(w, bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(origin)))
						return
					}
					header.Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
					writeReader(w, f)
					return
				}
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		var targetFromUA bool
		if target == "" {
			target = strings.ToLower(query.Get("target"))
			targetFromUA = targets[target] == 0
			if targetFromUA {
				target = getBuildTargetByUA(r.UserAgent())
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
				appendVaryHeader(header, "User-Agent")
			}
			redirect(w, fmt.Sprintf("%s%s/%s@%s%s%s", origin, registryPrefix, pkgName, pkgVersion, subPath, qs), false)
			return
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
						header.Set("Cache-Control", ccImmutable)
						writeStatus(w, 400, fmt.Sprintf("Invalid deps query: %v not found", v))
						return
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
					esmPath.PackageId(),
					args,
				), esmPath.SubPath))
				content, stat, err = esmStorage.Get(savePath)
				return
			}
			content, _, err := readDts()
			if err != nil {
				if err != storage.ErrNotFound {
					writeStatus(w, 500, "Storage error, please try again")
					return
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
								header.Set("Cache-Control", ccImmutable)
							} else {
								header.Set("Cache-Control", ccOneDay)
							}
							writeStatus(w, 404, "Types Not Found")
							return
						}
						writeStatus(w, 500, "Failed to build types: "+output.err.Error())
						return
					}
				case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
					header.Set("Cache-Control", ccMustRevalidate)
					writeStatus(w, http.StatusRequestTimeout, "timeout, the types is waiting to be built, please try refreshing the page.")
					return
				}
				content, _, err = readDts()
			}
			if err != nil {
				if err == storage.ErrNotFound {
					if isExactVersion {
						header.Set("Cache-Control", ccImmutable)
					} else {
						header.Set("Cache-Control", ccOneDay)
					}
					writeStatus(w, 404, "Types Not Found")
					return
				}
				writeStatus(w, 500, err.Error())
				return
			}
			defer content.Close()
			buffer, err := io.ReadAll(content)
			if err != nil {
				writeStatus(w, 500, err.Error())
				return
			}
			header.Set("Content-Type", ctTypeScript)
			header.Set("Cache-Control", ccImmutable)
			writeBody(w, bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(origin)))
			return
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
			if before, ok := strings.CutSuffix(esmPath.SubPath, ".bundle"); ok {
				esmPath.SubPath = before
				bundleMode = BundleDeps
			} else if before, ok := strings.CutSuffix(esmPath.SubPath, ".nobundle"); ok {
				esmPath.SubPath = before
				bundleMode = BundleFalse
			}
			if before, ok := strings.CutSuffix(esmPath.SubPath, ".development"); ok {
				esmPath.SubPath = before
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
			writeStatus(w, 500, err.Error())
			return
		}
		if !ok {
			ch := buildQueue.Add(build)
			select {
			case output := <-ch:
				if output.err != nil {
					msg := output.err.Error()
					if msg == "could not resolve build entry" || strings.HasSuffix(msg, " not found") || strings.Contains(msg, "is not exported from package") || strings.Contains(msg, "no such file or directory") {
						header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
						writeStatus(w, 404, msg)
						return
					}
					writeStatus(w, 500, msg)
					return
				}
				buildMeta = output.meta
				// Follow a freshly built version in the default URL immediately.
				invalidateDistTagCacheIfNewer(esmPath.PkgName, esmPath.PkgVersion)
			case <-time.After(time.Duration(config.BuildWaitTime) * time.Second):
				header.Set("Cache-Control", ccMustRevalidate)
				writeStatus(w, http.StatusRequestTimeout, "timeout, the module is waiting to be built, please try refreshing the page.")
				return
			}
		}

		if buildMeta.CSSEntry != "" {
			url := getCSSEntryRedirectURL(origin, esmPath, buildMeta.CSSEntry)
			redirect(w, url, isExactVersion)
			return
		}

		// redirect to `*.d.ts` file
		if buildMeta.TypesOnly {
			dtsUrl := origin + buildMeta.Dts
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", ctJavaScript)
			header.Set("Cache-Control", ccImmutable)
			if r.Method == http.MethodHead {
				writeBody(w, []byte{})
				return
			}
			writeBody(w, []byte("export default null;\n"))
			return
		}

		// redirect to package css from `?css`
		if query.Has("css") && esmPath.SubPath == "" {
			if !buildMeta.CSSInJS {
				if isExactVersion {
					header.Set("Cache-Control", ccImmutable)
				} else {
					header.Set("Cache-Control", ccOneDay)
				}
				writeStatus(w, 404, "Package CSS not found")
				return
			}
			url := origin + strings.TrimSuffix(build.Path(), ".mjs") + ".css"
			redirect(w, url, isExactVersion)
			return
		}

		if query.Has("meta") {
			metaJson := map[string]any{
				"name":    esmPath.PkgName,
				"version": esmPath.PkgVersion,
				"module":  build.Path(),
			}
			if esmPath.GhPrefix {
				metaJson["gh"] = true
			}
			if esmPath.PrPrefix {
				metaJson["pr"] = true
			}
			if esmPath.SubPath == "" {
				packageJson, err := npmrc.getPackageInfo(esmPath.PkgName, esmPath.PkgVersion)
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
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
					writeStatus(w, 500, err.Error())
					return
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
					writeStatus(w, 500, "Storage error, please try again")
					return
				}
				defer f.Close()
				sha := sha512.New384()
				_, err = io.Copy(sha, f)
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
				integrity = "sha384-" + base64.StdEncoding.EncodeToString(sha.Sum(nil))
				buildMeta.Integrity = integrity
				err = metaDB.Put(build.Path(), encodeBuildMeta(buildMeta))
				if err != nil {
					writeStatus(w, 500, err.Error())
					return
				}
			}
			metaJson["integrity"] = integrity
			if isExactVersion {
				header.Set("Cache-Control", ccImmutable)
			} else {
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
			}
			writeJSON(w, 200, metaJson)
			return
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
				buf := &bytes.Buffer{}
				esmPath := build.Path()
				fmt.Fprintf(buf, "export * from \"%s\";\n", esmPath)
				if buildMeta.ExportDefault {
					fmt.Fprintf(buf, "export { default } from \"%s\";\n", esmPath)
				}
				header.Set("Content-Type", ctJavaScript)
				header.Set("Cache-Control", ccImmutable)
				writeBody(w, buf.Bytes())
				return
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
				writeStatus(w, 500, "Storage error, please try again")
				return
			}
			header.Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			header.Set("Cache-Control", ccImmutable)
			if strings.HasSuffix(savePath, ".css") {
				header.Set("Content-Type", ctCSS)
			} else if endsWith(savePath, ".map") {
				header.Set("Content-Type", ctJSON)
			} else {
				header.Set("Content-Type", ctJavaScript)
				if query.Has("worker") {
					defer f.Close()
					moduleUrl := origin + build.Path()
					if !buildMeta.CJS && len(exports) > 0 {
						moduleUrl += "?exports=" + strings.Join(exports, ",")
					}
					writeBody(w, fmt.Appendf(nil,
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					))
					return
				}
				if noDts := query.Has("no-dts") || query.Has("no-check"); !noDts && buildMeta.Dts != "" {
					header.Set("X-TypeScript-Types", origin+buildMeta.Dts)
					header.Set("Access-Control-Expose-Headers", "X-TypeScript-Types")
				}
				if !buildMeta.CJS && len(exports) > 0 {
					defer f.Close()
					xxh := xxhash.New()
					xxh.Write([]byte(strings.Join(exports, ",")))
					savePath = strings.TrimSuffix(savePath, ".mjs") + "_" + base64.RawURLEncoding.EncodeToString(xxh.Sum(nil)) + ".mjs"
					f2, stat, err := esmStorage.Get(savePath)
					if err == nil {
						header.Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
						writeReader(w, f2)
						return
					}
					if err != storage.ErrNotFound {
						logger.Errorf("storage.get(%s): %v", savePath, err)
						writeStatus(w, 500, "Storage error, please try again")
						return
					}
					code, err := io.ReadAll(f)
					if err != nil {
						writeStatus(w, 500, err.Error())
						return
					}
					ret, err := treeShake(npmrc, esmPath.Package(), code, exports, targets[target])
					if err != nil {
						writeStatus(w, 500, err.Error())
						return
					}
					go esmStorage.Put(savePath, bytes.NewReader(ret))
					// note: the source map is dropped
					writeBody(w, ret)
					return
				}
			}
			header.Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
			writeReader(w, f)
			return
		}

		buf := &bytes.Buffer{}
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
			if len(buildMeta.Imports) > 0 && !query.Has("exports") {
				for _, dep := range buildMeta.Imports {
					fmt.Fprintf(buf, "import \"%s\";\n", dep)
				}
			}
			esmPath := build.Path()
			if !buildMeta.CJS && len(exports) > 0 {
				esmPath += "?exports=" + strings.Join(exports, ",")
			}
			fmt.Fprintf(buf, "export * from \"%s\";\n", esmPath)
			if buildMeta.ExportDefault && (len(exports) == 0 || slices.Contains(exports, "default")) {
				fmt.Fprintf(buf, "export { default } from \"%s\";\n", esmPath)
			}
			if buildMeta.CJS && len(exports) > 0 {
				fmt.Fprintf(buf, "import _ from \"%s\";\n", esmPath)
				fmt.Fprintf(buf, "export const { %s } = _;\n", strings.Join(exports, ", "))
			}
			header.Set("X-ESM-Path", esmPath)
			if noDts := query.Has("no-dts") || query.Has("no-check"); !noDts && buildMeta.Dts != "" {
				header.Set("X-TypeScript-Types", origin+buildMeta.Dts)
				header.Set("Access-Control-Expose-Headers", "X-ESM-Path, X-TypeScript-Types")
			} else {
				header.Set("Access-Control-Expose-Headers", "X-ESM-Path")
			}
		}

		if targetFromUA {
			appendVaryHeader(header, "User-Agent")
		}
		if isExactVersion {
			header.Set("Cache-Control", ccImmutable)
		} else {
			header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
		}
		header.Set("Content-Type", ctJavaScript)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeBody(w, buf.Bytes())
	})
}

func getOrigin(r *http.Request) string {
	if r.Host == "raw.esm.sh" {
		return "https://raw.esm.sh"
	}
	if config.CdnOrigin != "" {
		return config.CdnOrigin
	}
	proto := "http:"
	if cfVisitor := r.Header.Get("CF-Visitor"); cfVisitor != "" {
		if strings.Contains(cfVisitor, "\"https\"") {
			proto = "https:"
		}
	} else if r.TLS != nil {
		proto = "https:"
	}
	return proto + "//" + r.Host
}

func redirect(w http.ResponseWriter, url string, isMovedPermanently bool) {
	h := w.Header()
	code := http.StatusFound
	if isMovedPermanently {
		code = http.StatusMovedPermanently
		h.Set("Cache-Control", ccImmutable)
	} else {
		h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", config.NpmQueryCacheTTL))
	}
	h.Set("Location", url)
	w.WriteHeader(code)
}

func errorJS(w http.ResponseWriter, message string) {
	buf := &bytes.Buffer{}
	buf.WriteString("/* esm.sh - error */\n")
	buf.WriteString("throw new Error(")
	buf.Write(utils.MustEncodeJSON(message))
	buf.WriteString(");\n")
	buf.WriteString("export default null;\n")
	h := w.Header()
	h.Set("Content-Type", ctJavaScript)
	h.Set("Cache-Control", ccImmutable)
	writeBody(w, buf.Bytes())
}

func getCSSEntryRedirectURL(origin string, esmPath EsmPath, cssEntry string) string {
	return origin + "/" + esmPath.PackageId() + utils.NormalizePathname(cssEntry)
}
