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

		// use esm-worker build version if possible
		BUILD_VERSION := VERSION
		if v := ctx.R.Header.Get("X-Esm-Worker-Version"); v != "" && strings.HasPrefix(v, "v") {
			i, e := strconv.Atoi(v[1:])
			if e == nil && i > 0 {
				BUILD_VERSION = i
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

		// serve the CLI script for Deno/Node.js/Bun runtime
		if userAgent == "undici" || strings.HasPrefix(userAgent, "Node/") || strings.HasPrefix(userAgent, "Deno/") || strings.HasPrefix(userAgent, "Bun/") {
			if pathname == "/" || regexpCliPath.MatchString(pathname) {
				if strings.HasPrefix(userAgent, "Deno/") {
					cliTs, err := embedFS.ReadFile("server/embed/CLI.deno.ts")
					if err != nil {
						return err
					}
					header.Set("Content-Type", "application/typescript; charset=utf-8")
					return bytes.ReplaceAll(cliTs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", BUILD_VERSION)))
				}
				if userAgent == "undici" || strings.HasPrefix(userAgent, "Node/") || strings.HasPrefix(userAgent, "Bun/") {
					cliJs, err := embedFS.ReadFile("server/embed/CLI.node.js")
					if err != nil {
						return err
					}
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					cliJs = bytes.ReplaceAll(cliJs, []byte("v{VERSION}"), []byte(fmt.Sprintf("v%d", BUILD_VERSION)))
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
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(cdnOrigin+cfg.CdnBasePath))
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", BUILD_VERSION)))
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

			header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
			return map[string]interface{}{
				"buildQueue": q[:i],
				"version":    BUILD_VERSION,
				"uptime":     time.Since(startTime).String(),
			}

		case "/esma-target":
			return getBuildTargetByUA(userAgent)

		case "/error.js":
			query := ctx.R.URL.Query()
			switch query.Get("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Errorf(
					`could not resolve "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-builtin-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported Node builtin module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-native-module":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported node native module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported NPM package "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Errorf(
					`unsupported file dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			default:
				return throwErrorJS(ctx, fmt.Errorf("unknown error"), true)
			}

		case "/favicon.ico":
			return rex.Status(404, "not found")
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			modTime := startTime
			if fs, ok := embedFS.(*devFS); ok {
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
				data = bytes.ReplaceAll(data, []byte("{buildVersion}"), []byte(fmt.Sprintf("v%d", BUILD_VERSION)))
			}
			header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 10*60))
			return rex.Content(pathname, modTime, bytes.NewReader(data))
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
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
			header.Add("Vary", "User-Agent")
			return rex.Content(savaPath, fi.ModTime(), r) // auto closed
		}

		var hasBuildVerPrefix bool
		var hasPinedVerParam bool
		var hasStablePrefix bool
		var outdatedBuildVersion string

		// check build version prefix or `?pin=` query
		buildVersion := BUILD_VERSION
		if strings.HasPrefix(pathname, "/stable/") {
			pathname = strings.TrimPrefix(pathname, "/stable")
			hasBuildVerPrefix = true
			hasStablePrefix = true
		} else if strings.HasPrefix(pathname, fmt.Sprintf("/v%d/", BUILD_VERSION)) || pathname == fmt.Sprintf("/v%d", BUILD_VERSION) {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
		} else if regexpBuildVersionPath.MatchString(pathname) {
			// check possible fixed build version
			a := strings.Split(pathname, "/")
			v, _ := strconv.Atoi(a[1][1:])
			if v == 0 || v > BUILD_VERSION {
				return rex.Status(400, "Bad Request")
			}
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			outdatedBuildVersion = a[1]
			buildVersion = v
		} else if strings.Contains(ctx.R.URL.RawQuery, "pin=") {
			// Otherwise check "?pin=" query
			pin := ctx.R.URL.Query().Get("pin")
			if pin != "" && strings.HasPrefix(pin, "v") {
				i, err := strconv.Atoi(pin[1:])
				if err == nil && i > 0 && i < BUILD_VERSION {
					buildVersion = i
					hasPinedVerParam = true
				}
			}
		}

		// serve internal scripts
		if pathname == "/build" || pathname == "/run" || pathname == "/hot" {
			if !hasBuildVerPrefix && !hasPinedVerParam {
				url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, BUILD_VERSION, pathname)
				if ctx.R.URL.RawQuery != "" {
					url += "?" + ctx.R.URL.RawQuery
				}
				// redirect to the url with build version prefix
				return rex.Redirect(url, http.StatusFound)
			}

			filename := pathname[1:]
			data, err := embedFS.ReadFile(fmt.Sprintf("%s.ts", filename))
			if err != nil {
				return rex.Status(404, "Not Found")
			}

			//  add 'X-TypeScript-Types' header for the `/hot` script
			if pathname == "/hot" {
				header.Set("X-TypeScript-Types", fmt.Sprintf("%s%s/v%d/hot.d.ts", cdnOrigin, cfg.CdnBasePath, buildVersion))
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
					fv := ctx.R.URL.Query().Get("refs")
					if fv != "" {
						var refs = make([]string, strings.Count(fv, ",")+1)
						for i, ref := range strings.Split(fv, ",") {
							url := fmt.Sprintf("%s%s/v%d/%s", cdnOrigin, cfg.CdnBasePath, buildVersion, ref)
							refs[i] = fmt.Sprintf("/// <reference path=\"%s\" />", url)
						}
						data = concatBytes([]byte(strings.Join(refs, "\n")), data)
					}
					header.Set("Content-Type", "application/typescript; charset=utf-8")
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
					return rex.Content(pathname, startTime, bytes.NewReader(data))
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

		if extraQuery != "" {
			qs := []string{extraQuery}
			if ctx.R.URL.RawQuery != "" {
				qs = append(qs, ctx.R.URL.RawQuery)
			}
			ctx.R.URL.RawQuery = strings.Join(qs, "&")
		}
		query := ctx.R.URL.Query()

		pkgAllowed := cfg.AllowList.IsPackageAllowed(reqPkg.Name)
		pkgBanned := cfg.BanList.IsPackageBanned(reqPkg.Name)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		// fix url related `import.meta.url`
		if hasBuildVerPrefix && endsWith(reqPkg.SubPath, ".wasm", ".json") {
			extname := path.Ext(reqPkg.SubPath)
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
			url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, BUILD_VERSION, pathname)
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
		if !hasBuildVerPrefix && !reqPkg.FromEsmsh && !strings.Contains(pathname, "@"+reqPkg.Version) {
			pkgName := reqPkg.Name
			bvPrefix := ""
			eaSign := ""
			subPath := ""
			query := ""
			if strings.HasPrefix(pkgName, "@jsr/") {
				pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
			}
			if endsWith(pathname, ".d.ts", ".d.mts") {
				if outdatedBuildVersion != "" {
					bvPrefix = fmt.Sprintf("/%s", outdatedBuildVersion)
				} else {
					bvPrefix = fmt.Sprintf("/v%d", BUILD_VERSION)
				}
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
					return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, ghPrefix, eaSign, pkgName, reqPkg.Version, query, subPath), http.StatusFound)
				}
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s%s/%s%s@%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, ghPrefix, eaSign, pkgName, reqPkg.Version, subPath, query), http.StatusFound)
		}

		// redirect to the url with full package version with build version prefix
		if hasBuildVerPrefix && !strings.Contains(pathname, "@"+reqPkg.Version) {
			bvPrefix := ""
			subPath := ""
			query := ""
			if stableBuild[reqPkg.Name] {
				bvPrefix = "/stable"
			} else if outdatedBuildVersion != "" {
				bvPrefix = fmt.Sprintf("/%s", outdatedBuildVersion)
			} else {
				bvPrefix = fmt.Sprintf("/v%d", BUILD_VERSION)
			}
			if reqPkg.SubPath != "" {
				subPath = "/" + reqPkg.SubPath
			}
			if ctx.R.URL.RawQuery != "" {
				query = "?" + ctx.R.URL.RawQuery
			}
			return rex.Redirect(fmt.Sprintf("%s%s%s/%s%s%s", cdnOrigin, cfg.CdnBasePath, bvPrefix, reqPkg.VersionName(), subPath, query), http.StatusFound)
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
		if v := query.Get("path"); v != "" {
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
			case ".mjs", ".js", ".jsx", ".ts", ".mts", ".tsx":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					if !hasBuildVerPrefix {
						url := fmt.Sprintf("%s%s/v%d%s", cdnOrigin, cfg.CdnBasePath, BUILD_VERSION, pathname)
						return rex.Redirect(url, http.StatusMovedPermanently)
					}
					reqType = "types"
				} else if ctx.R.URL.Query().Has("raw") {
					reqType = "raw"
				} else if hasBuildVerPrefix && hasTargetSegment(reqPkg.SubPath) {
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
				if hasBuildVerPrefix && hasTargetSegment(reqPkg.SubPath) {
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
		if hasBuildVerPrefix && (reqType == "builds" || reqType == "types") {
			var savePath string
			if outdatedBuildVersion != "" {
				savePath = path.Join(reqType, outdatedBuildVersion, pathname)
			} else if hasStablePrefix {
				savePath = path.Join(reqType, fmt.Sprintf("v%d", STABLE_VERSION), pathname)
			} else {
				savePath = path.Join(reqType, fmt.Sprintf("v%d", BUILD_VERSION), pathname)
			}
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
		target := strings.ToLower(query.Get("target"))
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
			for _, p := range strings.Split(query.Get("alias"), ",") {
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
			for _, p := range strings.Split(query.Get("deps"), ",") {
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
			value := query.Get("exports") + "," + query.Get("cjs-exports")
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
			for _, p := range strings.Split(query.Get("conditions"), ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					conditions.Add(p)
				}
			}
		}

		// check deno/std version by `?deno-std=VER` query
		dsv := denoStdVersion
		fv := query.Get("deno-std")
		if fv != "" && regexpFullVersion.MatchString(fv) && semverLessThan(fv, denoStdVersion) && target == "deno" {
			dsv = fv
		}

		// check `?external` query
		for _, p := range strings.Split(query.Get("external"), ",") {
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
		bundleDeps := ctx.Form.Has("bundle") || ctx.Form.Has("standalone") || ctx.Form.Has("bundle-deps")
		noBundle := !bundleDeps && (ctx.Form.Has("bundless") || ctx.Form.Has("no-bundle"))
		isDev := ctx.Form.Has("dev")
		isPined := hasBuildVerPrefix || hasPinedVerParam || stableBuild[reqPkg.Name]
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

		// parse and use `X-` prefix
		if hasBuildVerPrefix {
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

		// clear build args for main entry of stable builds
		if stableBuild[reqPkg.Name] && reqPkg.SubModule == "" {
			buildArgs = BuildArgs{
				external:   newStringSet(),
				exports:    newStringSet(),
				conditions: buildArgs.conditions,
			}
		}

		// check if it's a build file
		isBuildFile := false
		if hasBuildVerPrefix && (endsWith(reqPkg.SubPath, ".mjs", ".js", ".css")) {
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
							bundleDeps = true
						} else if strings.HasSuffix(submodule, ".bundless") {
							submodule = strings.TrimSuffix(submodule, ".bundless")
							noBundle = true
						}
						if strings.HasSuffix(submodule, ".development") {
							submodule = strings.TrimSuffix(submodule, ".development")
							isDev = true
						}
						isMjs := strings.HasSuffix(reqPkg.SubPath, ".mjs")
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
						reqPkg.SubModule = submodule
						target = maybeTarget
						isBuildFile = true
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
			if !isBuildFile && !isPined {
				// find previous build version
				for i := 0; i < BUILD_VERSION; i++ {
					id := fmt.Sprintf("v%d/%s", BUILD_VERSION-(i+1), strings.Join(strings.Split(buildId, "/")[1:], "/"))
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
				case <-time.After(10 * time.Minute):
					buildQueue.RemoveConsumer(task, c)
					header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
					return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
				}
			}
		}

		// should redirect to `*.d.ts` file
		if esm.TypesOnly {
			dtsUrl := fmt.Sprintf("%s%s/%s", cdnOrigin, cfg.CdnBasePath, strings.TrimPrefix(esm.Dts, "/"))
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			if fallback {
				header.Set("Cache-Control", "private, no-store, no-cache, must-revalidate")
			} else {
				if isPined {
					header.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 7*24*3600)) // cache for 7 days
				}
			}
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
			code := 302
			if isPined {
				code = 301
			}
			return rex.Redirect(url, code)
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
				header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", 7*24*3600)) // cache for 7 days
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
