package server

import (
	"bytes"
	"fmt"
	"io"
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
		pathname := ctx.Path.String()
		userAgent := ctx.R.UserAgent()
		header := ctx.W.Header()
		cdnOrigin := getCdnOrign(ctx)

		// ban malicious requests
		if strings.HasPrefix(pathname, ".") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		CTX_BUILD_VERSION := VERSION
		if v := ctx.R.Header.Get("X-Esm-Worker-Version"); v != "" && strings.HasPrefix(v, "v") {
			i, e := strconv.Atoi(v[1:])
			if e == nil && i > 0 {
				CTX_BUILD_VERSION = i
			}
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

		if userAgent == "undici" || strings.HasPrefix(userAgent, "Node/") || strings.HasPrefix(userAgent, "Deno/") || strings.HasPrefix(userAgent, "Bun/") {
			if pathname == "/" || regexpCliPath.MatchString(pathname) {
				if strings.HasPrefix(userAgent, "Deno/") {
					cliTs, err := embedFS.ReadFile("server/embed/CLI.deno.ts")
					if err != nil {
						return err
					}
					header.Set("Content-Type", "application/typescript; charset=utf-8")
					return bytes.ReplaceAll(cliTs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", CTX_BUILD_VERSION)))
				}
				if userAgent == "undici" || strings.HasPrefix(userAgent, "Node/") || strings.HasPrefix(userAgent, "Bun/") {
					cliJs, err := embedFS.ReadFile("server/embed/CLI.node.js")
					if err != nil {
						return err
					}
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					cliJs = bytes.ReplaceAll(cliJs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", CTX_BUILD_VERSION)))
					return bytes.ReplaceAll(cliJs, []byte("https://esm.sh"), []byte(cdnOrigin+cfg.CdnBasePath))
				}
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
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/master/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte("{origin}"+cfg.CdnBasePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", CTX_BUILD_VERSION)))
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
						"bundle":    t.BundleDeps,
						"bv":        t.BuildVersion,
						"consumers": t.consumers,
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

			n := 0
			purgeTimers.Range(func(key, value interface{}) bool {
				n++
				return true
			})

			nsStatus := "IOERROR"
			client := &http.Client{Timeout: 2 * time.Second}
			res, err := client.Get(fmt.Sprintf("http://localhost:%d", cfg.NsPort))
			if err == nil {
				out, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err == nil {
					nsStatus = string(out)
				}
			}
			if nsStatus != "READY" {
				// whoops, can't connect to node service,
				// kill current process for getting new one
				kill(nsPidFile)
			}

			header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
			return map[string]interface{}{
				"buildQueue":  q[:i],
				"purgeTimers": n,
				"ns":          nsStatus,
				"version":     CTX_BUILD_VERSION,
				"uptime":      time.Since(startTime).String(),
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

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile("server" + pathname)
			if err != nil {
				return err
			}
			if strings.HasSuffix(pathname, ".js") {
				data = bytes.ReplaceAll(data, []byte("{origin}"), []byte(cdnOrigin))
				data = bytes.ReplaceAll(data, []byte("{basePath}"), []byte(cfg.CdnBasePath))
			}
			header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
			return rex.Content(pathname, startTime, bytes.NewReader(data))
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
		}

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
			header.Add("Vary", "User-Agent")
			return rex.Content(savaPath, fi.ModTime(), r) // auto closed
		}

		var hasBuildVerPrefix bool
		var hasStablePrefix bool
		var outdatedBuildVer string

		// check build version prefix
		buildBasePath := fmt.Sprintf("/v%d", CTX_BUILD_VERSION)
		if strings.HasPrefix(pathname, "/stable/") {
			pathname = strings.TrimPrefix(pathname, "/stable")
			hasBuildVerPrefix = true
			hasStablePrefix = true
		} else if strings.HasPrefix(pathname, buildBasePath+"/") || pathname == buildBasePath {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			// Otherwise check possible fixed version
		} else if regexpBuildVersionPath.MatchString(pathname) {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			outdatedBuildVer = a[1]
		}

		if pathname == "/build" || pathname == "/run" || pathname == "/hot" || strings.HasPrefix(pathname, "/hot-features/") {
			if !hasBuildVerPrefix && !ctx.Form.Has("pin") {
				url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, CTX_BUILD_VERSION, pathname)
				return rex.Redirect(url, http.StatusFound)
			}
			name := pathname[1:]
			if strings.HasPrefix(name, "hot-features/") {
				name = "server/embed/" + name
			}
			data, err := embedFS.ReadFile(fmt.Sprintf("%s.ts", name))
			if err != nil {
				return rex.Status(404, err.Error())
			}

			if pathname == "/hot" {
				features := strings.Split(ctx.R.URL.RawQuery, "+")
				for _, name := range features {
					if name == "tsx" || name == "vue" {
						data = bytes.ReplaceAll(
							data,
							[]byte(fmt.Sprintf(`const %s = featureDisabled("%s");`, name, name)),
							[]byte(fmt.Sprintf(`import %s from "%s%s/v%d/hot-features/%s";`, name, cdnOrigin, cfg.CdnBasePath, CTX_BUILD_VERSION, name)),
						)
					}
				}
			}

			target := getBuildTargetByUA(userAgent)
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
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			header.Add("Vary", "User-Agent")
			return data
		}

		// virtual file for esm.sh/hot
		if strings.HasPrefix(pathname, "/hot/") {
			placeholder := []byte{}
			_, ext := utils.SplitByLastByte(pathname, '.')
			switch ext {
			case "css":
				placeholder = []byte(".hot-app{visibility:hidden;}")
			case "json":
				placeholder = []byte("null")
			}
			return rex.Content(pathname[5:], time.Now(), bytes.NewReader(placeholder))
		}

		if pathname == "/server" {
			if !hasBuildVerPrefix && !ctx.Form.Has("pin") {
				url := fmt.Sprintf("%s%s/v%d/server", cdnOrigin, cfg.CdnBasePath, CTX_BUILD_VERSION)
				return rex.Redirect(url, http.StatusFound)
			}
			var data []byte
			var err error
			cType := "application/javascript; charset=utf-8"
			if strings.HasPrefix(userAgent, "Deno/") {
				data, err = embedFS.ReadFile("server/embed/server.deno.ts")
				if err != nil {
					return err
				}
				cType = "application/typescript; charset=utf-8"
			} else if userAgent == "undici" || strings.HasPrefix(userAgent, "Node/") || strings.HasPrefix(userAgent, "Bun/") {
				data, err = embedFS.ReadFile("server/embed/server.node.js")
				if err != nil {
					return err
				}
			} else {
				data = []byte("/* esm.sh - error */\nconsole.error('esm.sh server is not supported in browser environment.');")
			}
			header.Set("Content-Type", cType)
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			return data
		}

		// use embed polyfills/types if possible
		if hasBuildVerPrefix && strings.Count(pathname, "/") == 1 {
			if strings.HasSuffix(pathname, ".js") {
				data, err := embedFS.ReadFile("server/embed/polyfills" + pathname)
				if err == nil {
					target := getBuildTargetByUA(userAgent)
					code, err := minify(string(data), targets[target], api.LoaderJS)
					if err != nil {
						return throwErrorJS(ctx, fmt.Errorf("transform error: %v", err), false)
					}
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
					header.Add("Vary", "User-Agent")
					return rex.Content(pathname, startTime, bytes.NewReader(code))
				}
			}
			if strings.HasSuffix(pathname, ".d.ts") {
				data, err := embedFS.ReadFile("server/embed/types" + pathname)
				if err == nil {
					header.Set("Content-Type", "application/typescript; charset=utf-8")
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
					return rex.Content(pathname, startTime, bytes.NewReader(data))
				}
			}
		}

		// ban malicious requests by banList
		// trim the leading `/` in pathname to get the package name
		// e.g. /@ORG/PKG -> @ORG/PKG
		packageFullName := pathname[1:]
		pkgAllowed := cfg.AllowList.IsPackageAllowed(packageFullName)
		pkgBanned := cfg.BanList.IsPackageBanned(packageFullName)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		external := newStringSet()
		// check `/*pathname`
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

		if reqPkg.Name == "apps" {
			// resvered package name
			return rex.Status(404, "not found")
		}

		// fix url related `import.meta.url`
		if hasBuildVerPrefix && endsWith(reqPkg.Subpath, ".wasm", ".json") {
			extname := path.Ext(reqPkg.Subpath)
			dir := path.Join(cfg.WorkDir, "npm", reqPkg.Name+"@"+reqPkg.Version)
			if !dirExists(dir) {
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
					if strings.HasSuffix(reqPkg.Subpath, f) {
						file = f
						break
					}
				}
				if file == "" {
					for _, f := range files {
						if path.Base(reqPkg.Subpath) == path.Base(f) {
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
		if strings.HasPrefix(reqPkg.Name, "@types/") && (reqPkg.Submodule == "" || !strings.HasSuffix(reqPkg.Submodule, ".d.ts")) {
			url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, CTX_BUILD_VERSION, pathname)
			if reqPkg.Submodule == "" {
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
		if css := cssPackages[reqPkg.Name]; css != "" && reqPkg.Submodule == "" {
			url := fmt.Sprintf("%s%s/%s/%s", cdnOrigin, cfg.CdnBasePath, reqPkg.String(), css)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// use extra query like `/react-dom@18.2.0&external=react&dev/client`
		if extraQuery != "" {
			qs := []string{extraQuery}
			if ctx.R.URL.RawQuery != "" {
				qs = append(qs, ctx.R.URL.RawQuery)
			}
			ctx.R.URL.RawQuery = strings.Join(qs, "&")
		}

		ghPrefix := ""
		if reqPkg.FromGithub {
			ghPrefix = "/gh"
		}

		// redirect to the url with full package version
		if !hasBuildVerPrefix && !reqPkg.FromEsmsh && !strings.HasPrefix(pathname, fmt.Sprintf("%s/%s@%s", ghPrefix, reqPkg.Name, reqPkg.Version)) {
			bvPrefix := ""
			eaSign := ""
			subPath := ""
			query := ""
			if endsWith(pathname, ".d.ts", ".d.mts") {
				if outdatedBuildVer != "" {
					bvPrefix = fmt.Sprintf("/%s", outdatedBuildVer)
				} else {
					bvPrefix = fmt.Sprintf("/v%d", CTX_BUILD_VERSION)
				}
			}
			if external.Has("*") {
				eaSign = "*"
			}
			if reqPkg.Subpath != "" {
				subPath = "/" + reqPkg.Subpath
			}
			if ctx.R.URL.RawQuery != "" {
				if extraQuery != "" {
					query = "&" + ctx.R.URL.RawQuery
					return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, ghPrefix, eaSign, reqPkg.Name, reqPkg.Version, query, subPath), http.StatusFound)
				}
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, ghPrefix, eaSign, reqPkg.Name, reqPkg.Version, subPath, query), http.StatusFound)
		}

		// redirect to the url with full package version with build version prefix
		if hasBuildVerPrefix && !strings.HasPrefix(pathname, fmt.Sprintf("%s/%s@%s", ghPrefix, reqPkg.Name, reqPkg.Version)) {
			bvPrefix := ""
			subPath := ""
			query := ""
			if hasBuildVerPrefix {
				if stableBuild[reqPkg.Name] {
					bvPrefix = "/stable"
				} else if outdatedBuildVer != "" {
					bvPrefix = fmt.Sprintf("/%s", outdatedBuildVer)
				} else {
					bvPrefix = fmt.Sprintf("/v%d", CTX_BUILD_VERSION)
				}
			}
			if reqPkg.Subpath != "" {
				subPath = "/" + reqPkg.Subpath
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s/%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, reqPkg.VersionName(), subPath, query), http.StatusFound)
		}

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"jsx-runtime", "jsx-dev-runtime"} {
			if strings.HasSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime) {
				if reqPkg.Submodule == "" {
					reqPkg.Submodule = jsxRuntime
				} else {
					reqPkg.Submodule = reqPkg.Submodule + "/" + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", reqPkg.Name, reqPkg.Submodule)
				ctx.R.URL.RawQuery = strings.TrimSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime)
			}
		}

		// or use `?path=$PATH` query to override the pathname
		if v := ctx.Form.Value("path"); v != "" {
			reqPkg.Submodule = utils.CleanPath(v)[1:]
		}

		var reqType string
		if reqPkg.Subpath != "" {
			ext := path.Ext(reqPkg.Subpath)
			switch ext {
			case ".mjs", ".js", ".jsx", ".ts", ".mts", ".tsx":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					if !hasBuildVerPrefix {
						url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, CTX_BUILD_VERSION, pathname)
						return rex.Redirect(url, http.StatusMovedPermanently)
					}
					reqType = "types"
				} else if ctx.R.URL.Query().Has("raw") {
					reqType = "raw"
				} else if hasBuildVerPrefix && hasTargetSegment(reqPkg.Subpath) {
					reqType = "builds"
				}
			case ".wasm":
				if ctx.Form.Has("module") {
					buf := &bytes.Buffer{}
					wasmUrl := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.CdnBasePath, pathname)
					fmt.Fprintf(buf, "/* esm.sh - CompiledWasm */\n")
					fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					return buf
				} else {
					reqType = "raw"
				}
			case ".css", ".map":
				if hasBuildVerPrefix && hasTargetSegment(reqPkg.Subpath) {
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
			savePath := path.Join(cfg.WorkDir, installDir, "node_modules", reqPkg.Name, reqPkg.Subpath)
			fi, err := os.Lstat(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				task := &BuildTask{
					CdnOrigin: cdnOrigin,
					Pkg:       reqPkg,
					Args: BuildArgs{
						alias:      map[string]string{},
						deps:       PkgSlice{},
						external:   newStringSet(),
						exports:    newStringSet(),
						conditions: newStringSet(),
					},
					Target: "raw",
				}
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return rex.Status(500, "Fail to install package: "+output.err.Error())
					}
					fi, err = os.Lstat(savePath)
					if err != nil {
						if os.IsExist(err) {
							return rex.Status(500, err.Error())
						}
						return rex.Status(404, "File Not Found")
					}
				case <-time.After(10 * time.Minute):
					buildQueue.RemoveConsumer(task, c)
					header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
					return rex.Status(http.StatusRequestTimeout, "timeout, we are downloading package hardly, please try again later!")
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
		if hasBuildVerPrefix && (reqType == "builds" || reqType == "types") {
			var savePath string
			if outdatedBuildVer != "" {
				savePath = path.Join(reqType, outdatedBuildVer, pathname)
			} else if hasStablePrefix {
				savePath = path.Join(reqType, fmt.Sprintf("v%d", STABLE_VERSION), pathname)
			} else {
				savePath = path.Join(reqType, fmt.Sprintf("v%d", CTX_BUILD_VERSION), pathname)
			}
			if reqType == "types" {
				savePath = path.Join("types", getTypesRoot(cdnOrigin), strings.TrimPrefix(savePath, "types/"))
			}
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
				r, err := fs.OpenFile(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if reqType == "types" {
					header.Set("Content-Type", "application/typescript; charset=utf-8")
				} else if endsWith(pathname, ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx") {
					header.Set("Content-Type", "application/javascript; charset=utf-8")
				} else if strings.HasSuffix(savePath, ".map") {
					header.Set("Content-Type", "application/json; charset=utf-8")
				}
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
				if ctx.Form.Has("worker") && reqType == "builds" {
					defer r.Close()
					buf, err := io.ReadAll(r)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					code := bytes.TrimSuffix(buf, []byte(fmt.Sprintf(`//# sourceMappingURL=%s.map`, path.Base(savePath))))
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					return fmt.Sprintf(`export default function workerFactory(inject) { const blob = new Blob([%s, typeof inject === "string" ? "\n// inject\n" + inject : ""], { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module" })}`, utils.MustEncodeJSON(string(code)))
				}
				return rex.Content(savePath, fi.ModTime(), r) // auto closed
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		target := strings.ToLower(ctx.Form.Value("target"))
		targetFromUA := targets[target] == 0
		if targetFromUA {
			target = getBuildTargetByUA(userAgent)
		}
		if strings.HasPrefix(target, "es") && includes(nativeNodePackages, reqPkg.Name) {
			return throwErrorJS(ctx, fmt.Errorf(
				`unsupported npm package "%s": native node module is not supported in browser`,
				reqPkg.Name,
			), false)
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
		if (ctx.Form.Has("exports") || ctx.Form.Has("cjs-exports")) && !stableBuild[reqPkg.Name] {
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

		// check build version
		buildVersion := CTX_BUILD_VERSION
		pv := outdatedBuildVer
		if outdatedBuildVer == "" {
			pv = ctx.Form.Value("pin")
		}
		if pv != "" && strings.HasPrefix(pv, "v") {
			i, err := strconv.Atoi(pv[1:])
			if err == nil && i > 0 && i < CTX_BUILD_VERSION {
				buildVersion = i
			}
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
		bundleDeps := (ctx.Form.Has("bundle") || ctx.Form.Has("standalone") || ctx.Form.Has("bundle-deps")) && !stableBuild[reqPkg.Name]
		noBundle := !bundleDeps && (ctx.Form.Has("bundless") || ctx.Form.Has("no-bundle")) && !stableBuild[reqPkg.Name]
		isDev := ctx.Form.Has("dev")
		isPined := ctx.Form.Has("pin") || hasBuildVerPrefix || stableBuild[reqPkg.Name]
		isWorker := ctx.Form.Has("worker")
		noCheck := ctx.Form.Has("no-check") || ctx.Form.Has("no-dts")
		ignoreRequire := ctx.Form.Has("ignore-require") || reqPkg.Name == "@unocss/preset-icons"
		keepNames := ctx.Form.Has("keep-names")
		ignoreAnnotations := ctx.Form.Has("ignore-annotations")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((reqPkg.Name == "react" && reqPkg.Submodule == "jsx-dev-runtime") || reqPkg.Name == "react-refresh") {
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

		// parse and use `X-` prefix
		if hasBuildVerPrefix {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				reqPkg.Submodule = strings.Join(a[1:], "/")
				args, err := decodeBuildArgsPrefix(a[0])
				if err != nil {
					return throwErrorJS(ctx, err, false)
				}
				reqPkg.Subpath = strings.Join(strings.Split(reqPkg.Subpath, "/")[1:], "/")
				if args.denoStdVersion == "" {
					// ensure deno/std version used
					args.denoStdVersion = denoStdVersion
				}
				buildArgs = args
			}
		}

		// clear build args for main entry of stable builds
		if stableBuild[reqPkg.Name] && reqPkg.Submodule == "" {
			buildArgs = BuildArgs{
				external:   newStringSet(),
				exports:    newStringSet(),
				conditions: buildArgs.conditions,
			}
		}

		// check if it's build path
		isBarePath := false
		if hasBuildVerPrefix && (endsWith(reqPkg.Subpath, ".mjs", ".js", ".css")) {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
					pkgName := strings.TrimSuffix(path.Base(reqPkg.Name), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(reqPkg.Subpath, ".js") {
						if submodule == pkgName+".css" {
							reqPkg.Submodule = ""
							target = maybeTarget
							isBarePath = true
						} else {
							url := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, reqPkg.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						if strings.HasSuffix(submodule, ".bundle") {
							submodule = strings.TrimSuffix(submodule, ".bundle")
							bundleDeps = true
						} else if strings.HasSuffix(submodule, ".bundless") {
							submodule = strings.TrimSuffix(submodule, ".bundless")
							noBundle = true
						}
						if strings.HasSuffix(submodule, ".development") {
							submodule = strings.TrimSuffix(submodule, ".development")
							isDev = true
						}
						isMjs := strings.HasSuffix(reqPkg.Subpath, ".mjs")
						// fix old build `/stable/react/deno/react.js` to `/stable/react/deno/react.mjs`
						if !isMjs && submodule == pkgName && stableBuild[reqPkg.Name] {
							url := fmt.Sprintf(
								"%s%s/stable/%s@%s/%s/%s.mjs",
								cdnOrigin,
								cfg.CdnBasePath,
								reqPkg.Name,
								reqPkg.Version,
								maybeTarget,
								reqPkg.Name,
							)
							return rex.Redirect(url, http.StatusMovedPermanently)
						}
						if strings.HasPrefix(reqPkg.Name, "~") {
							submodule = ""
						} else if isMjs && submodule == pkgName {
							submodule = ""
						}
						// workaround for es5-ext weird "/#/" path
						if submodule != "" && reqPkg.Name == "es5-ext" {
							submodule = strings.ReplaceAll(submodule, "/$$/", "/#/")
						}
						reqPkg.Submodule = submodule
						target = maybeTarget
						isBarePath = true
					}
				}
			}
		}

		// build and return dts
		if hasBuildVerPrefix && reqType == "types" {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				savePath = path.Join(fmt.Sprintf(
					"types/%s/v%d%s/%s@%s/%s",
					getTypesRoot(cdnOrigin),
					buildVersion,
					ghPrefix,
					reqPkg.Name,
					reqPkg.Version,
					encodeBuildArgsPrefix(buildArgs, reqPkg, true),
				), reqPkg.Subpath)
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
					Args:         buildArgs,
					CdnOrigin:    cdnOrigin,
					BuildVersion: buildVersion,
					Pkg:          reqPkg,
					Target:       "types",
				}
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return rex.Status(500, "types: "+output.err.Error())
					}
				case <-time.After(10 * time.Minute):
					buildQueue.RemoveConsumer(task, c)
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
			Args:         buildArgs,
			CdnOrigin:    cdnOrigin,
			BuildVersion: buildVersion,
			Pkg:          reqPkg,
			Target:       target,
			Dev:          isDev,
			BundleDeps:   bundleDeps || isWorker,
			NoBundle:     noBundle,
		}

		buildId := task.ID()
		esm, hasBuild := queryESMBuild(buildId)
		fallback := false

		if !hasBuild {
			if !isBarePath && !isPined {
				// find previous build version
				for i := 0; i < CTX_BUILD_VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", CTX_BUILD_VERSION-(i+1), strings.Join(strings.Split(buildId, "/")[1:], "/"))
					esm, hasBuild = queryESMBuild(id)
					if hasBuild {
						log.Warn("fallback to previous build:", id)
						fallback = true
						buildId = id
						break
					}
				}
			}

			// if the previous build exists and is not pin/bare mode, then build current module in backgound,
			// or wait the current build task for 60 seconds
			if esm != nil {
				buildQueue.Add(task, "")
			} else {
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						if m := output.err.Error(); strings.Contains(m, "no such file or directory") ||
							strings.Contains(m, "is not exported from package") {
							// redirect old build path (.js) to new build path (.mjs)
							if strings.HasSuffix(reqPkg.Subpath, "/"+reqPkg.Name+".js") {
								url := strings.TrimSuffix(ctx.R.URL.String(), ".js") + ".mjs"
								return rex.Redirect(url, http.StatusMovedPermanently)
							}
							header.Set("Cache-Control", "public, max-age=31536000, immutable")
							return rex.Status(404, "Module not found")
						}
						return throwErrorJS(ctx, output.err, false)
					}
					esm = output.meta
				case <-time.After(10 * time.Minute):
					buildQueue.RemoveConsumer(task, c)
					header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
					return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
				}
			}
		}

		// should redirect to `*.d.ts` file
		if esm.TypesOnly {
			dtsUrl := fmt.Sprintf(
				"%s%s/%s",
				cdnOrigin,
				cfg.CdnBasePath,
				strings.TrimPrefix(esm.Dts, "/"),
			)
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			if fallback {
				header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
			} else {
				if isPined {
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 24*3600)) // cache for 24 hours
				}
			}
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if isPkgCss && reqPkg.Submodule == "" {
			if !esm.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s/%s.css", cdnOrigin, cfg.CdnBasePath, strings.TrimSuffix(buildId, path.Ext(buildId)))
			code := 302
			if isPined {
				code = 301
			}
			return rex.Redirect(url, code)
		}

		if isBarePath {
			savePath := task.getSavepath()
			if strings.HasSuffix(reqPkg.Subpath, ".css") {
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
			if isWorker && endsWith(savePath, ".mjs", ".js") {
				buf, err := io.ReadAll(f)
				f.Close()
				if err != nil {
					return rex.Status(500, err.Error())
				}
				code := bytes.TrimSuffix(buf, []byte(fmt.Sprintf(`//# sourceMappingURL=%s.map`, path.Base(savePath))))
				header.Set("Content-Type", "application/javascript; charset=utf-8")
				return fmt.Sprintf(`export default function workerFactory(inject) { const blob = new Blob([%s, typeof inject === "string" ? "\n// inject\n" + inject : ""], { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module" })}`, utils.MustEncodeJSON(string(code)))
			}
			if endsWith(savePath, ".mjs", ".js") {
				header.Set("Content-Type", "application/javascript; charset=utf-8")
			}
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, EOL)

		if isWorker {
			fmt.Fprintf(buf, `export { default } from "%s/%s?worker";`, cfg.CdnBasePath, buildId)
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
			dtsUrl := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.CdnBasePath, esm.Dts)
			header.Set("X-TypeScript-Types", dtsUrl)
		}
		if fallback {
			header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
		} else {
			if isPined {
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 24*3600)) // cache for 24 hours
			}
		}
		if targetFromUA {
			header.Add("Vary", "User-Agent")
		}
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
