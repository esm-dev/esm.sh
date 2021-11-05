package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"esm.sh/server/storage"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

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
	},
}

// esm query middleware for rex
func query(devMode bool) rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()
		if strings.HasPrefix(pathname, ".") {
			return rex.Status(400, "Bad Request")
		}

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
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte("/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/alephjs/esm.sh/blob/master/HOSTING.md"))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			list, err := db.List("error")
			if err != nil {
				return rex.Status(500, err.Error())
			}
			return map[string]interface{}{
				"version": VERSION,
				"uptime":  time.Now().Sub(startTime).Milliseconds(),
				"errors":  list,
			}

		case "/favicon.ico":
			return rex.Status(404, "not found")

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
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile("server" + pathname)
			if err != nil {
				data, err = embedFS.ReadFile(pathname[7:]) // /embed/test/**/*
			}
			if err == nil {
				switch path.Ext(pathname) {
				case ".js", ".jsx", ".ts", ".tsx":
					opts := buildOptions{
						target: getTargetByUA(ctx.R.UserAgent()),
						cache:  !devMode,
						minify: !ctx.Form.IsNil("minify"),
					}
					if !ctx.Form.IsNil("bundle") {
						hostname := ctx.R.Host
						isLocalHost := hostname == "localhost" || strings.HasPrefix(hostname, "localhost:")
						proto := "https"
						if isLocalHost {
							proto = "http"
						}
						opts.bundle = true
						opts.origin = proto + "://" + hostname
					}
					data, err = buildSync(pathname, string(data), opts)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", pkgCacheTimeout))
					return rex.Content(pathname+".js", startTime, bytes.NewReader(data))
				default:
					ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", pkgCacheTimeout))
					return rex.Content(pathname, startTime, bytes.NewReader(data))
				}
			}
		}

		hasBuildVerPrefix := strings.HasPrefix(pathname, fmt.Sprintf("/v%d/", VERSION))
		prevBuildVer := ""
		if hasBuildVerPrefix {
			pathname = strings.TrimPrefix(pathname, fmt.Sprintf("/v%d", VERSION))
		} else if regBuildVersionPath.MatchString(pathname) {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			prevBuildVer = a[1]
		}

		// serve embed polyfills/types
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
		reqPkg, err := parsePkg(pathname)
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

			case ".json", ".css", ".pcss", "postcss", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm", ".xml", ".yaml", ".svg", ".png", ".eot", ".ttf", ".woff", ".woff2":
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
			shouldRedirect := !regFullVersionPath.MatchString(pathname)
			hostname := ctx.R.Host
			isLocalHost := hostname == "localhost" || strings.HasPrefix(hostname, "localhost:")
			proto := "https"
			if isLocalHost {
				proto = "http"
			}
			if !isLocalHost && cdnDomain != "" && hostname != cdnDomain {
				shouldRedirect = true
				hostname = cdnDomain
				proto = "https"
			}
			if shouldRedirect {
				url := fmt.Sprintf("%s://%s/%s", proto, hostname, reqPkg.String())
				return rex.Redirect(url, http.StatusTemporaryRedirect)
			}
			savePath := path.Join("raw", reqPkg.String())
			exists, modtime, err := fs.Exists(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if exists {
				r, err := fs.ReadFile(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if strings.HasSuffix(pathname, ".ts") {
					ctx.SetHeader("Content-Type", "application/typescript")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(savePath, modtime, r)
			}
			resp, err := httpClient.Get(fmt.Sprintf("https://unpkg.com/%s", reqPkg.String()))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 500 {
				return rex.Err(http.StatusBadGateway)
			}
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode >= 400 {
				return rex.Status(resp.StatusCode, string(data))
			}
			err = fs.WriteData(savePath, data)
			if err != nil {
				return err
			}
			for key, values := range resp.Header {
				for _, value := range values {
					ctx.AddHeader(key, value)
				}
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return data
		}

		// serve build files
		if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
			var savePath string
			if prevBuildVer != "" {
				savePath = path.Join(storageType, prevBuildVer, pathname)
			} else {
				savePath = path.Join(storageType, fmt.Sprintf("v%d", VERSION), pathname)
			}

			exists, modtime, err := fs.Exists(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}

			if exists {
				r, err := fs.ReadFile(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if strings.HasSuffix(savePath, ".css") && !ctx.Form.IsNil("module") {
					data, err := ioutil.ReadAll(r)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					cssStr, _ := json.Marshal(string(data))
					jsCode := fmt.Sprintf(cssLoaderTpl, strings.TrimPrefix(savePath, "builds"), cssStr)
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					return rex.Content(savePath+".js", modtime, bytes.NewReader([]byte(jsCode)))
				}
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(savePath, modtime, r)
			}
		}

		// check `deps` query
		deps := PkgSlice{}
		for _, p := range strings.Split(ctx.Form.Value("deps"), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				m, err := parsePkg(p)
				if err != nil {
					if strings.HasSuffix(err.Error(), "not found") {
						continue
					}
					return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", p))
				}
				if !deps.Has(m.Name) {
					deps = append(deps, *m)
				}
			}
		}

		// check `alias` query
		alias := map[string]string{}
		for _, p := range strings.Split(ctx.Form.Value("alias"), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				name, to := utils.SplitByFirstByte(p, ':')
				name = strings.TrimSpace(name)
				to = strings.TrimSpace(to)
				if name != "" && to != "" {
					alias[name] = to
				}
			}
		}

		// determine build target
		var target string
		ua := ctx.R.UserAgent()
		if strings.HasPrefix(ua, "Deno/") {
			target = "deno"
		} else {
			target = strings.ToLower(ctx.Form.Value("target"))
			if _, ok := targets[target]; !ok {
				target = getTargetByUA(ua)
			}
		}

		buildVersion := VERSION
		value := ctx.Form.Value("pin")
		if strings.HasPrefix(value, "v") {
			i, err := strconv.Atoi(value[1:])
			if err == nil && i > 0 && i < VERSION {
				buildVersion = i
			}
		}

		css := !ctx.Form.IsNil("css")
		cssAsModule := css && !ctx.Form.IsNil("module")
		isBare := false
		isBundleMode := !ctx.Form.IsNil("bundle")
		isDev := !ctx.Form.IsNil("dev")
		isPined := !ctx.Form.IsNil("pin")
		isWorkder := !ctx.Form.IsNil("worker")
		noCheck := !ctx.Form.IsNil("no-check")

		// parse `resolvePrefix`
		if hasBuildVerPrefix {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				s, err := atobUrl(strings.TrimPrefix(a[0], "X-"))
				if err == nil {
					for _, p := range strings.Split(s, ",") {
						if strings.HasPrefix(p, "alias:") {
							for _, p := range strings.Split(strings.TrimPrefix(p, "alias:"), ",") {
								p = strings.TrimSpace(p)
								if p != "" {
									name, to := utils.SplitByFirstByte(p, ':')
									name = strings.TrimSpace(name)
									to = strings.TrimSpace(to)
									if name != "" && to != "" {
										alias[name] = to
									}
								}
							}
						} else if strings.HasPrefix(p, "deps:") {
							for _, p := range strings.Split(strings.TrimPrefix(p, "deps:"), ",") {
								p = strings.TrimSpace(p)
								if p != "" {
									if strings.HasPrefix(p, "@") {
										scope, name := utils.SplitByFirstByte(p, '_')
										p = scope + "/" + name
									}
									m, err := parsePkg(p)
									if err != nil {
										if strings.HasSuffix(err.Error(), "not found") {
											continue
										}
										return throwErrorJS(ctx, err)
									}
									if !deps.Has(m.Name) {
										deps = append(deps, *m)
									}
								}
							}
						}
					}
				}
				reqPkg.Submodule = strings.Join(a[1:], "/")
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
				BuildVersion: buildVersion,
				Pkg:          *reqPkg,
				Deps:         deps,
				Alias:        alias,
				Target:       "types",
				stage:        "init",
			}
			var savePath string
			findTypesFile := func() (bool, time.Time, error) {
				savePath = path.Join(fmt.Sprintf(
					"types/v%d/%s@%s/%s",
					VERSION,
					reqPkg.Name,
					reqPkg.Version,
					task.resolvePrefix(),
				), reqPkg.Submodule)
				if strings.HasSuffix(savePath, "~.d.ts") {
					savePath = strings.TrimSuffix(savePath, "~.d.ts")
					ok, _, err := fs.Exists(path.Join(savePath, "index.d.ts"))
					if err != nil {
						return false, time.Time{}, err
					}
					if ok {
						savePath = path.Join(savePath, "index.d.ts")
					} else {
						savePath += ".d.ts"
					}
				}
				return fs.Exists(savePath)
			}
			exists, modtime, err := findTypesFile()
			if err == nil && !exists {
				_, err = task.Build()
				if err == nil {
					exists, modtime, err = findTypesFile()
				}
			}
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if !exists {
				return rex.Status(404, "Types not found")
			}
			r, err := fs.ReadFile(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, modtime, r)
		}

		task := &BuildTask{
			BuildVersion: buildVersion,
			Pkg:          *reqPkg,
			Deps:         deps,
			Alias:        alias,
			Target:       target,
			BundleMode:   isBundleMode,
			DevMode:      isDev,
			stage:        "init",
		}
		taskID := task.ID()
		esm, err := findESM(taskID)
		if err != nil && err != storage.ErrNotFound {
			return rex.Status(500, err.Error())
		}
		if err == storage.ErrNotFound {
			if !isBare && !isPined {
				// find previous build version
				for i := 0; i < VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", VERSION-(i+1), taskID[len(fmt.Sprintf("v%d/", VERSION)):])
					esm, err = findESM(id)
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
				// todo: maybe don't build
				pushBuildTask(task)
			} else {
				err = pushBuildTask(task)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				n := pkgRequstTimeout * 10
				if isDev {
					n *= 10
				}
				for i := 0; i < n; i++ {
					esm, err = findESM(taskID)
					if err == nil {
						break
					}
					if err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					}
					if i == n-1 {
						return rex.Status(http.StatusRequestTimeout, "timeout, we are transforming the types hardly, please try later!")
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		if css {
			if esm.PackageCSS {
				hostname := ctx.R.Host
				proto := "https"
				if hostname == "localhost" || strings.HasPrefix(hostname, "localhost:") {
					proto = "http"
				}
				url := fmt.Sprintf("%s://%s/%s.css", proto, hostname, strings.TrimSuffix(taskID, ".js"))
				if cssAsModule {
					url += "?module"
				}
				code := http.StatusTemporaryRedirect
				if regFullVersionPath.MatchString(pathname) {
					code = http.StatusPermanentRedirect
				}
				return rex.Redirect(url, code)
			}
			return rex.Status(404, "Package CSS not found")
		}

		if isBare {
			savePath := path.Join(
				"builds",
				taskID,
			)
			exists, modtime, err := fs.Exists(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if !exists {
				return rex.Status(404, "File not found")
			}
			r, err := fs.ReadFile(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, modtime, r)
		}

		buf := bytes.NewBuffer(nil)
		origin := "/"
		if cdnDomain != "" && cdnDomain != "localhost" && !strings.HasPrefix(cdnDomain, "localhost:") && !isWorkder {
			origin = fmt.Sprintf("https://%s/", cdnDomain)
		}
		if isWorkder {
			hostname := ctx.R.Host
			isLocalHost := hostname == "localhost" || strings.HasPrefix(hostname, "localhost:")
			proto := "https"
			if isLocalHost {
				proto = "http"
			}
			origin = fmt.Sprintf("%s://%s/", proto, hostname)
		}

		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")
		if isWorkder {
			fmt.Fprintf(buf, `export default function WorkerWrapper() {%s  return new Worker('%s%s', { type: 'module' })%s}`, "\n", origin, taskID, "\n")
		} else {
			fmt.Fprintf(buf, `export * from "%s%s";%s`, origin, taskID, "\n")
			if esm.ExportDefault {
				fmt.Fprintf(
					buf,
					`export { default } from "%s%s";%s`,
					origin,
					taskID,
					"\n",
				)
			}
		}

		if esm.Dts != "" && !noCheck && !isWorkder {
			value := fmt.Sprintf(
				"%s%s",
				origin,
				strings.TrimPrefix(esm.Dts, "/"),
			)
			ctx.SetHeader("X-TypeScript-Types", value)
			ctx.SetHeader("Access-Control-Expose-Headers", "X-TypeScript-Types")
		}
		ctx.SetHeader("Cache-Tag", "entry")
		ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", pkgCacheTimeout))
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
