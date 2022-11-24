package server

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"esm.sh/server/storage"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

var banList = map[string]bool{
	"/@withfig/autocomplete": true,
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		Dial: func(network, addr string) (conn net.Conn, err error) {
			conn, err = net.DialTimeout(network, addr, 15*time.Second)
			if err != nil {
				return conn, err
			}

			// Set a one-time deadline for potential SSL handshaking
			conn.SetDeadline(time.Now().Add(60 * time.Second))
			return conn, nil
		},
		MaxIdleConnsPerHost:   6,
		ResponseHeaderTimeout: 60 * time.Second,
		Proxy:                 http.ProxyFromEnvironment,
	},
}

type esmHandlerOptions = struct {
	origin      string
	basePath    string
	unpkgOrigin string
}

// esm.sh query middleware for rex
func esmHandler(options esmHandlerOptions) rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()

		// ban malicious requests
		if strings.HasPrefix(pathname, ".") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(400, "Bad Request")
		}

		// ban malicious requests by banList
		for prefix := range banList {
			if strings.HasPrefix(pathname, prefix) {
				return rex.Status(403, "forbidden")
			}
		}

		// strip loc
		if strings.ContainsRune(pathname, ':') {
			pathname = regLocPath.ReplaceAllString(pathname, "$1")
		}

		var origin string
		if options.origin != "" {
			origin = strings.TrimSuffix(options.origin, "/")
		} else {
			proto := "http"
			if ctx.R.TLS != nil {
				proto = "https"
			}
			origin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
		}
		// force to use https for esm.sh
		if origin == "http://esm.sh" {
			origin = "https://esm.sh"
		}

		// redirect `/@types/` to `.d.ts` files
		if strings.HasPrefix(pathname, "/@types/") {
			url := fmt.Sprintf("%s/v%d%s", origin, VERSION, pathname)
			if !strings.HasSuffix(url, ".d.ts") {
				url += "~.d.ts"
			}
			return rex.Redirect(url, http.StatusFound)
		}

		// Build prefix may only be served from "${options.basePath}/..."
		if options.basePath != "" {
			if strings.HasPrefix(pathname, options.basePath+"/") {
				pathname = strings.TrimPrefix(pathname, options.basePath)
			} else {
				url := strings.TrimPrefix(ctx.R.URL.String(), options.basePath)
				url = fmt.Sprintf("%s/%s", options.basePath, url)
				return rex.Redirect(url, http.StatusFound)
			}
		}

		var hasBuildVerPrefix bool
		var outdatedBuildVer string

		// Check build version
		buildBasePath := fmt.Sprintf("/v%d", VERSION)
		if strings.HasPrefix(pathname, "/stable/") {
			pathname = strings.TrimPrefix(pathname, "/stable")
			hasBuildVerPrefix = true
		} else if strings.HasPrefix(pathname, buildBasePath+"/") || pathname == buildBasePath {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			// Otherwise check possible fixed version
		} else if regBuildVersionPath.MatchString(pathname) {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			outdatedBuildVer = a[1]
		}

		// match static routess
		switch pathname {
		case "/":
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
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte(options.basePath+"/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/ije/esm.sh/blob/master/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte("{origin}"+options.basePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			html = bytes.ReplaceAll(html, []byte("{basePath}"), []byte(options.basePath))
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
			res, err := http.Get(fmt.Sprintf("http://localhost:%d", nsPort))
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
					`Can't resolve "%s" (Imported by "%s")`,
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

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile("server" + pathname)
			if err != nil {
				// try `/embed/test/**/*`
				data, err = embedFS.ReadFile(pathname[7:])
			}
			if err == nil {
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
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
		reqPkg, unQuery, err := parsePkg(pathname)
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
				return rex.Redirect(fmt.Sprintf("%s/%s%s@%s%s%s", origin, eaSign, reqPkg.Name, reqPkg.Version, query, submodule), http.StatusFound)
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s/%s%s%s", origin, eaSign, reqPkg.String(), query), http.StatusFound)
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
			return rex.Redirect(fmt.Sprintf("%s%s/%s@%s%s%s", origin, prefix, reqPkg.Name, reqPkg.Version, subpath, query), http.StatusFound)
		}

		// since most transformers handle `jsxSource` by concating string "/jsx-runtime"
		// we need to support url like `https://esm.sh/react?dev&target=esnext/jsx-runtime`
		if (reqPkg.Name == "react" || reqPkg.Name == "preact") && strings.HasSuffix(ctx.R.URL.RawQuery, "/jsx-runtime") {
			ctx.R.URL.RawQuery = strings.TrimSuffix(ctx.R.URL.RawQuery, "/jsx-runtime")
			pathname = fmt.Sprintf("/%s/jsx-runtime", reqPkg.Name)
			reqPkg.Submodule = "jsx-runtime"
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
			// todo: transform ts/jsx/tsx for browser
			case ".ts", ".jsx", ".tsx":
				if hasBuildVerPrefix {
					if strings.HasSuffix(pathname, ".d.ts") {
						storageType = "types"
					}
				} else if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			case ".json", ".css", ".pcss", ".postcss", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm", ".xml", ".yaml", ".md", ".svg", ".png", ".jpg", ".webp", ".gif", ".eot", ".ttf", ".otf", ".woff", ".woff2":
				if hasBuildVerPrefix {
					if strings.HasSuffix(pathname, ".css") {
						storageType = "builds"
					}
				} else if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			}
		}

		// serve raw dist files like CSS that is fetching from unpkg.com
		if storageType == "raw" {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !regFullVersionPath.MatchString(pathname) {
					url := fmt.Sprintf("%s/%s", origin, reqPkg.String())
					http.Redirect(w, r, url, http.StatusFound)
					return
				}
				savePath := path.Join("raw", reqPkg.String())
				exists, size, modtime, err := fs.Exists(savePath)
				if err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}

				// fetch the non-existent file from unpkg.com and save to fs
				if !exists {
					resp, err := httpClient.Get(fmt.Sprintf("%s/%s", strings.TrimSuffix(options.unpkgOrigin, "/"), reqPkg.String()))
					if err != nil {
						w.WriteHeader(500)
						w.Write([]byte(err.Error()))
						return
					}
					defer resp.Body.Close()

					if resp.StatusCode >= 500 {
						w.WriteHeader(http.StatusBadGateway)
						w.Write([]byte("Bad Gateway"))
						return
					}

					if resp.StatusCode >= 400 {
						w.WriteHeader(http.StatusBadGateway)
						io.Copy(w, resp.Body)
						return
					}

					size, err = fs.WriteFile(savePath, resp.Body)
					if err != nil {
						w.WriteHeader(500)
						w.Write([]byte(err.Error()))
						return
					}
				}

				f, err := fs.ReadFile(savePath, size)
				if err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}
				defer f.Close()

				if strings.HasSuffix(pathname, ".ts") {
					w.Header().Set("Content-Type", "application/typescript")
				}
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeContent(w, r, savePath, modtime, f)
			})
		}

		// serve build files
		if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
			var savePath string
			if outdatedBuildVer != "" {
				savePath = path.Join(storageType, outdatedBuildVer, pathname)
			} else {
				savePath = path.Join(storageType, fmt.Sprintf("v%d", VERSION), pathname)
			}

			exists, size, modtime, err := fs.Exists(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}

			if exists {
				r, err := fs.ReadFile(savePath, size)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(savePath, modtime, r)
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
				m, _, err := parsePkg(p)
				if err != nil {
					if strings.HasSuffix(err.Error(), "not found") {
						continue
					}
					return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", p))
				}
				if !deps.Has(m.Name) && m.Name != reqPkg.Name {
					deps = append(deps, *m)
				}
			}
		}

		// check `?exports` query
		treeShaking := newStringSet()
		for _, p := range strings.Split(ctx.Form.Value("exports"), ",") {
			p = strings.TrimSpace(p)
			if regJSIdent.MatchString(p) {
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
		if fv != "" && regFullVersion.MatchString(fv) && target == "deno" {
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
		sourcemap := ctx.Form.Has("sourcemap") || ctx.Form.Has("source-map")

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
		if !isDev {
			if (reqPkg.Name == "react" && reqPkg.Submodule == "jsx-dev-runtime") || reqPkg.Name == "react-refresh" {
				isDev = true
			}
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
			sourcemap:         sourcemap,
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
				buildArgs = args
			}
		}

		// check whether it is `bare` mode
		if hasBuildVerPrefix && endsWith(pathname, ".js") {
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
					reqPkg.Submodule = submodule
					target = a[0]
					isBare = true
				}
			}
		}

		if hasBuildVerPrefix && storageType == "types" {
			task := &BuildTask{
				BuildArgs:    buildArgs,
				CdnOrigin:    origin,
				BasePath:     options.basePath,
				BuildVersion: buildVersion,
				Pkg:          *reqPkg,
				Target:       "types",
				stage:        "-",
			}
			var savePath string
			findTypesFile := func() (bool, int64, time.Time, error) {
				savePath = path.Join(fmt.Sprintf(
					"types/v%d/%s@%s/%s",
					buildVersion,
					reqPkg.Name,
					reqPkg.Version,
					encodeBuildArgsPrefix(buildArgs, reqPkg.Name, true),
				), reqPkg.Submodule)
				if strings.HasSuffix(savePath, "~.d.ts") {
					savePath = strings.TrimSuffix(savePath, "~.d.ts")
					ok, _, _, err := fs.Exists(path.Join(savePath, "index.d.ts"))
					if err != nil {
						return false, 0, time.Time{}, err
					}
					if ok {
						savePath = path.Join(savePath, "index.d.ts")
					} else {
						savePath += ".d.ts"
					}
				}
				return fs.Exists(savePath)
			}
			exists, size, modtime, err := findTypesFile()
			if err == nil && !exists {
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
			if err != nil {
				return rex.Status(500, err.Error())
			}
			var r io.ReadSeeker
			r, err = fs.ReadFile(savePath, size)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				r = bytes.NewReader([]byte("/* fake(empty) types */"))
			}
			ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, modtime, r) // auto close
		}

		task := &BuildTask{
			BuildArgs:    buildArgs,
			CdnOrigin:    origin,
			BasePath:     options.basePath,
			BuildVersion: buildVersion,
			Pkg:          *reqPkg,
			Target:       target,
			DevMode:      isDev,
			BundleMode:   isBundleMode || isWorker,
			stage:        "init",
		}
		taskID := task.ID()
		esm, err := findModule(taskID)
		if err != nil && err != storage.ErrNotFound {
			return rex.Status(500, err.Error())
		}
		if err == storage.ErrNotFound {
			if !isBare && !isPined {
				// find previous build version
				for i := 0; i < VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", VERSION-(i+1), taskID[len(fmt.Sprintf("v%d/", VERSION)):])
					esm, err = findModule(id)
					if err != nil && err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					}
					if err == nil {
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

		// todo: redirect to .d.ts
		if esm.TypesOnly {
			if esm.Dts != "" && !noCheck {
				value := fmt.Sprintf(
					"%s%s/%s",
					origin,
					options.basePath,
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

			if !regFullVersionPath.MatchString(pathname) || !isPined {
				url := fmt.Sprintf("%s/%s.css", origin, strings.TrimSuffix(taskID, ".js"))
				return rex.Redirect(url, http.StatusFound)
			}

			taskID = fmt.Sprintf("%s.css", strings.TrimSuffix(taskID, ".js"))
			isBare = true
		}

		if isBare {
			savePath := path.Join(
				"builds",
				taskID,
			)
			exists, size, modtime, err := fs.Exists(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if !exists {
				return rex.Status(404, "File not found")
			}
			r, err := fs.ReadFile(savePath, size)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if !hasBuildVerPrefix && esm.Dts != "" && !noCheck && !isWorker {
				url := fmt.Sprintf(
					"%s%s/%s",
					origin,
					options.basePath,
					strings.TrimPrefix(esm.Dts, "/"),
				)
				ctx.SetHeader("X-TypeScript-Types", url)
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, modtime, r)
		}

		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")

		if isWorker {
			fmt.Fprintf(buf, `export default function workerFactory() {%s  return new Worker('%s/%s', { type: 'module' })%s}`, "\n", origin, taskID, "\n")
		} else {
			fmt.Fprintf(buf, `export * from "%s%s/%s";%s`, origin, options.basePath, taskID, "\n")
			if (esm.CJS || esm.ExportDefault) && (treeShaking.Size() == 0 || treeShaking.Has("default")) {
				fmt.Fprintf(
					buf,
					`export { default } from "%s%s/%s";%s`,
					origin,
					options.basePath,
					taskID,
					"\n",
				)
			}
		}

		if esm.Dts != "" && !noCheck && !isWorker {
			url := fmt.Sprintf(
				"%s%s/%s",
				origin,
				options.basePath,
				strings.TrimPrefix(esm.Dts, "/"),
			)
			ctx.SetHeader("X-TypeScript-Types", url)
		}

		if regFullVersionPath.MatchString(pathname) {
			if isPined {
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
