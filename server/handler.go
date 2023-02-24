package server

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ije/esm.sh/server/storage"

	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

// esm.sh query middleware for rex
func esmHandler() rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()

		// ban malicious requests
		if strings.HasPrefix(pathname, ".") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// Build prefix may only be served from "${cfg.BasePath}/..."
		if cfg.BasePath != "" {
			if strings.HasPrefix(pathname, cfg.BasePath) {
				pathname = strings.TrimPrefix(pathname, cfg.BasePath)
			} else {
				url := strings.TrimPrefix(ctx.R.URL.String(), cfg.BasePath)
				url = fmt.Sprintf("%s/%s", cfg.BasePath, url)
				return rex.Redirect(url, http.StatusFound)
			}
		}

		// static routes
		switch pathname {
		case "/":
			// return deno cli script if the `User-Agent` is "Deno"
			if strings.HasPrefix(ctx.R.UserAgent(), "Deno/") {
				cliTs, err := embedFS.ReadFile("server/embed/deno_cli.ts")
				if err != nil {
					return err
				}
				ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				return bytes.ReplaceAll(cliTs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", VERSION)))
			}
			indexHTML, err := embedFS.ReadFile("server/embed/index.html")
			if err != nil {
				return err
			}
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte(cfg.BasePath+"/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/ije/esm.sh/blob/master/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte("{origin}"+cfg.BasePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			html = bytes.ReplaceAll(html, []byte("{basePath}"), []byte(cfg.BasePath))
			ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			buildQueue.lock.RLock()
			q := make([]map[string]interface{}, buildQueue.list.Len())
			i := 0
			for el := buildQueue.list.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*queueTask)
				if ok {
					m := map[string]interface{}{
						"stage":      t.stage,
						"createTime": t.createTime.Format(http.TimeFormat),
						"consumers":  t.consumers,
						"pkg":        t.Pkg.String(),
						"target":     t.Target,
						"inProcess":  t.inProcess,
						"devMode":    t.DevMode,
						"bundleMode": t.BundleMode,
					}
					if !t.startTime.IsZero() {
						m["startTime"] = t.startTime.Format(http.TimeFormat)
					}
					if len(t.deps) > 0 {
						m["deps"] = t.deps.String()
					}
					q[i] = m
					i++
				}
			}
			buildQueue.lock.RUnlock()
			res, err := http.Get(fmt.Sprintf("http://localhost:%d", cfg.NsPort))
			if err != nil {
				kill(nsPidFile)
				return err
			}
			defer res.Body.Close()
			out, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return err
			}
			ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
			return map[string]interface{}{
				"ns":         string(out),
				"uptime":     time.Since(startTime).String(),
				"buildQueue": q[:i],
			}

		case "/build-target":
			return getTargetByUA(ctx.R.UserAgent())

		case "/error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Errorf(
					`Could not resolve "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			case "unsupported-nodejs-builtin-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`Unsupported nodejs builtin module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			default:
				return throwErrorJS(ctx, fmt.Errorf("Unknown error"))
			}

		case "/favicon.ico":
			return rex.Status(404, "not found")
		}

		proto := "http"
		if ctx.R.TLS != nil {
			proto = "https"
		}
		reqOrigin := fmt.Sprintf("%s://%s", proto, ctx.R.Host)

		var cdnOrigin string
		if cfg.Origin != "" {
			cdnOrigin = strings.TrimSuffix(cfg.Origin, "/")
		} else {
			cdnOrigin = reqOrigin
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile("server" + pathname)
			if err == nil {
				if strings.HasSuffix(pathname, ".js") {
					data = bytes.ReplaceAll(data, []byte("{origin}"), []byte(cdnOrigin))
					data = bytes.ReplaceAll(data, []byte("{basePath}"), []byte(cfg.BasePath))
				}
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
		}

		var hasBuildVerPrefix bool
		var hasStablePrefix bool
		var outdatedBuildVer string

		// check build version prefix
		buildBasePath := fmt.Sprintf("/v%d", VERSION)
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

		// ban malicious requests by banList
		// trim the leading `/` in pathname to get the package name
		// e.g. /@withfig/autocomplete -> @withfig/autocomplete
		packageFullName := pathname[1:]
		if cfg.BanList.IsPackageBanned(packageFullName) {
			return rex.Status(403, "forbidden")
		}

		if strings.HasPrefix(ctx.R.UserAgent(), "Deno/") && pathname == "/" {
			cliTs, err := embedFS.ReadFile("server/embed/deno_cli.ts")
			if err != nil {
				return err
			}
			ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
			return bytes.ReplaceAll(cliTs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", VERSION)))
		}

		external := newStringSet()
		// check `/*pathname`
		if strings.HasPrefix(pathname, "/*") {
			external.Add("*")
			pathname = "/" + pathname[2:]
		}

		// use embed polyfills/types if possible
		if hasBuildVerPrefix {
			data, err := embedFS.ReadFile("server/embed/polyfills" + pathname)
			if err == nil {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
			data, err = embedFS.ReadFile("server/embed/types" + pathname)
			if err == nil {
				ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
		}

		// get package info
		reqPkg, unQuery, err := validatePkgPath(pathname)
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

		// redirect `/@types/` to `.d.ts` files
		if strings.HasPrefix(reqPkg.Name, "@types/") && (reqPkg.Submodule == "" || !strings.HasSuffix(reqPkg.Submodule, ".d.ts")) {
			url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.BasePath, VERSION, pathname)
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
			return rex.Redirect(url, http.StatusFound)
		}

		// redirect to css for CSS packages
		css := cssPackages[reqPkg.Name]
		if css != "" && reqPkg.Submodule == "" {
			url := fmt.Sprintf("%s%s/%s/%s", cdnOrigin, cfg.BasePath, reqPkg.String(), css)
			return rex.Redirect(url, http.StatusFound)
		}

		// support `/react-dom@18.2.0&external=react&dev/client` with query `external=react&dev`
		if unQuery != "" {
			qs := []string{unQuery}
			if ctx.R.URL.RawQuery != "" {
				qs = append(qs, ctx.R.URL.RawQuery)
			}
			ctx.R.URL.RawQuery = strings.Join(qs, "&")
		}

		// redirect to the url with full package version
		if !hasBuildVerPrefix && !strings.HasPrefix(pathname, fmt.Sprintf("/%s@%s", reqPkg.Name, reqPkg.Version)) {
			eaSign := ""
			query := ""
			if external.Has("*") {
				eaSign = "*"
			}
			if unQuery != "" {
				submodule := ""
				if reqPkg.Submodule != "" {
					submodule = "/" + reqPkg.Submodule
				}
				if ctx.R.URL.RawQuery != "" {
					query = "&" + ctx.R.URL.RawQuery
				}
				return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, cfg.BasePath, eaSign, reqPkg.Name, reqPkg.Version, query, submodule), http.StatusFound)
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s/%s%s%s", cdnOrigin, cfg.BasePath, eaSign, reqPkg.String(), query), http.StatusFound)
		}

		// redirect to the url with full package version
		if hasBuildVerPrefix && !strings.HasPrefix(pathname, fmt.Sprintf("/%s@%s", reqPkg.Name, reqPkg.Version)) {
			prefix := ""
			subpath := ""
			query := ""
			if hasBuildVerPrefix {
				if stableBuild[reqPkg.Name] {
					prefix = "/stable"
				} else if outdatedBuildVer != "" {
					prefix = fmt.Sprintf("/%s", outdatedBuildVer)
				} else {
					prefix = fmt.Sprintf("/v%d", VERSION)
				}
			}
			a := strings.Split(pathname, "/")
			if strings.HasPrefix(reqPkg.Name, "@") {
				subpath = strings.Join(a[3:], "/")
			} else {
				subpath = strings.Join(a[2:], "/")
			}
			if subpath != "" {
				subpath = "/" + subpath
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s/%s@%s%s%s", cdnOrigin, cfg.BasePath, prefix, reqPkg.Name, reqPkg.Version, subpath, query), http.StatusFound)
		}

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` for jsx transformer
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

		var storageType string
		if reqPkg.Submodule != "" {
			switch path.Ext(pathname) {
			case ".js":
				if hasBuildVerPrefix {
					storageType = "builds"
				}
			case ".css", ".map":
				if hasBuildVerPrefix {
					storageType = "builds"
				} else if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			case ".jsx", ".ts", ".tsx":
				if hasBuildVerPrefix && strings.HasSuffix(pathname, ".d.ts") {
					storageType = "types"
				} else if len(strings.Split(pathname, "/")) > 2 {
					// todo: transform ts/jsx/tsx for browsers
					storageType = "raw"
				}
			case ".json", ".less", ".sass", ".scss", ".wasm", ".xml", ".yaml", ".md", ".svg", ".png", ".jpg", ".webp", ".gif", ".eot", ".ttf", ".otf", ".woff", ".woff2":
				if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			}
		}

		// serve raw dist files like CSS that is fetching from npmCDN
		if storageType == "raw" {
			if !regexpFullVersionPath.MatchString(pathname) {
				url := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.BasePath, reqPkg.String())
				return rex.Redirect(url, http.StatusFound)
			}

			savePath := path.Join("raw", reqPkg.String())
			_, err := fs.Stat(savePath)
			if err != nil && err != storage.ErrNotFound {
				return rex.Status(500, err.Error())
			}

			// fetch non-js file from npmCDN and save it to fs
			if err == storage.ErrNotFound {
				resp, err := httpClient.Get(fmt.Sprintf("%s/%s", strings.TrimSuffix(cfg.NpmCDN, "/"), reqPkg.String()))
				if err != nil && cfg.BackupNpmCDN != "" {
					resp, err = httpClient.Get(fmt.Sprintf("%s/%s", strings.TrimSuffix(cfg.BackupNpmCDN, "/"), reqPkg.String()))
				}
				if err != nil {
					return rex.Status(500, err.Error())
				}
				defer resp.Body.Close()

				if resp.StatusCode >= 500 {
					b, err := io.ReadAll(resp.Body)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					return rex.Status(http.StatusBadGateway, b)
				}

				if resp.StatusCode >= 400 {
					if resp.StatusCode == 404 {
						return rex.Status(404, "Not Found")
					}
					return rex.Status(http.StatusBadRequest, "Bad Request")
				}

				_, err = fs.WriteFile(savePath, resp.Body)
				if err != nil {
					return rex.Status(500, err.Error())
				}
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
			if strings.HasSuffix(pathname, ".ts") {
				ctx.SetHeader("Content-Type", "application/typescript")
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		// serve build files
		if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
			var savePath string
			if outdatedBuildVer != "" {
				savePath = path.Join(storageType, outdatedBuildVer, pathname)
			} else if hasStablePrefix {
				savePath = path.Join(storageType, "stable", pathname)
			} else {
				savePath = path.Join(storageType, fmt.Sprintf("v%d", VERSION), pathname)
			}

			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound && strings.HasSuffix(pathname, ".js.map") {
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
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				if ctx.Form.Has("worker") && storageType == "builds" {
					defer r.Close()
					code, err := ioutil.ReadAll(r)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
					return fmt.Sprintf(`export default function workerFactory(inject) { const blob = new Blob([%s, typeof inject === "string" ? "\n// inject\n" + inject : ""], { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module" })}`, utils.MustEncodeJSON(string(code)))
				}
				return rex.Content(savePath, fi.ModTime(), r) // auto closed
			}
		}

		// check `?alias` query
		alias := map[string]string{}
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

		// check `?deps` query
		deps := PkgSlice{}
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

		// check `?exports` query
		treeShaking := newStringSet()
		for _, p := range strings.Split(ctx.Form.Value("exports"), ",") {
			p = strings.TrimSpace(p)
			if regexpJSIdent.MatchString(p) {
				treeShaking.Add(p)
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		target := strings.ToLower(ctx.Form.Value("target"))
		targetFromUA := targets[target] == 0
		if targetFromUA {
			target = getTargetByUA(ctx.R.UserAgent())
		}

		// check build version
		buildVersion := VERSION
		pv := outdatedBuildVer
		if outdatedBuildVer == "" {
			pv = ctx.Form.Value("pin")
		}
		if pv != "" && strings.HasPrefix(pv, "v") {
			i, err := strconv.Atoi(pv[1:])
			if err == nil && i > 0 && i < VERSION {
				buildVersion = i
			}
		}

		// check deno/std version by `?deno-std=VER` query
		dsv := denoStdVersion
		fv := ctx.Form.Value("deno-std")
		if fv != "" && regexpFullVersion.MatchString(fv) && target == "deno" {
			dsv = fv
		}

		isBare := false
		isPkgCss := ctx.Form.Has("css")
		isBundleMode := ctx.Form.Has("bundle")
		isDev := ctx.Form.Has("dev")
		isPined := ctx.Form.Has("pin") || hasBuildVerPrefix
		isWorker := ctx.Form.Has("worker")
		noCheck := ctx.Form.Has("no-check") || ctx.Form.Has("no-dts")
		ignoreRequire := ctx.Form.Has("ignore-require") || ctx.Form.Has("no-require") || reqPkg.Name == "@unocss/preset-icons"
		keepNames := ctx.Form.Has("keep-names")
		ignoreAnnotations := ctx.Form.Has("ignore-annotations")

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

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((reqPkg.Name == "react" && reqPkg.Submodule == "jsx-dev-runtime") || reqPkg.Name == "react-refresh") {
			isDev = true
		}

		buildArgs := BuildArgs{
			denoStdVersion:    dsv,
			alias:             alias,
			deps:              deps,
			external:          external,
			treeShaking:       treeShaking,
			ignoreRequire:     ignoreRequire,
			keepNames:         keepNames,
			ignoreAnnotations: ignoreAnnotations,
		}

		// parse and use `X-` prefix
		if hasBuildVerPrefix {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				reqPkg.Submodule = strings.Join(a[1:], "/")
				args, err := decodeBuildArgsPrefix(a[0])
				if err != nil {
					return throwErrorJS(ctx, err)
				}
				if args.denoStdVersion == "" {
					// ensure deno/std version used
					args.denoStdVersion = denoStdVersion
				}
				buildArgs = args
			}
		}

		// check whether it is `bare` mode
		if hasBuildVerPrefix && (endsWith(pathname, ".js") || endsWith(pathname, ".css")) {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 1 {
				if _, ok := targets[a[0]]; ok {
					submodule := strings.TrimSuffix(strings.Join(a[1:], "/"), ".js")
					if endsWith(submodule, ".bundle") {
						submodule = strings.TrimSuffix(submodule, ".bundle")
						isBundleMode = true
					}
					if endsWith(submodule, ".development") {
						submodule = strings.TrimSuffix(submodule, ".development")
						isDev = true
					}
					pkgName := path.Base(reqPkg.Name)
					if submodule == pkgName || (strings.HasSuffix(pkgName, ".js") && submodule+".js" == pkgName) {
						submodule = ""
					}
					if submodule == pkgName+".css" {
						submodule = ""
						isPkgCss = true
					}
					// workaround for es5-ext weird "/#/" path
					if pkgName == "es5-ext" {
						submodule = strings.ReplaceAll(submodule, "/$$/", "/#/")
					}
					reqPkg.Submodule = submodule
					target = a[0]
					isBare = true
				}
			}
		}

		if hasBuildVerPrefix && storageType == "types" {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				savePath = path.Join(fmt.Sprintf(
					"types/v%d/%s@%s/%s",
					buildVersion,
					reqPkg.Name,
					reqPkg.Version,
					encodeBuildArgsPrefix(buildArgs, reqPkg.Name, true),
				), reqPkg.Submodule)
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
					BuildArgs:    buildArgs,
					CdnOrigin:    cdnOrigin,
					BuildVersion: buildVersion,
					Pkg:          reqPkg,
					Target:       "types",
					stage:        "-",
				}
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return rex.Status(500, "types: "+output.err.Error())
					}
				case <-time.After(time.Minute):
					buildQueue.RemoveConsumer(task, c)
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
			ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, fi.ModTime(), r) // auto closed
		}

		task := &BuildTask{
			BuildArgs:    buildArgs,
			CdnOrigin:    cdnOrigin,
			BuildVersion: buildVersion,
			Pkg:          reqPkg,
			Target:       target,
			DevMode:      isDev,
			BundleMode:   isBundleMode || isWorker,
			stage:        "init",
		}
		taskID := task.ID()
		esm, ok := queryESMBuild(taskID)
		if !ok {
			if !isBare && !isPined {
				// find previous build version
				for i := 0; i < VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", VERSION-(i+1), taskID[len(fmt.Sprintf("v%d/", VERSION)):])
					esm, ok = queryESMBuild(taskID)
					if ok {
						taskID = id
						break
					}
				}
			}

			// if the previous build exists and is not pin/bare mode, then build current module in backgound,
			// or wait the current build task for 30 seconds
			if esm != nil {
				// todo: maybe don't build?
				buildQueue.Add(task, "")
			} else {
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return throwErrorJS(ctx, output.err)
					}
					esm = output.meta
				case <-time.After(time.Minute):
					buildQueue.RemoveConsumer(task, c)
					return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
				}
			}
		}

		// should redirect to `*.d.ts` ?
		if esm.TypesOnly {
			if esm.Dts != "" && !noCheck {
				value := fmt.Sprintf(
					"%s%s/%s",
					cdnOrigin,
					cfg.BasePath,
					strings.TrimPrefix(esm.Dts, "/"),
				)
				ctx.SetHeader("X-TypeScript-Types", value)
			}
			ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
			ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
			return []byte("export default null;\n")
		}

		if isPkgCss {
			if !esm.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}

			if !regexpFullVersionPath.MatchString(pathname) || !isPined || targetFromUA {
				url := fmt.Sprintf("%s%s/%s.css", cdnOrigin, cfg.BasePath, strings.TrimSuffix(taskID, ".js"))
				return rex.Redirect(url, http.StatusFound)
			}

			taskID = fmt.Sprintf("%s.css", strings.TrimSuffix(taskID, ".js"))
			isBare = true
		}

		if isBare {
			savePath := path.Join("builds", taskID)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "File not found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := fs.OpenFile(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			if isWorker && strings.HasSuffix(taskID, ".js") {
				defer r.Close()
				code, err := ioutil.ReadAll(r)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
				return fmt.Sprintf(`export default function workerFactory() { const blob = new Blob([%s], { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module" })}`, utils.MustEncodeJSON(string(code)))
			}
			return rex.Content(savePath, fi.ModTime(), r) // auto closed
		}

		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")

		if isWorker {
			fmt.Fprintf(buf, `export { default } from "%s%s/%s?worker";`, cdnOrigin, cfg.BasePath, taskID)
		} else {
			fmt.Fprintf(buf, `export * from "%s%s/%s";%s`, cdnOrigin, cfg.BasePath, taskID, "\n")
			if (esm.CJS || esm.ExportDefault) && (treeShaking.Size() == 0 || treeShaking.Has("default")) {
				fmt.Fprintf(buf, `export { default } from "%s%s/%s";%s`, cdnOrigin, cfg.BasePath, taskID, "\n")
			}
			if esm.CJS && ctx.Form.Has("cjs-exports") {
				exports := newStringSet()
				for _, p := range strings.Split(ctx.Form.Value("cjs-exports"), ",") {
					p = strings.TrimSpace(p)
					if regexpJSIdent.MatchString(p) {
						exports.Add(p)
					}
				}
				if exports.Size() > 0 {
					fmt.Fprintf(buf, `import __cjs_exports$ from "%s%s/%s";%s`, cdnOrigin, cfg.BasePath, taskID, "\n")
					fmt.Fprintf(buf, `export const { %s } = __cjs_exports$;%s`, strings.Join(exports.Values(), ", "), "\n")
				}
			}
		}

		if esm.Dts != "" && !noCheck && !isWorker {
			dts := strings.TrimPrefix(esm.Dts, "/")
			if stableBuild[reqPkg.Name] {
				dts = strings.Join(strings.Split(dts, "/")[1:], "/")
				dts = fmt.Sprintf("v%d/%s", VERSION, dts)
			}
			url := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.BasePath, dts)
			ctx.SetHeader("X-TypeScript-Types", url)
		}

		if regexpFullVersionPath.MatchString(pathname) {
			if isPined && !targetFromUA {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 24*3600)) // cache for 24 hours
			}
		} else {
			ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60)) // cache for 10 minutes
		}
		if targetFromUA {
			ctx.AddHeader("Vary", "User-Agent")
		}
		ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
		return buf
	}
}

func throwErrorJS(ctx *rex.Context, err error) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(
		buf,
		`throw new Error("[esm.sh] " + %s);%s`,
		strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))),
		"\n",
	)
	fmt.Fprintf(buf, "export default null;\n")
	ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
	ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
	return rex.Status(500, buf)
}
