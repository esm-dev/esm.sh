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
	queue := newBuildQueue(runtime.NumCPU())

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()
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
		case "/favicon.ico":
			// todo: add esm.sh logo
			return rex.Err(404)
		case "/_error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Errorf(`Can't resolve "%s"`, ctx.Form.Value("name")))
			case "unsupported-nodejs-builtin-module":
				return throwErrorJS(ctx, fmt.Errorf(`Unsupported nodejs builtin module "%s"`, ctx.Form.Value("name")))
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
		case ".ts":
			if hasBuildVerPrefix && strings.HasSuffix(pathname, ".d.ts") {
				storageType = "types"
			} else if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		case ".css":
			if hasBuildVerPrefix {
				storageType = "builds"
			} else if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		case ".json", ".jsx", ".tsx", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm", ".xml", ".yaml", ".svg":
			if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		}
		if storageType == "raw" {
			m, err := parsePkg(pathname)
			if err != nil {
				return throwErrorJS(ctx, err)
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

		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			ua := ctx.R.UserAgent()
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

		deps := pkgSlice{}
		for _, p := range strings.Split(ctx.Form.Value("deps"), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
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

		isPkgCSS := !ctx.Form.IsNil("css")
		isDev := !ctx.Form.IsNil("dev")
		noCheck := !ctx.Form.IsNil("no-check")

		reqPkg, err := parsePkg(pathname)
		if err != nil {
			if strings.HasSuffix(err.Error(), "not found") {
				return throwErrorJS(ctx, err)
			}
			return throwErrorJS(ctx, err)
		}

		isBare := false
		if hasBuildVerPrefix && endsWith(pathname, ".js") {
			a := strings.Split(reqPkg.submodule, "/")
			if len(a) > 1 {
				if strings.HasPrefix(a[0], "deps=") {
					for _, p := range strings.Split(strings.TrimPrefix(a[0], "deps="), ",") {
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
					a = a[1:]
				}
			}
			if len(a) > 1 {
				if _, ok := targets[a[0]]; ok || a[0] == "esnext" {
					submodule := strings.TrimSuffix(strings.Join(a[1:], "/"), ".js")
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

		// todo: wait 1 second then down to previous build version
		task := &buildTask{
			pkg:    *reqPkg,
			deps:   deps,
			target: target,
			isDev:  isDev,
		}

		esm, pkgCSS, ok := findESM(task.ID())
		if !ok {
			output := <-queue.Add(task)
			if output.err != nil {
				return throwErrorJS(ctx, output.err)
			}
			esm = output.esm
			pkgCSS = output.pkgCSS
		} else {
			log.Debugf("esm %s,%s found", reqPkg, target)
		}

		if isPkgCSS {
			if pkgCSS {
				hostname := ctx.R.Host
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				url := fmt.Sprintf("%s://%s/%s.css", proto, hostname, task.ID())
				code := http.StatusTemporaryRedirect
				if regVersionPath.MatchString(pathname) {
					code = http.StatusPermanentRedirect
				}
				return rex.Redirect(url, code)
			}
			return throwErrorJS(ctx, fmt.Errorf("css not found"))
		}

		if isBare {
			fp := path.Join(
				config.storageDir,
				"builds",
				fmt.Sprintf("v%d", VERSION),
				pathname,
			)
			if fileExists(fp) {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(fp)
			}
			return rex.Err(404)
		}

		buf := bytes.NewBuffer(nil)
		importPrefix := "/"
		importSuffix := ".js"
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
		fmt.Fprintf(buf, `export * from "%s%s%s";%s`, importPrefix, task.ID(), importSuffix, "\n")

		if esm.Module != "" {
			for _, name := range esm.Exports {
				if name == "default" {
					fmt.Fprintf(
						buf,
						`export { default } from "%s%s%s";%s`,
						importPrefix,
						task.ID(),
						importSuffix,
						"\n",
					)
					break
				}
			}
		} else {
			fmt.Fprintf(
				buf,
				`export { default } from "%s%s%s";%s`,
				importPrefix,
				task.ID(),
				importSuffix,
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
	return buf
}
