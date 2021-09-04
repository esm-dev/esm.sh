package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

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
	queue := newBuildQueue(2 * runtime.NumCPU())

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

		case "/status.json":
			queue.lock.Lock()
			q := make([]map[string]interface{}, queue.queue.Len())
			i := 0
			for el := queue.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*task)
				if ok {
					q[i] = map[string]interface{}{
						"createTime": t.createTime.Unix(),
						"startTime":  t.startTime.Unix(),
						"consumers":  len(t.consumers),
						"pkg":        t.pkg.String(),
						"deps":       t.deps.String(),
						"target":     t.target,
						"inProcess":  t.inProcess,
						"isDev":      t.isDev,
						"bundle":     t.bundle,
					}
					i++
				}
			}
			queue.lock.Unlock()
			return map[string]interface{}{
				"queue": q[0:i],
			}

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

		// serve embed files
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

		var storageType string
		switch path.Ext(pathname) {
		case ".js":
			if hasBuildVerPrefix {
				storageType = "builds"
			}

		// todo: transform ts/jsx/tsx for browser
		case ".ts", ".jsx", ".tsx":
			if hasBuildVerPrefix && strings.HasSuffix(pathname, ".d.ts") {
				storageType = "types"
			} else if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}

		case ".json", ".css", ".pcss", "postcss", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm", ".xml", ".yaml", ".svg", ".png", ".eot", ".ttf", ".woff", ".woff2":
			if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		}

		// serve raw dist files like css that fetch from unpkg.com
		if storageType == "raw" {
			m, err := parsePkg(pathname)
			if err != nil {
				return err
			}
			if m.submodule != "" {
				shouldRedirect := !regVersionPath.MatchString(pathname)
				hostname := ctx.R.Host
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				if hostname == config.domain {
					if config.cdnDomain != "" {
						shouldRedirect = true
						hostname = config.cdnDomain
						proto = "https"
					}
					if config.cdnDomainChina != "" {
						var record Record
						err = mmdbr.Lookup(net.ParseIP(ctx.RemoteIP()), &record)
						if err == nil && record.Country.ISOCode == "CN" {
							shouldRedirect = true
							hostname = config.cdnDomainChina
							proto = "https"
						}
					}
				}
				if shouldRedirect {
					url := fmt.Sprintf("%s://%s/%s", proto, hostname, m.String())
					return rex.Redirect(url, http.StatusTemporaryRedirect)
				}
				cacheFile := path.Join(config.storageDir, "raw", m.String())
				if fileExists(cacheFile) {
					if strings.HasSuffix(pathname, ".ts") {
						ctx.SetHeader("Content-Type", "application/typescript")
					}
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					return rex.File(cacheFile)
				}
				unpkgDomain := "unpkg.com"
				if config.unpkgDomain != "" {
					unpkgDomain = config.unpkgDomain
				}
				resp, err := httpClient.Get(fmt.Sprintf("https://%s/%s", unpkgDomain, m.String()))
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
				err = ensureDir(path.Dir(cacheFile))
				if err != nil {
					return err
				}
				err = ioutil.WriteFile(cacheFile, data, 0644)
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
			storageType = ""
		}

		// serve build files
		if storageType != "" {
			var filepath string
			if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
				if prevBuildVer != "" {
					filepath = path.Join(config.storageDir, storageType, prevBuildVer, pathname)
				} else {
					filepath = path.Join(config.storageDir, storageType, fmt.Sprintf("v%d", VERSION), pathname)
				}
			} else {
				filepath = path.Join(config.storageDir, storageType, pathname)
			}
			if fileExists(filepath) {
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(filepath)
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

		// check `deps` query
		deps := pkgSlice{}
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
				if !deps.Has(m.name) {
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
		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			ua := ctx.R.UserAgent()
			// todo: check nodejs
			if strings.HasPrefix(ua, "Deno/") {
				target = "deno"
			} else {
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

		isPkgCSS := !ctx.Form.IsNil("css")
		isDev := !ctx.Form.IsNil("dev")
		bundleMode := !ctx.Form.IsNil("bundle") || !ctx.Form.IsNil("b")
		noCheck := !ctx.Form.IsNil("no-check")
		isBare := false

		if hasBuildVerPrefix && endsWith(pathname, ".js") {
			a := strings.Split(reqPkg.submodule, "/")
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
									if !deps.Has(m.name) {
										deps = append(deps, *m)
									}
								}
							}
						}
					}
				}
				a = a[1:]
			}
			if len(a) > 1 {
				if _, ok := targets[a[0]]; ok || a[0] == "esnext" {
					submodule := strings.TrimSuffix(strings.Join(a[1:], "/"), ".js")
					if endsWith(submodule, ".bundle") {
						submodule = strings.TrimSuffix(submodule, ".bundle")
						bundleMode = true
					}
					if endsWith(submodule, ".development") {
						submodule = strings.TrimSuffix(submodule, ".development")
						isDev = true
					}
					pkgName := path.Base(reqPkg.name)
					if submodule == pkgName || (strings.HasSuffix(pkgName, ".js") && submodule+".js" == pkgName) {
						submodule = ""
					}
					reqPkg.submodule = submodule
					target = a[0]
					isBare = true
				}
			}
		}

		task := &buildTask{
			pkg:    *reqPkg,
			deps:   deps,
			alias:  alias,
			target: target,
			isDev:  isDev,
			bundle: bundleMode,
		}

		taskID := task.ID()
		esm, pkgCSS, err := findESM(taskID)
		if err != nil {
			if !isBare {
				// find previous build version
				for i := 0; i < VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", VERSION-(i+1), taskID[len(fmt.Sprintf("v%d/", VERSION)):])
					esm, pkgCSS, err = findESM(id)
					if err == nil {
						taskID = id
						break
					}
				}
			}

			// if the previous build exists and not in bare mode, then build current module in backgound,
			// or wait the current build task for 30 seconds
			if err == nil {
				queue.Add(task)
			} else {
				select {
				case output := <-queue.Add(task):
					if output.err != nil {
						return throwErrorJS(ctx, output.err)
					}
					esm = output.esm
					pkgCSS = output.pkgCSS
				case <-time.After(time.Minute):
					return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try later!")
				}
			}
		}

		if isPkgCSS {
			if pkgCSS {
				hostname := ctx.R.Host
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				url := fmt.Sprintf("%s://%s/%s.css", proto, hostname, taskID)
				code := http.StatusTemporaryRedirect
				if regVersionPath.MatchString(pathname) {
					code = http.StatusPermanentRedirect
				}
				return rex.Redirect(url, code)
			}
			return rex.Status(404, "Package CSS not found")
		}

		if isBare {
			fp := path.Join(
				config.storageDir,
				"builds",
				taskID,
			)
			if !fileExists(fp) {
				return rex.Status(404, "File not found")
			}
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.File(fp)
		}

		buf := bytes.NewBuffer(nil)
		importPrefix := "/"
		if config.cdnDomain != "" {
			importPrefix = fmt.Sprintf("https://%s/", config.cdnDomain)
		}
		if config.cdnDomainChina != "" {
			var record Record
			err = mmdbr.Lookup(net.ParseIP(ctx.RemoteIP()), &record)
			if err == nil && record.Country.ISOCode == "CN" {
				importPrefix = fmt.Sprintf("https://%s/", config.cdnDomainChina)
			}
		}

		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")
		fmt.Fprintf(buf, `export * from "%s%s";%s`, importPrefix, taskID, "\n")
		if esm.ExportDefault {
			fmt.Fprintf(
				buf,
				`export { default } from "%s%s";%s`,
				importPrefix,
				taskID,
				"\n",
			)
		}

		if esm.Dts != "" && !noCheck {
			value := fmt.Sprintf(
				"%s%s",
				importPrefix,
				strings.TrimPrefix(
					path.Join("/", fmt.Sprintf("v%d", VERSION), esm.Dts),
					"/",
				),
			)
			ctx.SetHeader("X-TypeScript-Types", value)
			ctx.SetHeader("Access-Control-Expose-Headers", "X-TypeScript-Types")
		}
		ctx.SetHeader("Cache-Control", fmt.Sprintf("private, max-age=%d", refreshDuration))
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
