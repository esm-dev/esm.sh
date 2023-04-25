package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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

type Publish struct {
	Types string `json:"types"`
	Code  string `json:"code"`
}

func postHandler() rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if ctx.R.Method == "POST" {
			pathname := ctx.Path.String()
			if pathname != "/publish" {
				return rex.Err(404, "not found")
			}
			defer ctx.R.Body.Close()
			if ctx.R.Header.Get("Content-Type") != "application/json" {
				return rex.Err(400, "invalid content type, should be application/json")
			}
			var pub Publish
			err := json.NewDecoder(ctx.R.Body).Decode(&pub)
			if err != nil {
				return rex.Err(400, "failed to parse publish config: "+err.Error())
			}
			if pub.Code == "" {
				return rex.Err(400, "code is required")
			}
			input := &api.StdinOptions{
				Contents:   pub.Code,
				ResolveDir: "/",
				Sourcefile: "index.tsx",
				Loader:     api.LoaderTSX,
			}
			deps := map[string]string{}
			onResolver := func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				path := args.Path
				if isLocalSpecifier(path) {
					return api.OnResolveResult{}, errors.New("local specifier is not allowed")
				}
				if !isRemoteSpecifier(path) {
					pkg, _, err := validatePkgPath(strings.TrimPrefix(path, "npm:"))
					if err != nil {
						return api.OnResolveResult{}, err
					}
					path = pkg.Name
					if pkg.Submodule != "" {
						path += "/" + pkg.Submodule
					}
					deps[pkg.Name] = pkg.Version
				}
				return api.OnResolveResult{
					Path:     path,
					External: true,
				}, nil
			}
			ret := api.Build(api.BuildOptions{
				Outdir:           "/esbuild",
				Stdin:            input,
				Platform:         api.PlatformBrowser,
				Format:           api.FormatESModule,
				TreeShaking:      api.TreeShakingTrue,
				Target:           api.ESNext,
				Bundle:           true,
				MinifyWhitespace: true,
				MinifySyntax:     true,
				Write:            false,
				Plugins: []api.Plugin{
					{
						Name: "resolver",
						Setup: func(build api.PluginBuild) {
							build.OnResolve(api.OnResolveOptions{Filter: ".*"}, onResolver)
						},
					},
				},
			})
			if len(ret.Errors) > 0 {
				return rex.Err(400, "failed to validate code: "+ret.Errors[0].Text)
			}
			if len(ret.OutputFiles) == 0 {
				return rex.Err(400, "failed to validate code: no output files")
			}
			code := ret.OutputFiles[0].Contents
			if len(code) == 0 {
				return rex.Err(400, "code is empty")
			}
			h := sha1.New()
			h.Write(code)
			id := hex.EncodeToString(h.Sum(nil))
			key := "publish-" + id
			record, err := db.Get(key)
			if err != nil {
				return rex.Err(500, "failed to save code")
			}
			if record == nil {
				_, err = fs.WriteFile(path.Join("publish", id, "index.mjs"), bytes.NewReader(code))
				if err == nil {
					buf := bytes.NewBuffer(nil)
					enc := json.NewEncoder(buf)
					enc.Encode(map[string]interface{}{
						"name":         "~" + id,
						"version":      "0.0.0",
						"dependencies": deps,
						"type":         "module",
						"module":       "index.mjs",
					})
					_, err = fs.WriteFile(path.Join("publish", id, "package.json"), buf)
				}
				if err == nil {
					err = db.Put(key, utils.MustEncodeJSON(map[string]interface{}{
						"createdAt": time.Now().Unix(),
					}))
				}
			}
			if err != nil {
				return rex.Err(500, "failed to save code")
			}
			cdnOrigin := cfg.Origin
			if cdnOrigin == "" {
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				// use the request host as the origin if not set in config.json
				cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
			}
			return map[string]interface{}{
				"id":        id,
				"url":       fmt.Sprintf("%s/~%s", cdnOrigin, id),
				"bundleUrl": fmt.Sprintf("%s/~%s?bundle", cdnOrigin, id),
			}
		}
		return nil
	}
}

