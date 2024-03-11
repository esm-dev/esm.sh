package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

func esmHandler() rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		cdnOrigin := getCdnOrign(ctx)
		pathname := ctx.Path.String()
		header := ctx.W.Header()
		userAgent := ctx.R.UserAgent()

		// ban malicious requests
		if strings.HasPrefix(pathname, "/.") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// Build prefix may only be served from "${cfg.CdnBasePath}/..."
		if cfg.CdnBasePath != "" {
			if strings.HasPrefix(pathname, cfg.CdnBasePath) {
				pathname = strings.TrimPrefix(pathname, cfg.CdnBasePath)
			} else {
				url := strings.TrimPrefix(ctx.R.URL.String(), cfg.CdnBasePath)
				url = fmt.Sprintf("%s/%s", cfg.CdnBasePath, url)
				return rex.Redirect(url, http.StatusMovedPermanently)
			}
		}

		// static routes
		switch pathname {
		case "/":
			indexHTML, err := embedFS.ReadFile("server/embed/index.html")
			if err != nil {
				return err
			}
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte(cfg.CdnBasePath+"/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/main/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(cdnOrigin+cfg.CdnBasePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			html = bytes.ReplaceAll(html, []byte("{basePath}"), []byte(cfg.CdnBasePath))
			header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			q := make([]map[string]interface{}, buildQueue.list.Len())
			i := 0
			buildQueue.lock.RLock()
			for el := buildQueue.list.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*queueTask)
				if ok {
					m := map[string]interface{}{
						"bundle":    t.Bundle,
						"clients":   t.clients,
						"createdAt": t.createdAt.Format(http.TimeFormat),
						"dev":       t.Dev,
						"inProcess": t.inProcess,
						"pkg":       t.Pkg.String(),
						"stage":     t.stage,
						"target":    t.Target,
					}
					if !t.startedAt.IsZero() {
						m["startedAt"] = t.startedAt.Format(http.TimeFormat)
					}
					if len(t.Args.deps) > 0 {
						m["deps"] = t.Args.deps.String()
					}
					q[i] = m
					i++
				}
			}
			buildQueue.lock.RUnlock()

			header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
			return map[string]interface{}{
				"buildQueue": q[:i],
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
			}

		case "/esma-target":
			return getBuildTargetByUA(userAgent)

		case "/error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Errorf(
					`could not resolve "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-node-builtin-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported Node builtin module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-node-native-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported node native module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported NPM package "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported file dependency "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			default:
				return throwErrorJS(ctx, fmt.Errorf("unknown error"), true)
			}

		case "/favicon.ico":
			return rex.Status(404, "not found")
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			modTime := startTime
			if fs, ok := embedFS.(*DevFS); ok {
				if fi, err := fs.Lstat("server" + pathname); err == nil {
					modTime = fi.ModTime()
				}
			}
			data, err := embedFS.ReadFile("server" + pathname)
			if err != nil {
				return rex.Status(404, "not found")
			}
			if strings.HasSuffix(pathname, ".js") {
				data = bytes.ReplaceAll(data, []byte("{origin}"), []byte(cdnOrigin))
				data = bytes.ReplaceAll(data, []byte("{basePath}"), []byte(cfg.CdnBasePath))
			}
			header.Set("Cache-Control", "public, max-age=86400")
			return rex.Content(pathname, modTime, bytes.NewReader(data))
		}

		// serve modules created by the build API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByLastByte(pathname[2:], '.')
			if len(hash) != 40 || ext != "mjs" {
				return rex.Status(404, "not found")
			}
			target := getBuildTargetByUA(userAgent)
			savaPath := fmt.Sprintf("publish/+%s.%s.%s", hash, target, ext)
			fi, err := fs.Stat(savaPath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "not found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := fs.OpenFile(savaPath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			addVary(header, "User-Agent")
			return rex.Content(savaPath, fi.ModTime(), r) // auto closed
		}

		// serve build adn run scripts
		if pathname == "/build" || pathname == "/run" || pathname == "/hot" {
			data, err := embedFS.ReadFile(fmt.Sprintf("server/embed/%s.ts", pathname[1:]))
			if err != nil {
				return rex.Status(404, "Not Found")
			}

			etag := fmt.Sprintf(`"W/v%d"`, VERSION)
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == etag {
				header.Set("Cache-Control", "public, max-age=86400")
				return rex.Status(http.StatusNotModified, "")
			}

			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(ctx.Form.Value("target"))
			targetViaUA := targets[target] == 0
			if targetViaUA {
				target = getBuildTargetByUA(userAgent)
			}
			if target == "deno" || target == "denonext" {
				header.Set("Content-Type", "application/typescript; charset=utf-8")
			} else {
				code, err := minify(string(data), targets[target], api.LoaderTS)
				if err != nil {
					return throwErrorJS(ctx, fmt.Errorf("transform error: %v", err), false)
				}
				data = code
				header.Set("Content-Type", "application/javascript; charset=utf-8")
			}
			if targetViaUA {
				addVary(header, "User-Agent")
			}
			if ctx.Form.Value("v") != "" {
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				header.Set("Cache-Control", "public, max-age=86400")
				header.Set("ETag", etag)
			}
			if pathname == "/hot" {
				header.Set("X-Typescript-Types", fmt.Sprintf("%s%s/hot.d.ts", cdnOrigin, cfg.CdnBasePath))
			}
			return data
		}

		// serve node libs
		if strings.HasPrefix(pathname, "/node/") && strings.HasSuffix(pathname, ".js") {
			lib, ok := nodeLibs[pathname[1:]]
			if !ok {
				// empty module
				lib = "export default {}"
			}
			if strings.HasPrefix(pathname, "/node/chunk-") {
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				etag := fmt.Sprintf(`"W/v%d"`, VERSION)
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == etag {
					header.Set("Cache-Control", "public, max-age=86400")
					return rex.Status(http.StatusNotModified, "")
				}
				if ctx.Form.Value("v") != "" {
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					header.Set("Cache-Control", "public, max-age=86400")
					header.Set("ETag", etag)
				}
			}
			target := getBuildTargetByUA(userAgent)
			code, err := minify(lib, targets[target], api.LoaderJS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Errorf("transform error: %v", err), false)
			}
			addVary(header, "User-Agent")
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			return rex.Content(pathname, startTime, bytes.NewReader(code))
		}

		// use embed polyfills/types
		if endsWith(pathname, ".js", ".d.ts") && strings.Count(pathname, "/") == 1 {
			var data []byte
			var err error
			if strings.HasSuffix(pathname, ".js") {
				data, err = embedFS.ReadFile("server/embed/polyfills" + pathname)
				if err == nil {
					header.Set("Content-Type", "application/javascript; charset=utf-8")
				}
			} else {
				data, err = embedFS.ReadFile("server/embed/types" + pathname)
				if err == nil {
					header.Set("Content-Type", "application/typescript; charset=utf-8")
				}
			}
			if err == nil {
				etag := fmt.Sprintf(`"W/v%d"`, VERSION)
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == etag {
					header.Set("Cache-Control", "public, max-age=86400")
					return rex.Status(http.StatusNotModified, "")
				}
				if ctx.Form.Value("v") != "" {
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					header.Set("Cache-Control", "public, max-age=86400")
					header.Set("ETag", etag)
				}
				if strings.HasSuffix(pathname, ".js") {
					target := getBuildTargetByUA(userAgent)
					code, err := minify(string(data), targets[target], api.LoaderJS)
					if err != nil {
						return throwErrorJS(ctx, fmt.Errorf("transform error: %v", err), false)
					}
					addVary(header, "User-Agent")
					data = []byte(code)
				}
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
		}

		// check extra query like `/react-dom@18.2.0&external=react&dev/client`
		var extraQuery string
		if strings.ContainsRune(pathname, '@') && regexpPathWithVersion.MatchString(pathname) {
			if _, v := utils.SplitByLastByte(pathname, '@'); v != "" {
				if _, e := utils.SplitByFirstByte(v, '&'); e != "" {
					extraQuery, _ = utils.SplitByFirstByte(e, '/')
					if extraQuery != "" {
						qs := []string{extraQuery}
						if ctx.R.URL.RawQuery != "" {
							qs = append(qs, ctx.R.URL.RawQuery)
						}
						ctx.R.URL.RawQuery = strings.Join(qs, "&")
					}
				}
			}
		}

		// check `/*pathname` or `/gh/*pathname` pattern
		external := newStringSet()
		if strings.HasPrefix(pathname, "/*") {
			external.Add("*")
			pathname = "/" + pathname[2:]
		} else if strings.HasPrefix(pathname, "/gh/*") {
			external.Add("*")
			pathname = "/gh/" + pathname[5:]
		}

		// get package info
		reqPkg, extraQuery, err := validatePkgPath(pathname)
		if err != nil {
			status := 500
			message := err.Error()
			if message == "invalid path" {
				status = 400
			} else if strings.HasSuffix(message, "not found") {
				status = 404
			}
			return rex.Status(status, message)
		}

		pkgAllowed := cfg.AllowList.IsPackageAllowed(reqPkg.Name)
		pkgBanned := cfg.BanList.IsPackageBanned(reqPkg.Name)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		hasTargetSegmentinPath := hasTargetSegment(reqPkg.SubPath)

		// fix urls related to `import.meta.url`
		if hasTargetSegmentinPath && endsWith(reqPkg.SubPath, ".wasm", ".json") {
			extname := path.Ext(reqPkg.SubPath)
			dir := path.Join(cfg.WorkDir, "npm", reqPkg.Name+"@"+reqPkg.Version)
			if !existsDir(dir) {
				err := installPackage(dir, reqPkg)
				if err != nil {
					return rex.Status(500, err.Error())
				}
			}
			pkgRoot := path.Join(dir, "node_modules", reqPkg.Name)
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
				sort.Sort(sort.Reverse(PathSlice(files)))
				for _, f := range files {
					if strings.HasSuffix(reqPkg.SubPath, f) {
						file = f
						break
					}
				}
				if file == "" {
					for _, f := range files {
						if path.Base(reqPkg.SubPath) == path.Base(f) {
							file = f
							break
						}
					}
				}
			}
			if file == "" {
				return rex.Status(404, "File not found")
			}
			url := fmt.Sprintf("%s%s/%s@%s/%s", cdnOrigin, cfg.CdnBasePath, reqPkg.Name, reqPkg.Version, file)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// redirect `/@types/PKG` to main dts files
		if strings.HasPrefix(reqPkg.Name, "@types/") && (reqPkg.SubModule == "" || !strings.HasSuffix(reqPkg.SubModule, ".d.ts")) {
			url := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.CdnBasePath, pathname)
			if reqPkg.SubModule == "" {
				info, _, err := getPackageInfo("", reqPkg.Name, reqPkg.Version)
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
				url += "/" + types
			} else {
				url += "~.d.ts"
			}
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// redirect to main css path for CSS packages
		if css := cssPackages[reqPkg.Name]; css != "" && reqPkg.SubModule == "" {
			url := fmt.Sprintf("%s%s/%s/%s", cdnOrigin, cfg.CdnBasePath, reqPkg.String(), css)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		ghPrefix := ""
		if reqPkg.FromGithub {
			ghPrefix = "/gh"
		}

		// redirect to the url with full package version
		if !hasTargetSegmentinPath && !reqPkg.FromEsmsh && !strings.Contains(pathname, "@"+reqPkg.Version) {
			pkgName := reqPkg.Name
			eaSign := ""
			subPath := ""
			query := ""
			if strings.HasPrefix(pkgName, "@jsr/") {
				pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
			}

			if external.Has("*") {
				eaSign = "*"
			}
			if reqPkg.SubPath != "" {
				subPath = "/" + reqPkg.SubPath
			}
			if ctx.R.URL.RawQuery != "" {
				if extraQuery != "" {
					query = "&" + ctx.R.URL.RawQuery
					return rex.Redirect(fmt.Sprintf("%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, ghPrefix, eaSign, pkgName, reqPkg.Version, query, subPath), http.StatusFound)
				}
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, ghPrefix, eaSign, pkgName, reqPkg.Version, subPath, query), http.StatusFound)
		}

		// redirect to the url with full package version with build version prefix
		if hasTargetSegmentinPath && !strings.Contains(pathname, "@"+reqPkg.Version) {
			subPath := ""
			query := ""
			if reqPkg.SubPath != "" {
				subPath = "/" + reqPkg.SubPath
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s/%s%s%s", cdnOrigin, cfg.CdnBasePath, reqPkg.VersionName(), subPath, query), http.StatusFound)
		}

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"jsx-runtime", "jsx-dev-runtime"} {
			if strings.HasSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime) {
				if reqPkg.SubModule == "" {
					reqPkg.SubModule = jsxRuntime
				} else {
					reqPkg.SubModule = reqPkg.SubModule + "/" + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", reqPkg.Name, reqPkg.SubModule)
				ctx.R.URL.RawQuery = strings.TrimSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime)
			}
		}

		// or use `?path=$PATH` query to override the pathname
		if v := ctx.Form.Value("path"); v != "" {
			reqPkg.SubModule = utils.CleanPath(v)[1:]
		}

		// check request file type
		// - raw: serve raw dist or npm dist files like CSS/map etc..
		// - builds: serve js files built by esbuild
		// - types: serve `.d.ts` files
		var reqType string
		if reqPkg.SubPath != "" {
			ext := path.Ext(reqPkg.SubPath)
			switch ext {
			case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					reqType = "types"
				} else if ctx.R.URL.Query().Has("raw") {
					reqType = "raw"
				} else if hasTargetSegmentinPath {
					reqType = "builds"
				}
			case ".wasm":
				if ctx.Form.Has("module") {
					buf := &bytes.Buffer{}
					wasmUrl := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.CdnBasePath, pathname)
					fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
					fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					return buf
				} else {
					reqType = "raw"
				}
			case ".css", ".map":
				if hasTargetSegmentinPath {
					reqType = "builds"
				} else {
					reqType = "raw"
				}
			default:
				if ext != "" && assetExts[ext[1:]] {
					reqType = "raw"
				}
			}
		}

		// serve raw dist or npm dist files like CSS/map etc..
		if reqType == "raw" {
			installDir := fmt.Sprintf("npm/%s", reqPkg.VersionName())
			savePath := path.Join(cfg.WorkDir, installDir, "node_modules", reqPkg.Name, reqPkg.SubPath)
			fi, err := os.Lstat(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				// if the file not found, try to install the package
				err = installPackage(path.Join(cfg.WorkDir, installDir), reqPkg)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				// recheck the file
				fi, err = os.Lstat(savePath)
				if err != nil {
					if os.IsExist(err) {
						return rex.Status(500, err.Error())
					}
					return rex.Status(404, "File Not Found")
				}
			}
			content, err := os.Open(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				return rex.Status(404, "File Not Found")
			}
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			if endsWith(savePath, ".js", ".mjs", ".jsx") {
				header.Set("Content-Type", "application/javascript; charset=utf-8")
			} else if endsWith(savePath, ".ts", ".mts", ".tsx") {
				header.Set("Content-Type", "application/typescript; charset=utf-8")
			}
			return rex.Content(savePath, fi.ModTime(), content) // auto closed
		}

		// serve build files
		if hasTargetSegmentinPath && (reqType == "builds" || reqType == "types") {
			savePath := path.Join(reqType, pathname)
			if reqType == "types" {
				savePath = path.Join("types", getTypesRoot(cdnOrigin), strings.TrimPrefix(savePath, "types/"))
			}
			savePath = normalizeSavePath(savePath)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound && strings.HasSuffix(pathname, ".map") {
					return rex.Status(404, "Not found")
				}
				if err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
			}
			if err == nil {
				if reqType == "types" {
					header.Set("Content-Type", "application/typescript; charset=utf-8")
				} else if endsWith(pathname, ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx") {
					header.Set("Content-Type", "application/javascript; charset=utf-8")
				} else if strings.HasSuffix(savePath, ".map") {
					header.Set("Content-Type", "application/json; charset=utf-8")
				}
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
				if ctx.Form.Has("worker") && reqType == "builds" {
					moduleUrl := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.CdnBasePath, pathname)
					return fmt.Sprintf(
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					)
				}
				r, err := fs.OpenFile(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				return rex.Content(savePath, fi.ModTime(), r) // auto closed
			}
		}

		// check `?alias` query
		alias := map[string]string{}
		if ctx.Form.Has("alias") {
			for _, p := range strings.Split(ctx.Form.Value("alias"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					name, to := utils.SplitByFirstByte(p, ':')
					name = strings.TrimSpace(name)
					to = strings.TrimSpace(to)
					if name != "" && to != "" && name != reqPkg.Name {
						alias[name] = to
					}
				}
			}
		}

		// check `?deps` query
		deps := PkgSlice{}
		if ctx.Form.Has("deps") {
			for _, p := range strings.Split(ctx.Form.Value("deps"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					m, _, err := validatePkgPath(p)
					if err != nil {
						if strings.HasSuffix(err.Error(), "not found") {
							continue
						}
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", p))
					}
					if reqPkg.Name == "react-dom" && m.Name == "react" {
						// the `react` version always matches `react-dom` version
						continue
					}
					if !deps.Has(m.Name) && m.Name != reqPkg.Name {
						deps = append(deps, m)
					}
				}
			}
		}

		// check `?exports` query
		exports := newStringSet()
		if ctx.Form.Has("exports") || ctx.Form.Has("cjs-exports") {
			value := ctx.Form.Value("exports") + "," + ctx.Form.Value("cjs-exports")
			for _, p := range strings.Split(value, ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					exports.Add(p)
				}
			}
		}

		// check `?conditions` query
		conditions := newStringSet()
		if ctx.Form.Has("conditions") {
			for _, p := range strings.Split(ctx.Form.Value("conditions"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					conditions.Add(p)
				}
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		target := strings.ToLower(ctx.Form.Value("target"))
		targetViaUA := targets[target] == 0
		if targetViaUA {
			target = getBuildTargetByUA(userAgent)
		}

		// check deno/std version by `?deno-std=VER` query
		dsv := denoStdVersion
		fv := ctx.Form.Value("deno-std")
		if fv != "" && regexpFullVersion.MatchString(fv) && semverLessThan(fv, denoStdVersion) && target == "deno" {
			dsv = fv
		}

		// check `?external` query
		for _, p := range strings.Split(ctx.Form.Value("external"), ",") {
			p = strings.TrimSpace(p)
			if p == "*" {
				external.Reset()
				external.Add("*")
				break
			}
			if p != "" {
				external.Add(p)
			}
		}

		isPkgCss := ctx.Form.Has("css")
		bundle := (ctx.Form.Has("bundle") && ctx.Form.Value("bundle") != "false") || ctx.Form.Has("standalone")
		noBundle := !bundle && (ctx.Form.Has("no-bundle") || ctx.Form.Value("bundle") == "false")
		isDev := ctx.Form.Has("dev")
		isWorker := ctx.Form.Has("worker")
		noCheck := ctx.Form.Has("no-check") || ctx.Form.Has("no-dts")
		ignoreRequire := ctx.Form.Has("ignore-require") || reqPkg.Name == "@unocss/preset-icons"
		keepNames := ctx.Form.Has("keep-names")
		ignoreAnnotations := ctx.Form.Has("ignore-annotations")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((reqPkg.Name == "react" && reqPkg.SubModule == "jsx-dev-runtime") || reqPkg.Name == "react-refresh") {
			isDev = true
		}

		buildArgs := BuildArgs{
			alias:             alias,
			conditions:        conditions,
			denoStdVersion:    dsv,
			deps:              deps,
			external:          external,
			ignoreAnnotations: ignoreAnnotations,
			ignoreRequire:     ignoreRequire,
			keepNames:         keepNames,
			exports:           exports,
		}

		// parse `X-` prefix
		if hasTargetSegmentinPath {
			a := strings.Split(reqPkg.SubModule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				reqPkg.SubModule = strings.Join(a[1:], "/")
				args, err := decodeBuildArgsPrefix(a[0])
				if err != nil {
					return throwErrorJS(ctx, err, false)
				}
				reqPkg.SubPath = strings.Join(strings.Split(reqPkg.SubPath, "/")[1:], "/")
				if args.denoStdVersion == "" {
					// ensure deno/std version used
					args.denoStdVersion = denoStdVersion
				}
				buildArgs = args
			}
		}

		// check if it's a build file
		isBuildFile := false
		if hasTargetSegmentinPath && (endsWith(reqPkg.SubPath, ".mjs", ".js", ".css")) {
			a := strings.Split(reqPkg.SubModule, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
					pkgName := strings.TrimSuffix(path.Base(reqPkg.Name), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(reqPkg.SubPath, ".js") {
						if submodule == pkgName+".css" {
							reqPkg.SubModule = ""
							target = maybeTarget
							isBuildFile = true
						} else {
							url := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, reqPkg.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						if strings.HasSuffix(submodule, ".bundle") {
							submodule = strings.TrimSuffix(submodule, ".bundle")
							bundle = true
						} else if strings.HasSuffix(submodule, ".nobundle") {
							submodule = strings.TrimSuffix(submodule, ".nobundle")
							noBundle = true
						}
						if strings.HasSuffix(submodule, ".development") {
							submodule = strings.TrimSuffix(submodule, ".development")
							isDev = true
						}
						isMjs := strings.HasSuffix(reqPkg.SubPath, ".mjs")
						if strings.HasPrefix(reqPkg.Name, "~") {
							submodule = ""
						} else if isMjs && submodule == pkgName {
							submodule = ""
						}
						// workaround for es5-ext weird "/#/" path
						if submodule != "" && reqPkg.Name == "es5-ext" {
							submodule = strings.ReplaceAll(submodule, "/$$/", "/#/")
						}
						reqPkg.SubModule = submodule
						target = maybeTarget
						isBuildFile = true
					}
				}
			}
		}

		// build and return dts
		if reqType == "types" {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				savePath = path.Join(fmt.Sprintf(
					"types/%s%s/%s@%s/%s",
					getTypesRoot(cdnOrigin),
					ghPrefix,
					reqPkg.Name,
					reqPkg.Version,
					encodeBuildArgsPrefix(buildArgs, reqPkg, true),
				), reqPkg.SubPath)
				if strings.HasSuffix(savePath, "~.d.ts") {
					savePath = strings.TrimSuffix(savePath, "~.d.ts")
					_, err := fs.Stat(path.Join(savePath, "index.d.ts"))
					if err != nil && err != storage.ErrNotFound {
						return "", nil, err
					}
					if err == nil {
						savePath = path.Join(savePath, "index.d.ts")
					} else {
						savePath += ".d.ts"
					}
				}
				fi, err = fs.Stat(savePath)
				return savePath, fi, err
			}
			_, _, err := findDts()
			if err == storage.ErrNotFound {
				task := &BuildTask{
					Args:      buildArgs,
					CdnOrigin: cdnOrigin,
					Pkg:       reqPkg,
					Target:    "types",
				}
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return rex.Status(500, "types: "+output.err.Error())
					}
				case <-time.After(time.Duration(cfg.BuildWaitTimeout) * time.Second):
					buildQueue.RemoveClient(task, c)
					header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
					return rex.Status(http.StatusRequestTimeout, "timeout, we are transforming the types hardly, please try again later!")
				}
			}
			savePath, fi, err := findDts()
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Types not found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := fs.OpenFile(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Content-Type", "application/typescript; charset=utf-8")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, fi.ModTime(), r) // auto closed
		}

		task := &BuildTask{
			Args:      buildArgs,
			CdnOrigin: cdnOrigin,
			Pkg:       reqPkg,
			Target:    target,
			Dev:       isDev,
			Bundle:    bundle,
			NoBundle:  noBundle,
		}

		buildId := task.ID()
		esm, hasBuild := queryESMBuild(buildId)
		if !hasBuild {
			c := buildQueue.Add(task, ctx.RemoteIP())
			select {
			case output := <-c.C:
				if output.err != nil {
					msg := output.err.Error()
					if strings.Contains(msg, "no such file or directory") ||
						strings.Contains(msg, "is not exported from package") {
						// redirect old build path (.js) to new build path (.mjs)
						if strings.HasSuffix(reqPkg.SubPath, "/"+reqPkg.Name+".js") {
							url := strings.TrimSuffix(ctx.R.URL.String(), ".js") + ".mjs"
							return rex.Redirect(url, http.StatusMovedPermanently)
						}
						header.Set("Cache-Control", "public, max-age=31536000, immutable")
						return rex.Status(404, "Module not found")
					}
					if strings.HasSuffix(msg, " not found") {
						return rex.Status(404, msg)
					}
					return throwErrorJS(ctx, output.err, false)
				}
				esm = output.meta
			case <-time.After(time.Duration(cfg.BuildWaitTimeout) * time.Second):
				buildQueue.RemoveClient(task, c)
				header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
				return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
			}
		}

		// should redirect to `*.d.ts` file
		if esm.TypesOnly {
			dtsUrl := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, esm.Dts)
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if isPkgCss && reqPkg.SubModule == "" {
			if !esm.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s/%s.css", cdnOrigin, cfg.CdnBasePath, strings.TrimSuffix(buildId, path.Ext(buildId)))
			return rex.Redirect(url, 301)
		}

		if isBuildFile {
			savePath := task.getSavepath()
			if strings.HasSuffix(reqPkg.SubPath, ".css") {
				base, _ := utils.SplitByLastByte(savePath, '.')
				savePath = base + ".css"
			}
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "File not found")
				}
				return rex.Status(500, err.Error())
			}
			f, err := fs.OpenFile(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			if endsWith(savePath, ".mjs", ".js") {
				header.Set("Content-Type", "application/javascript; charset=utf-8")
				if isWorker {
					moduleUrl := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, buildId)
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
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, EOL)

		if isWorker {
			moduleUrl := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, buildId)
			fmt.Fprintf(buf,
				`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
				moduleUrl,
				moduleUrl,
			)
		} else {
			if len(esm.Deps) > 0 {
				// TODO: lookup deps of deps?
				for _, dep := range esm.Deps {
					if strings.HasPrefix(dep, "/") && cfg.CdnBasePath != "" {
						dep = cfg.CdnBasePath + dep
					}
					fmt.Fprintf(buf, `import "%s";%s`, dep, EOL)
				}
			}
			header.Set("X-Esm-Id", buildId)
			fmt.Fprintf(buf, `export * from "%s/%s";%s`, cfg.CdnBasePath, buildId, EOL)
			if (esm.FromCJS || esm.HasExportDefault) && (exports.Len() == 0 || exports.Has("default")) {
				fmt.Fprintf(buf, `export { default } from "%s/%s";%s`, cfg.CdnBasePath, buildId, EOL)
			}
			if esm.FromCJS && exports.Len() > 0 {
				fmt.Fprintf(buf, `import __cjs_exports$ from "%s/%s";%s`, cfg.CdnBasePath, buildId, EOL)
				fmt.Fprintf(buf, `export const { %s } = __cjs_exports$;%s`, strings.Join(exports.Values(), ", "), EOL)
			}
		}

		if esm.Dts != "" && !noCheck && !isWorker {
			dtsUrl := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, esm.Dts)
			header.Set("X-TypeScript-Types", dtsUrl)
		}
		if targetViaUA {
			addVary(header, "User-Agent")
		}
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
		header.Set("Content-Length", strconv.Itoa(buf.Len()))
		header.Set("Content-Type", "application/javascript; charset=utf-8")
		if ctx.R.Method == http.MethodHead {
			return []byte{}
		}
		return buf
	}
}

func getCdnOrign(ctx *rex.Context) string {
	cdnOrigin := ctx.R.Header.Get("X-Real-Origin")
	if cdnOrigin == "" {
		cdnOrigin = cfg.CdnOrigin
	}
	if cdnOrigin == "" {
		proto := "http"
		if ctx.R.TLS != nil {
			proto = "https"
		}
		// use the request host as the origin if not set in config.json
		cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
	}
	return cdnOrigin
}

func addVary(header http.Header, key string) {
	vary := header.Get("Vary")
	if vary == "" {
		header.Set("Vary", key)
	} else {
		header.Set("Vary", vary+", "+key)
	}
}

func hasTargetSegment(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if targets[part] > 0 {
			return true
		}
	}
	return false
}

func throwErrorJS(ctx *rex.Context, err error, static bool) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(
		buf,
		`throw new Error("[esm.sh] " + %s);%s`,
		strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))),
		"\n",
	)
	fmt.Fprintf(buf, "export default null;\n")
	if static {
		ctx.W.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		ctx.W.Header().Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
	}
	ctx.W.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	return rex.Status(500, buf)
}

func getTypesRoot(cdnOrigin string) string {
	url, err := url.Parse(cdnOrigin)
	if err != nil {
		return "-"
	}
	return strings.ReplaceAll(url.Host, ":", "_")
}
