package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"esm.sh/server/storage"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
	"github.com/mssola/user_agent"
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
func query() rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()
		if strings.HasPrefix(pathname, ".") {
			return rex.Status(400, "Bad Request")
		}

		switch pathname {
		case "/":
			indexHTML, err := embedFS.ReadFile("embed/index.html")
			if err != nil {
				return err
			}
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			readmeStr := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStr)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/favicon.svg":
			data, err := embedFS.ReadFile("embed/favicon.svg")
			if err != nil {
				return err
			}
			return rex.Content("favicon.svg", startTime, bytes.NewReader(data))

		case "/favicon.ico":
			return rex.Redirect("/favicon.svg", http.StatusPermanentRedirect)

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
		if strings.HasPrefix(pathname, "/embed/assets/") || strings.HasPrefix(pathname, "/embed/test/") {
			data, err := embedFS.ReadFile(pathname[1:])
			if err != nil {
				return err
			}
			return rex.Content(pathname, startTime, bytes.NewReader(data))
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
			data, err := embedFS.ReadFile("embed/polyfills" + pathname)
			if err == nil {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
			data, err = embedFS.ReadFile("embed/types" + pathname)
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
			shouldRedirect := !regVersionPath.MatchString(pathname)
			isTLS := ctx.R.TLS != nil
			hostname := ctx.R.Host
			proto := "http"
			if isTLS {
				proto = "https"
			}
			if isTLS && cdnDomain != "" && hostname != cdnDomain {
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
			if resp.StatusCode != 200 {
				return rex.Err(http.StatusBadGateway)
			}
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
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
				target = "es2015"
				name, version := user_agent.New(ua).Browser()
				if engine, ok := engines[strings.ToLower(name)]; ok {
					a := strings.Split(version, ".")
					if len(a) > 3 {
						version = strings.Join(a[:3], ".")
					}
					unspportEngineFeatures := validateEngineFeatures(api.Engine{
						Name:    engine,
						Version: version,
					})
					for _, t := range []string{
						"es2021",
						"es2020",
						"es2019",
						"es2018",
						"es2017",
						"es2016",
					} {
						unspportESMAFeatures := validateESMAFeatures(targets[t])
						if unspportEngineFeatures <= unspportESMAFeatures {
							target = t
							break
						}
					}
				}
			}
		}

		bundleMode := !ctx.Form.IsNil("bundle") || !ctx.Form.IsNil("b")
		noCheck := !ctx.Form.IsNil("no-check")
		isDev := !ctx.Form.IsNil("dev")
		isPkgCSS := !ctx.Form.IsNil("css")
		isBare := false

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
						bundleMode = true
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
				Pkg:    *reqPkg,
				Deps:   deps,
				Alias:  alias,
				Target: "types",
				stage:  "init",
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
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if !exists {
				err := pushBuild(task)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				n := pkgRequstTimeout * 10
				if isDev {
					n *= 2
				}
				for i := 0; i < n; i++ {
					exists, modtime, err = findTypesFile()
					if err != nil {
						return rex.Status(500, err.Error())
					}
					if exists {
						break
					}
					if i == n-1 {
						return rex.Status(http.StatusRequestTimeout, "timeout, we are transforming the types hardly, please try later!")
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
			if !exists {
				return rex.Status(404, "File not found")
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
			Pkg:        *reqPkg,
			Deps:       deps,
			Alias:      alias,
			Target:     target,
			BundleMode: bundleMode,
			IsDev:      isDev,
			stage:      "init",
		}
		taskID := task.ID()
		esm, err := findESM(taskID)
		if err != nil && err != storage.ErrNotFound {
			return rex.Status(500, err.Error())
		}
		if err == storage.ErrNotFound {
			if !isBare {
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

			// if the previous build exists and not in bare mode, then build current module in backgound,
			// or wait the current build task for 30 seconds
			if esm != nil {
				// todo: maybe don't build
				pushBuild(task)
			} else {
				err := pushBuild(task)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				n := pkgRequstTimeout * 10
				if isDev {
					n *= 2
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

		if isPkgCSS {
			if esm.PackageCSS {
				hostname := ctx.R.Host
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				url := fmt.Sprintf("%s://%s/%s.css", proto, hostname, strings.TrimSuffix(taskID, ".js"))
				code := http.StatusTemporaryRedirect
				if regVersionPath.MatchString(pathname) {
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
		if cdnDomain == "localhost" || strings.HasPrefix(cdnDomain, "localhost:") {
			origin = fmt.Sprintf("http://%s/", cdnDomain)
		} else if cdnDomain != "" {
			origin = fmt.Sprintf("https://%s/", cdnDomain)
		}

		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")
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

		if esm.Dts != "" && !noCheck {
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