func getHandler() rex.Handle {
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
				return rex.Redirect(url, http.StatusMovedPermanently)
			}
		}

		// static routes
		switch pathname {
		case "/":
			// return deno cli script if the `User-Agent` is "Deno"
			if strings.HasPrefix(ctx.R.UserAgent(), "Deno/") {
				cliTs, err := embedFS.ReadFile("CLI.ts")
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
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/master/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte("{origin}"+cfg.BasePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			html = bytes.ReplaceAll(html, []byte("{basePath}"), []byte(cfg.BasePath))
			ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			q := make([]map[string]interface{}, buildQueue.list.Len())
			i := 0
			buildQueue.lock.RLock()
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
						"dev":        t.Dev,
						"bundle":     t.Bundle,
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

			n := 0
			purgeTimers.Range(func(key, value interface{}) bool {
				n++
				return true
			})

			res, err := fetch(fmt.Sprintf("http://localhost:%d", cfg.NsPort))
			if err != nil {
				kill(nsPidFile)
				return err
			}
			defer res.Body.Close()
			out, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}

			ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
			return map[string]interface{}{
				"buildQueue":  q[:i],
				"purgeTimers": n,
				"ns":          string(out),
				"version":     VERSION,
				"uptime":      time.Since(startTime).String(),
			}

		case "/build-target":
			return getTargetByUA(ctx.R.UserAgent())

		case "/error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Errorf(
					`could not resolve "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			case "unsupported-nodejs-builtin-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported nodejs builtin module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported Npm package "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported file dependency "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				))
			default:
				return throwErrorJS(ctx, fmt.Errorf("unknown error"))
			}

		case "/favicon.ico":
			return rex.Status(404, "not found")
		}

		cdnOrigin := cfg.Origin
		if cdnOrigin == "" {
			proto := "http"
			if ctx.R.TLS != nil {
				proto = "https"
			}
			// use the request host as the origin if not set in config.json
			cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
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

		// check if the request is for the CLI, support version prefix
		if strings.HasPrefix(ctx.R.UserAgent(), "Deno/") && pathname == "/" {
			cliTs, err := embedFS.ReadFile("CLI.ts")
			if err != nil {
				return err
			}
			ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
			return bytes.ReplaceAll(cliTs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", VERSION)))
		}

		// use embed polyfills/types if possible
		if hasBuildVerPrefix && strings.Count(pathname, "/") == 1 {
			if strings.HasSuffix(pathname, ".js") {
				data, err := embedFS.ReadFile("server/embed/polyfills" + pathname)
				if err == nil {
					ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					return rex.Content(pathname, startTime, bytes.NewReader(data))
				}
			}
			if strings.HasSuffix(pathname, ".d.ts") {
				data, err := embedFS.ReadFile("server/embed/types" + pathname)
				if err == nil {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					return rex.Content(pathname, startTime, bytes.NewReader(data))
				}
			}
		}

		// ban malicious requests by banList
		// trim the leading `/` in pathname to get the package name
		// e.g. /@withfig/autocomplete -> @withfig/autocomplete
		packageFullName := pathname[1:]
		pkgBanned := cfg.BanList.IsPackageBanned(packageFullName)
		if pkgBanned {
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

		if includes(nativeNodePackages, reqPkg.Name) {
			return throwErrorJS(ctx, fmt.Errorf(
				`unsupported npm package "%s": native node modules are not supported yet`,
				reqPkg.Name,
			))
		}

		// redirect to real wasm file: `/v100/PKG/es2022/foo.wasm` -> `/PKG/foo.wasm`
		if hasBuildVerPrefix && strings.HasSuffix(reqPkg.Submodule, ".wasm") {
			pkgRoot := path.Join(cfg.WorkDir, "npm", reqPkg.Name+"@"+reqPkg.Version, "node_modules", reqPkg.Name)
			wasmFiles, err := findFiles(pkgRoot, func(fp string) bool {
				return strings.HasSuffix(fp, ".wasm")
			})
			if err != nil {
				return rex.Status(500, err.Error())
			}
			var wasmFile string
			if l := len(wasmFiles); l == 1 {
				wasmFile = wasmFiles[0]
			} else if l > 1 {
				sort.Sort(sort.Reverse(PathSlice(wasmFiles)))
				for _, f := range wasmFiles {
					if strings.Contains(reqPkg.Subpath, f) {
						wasmFile = f
						break
					}
				}
			}
			if wasmFile == "" {
				return rex.Status(404, "Wasm File not found")
			}
			url := fmt.Sprintf("%s%s/%s@%s/%s", cdnOrigin, cfg.BasePath, reqPkg.Name, reqPkg.Version, wasmFile)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// redirect `/@types/PKG` to main dts files
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
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// redirect to main css path for CSS packages
		if css := cssPackages[reqPkg.Name]; css != "" && reqPkg.Submodule == "" {
			url := fmt.Sprintf("%s%s/%s/%s", cdnOrigin, cfg.BasePath, reqPkg.String(), css)
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
					bvPrefix = fmt.Sprintf("/v%d", VERSION)
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
					return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.BasePath, bvPrefix, ghPrefix, eaSign, reqPkg.Name, reqPkg.Version, query, subPath), http.StatusFound)
				}
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.BasePath, bvPrefix, ghPrefix, eaSign, reqPkg.Name, reqPkg.Version, subPath, query), http.StatusFound)
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
					bvPrefix = fmt.Sprintf("/v%d", VERSION)
				}
			}
			if reqPkg.Subpath != "" {
				subPath = "/" + reqPkg.Subpath
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s/%s%s%s", cdnOrigin, cfg.BasePath, bvPrefix, reqPkg.VersionName(), subPath, query), http.StatusFound)
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

		var storageType string
		if reqPkg.Submodule != "" {
			switch path.Ext(pathname) {
			case ".mjs", ".js":
				if hasBuildVerPrefix {
					storageType = "builds"
				}
			case ".css", ".map":
				if hasBuildVerPrefix {
					storageType = "builds"
				} else if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			case ".jsx", ".ts", ".mts", ".tsx":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					if !hasBuildVerPrefix {
						url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.BasePath, VERSION, pathname)
						return rex.Redirect(url, http.StatusMovedPermanently)
					}
					storageType = "types"
				} else if len(strings.Split(pathname, "/")) > 2 {
					// todo: transform ts/jsx/tsx for browsers
					storageType = "raw"
				}
			case ".wasm":
				if ctx.Form.Has("module") {
					buf := &bytes.Buffer{}
					wasmUrl := fmt.Sprintf("%s%s%s", cdnOrigin, cfg.BasePath, pathname)
					fmt.Fprintf(buf, "/* esm.sh - CompiledWasm */\n")
					fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
					return buf
				} else if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			case ".less", ".sass", ".scss", ".html", ".htm", ".md", ".txt", ".json", ".xml", ".yml", ".yaml", ".svg", ".png", ".jpg", ".webp", ".gif", ".eot", ".ttf", ".otf", ".woff", ".woff2":
				if len(strings.Split(pathname, "/")) > 2 {
					storageType = "raw"
				}
			}
		}

		// serve raw dist or npm dist files like CSS/map etc..
		if storageType == "raw" {
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
					BuildArgs: BuildArgs{
						alias:       map[string]string{},
						deps:        PkgSlice{},
						external:    newStringSet(),
						treeShaking: newStringSet(),
						conditions:  newStringSet(),
					},
					Target: "raw",
					stage:  "pending",
				}
				c := buildQueue.Add(task, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						return rex.Status(500, "Fail to install package: "+output.err.Error())
					}
					fi, err = os.Lstat(savePath)
					if err != nil {
						return rex.Status(500, err.Error())
					}
				case <-time.After(time.Minute):
					buildQueue.RemoveConsumer(task, c)
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
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content(savePath, fi.ModTime(), content) // auto closed
		}

		// serve build files
		if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
			var savePath string
			if outdatedBuildVer != "" {
				savePath = path.Join(storageType, outdatedBuildVer, pathname)
			} else if hasStablePrefix {
				savePath = path.Join(storageType, fmt.Sprintf("v%d", STABLE_VERSION), pathname)
			} else {
				savePath = path.Join(storageType, fmt.Sprintf("v%d", VERSION), pathname)
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
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				} else if endsWith(pathname, ".js", ".mjs") {
					ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
				} else if strings.HasSuffix(savePath, ".map") {
					ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				if ctx.Form.Has("worker") && storageType == "builds" {
					defer r.Close()
					buf, err := ioutil.ReadAll(r)
					if err != nil {
						return rex.Status(500, err.Error())
					}
					code := bytes.TrimSuffix(buf, []byte(fmt.Sprintf(`//# sourceMappingURL=%s.map`, path.Base(savePath))))
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
		if !stableBuild[reqPkg.Name] {
			for _, p := range strings.Split(ctx.Form.Value("exports"), ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					treeShaking.Add(p)
				}
			}
		}

		// check `?conditions` query
		conditions := newStringSet()
		for _, p := range strings.Split(ctx.Form.Value("conditions"), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				conditions.Add(p)
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

		isBare := false
		isPkgCss := ctx.Form.Has("css")
		isBundle := ctx.Form.Has("bundle") && !stableBuild[reqPkg.Name]
		isDev := ctx.Form.Has("dev")
		isPined := ctx.Form.Has("pin") || hasBuildVerPrefix || stableBuild[reqPkg.Name]
		isWorker := ctx.Form.Has("worker")
		noCheck := ctx.Form.Has("no-check") || ctx.Form.Has("no-dts")
		ignoreRequire := ctx.Form.Has("ignore-require") || ctx.Form.Has("no-require") || reqPkg.Name == "@unocss/preset-icons"
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
			treeShaking:       treeShaking,
		}

		// clear build args for stable build
		if stableBuild[reqPkg.Name] && reqPkg.Submodule == "" {
			buildArgs = BuildArgs{
				external:    newStringSet(),
				treeShaking: newStringSet(),
				conditions:  newStringSet(),
			}
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
				reqPkg.Subpath = strings.Join(strings.Split(reqPkg.Subpath, "/")[1:], "/")
				if args.denoStdVersion == "" {
					// ensure deno/std version used
					args.denoStdVersion = denoStdVersion
				}
				buildArgs = args
			}
		}

		// check `bare` mode
		if hasBuildVerPrefix && (endsWith(reqPkg.Subpath, ".mjs", ".js", ".css")) {
			a := strings.Split(reqPkg.Submodule, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
					pkgName := strings.TrimSuffix(path.Base(reqPkg.Name), ".js")
					if strings.HasSuffix(submodule, ".css") {
						if submodule == pkgName+".css" {
							reqPkg.Submodule = ""
							target = maybeTarget
							isBare = true
						} else {
							url := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.BasePath, reqPkg.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						if endsWith(submodule, ".bundle") {
							submodule = strings.TrimSuffix(submodule, ".bundle")
							isBundle = true
						}
						if endsWith(submodule, ".development") {
							submodule = strings.TrimSuffix(submodule, ".development")
							isDev = true
						}
						isPkgEntry := strings.HasSuffix(reqPkg.Subpath, ".mjs") // <- /v100/react@18.2.0/es2022/react.mjs
						if submodule == pkgName && !isPkgEntry && stableBuild[reqPkg.Name] {
							url := fmt.Sprintf("%s%s/%s@%s", cdnOrigin, cfg.BasePath, reqPkg.Name, reqPkg.Version)
							return rex.Redirect(url, http.StatusMovedPermanently)
						}
						if submodule == pkgName && isPkgEntry {
							submodule = ""
						}
						// workaround for es5-ext weird "/#/" path
						if submodule != "" && reqPkg.Name == "es5-ext" {
							submodule = strings.ReplaceAll(submodule, "/$$/", "/#/")
						}
						reqPkg.Submodule = submodule
						target = maybeTarget
						isBare = true
					}
				}
			}
		}

		if hasBuildVerPrefix && storageType == "types" {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				savePath = path.Join(fmt.Sprintf(
					"types/v%d%s/%s@%s/%s",
					buildVersion,
					ghPrefix,
					reqPkg.Name,
					reqPkg.Version,
					encodeBuildArgsPrefix(buildArgs, reqPkg.Name, true),
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
			Dev:          isDev,
			Bundle:       isBundle || isWorker,
			stage:        "pending",
		}

		taskID := task.ID()
		esm, hasBuild := queryESMBuild(taskID)
		fallback := false

		if !hasBuild {
			if !isBare && !isPined {
				// find previous build version
				for i := 0; i < VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", VERSION-(i+1), strings.Join(strings.Split(taskID, "/")[1:], "/"))
					esm, hasBuild = queryESMBuild(id)
					if hasBuild {
						log.Warn("fallback to previous build:", id)
						fallback = true
						taskID = id
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
						return throwErrorJS(ctx, output.err)
					}
					esm = output.meta
				case <-time.After(time.Minute):
					buildQueue.RemoveConsumer(task, c)
					return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
				}
			}
		}

		// should redirect to `*.d.ts` file
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

		// redirect to package css from `?css`
		if isPkgCss && reqPkg.Submodule == "" {
			if !esm.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s/%s.css", cdnOrigin, cfg.BasePath, strings.TrimSuffix(taskID, path.Ext(taskID)))
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		if isBare {
			savePath := task.getSavepath()
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
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			if isWorker && endsWith(savePath, ".mjs", ".js") {
				buf, err := ioutil.ReadAll(f)
				f.Close()
				if err != nil {
					return rex.Status(500, err.Error())
				}
				code := bytes.TrimSuffix(buf, []byte(fmt.Sprintf(`//# sourceMappingURL=%s.map`, path.Base(savePath))))
				ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
				return fmt.Sprintf(`export default function workerFactory() { const blob = new Blob([%s], { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module" })}`, utils.MustEncodeJSON(string(code)))
			}
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, reqPkg, "\n")

		if isWorker {
			fmt.Fprintf(buf, `export { default } from "%s%s/%s?worker";`, cdnOrigin, cfg.BasePath, taskID)
		} else {
			fmt.Fprintf(buf, `export * from "%s%s/%s";%s`, cdnOrigin, cfg.BasePath, taskID, "\n")
			if (esm.CJS || esm.HasExportDefault) && (treeShaking.Size() == 0 || treeShaking.Has("default")) {
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
		if fallback {
			ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
		} else {
			if isPined {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				ctx.SetHeader("Cache-Control", fmt.Sprintf("public, max-age=%d", 24*3600)) // cache for 24 hours
			}
		}
		if targetFromUA {
			ctx.AddHeader("Vary", "User-Agent")
		}
		ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
		return buf
	}
}

func XAuth(secret string) rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if secret != "" && ctx.R.Header.Get("X-Auth-Secret") != secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
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
