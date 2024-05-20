package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
)

var (
	ccMustRevalidate = "public, max-age=0, must-revalidate"
	cc10min          = "public, max-age=600"
	cc1day           = "public, max-age=86400"
	ccImmutable      = "public, max-age=31536000, immutable"
	ctJavascript     = "application/javascript; charset=utf-8"
	ctTypescript     = "application/typescript; charset=utf-8"
)

func router() rex.Handle {
	startTime := time.Now()

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()
		header := ctx.W.Header()
		userAgent := ctx.R.UserAgent()

		cdnOrigin := ctx.R.Header.Get("X-Real-Origin")
		// use current host as cdn origin if not set
		if cdnOrigin == "" {
			proto := "http"
			if ctx.R.TLS != nil {
				proto = "https"
			}
			cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
		}

		// ban malicious requests
		if strings.HasPrefix(pathname, "/.") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// handle POST requests
		if ctx.R.Method == "POST" {
			switch ctx.Path.String() {
			case "/transform":
				var input TransofrmInput
				err := json.NewDecoder(io.LimitReader(ctx.R.Body, 2*1024*1024)).Decode(&input)
				ctx.R.Body.Close()
				if err != nil {
					return rex.Err(400, "require valid json body")
				}
				if input.Code == "" {
					return rex.Err(400, "Code is required")
				}
				if len(input.Code) > 1024*1024 {
					return rex.Err(429, "Code is too large")
				}
				if targets[input.Target] == 0 {
					input.Target = getBuildTargetByUA(ctx.R.UserAgent())
				}
				var loader string
				extname := path.Ext(input.Filename)
				switch extname {
				case ".js", ".jsx", ".ts", ".tsx":
					loader = extname[1:]
				default:
					loader = "js"
				}

				h := sha1.New()
				h.Write([]byte(loader))
				h.Write([]byte(input.Code))
				h.Write([]byte(input.ImportMap))
				hash := hex.EncodeToString(h.Sum(nil))

				// if previous build exists, return it directly
				savePath := fmt.Sprintf("modules/%s.%s.mjs", hash, input.Target)
				_, err = fs.Stat(savePath)
				if err == nil {
					r, err := fs.Open(savePath)
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					code, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					return map[string]interface{}{
						"code": string(code),
					}
				}
				code, err := transform(input)
				if err != nil {
					if strings.HasPrefix(err.Error(), "<400> ") {
						return rex.Err(400, err.Error()[6:])
					}
					return rex.Err(500, "failed to save code")
				}
				go fs.WriteFile(savePath, strings.NewReader(code))
				ctx.W.Header().Set("Cache-Control", ccMustRevalidate)
				return map[string]interface{}{
					"code": code,
				}
			case "/purge":
				packageName := ctx.Form.Value("package")
				version := ctx.Form.Value("version")
				github := ctx.Form.Has("github")
				zoneId := ctx.Form.Value("zone-id")
				if packageName == "" {
					return rex.Err(400, "packageName is required")
				}
				prefix := packageName + "@"
				if version != "" {
					prefix += version
				}
				if github {
					prefix = fmt.Sprintf("gh/%s", packageName)
				}
				if zoneId != "" {
					prefix = fmt.Sprintf("%s/%s", zoneId, prefix)
				}
				deletedRecords, err := db.DeleteAll(prefix)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				removedFiles := []string{}
				for _, kv := range deletedRecords {
					var ret BuildResult
					filename := string(kv[0])
					if json.Unmarshal(kv[1], &ret) == nil {
						savePath := fmt.Sprintf("builds/%s", filename)
						go fs.Remove(savePath)
						go fs.Remove(savePath + ".map")
						if ret.PackageCSS {
							cssFilename := strings.TrimSuffix(filename, path.Ext(filename)) + ".css"
							go fs.Remove(fmt.Sprintf("builds/%s", cssFilename))
							removedFiles = append(removedFiles, cssFilename)
						}
						removedFiles = append(removedFiles, filename)
					}
				}
				return removedFiles
			default:
				return rex.Err(404, "not found")
			}
		}

		// static routes
		switch pathname {
		case "/":
			eTag := fmt.Sprintf(`W/"v%d"`, VERSION)
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == eTag {
				return rex.Status(http.StatusNotModified, "")
			}
			indexHTML, err := embedFS.ReadFile("server/embed/index.html")
			if err != nil {
				return err
			}
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			readme = bytes.ReplaceAll(readme, []byte("./server/embed/"), []byte("/embed/"))
			readme = bytes.ReplaceAll(readme, []byte("./HOSTING.md"), []byte("https://github.com/esm-dev/esm.sh/blob/main/HOSTING.md"))
			readme = bytes.ReplaceAll(readme, []byte("https://esm.sh"), []byte(cdnOrigin))
			readmeStrLit := mustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			header.Set("Cache-Control", ccMustRevalidate)
			header.Set("ETag", eTag)
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			q := make([]map[string]interface{}, buildQueue.queue.Len())
			i := 0

			buildQueue.lock.RLock()
			for el := buildQueue.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*QueueTask)
				if ok {
					m := map[string]interface{}{
						"clients":   t.clients,
						"createdAt": t.createdAt.Format(http.TimeFormat),
						"inProcess": t.inProcess,
						"path":      t.path,
						"pkg":       t.pkg.String(),
						"stage":     t.stage,
					}
					if !t.startedAt.IsZero() {
						m["startedAt"] = t.startedAt.Format(http.TimeFormat)
					}
					if len(t.args.deps) > 0 {
						m["deps"] = t.args.deps.String()
					}
					q[i] = m
					i++
				}
			}
			buildQueue.lock.RUnlock()

			header.Set("Cache-Control", ccMustRevalidate)
			return map[string]interface{}{
				"buildQueue": q[:i],
				"version":    VERSION,
				"uptime":     time.Since(startTime).String(),
			}

		case "/esma-target":
			header.Set("Cache-Control", ccMustRevalidate)
			return getBuildTargetByUA(userAgent)

		case "/error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Could not resolve "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-node-builtin-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported Node builtin module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-node-native-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported node native module "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported NPM package "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported file dependency "%s" (Imported by "%s")`,
					ctx.Form.Value("name"),
					ctx.Form.Value("importer"),
				), true)
			default:
				return throwErrorJS(ctx, "Unknown error", true)
			}

		case "/favicon.ico":
			favicon, err := embedFS.ReadFile("server/embed/favicon.ico")
			if err != nil {
				return err
			}
			header.Set("Cache-Control", ccImmutable)
			return rex.Content("favicon.ico", startTime, bytes.NewReader(favicon))
		}

		// strip loc suffix
		if strings.ContainsRune(pathname, ':') {
			pathname = regexpLocPath.ReplaceAllString(pathname, "$1")
		}

		// serve run and sw scripts
		if pathname == "/run" || pathname == "/sw" {
			data, err := embedFS.ReadFile(fmt.Sprintf("server/embed/%s.ts", pathname[1:]))
			if err != nil {
				return rex.Status(404, "Not Found")
			}

			etag := fmt.Sprintf(`W/"v%d"`, VERSION)
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == etag {
				return rex.Status(http.StatusNotModified, "")
			}

			// determine build target by `?target` query or `User-Agent` header
			target := strings.ToLower(ctx.Form.Value("target"))
			targetViaUA := targets[target] == 0
			if targetViaUA {
				target = getBuildTargetByUA(userAgent)
			}

			// inject `fire()` to the sw script when `?fire` is attached
			if pathname == "/sw" && ctx.Form.Has("fire") {
				data = concatBytes(data, []byte("\nsw.fire();\n"))
			}

			code, err := minify(string(data), targets[target], api.LoaderTS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}
			header.Set("Content-Type", ctJavascript)
			if targetViaUA {
				appendVaryHeader(header, "User-Agent")
			}
			if ctx.Form.Value("v") != "" {
				header.Set("Cache-Control", ccImmutable)
			} else {
				header.Set("Cache-Control", cc1day)
				header.Set("ETag", etag)
			}
			if pathname == "/sw" {
				header.Set("X-Typescript-Types", fmt.Sprintf("%s/sw.d.ts", cdnOrigin))
			}
			return code
		}

		// serve embed assets
		if strings.HasPrefix(pathname, "/embed/") {
			modTime := startTime
			if fs, ok := embedFS.(*MockEmbedFS); ok {
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
			}
			header.Set("Cache-Control", cc1day)
			return rex.Content(pathname, modTime, bytes.NewReader(data))
		}

		// serve modules created by the build API
		if strings.HasPrefix(pathname, "/+") {
			hash, ext := utils.SplitByLastByte(pathname[2:], '.')
			if len(hash) != 40 || ext != "mjs" {
				return rex.Status(404, "not found")
			}
			target := getBuildTargetByUA(userAgent)
			savePath := fmt.Sprintf("modules/%s.%s.%s", hash, target, ext)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "not found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := fs.Open(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Content-Type", ctJavascript)
			header.Set("Cache-Control", ccImmutable)
			appendVaryHeader(header, "User-Agent")
			return rex.Content(savePath, fi.ModTime(), r) // auto closed
		}

		// serve node libs
		if strings.HasPrefix(pathname, "/node/") && strings.HasSuffix(pathname, ".js") {
			lib, ok := nodeLibs[pathname[1:]]
			if !ok {
				// empty module
				lib = "export default {}"
			}
			if strings.HasPrefix(pathname, "/node/chunk-") {
				header.Set("Cache-Control", ccImmutable)
			} else {
				etag := fmt.Sprintf(`W/"v%d"`, VERSION)
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == etag {
					return rex.Status(http.StatusNotModified, "")
				}
				if ctx.Form.Value("v") != "" {
					header.Set("Cache-Control", ccImmutable)
				} else {
					header.Set("Cache-Control", cc1day)
					header.Set("ETag", etag)
				}
			}
			target := getBuildTargetByUA(userAgent)
			code, err := minify(lib, targets[target], api.LoaderJS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}
			appendVaryHeader(header, "User-Agent")
			header.Set("Content-Type", ctJavascript)
			return rex.Content(pathname, startTime, bytes.NewReader(code))
		}

		// use embed polyfills/types
		if endsWith(pathname, ".js", ".d.ts") && strings.Count(pathname, "/") == 1 {
			var data []byte
			var err error
			isDts := strings.HasSuffix(pathname, ".d.ts")
			if isDts {
				data, err = embedFS.ReadFile("server/embed/types" + pathname)
			} else {
				data, err = embedFS.ReadFile("server/embed/polyfills" + pathname)
			}
			if err == nil {
				etag := fmt.Sprintf(`W/"v%d"`, VERSION)
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == etag {
					return rex.Status(http.StatusNotModified, "")
				}
				if ctx.Form.Value("v") != "" {
					header.Set("Cache-Control", ccImmutable)
				} else {
					header.Set("Cache-Control", cc1day)
					header.Set("ETag", etag)
				}
				if isDts {
					header.Set("Content-Type", ctTypescript)
				} else {
					target := getBuildTargetByUA(userAgent)
					code, err := minify(string(data), targets[target], api.LoaderJS)
					if err != nil {
						return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
					}
					data = []byte(code)
					header.Set("Content-Type", ctJavascript)
					appendVaryHeader(header, "User-Agent")
				}
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
		}

		// check `/*pathname` or `/gh/*pathname` pattern
		external := NewStringSet()
		if strings.HasPrefix(pathname, "/*") {
			external.Add("*")
			pathname = "/" + pathname[2:]
		} else if strings.HasPrefix(pathname, "/gh/*") {
			external.Add("*")
			pathname = "/gh/" + pathname[5:]
		}

		var npmrc *NpmRC
		if rc := ctx.R.Header.Get("X-Npmrc"); rc != "" {
			rc, err := NewNpmRcFromJSON([]byte(rc))
			if err != nil {
				return rex.Status(400, "Invalid Npmrc Header")
			}
			npmrc = rc
		} else {
			npmrc = NewNpmRcFromConfig(config)
		}

		zoneId := ctx.R.Header.Get("X-Zone-Id")
		if zoneId != "" && !valid.IsDomain(zoneId) {
			zoneId = ""
		}
		if zoneId != "" {
			pkgName, _, _, _ := splitPkgPath(pathname)
			scopeName := ""
			if strings.HasPrefix(pkgName, "@") {
				return pkgName[:strings.Index(pkgName, "/")]
			}
			if scopeName != "" {
				reg, ok := npmrc.Registries[scopeName]
				if !ok || (reg.Registry == jsrRegistry && reg.Token == "" && (reg.User == "" || reg.Password == "")) {
					zoneId = ""
				}
			} else if npmrc.Registry == npmRegistry && npmrc.Token == "" && (npmrc.User == "" || npmrc.Password == "") {
				zoneId = ""
			}
		}
		if zoneId != "" {
			npmrc.zoneId = zoneId
			cdnOrigin = fmt.Sprintf("https://%s", zoneId)
		}

		// get package info
		pkg, extraQuery, caretVersion, isTargetUrl, err := validatePkgPath(npmrc, pathname)
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

		// apply _extra query_ to the url
		if extraQuery != "" {
			qs := []string{extraQuery}
			if ctx.R.URL.RawQuery != "" {
				qs = append(qs, ctx.R.URL.RawQuery)
			}
			ctx.R.URL.RawQuery = strings.Join(qs, "&")
		}

		pkgAllowed := config.AllowList.IsPackageAllowed(pkg.Name)
		pkgBanned := config.BanList.IsPackageBanned(pkg.Name)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		ghPrefix := ""

		if pkg.FromGithub {
			ghPrefix = "/gh"
		}

		// redirect `/@types/PKG` to it's main dts file
		if strings.HasPrefix(pkg.Name, "@types/") && pkg.SubModule == "" {
			info, err := npmrc.getPackageInfo(pkg.Name, pkg.Version)
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
			return rex.Redirect(fmt.Sprintf("%s/%s", cdnOrigin, types), http.StatusFound)
		}

		// redirect to main css path for CSS packages
		if css := cssPackages[pkg.Name]; css != "" && pkg.SubModule == "" {
			url := fmt.Sprintf("%s/%s/%s", cdnOrigin, pkg.String(), css)
			return rex.Redirect(url, http.StatusFound)
		}

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"jsx-runtime", "jsx-dev-runtime"} {
			if strings.HasSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime) {
				if pkg.SubModule == "" {
					pkg.SubModule = jsxRuntime
				} else {
					pkg.SubModule = pkg.SubModule + "/" + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", pkg.Name, pkg.SubModule)
				ctx.R.URL.RawQuery = strings.TrimSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime)
			}
		}

		// or use `?path=$PATH` query to override the pathname
		if v := ctx.Form.Value("path"); v != "" {
			pkg.SubModule = utils.CleanPath(v)[1:]
		}

		// check the response file type
		// - raw: serve raw dist or npm dist files like CSS/map etc..
		// - build: serve es module files
		// - types: serve `.d.ts` files
		resType := "main"
		if pkg.SubPath != "" {
			ext := path.Ext(pkg.SubPath)
			switch ext {
			case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					isTargetUrl = false
					resType = "types"
				} else if ctx.R.URL.Query().Has("raw") {
					isTargetUrl = false
					resType = "raw"
				} else if isTargetUrl {
					resType = "build"
				}
			case ".css", ".mjs.map", ".js.map":
				if isTargetUrl {
					resType = "build"
				} else {
					resType = "raw"
				}
			default:
				if ext != "" && assetExts[ext[1:]] {
					resType = "raw"
				}
			}
		}

		// redirect to the url with full package version
		if !strings.Contains(pathname, pkg.FullName()) {
			if !isTargetUrl {
				skipRedirect := caretVersion && resType == "main" && !pkg.FromGithub
				if !skipRedirect {
					pkgName := pkg.Name
					eaSign := ""
					subPath := ""
					query := ""
					if strings.HasPrefix(pkgName, "@jsr/") {
						pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
					}
					if external.Has("*") {
						eaSign = "*"
					}
					if pkg.SubPath != "" {
						subPath = "/" + pkg.SubPath
					}
					header.Set("Cache-Control", cc10min)
					if rawQuery := ctx.R.URL.RawQuery; rawQuery != "" {
						if extraQuery != "" {
							query = "&" + rawQuery
							return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, eaSign, pkgName, pkg.Version, query, subPath), http.StatusFound)
						}
						query = "?" + rawQuery
					}
					return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, eaSign, pkgName, pkg.Version, subPath, query), http.StatusFound)
				}
			} else {
				subPath := ""
				query := ""
				if pkg.SubPath != "" {
					subPath = "/" + pkg.SubPath
				}
				if ctx.R.URL.RawQuery != "" {
					query = "?" + ctx.R.URL.RawQuery
				}
				header.Set("Cache-Control", cc10min)
				return rex.Redirect(fmt.Sprintf("%s/%s%s%s", cdnOrigin, pkg.FullName(), subPath, query), http.StatusFound)
			}
		}

		// serve `*.wasm` as a es module (nees top-level-await support)
		if resType == "raw" && strings.HasSuffix(pkg.SubPath, ".wasm") && ctx.Form.Has("module") {
			buf := &bytes.Buffer{}
			wasmUrl := cdnOrigin + pathname
			fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
			fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(mustEncodeJSON(wasmUrl))))
			header.Set("Cache-Control", ccImmutable)
			header.Set("Content-Type", ctJavascript)
			return buf
		}

		// fix url that is related to `import.meta.url`
		if resType == "raw" && isTargetUrl {
			extname := path.Ext(pkg.SubPath)
			dir := path.Join(npmrc.Dir(), pkg.FullName())
			if !existsDir(dir) {
				err := npmrc.installPackage(pkg)
				if err != nil {
					return rex.Status(500, err.Error())
				}
			}
			pkgRoot := path.Join(dir, "node_modules", pkg.Name)
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
					if strings.HasSuffix(pkg.SubPath, f) {
						file = f
						break
					}
				}
				if file == "" {
					for _, f := range files {
						if path.Base(pkg.SubPath) == path.Base(f) {
							file = f
							break
						}
					}
				}
			}
			if file == "" {
				return rex.Status(404, "File not found")
			}
			url := fmt.Sprintf("%s/%s@%s/%s", cdnOrigin, pkg.Name, pkg.Version, file)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// serve raw dist or npm dist files like CSS/map etc..
		if resType == "raw" {
			savePath := path.Join(npmrc.Dir(), pkg.FullName(), "node_modules", pkg.Name, pkg.SubPath)
			fi, err := os.Lstat(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				// if the file not found, try to install the package
				err = npmrc.installPackage(pkg)
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
			if fi.Size() > 100*1024*1024 {
				return rex.Status(403, "File Too Large")
			}
			content, err := os.Open(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				return rex.Status(404, "File Not Found")
			}
			header.Set("Cache-Control", ccImmutable)
			if endsWith(savePath, ".js", ".mjs", ".jsx") {
				header.Set("Content-Type", ctJavascript)
			} else if endsWith(savePath, ".ts", ".mts", ".tsx") {
				header.Set("Content-Type", ctTypescript)
			}
			return rex.Content(savePath, fi.ModTime(), content) // auto closed
		}

		// serve build/types files
		if (isTargetUrl && resType == "build") || resType == "types" {
			var savePath string
			if resType == "build" {
				savePath = path.Join("builds", pathname)
			} else {
				savePath = path.Join("types", pathname)
			}
			savePath = normalizeSavePath(zoneId, savePath)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound && strings.HasPrefix(pathname, ".map") {
					return rex.Status(404, "Not found")
				}
				if err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
			}
			if err == nil {
				if resType == "types" {
					_, err := fs.Stat(savePath + ".vo")
					if err != nil && err != storage.ErrNotFound {
						return rex.Status(500, err.Error())
					}
					if err == nil {
						r, err := fs.Open(savePath)
						if err != nil {
							return rex.Status(500, err.Error())
						}
						defer r.Close()
						buffer, err := io.ReadAll(r)
						if err != nil {
							return rex.Status(500, err.Error())
						}
						header.Set("Content-Type", ctTypescript)
						header.Set("Cache-Control", ccImmutable)
						return bytes.ReplaceAll(buffer, []byte("__ESM_CDN_ORIGIN__"), []byte(cdnOrigin))
					}
				}
				if ctx.Form.Has("worker") && resType == "build" {
					moduleUrl := cdnOrigin + pathname
					header.Set("Content-Type", ctJavascript)
					header.Set("Cache-Control", ccImmutable)
					return fmt.Sprintf(
						`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
						moduleUrl,
						moduleUrl,
					)
				}
				r, err := fs.Open(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				if resType == "types" {
					header.Set("Content-Type", ctTypescript)
				} else if strings.HasPrefix(pathname, ".map") {
					header.Set("Content-Type", "application/json; charset=utf-8")
				} else {
					header.Set("Content-Type", ctJavascript)
				}
				header.Set("Cache-Control", ccImmutable)
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
					if name != "" && to != "" && name != pkg.Name {
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
					m, _, _, _, err := validatePkgPath(npmrc, p)
					if err != nil {
						if strings.HasSuffix(err.Error(), "not found") {
							continue
						}
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", p))
					}
					if pkg.Name == "react-dom" && m.Name == "react" {
						// the `react` version always matches `react-dom` version
						continue
					}
					if !deps.Has(m.Name) && m.Name != pkg.Name {
						deps = append(deps, m)
					}
				}
			}
		}

		// check `?exports` query
		exports := NewStringSet()
		if ctx.Form.Has("exports") {
			value := ctx.Form.Value("exports")
			for _, p := range strings.Split(value, ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					exports.Add(p)
				}
			}
		}

		// check `?conditions` query
		conditions := NewStringSet()
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

		buildArgs := BuildArgs{
			alias:      alias,
			conditions: conditions,
			deps:       deps,
			exports:    exports,
			external:   external,
		}

		// build and return dts
		if resType == "types" {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, pkg, true); a != "" {
					args = "X-" + a
				}
				savePath = path.Join(fmt.Sprintf(
					"types%s/%s@%s/%s",
					ghPrefix,
					pkg.Name,
					pkg.Version,
					args,
				), pkg.SubPath)
				fi, err = fs.Stat(savePath)
				return savePath, fi, err
			}
			_, _, err := findDts()
			if err == storage.ErrNotFound {
				buildCtx := NewBuildContext(zoneId, npmrc, pkg, buildArgs, "types", BundleDefault, false, false)
				c := buildQueue.Add(buildCtx, ctx.RemoteIP())
				select {
				case output := <-c.C:
					if output.err != nil {
						if output.err.Error() == "types not found" {
							return rex.Status(404, "Types Not Found")
						}
						return rex.Status(500, "types: "+output.err.Error())
					}
				case <-time.After(time.Duration(config.BuildTimeout) * time.Second):
					header.Set("Cache-Control", ccMustRevalidate)
					return rex.Status(http.StatusRequestTimeout, "timeout, we are transforming the types hardly, please try again later!")
				}
			}
			savePath, fi, err := findDts()
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Types Not Found")
				}
				return rex.Status(500, err.Error())
			}
			_, err = fs.Stat(savePath + ".vo")
			if err != nil && err != storage.ErrNotFound {
				return rex.Status(500, err.Error())
			}
			if err == nil {
				r, err := fs.Open(savePath)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				defer r.Close()
				buffer, err := io.ReadAll(r)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				header.Set("Content-Type", ctTypescript)
				header.Set("Cache-Control", ccImmutable)
				return bytes.ReplaceAll(buffer, []byte("__ESM_CDN_ORIGIN__"), []byte(cdnOrigin))
			}
			r, err := fs.Open(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Content-Type", ctTypescript)
			header.Set("Cache-Control", ccImmutable)
			return rex.Content(savePath, fi.ModTime(), r) // auto closed
		}

		// check `X-` prefix
		var hasXPrefix bool
		if isTargetUrl {
			a := strings.Split(pkg.SubModule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				pkg.SubModule = strings.Join(a[1:], "/")
				args, err := decodeBuildArgs(npmrc, strings.TrimPrefix(a[0], "X-"))
				if err != nil {
					return throwErrorJS(ctx, err.Error(), false)
				}
				pkg.SubPath = strings.Join(strings.Split(pkg.SubPath, "/")[1:], "/")
				buildArgs = args
				hasXPrefix = true
			}
		}

		if !hasXPrefix {
			// check `?jsx-rutnime` query
			var jsxRuntime *Pkg = nil
			if v := ctx.Form.Value("jsx-runtime"); v != "" {
				m, _, _, _, err := validatePkgPath(npmrc, v)
				if err != nil {
					return rex.Status(400, fmt.Sprintf("Invalid jsx-runtime query: %v not found", v))
				}
				jsxRuntime = &m
			}

			externalRequire := ctx.Form.Has("external-require")
			// force "unocss/preset-icons" to external `require` calls
			if !externalRequire && pkg.Name == "@unocss/preset-icons" {
				externalRequire = true
			}

			buildArgs.externalRequire = externalRequire
			buildArgs.jsxRuntime = jsxRuntime
			buildArgs.keepNames = ctx.Form.Has("keep-names")
			buildArgs.ignoreAnnotations = ctx.Form.Has("ignore-annotations")
		}

		bundleMode := BundleDefault
		if (ctx.Form.Has("bundle") && ctx.Form.Value("bundle") != "false") || ctx.Form.Has("bundle-all") || ctx.Form.Has("bundle-deps") || ctx.Form.Has("standalone") {
			bundleMode = BundleAll
		} else if ctx.Form.Value("bundle") == "false" || ctx.Form.Has("no-bundle") {
			bundleMode = BundleFalse
		}

		isDev := ctx.Form.Has("dev")
		isPkgCss := ctx.Form.Has("css")
		isWorker := ctx.Form.Has("worker")
		noDts := ctx.Form.Has("no-dts") || ctx.Form.Has("no-check")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((pkg.Name == "react" && pkg.SubModule == "jsx-dev-runtime") || pkg.Name == "react-refresh") {
			isDev = true
		}

		// check if it's a build file
		// var isBuildFile bool
		if isTargetUrl && (endsWith(pkg.SubPath, ".mjs", ".js", ".css")) {
			a := strings.Split(pkg.SubModule, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
					pkgName := strings.TrimSuffix(path.Base(pkg.Name), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(pkg.SubPath, ".js") {
						if submodule == pkgName+".css" {
							pkg.SubModule = ""
							target = maybeTarget
							isBuildFile = true
						} else {
							url := fmt.Sprintf("%s/%s", cdnOrigin, pkg.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						if strings.HasSuffix(submodule, ".bundle") {
							submodule = strings.TrimSuffix(submodule, ".bundle")
							bundleMode = BundleAll
						} else if strings.HasSuffix(submodule, ".nobundle") {
							submodule = strings.TrimSuffix(submodule, ".nobundle")
							bundleMode = BundleFalse
						}
						if strings.HasSuffix(submodule, ".development") {
							submodule = strings.TrimSuffix(submodule, ".development")
							isDev = true
						}
						isMjs := strings.HasSuffix(pkg.SubPath, ".mjs")
						if isMjs && submodule == pkgName {
							submodule = ""
						}
						pkg.SubModule = submodule
						target = maybeTarget
						isBuildFile = true
					}
				}
			}
		}

		buildCtx := NewBuildContext(zoneId, npmrc, pkg, buildArgs, target, bundleMode, isDev, !config.DisableSourceMap)
		esmPath := buildCtx.Path()
		ret, hasBuild := buildCtx.Query()
		if !hasBuild {
			c := buildQueue.Add(buildCtx, ctx.RemoteIP())
			select {
			case output := <-c.C:
				if output.err != nil {
					msg := output.err.Error()
					if strings.Contains(msg, "no such file or directory") ||
						strings.Contains(msg, "is not exported from package") {
						// redirect old build path (.js) to new build path (.mjs)
						if strings.HasSuffix(pkg.SubPath, "/"+pkg.Name+".js") {
							url := strings.TrimSuffix(ctx.R.URL.String(), ".js") + ".mjs"
							return rex.Redirect(url, http.StatusFound)
						}
						header.Set("Cache-Control", ccImmutable)
						return rex.Status(404, "Module not found")
					}
					if strings.HasSuffix(msg, " not found") {
						return rex.Status(404, msg)
					}
					return throwErrorJS(ctx, output.err.Error(), false)
				}
				ret = output.result
			case <-time.After(time.Duration(config.BuildTimeout) * time.Second):
				header.Set("Cache-Control", ccMustRevalidate)
				return rex.Status(http.StatusRequestTimeout, "timeout, we are building the package hardly, please try again later!")
			}
		}

		// should redirect to `*.d.ts` file
		if ret.TypesOnly {
			dtsUrl := cdnOrigin + ret.Dts
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", ctJavascript)
			header.Set("Cache-Control", ccImmutable)
			if ctx.R.Method == http.MethodHead {
				return []byte{}
			}
			return []byte("export default null;\n")
		}

		// redirect to package css from `?css`
		if isPkgCss && pkg.SubModule == "" {
			if !ret.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s.css", cdnOrigin, strings.TrimSuffix(esmPath, path.Ext(esmPath)))
			return rex.Redirect(url, 301)
		}

		// if it's a module url, return the module content
		if isBuildFile {
			savePath := buildCtx.getSavepath()
			if strings.HasSuffix(pkg.SubPath, ".css") {
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
			f, err := fs.Open(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Cache-Control", ccImmutable)
			if endsWith(savePath, ".mjs", ".js") {
				header.Set("Content-Type", ctJavascript)
				if isWorker {
					moduleUrl := cdnOrigin + esmPath
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
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, pkg, EOL)

		if isWorker {
			moduleUrl := cdnOrigin + esmPath
			fmt.Fprintf(buf,
				`export default function workerFactory(injectOrOptions) { const options = typeof injectOrOptions === "string" ? { inject: injectOrOptions }: injectOrOptions ?? {}; const { inject, name = "%s" } = options; const blob = new Blob(['import * as $module from "%s";', inject].filter(Boolean), { type: "application/javascript" }); return new Worker(URL.createObjectURL(blob), { type: "module", name })}`,
				moduleUrl,
				moduleUrl,
			)
		} else {
			if len(ret.Deps) > 0 {
				for _, dep := range ret.Deps {
					fmt.Fprintf(buf, `import "%s";%s`, dep, EOL)
				}
			}
			header.Set("x-esm-path", esmPath)
			fmt.Fprintf(buf, `export * from "%s";%s`, esmPath, EOL)
			if (ret.FromCJS || ret.HasDefaultExport) && (exports.Len() == 0 || exports.Has("default")) {
				fmt.Fprintf(buf, `export { default } from "%s";%s`, esmPath, EOL)
			}
			if ret.FromCJS && exports.Len() > 0 {
				fmt.Fprintf(buf, `import __cjs_exports$ from "%s";%s`, esmPath, EOL)
				fmt.Fprintf(buf, `export const { %s } = __cjs_exports$;%s`, strings.Join(exports.Values(), ", "), EOL)
			}
		}

		if ret.Dts != "" && !noDts && !isWorker {
			dtsUrl := cdnOrigin + ret.Dts
			header.Set("X-TypeScript-Types", dtsUrl)
		}
		if targetViaUA {
			appendVaryHeader(header, "User-Agent")
		}
		if caretVersion {
			header.Set("Cache-Control", cc10min)
		} else {
			header.Set("Cache-Control", ccImmutable)
		}
		header.Set("Content-Length", strconv.Itoa(buf.Len()))
		header.Set("Content-Type", ctJavascript)
		if ctx.R.Method == http.MethodHead {
			return []byte{}
		}
		return buf
	}
}

func appendVaryHeader(header http.Header, key string) {
	vary := header.Get("Vary")
	if vary == "" {
		header.Set("Vary", key)
	} else {
		header.Set("Vary", vary+", "+key)
	}
}

func throwErrorJS(ctx *rex.Context, message string, static bool) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(buf, "throw new Error(%s);\n", mustEncodeJSON(strings.TrimSpace("[esm.sh] "+message)))
	fmt.Fprintf(buf, "export default null;\n")
	if static {
		ctx.W.Header().Set("Cache-Control", ccImmutable)
	} else {
		ctx.W.Header().Set("Cache-Control", ccMustRevalidate)
	}
	ctx.W.Header().Set("Content-Type", ctJavascript)
	return rex.Status(500, buf)
}
