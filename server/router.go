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

type ResType uint8

const (
	// module bare name
	ResBare BundleMode = iota
	// build js/css file
	ResBuild
	// build map file
	ResBuildMap
	// *.d.ts or *.d.mts file
	ResTypes
	// package raw file
	ResRaw
)

const (
	ccMustRevalidate = "public, max-age=0, must-revalidate"
	cc10min          = "public, max-age=600"
	cc1day           = "public, max-age=86400"
	ccImmutable      = "public, max-age=31536000, immutable"
	ctJavaScript     = "application/javascript; charset=utf-8"
	ctTypeScript     = "application/typescript; charset=utf-8"
	ctJSON           = "application/json; charset=utf-8"
	ctCSS            = "text/css; charset=utf-8"
)

func auth(secret string) rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if secret != "" && ctx.R.Header.Get("Authorization") != "Bearer "+secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
	}
}

func router() rex.Handle {
	startTime := time.Now()
	globalETag := fmt.Sprintf(`W/"v%d"`, VERSION)

	return func(ctx *rex.Context) interface{} {
		pathname := ctx.Path.String()
		header := ctx.W.Header()
		userAgent := ctx.R.UserAgent()

		// ban malicious requests
		if strings.HasPrefix(pathname, "/.") || strings.HasSuffix(pathname, ".php") {
			return rex.Status(404, "not found")
		}

		// handle POST requests
		if ctx.R.Method == "POST" {
			switch ctx.Path.String() {
			case "/transform":
				var input TransformInput
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
					input.Target = "esnext"
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
				h.Write([]byte(input.Target))
				h.Write([]byte(fmt.Sprintf("%v", input.SourceMap)))
				hash := hex.EncodeToString(h.Sum(nil))

				// if previous build exists, return it directly
				savePath := fmt.Sprintf("modules/%s.mjs", hash)
				if file, err := fs.Open(savePath); err == nil {
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						return rex.Err(500, "failed to read code")
					}
					output := TransformOutput{
						Code: string(data),
					}
					file, err = fs.Open(savePath + ".map")
					if err == nil {
						data, err = io.ReadAll(file)
						file.Close()
						if err == nil {
							output.Map = string(data)
						}
					}
					return output
				}

				output, err := transform(input)
				if err != nil {
					if strings.HasPrefix(err.Error(), "<400> ") {
						return rex.Err(400, err.Error()[6:])
					}
					return rex.Err(500, "failed to save code")
				}
				if len(output.Map) > 0 {
					output.Code = fmt.Sprintf("%s//# sourceMappingURL=%s", output.Code, path.Base(savePath)+".map")
					go fs.WriteFile(savePath+".map", strings.NewReader(output.Map))
				}
				go fs.WriteFile(savePath, strings.NewReader(output.Code))
				ctx.W.Header().Set("Cache-Control", ccMustRevalidate)
				return output
			case "/purge":
				zoneId := ctx.Form.Value("zone-id")
				packageName := ctx.Form.Value("package")
				version := ctx.Form.Value("version")
				github := ctx.Form.Has("github")
				if packageName == "" {
					return rex.Err(400, "packageName is required")
				}
				prefix := "/" + packageName + "@"
				if version != "" {
					prefix += version
				}
				if github {
					prefix = "/gh" + prefix
				}
				if zoneId != "" {
					prefix = zoneId + prefix
				}
				deletedKeys, err := db.DeleteAll(prefix)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				for _, esmPath := range deletedKeys {
					if zoneId != "" {
						esmPath = esmPath[len(zoneId):]
					}
					pkgName, version, _, _ := splitPkgPath(esmPath)
					go fs.RemoveAll(fmt.Sprintf("builds/%s@%s/", pkgName, version))
					go fs.RemoveAll(fmt.Sprintf("types/%s@%s/", pkgName, version))
					log.Info("purged", esmPath)
				}
				return deletedKeys
			default:
				return rex.Err(404, "not found")
			}
		}

		// strip trailing slash
		if pathname != "/" && strings.HasSuffix(pathname, "/") {
			pathname = strings.TrimRight(pathname, "/")
		}

		cdnOrigin := ctx.R.Header.Get("X-Real-Origin")
		// use current host as cdn origin if not set
		if cdnOrigin == "" {
			proto := "http"
			if ctx.R.TLS != nil {
				proto = "https"
			}
			cdnOrigin = fmt.Sprintf("%s://%s", proto, ctx.R.Host)
		}

		// static routes
		switch pathname {
		case "/":
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == globalETag {
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
			if globalETag != "" {
				header.Set("ETag", globalETag)
			}
			return rex.Content("index.html", startTime, bytes.NewReader(html))

		case "/status.json":
			q := make([]map[string]interface{}, buildQueue.queue.Len())
			i := 0

			buildQueue.lock.RLock()
			for el := buildQueue.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*BuildTask)
				if ok {
					m := map[string]interface{}{
						"clients":   t.clients,
						"createdAt": t.createdAt.Format(http.TimeFormat),
						"inProcess": t.inProcess,
						"path":      t.Path(),
						"stage":     t.stage,
					}
					if !t.startedAt.IsZero() {
						m["startedAt"] = t.startedAt.Format(http.TimeFormat)
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
			switch query := ctx.R.URL.Query(); query.Get("type") {
			case "resolve":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Could not resolve "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-builtin-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported Node builtin module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-node-native-module":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported node native module "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-npm-package":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported NPM package "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
				), true)
			case "unsupported-file-dependency":
				return throwErrorJS(ctx, fmt.Sprintf(
					`Unsupported file dependency "%s" (Imported by "%s")`,
					query.Get("name"),
					query.Get("importer"),
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

		// serve run and hot scripts
		if pathname == "/run" || pathname == "/hot" {
			data, err := embedFS.ReadFile(fmt.Sprintf("server/embed/%s.ts", pathname[1:]))
			if err != nil {
				return rex.Status(404, "Not Found")
			}

			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == globalETag {
				return rex.Status(http.StatusNotModified, "")
			}

			// determine build target by `?target` query or `User-Agent` header
			query := ctx.R.URL.Query()
			target := strings.ToLower(query.Get("target"))
			targetByUA := targets[target] == 0
			if targetByUA {
				target = getBuildTargetByUA(userAgent)
			}

			if pathname == "/run" {
				data = bytes.ReplaceAll(data, []byte("$TARGET"), []byte(fmt.Sprintf(`"%s"`, target)))
			}

			code, err := minify(string(data), targets[target], api.LoaderTS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}
			header.Set("Content-Type", ctJavaScript)
			if targetByUA {
				appendVaryHeader(header, "User-Agent")
			}
			if query.Get("v") != "" {
				header.Set("Cache-Control", ccImmutable)
			} else {
				header.Set("Cache-Control", cc1day)
				if globalETag != "" {
					header.Set("ETag", globalETag)
				}
			}
			if pathname == "/hot" {
				header.Set("X-Typescript-Types", fmt.Sprintf("%s/hot.d.ts", cdnOrigin))
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
			hash, ext := utils.SplitByFirstByte(pathname[2:], '.')
			if len(hash) != 40 {
				return rex.Status(404, "Not Found")
			}
			savePath := fmt.Sprintf("modules/%s.%s", hash, ext)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Not Found")
				}
				return rex.Status(500, err.Error())
			}
			f, err := fs.Open(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			if strings.HasSuffix(pathname, ".map") {
				header.Set("Content-Type", ctJSON)
			} else {
				header.Set("Content-Type", ctJavaScript)
			}
			header.Set("Cache-Control", ccImmutable)
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
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
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == globalETag {
					return rex.Status(http.StatusNotModified, "")
				}
				if query := ctx.R.URL.Query(); query.Get("v") != "" {
					header.Set("Cache-Control", ccImmutable)
				} else {
					header.Set("Cache-Control", cc1day)
					if globalETag != "" {
						header.Set("ETag", globalETag)
					}
				}
			}
			target := getBuildTargetByUA(userAgent)
			code, err := minify(lib, targets[target], api.LoaderJS)
			if err != nil {
				return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
			}
			appendVaryHeader(header, "User-Agent")
			header.Set("Content-Type", ctJavaScript)
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
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == globalETag {
					return rex.Status(http.StatusNotModified, "")
				}
				if query := ctx.R.URL.Query(); query.Get("v") != "" {
					header.Set("Cache-Control", ccImmutable)
				} else {
					header.Set("Cache-Control", cc1day)
					if globalETag != "" {
						header.Set("ETag", globalETag)
					}
				}
				if isDts {
					header.Set("Content-Type", ctTypeScript)
				} else {
					target := getBuildTargetByUA(userAgent)
					code, err := minify(string(data), targets[target], api.LoaderJS)
					if err != nil {
						return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
					}
					data = []byte(code)
					header.Set("Content-Type", ctJavaScript)
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
			npmrc = NewNpmRcFromConfig()
		}

		zoneId := ctx.R.Header.Get("X-Zone-Id")
		if zoneId != "" {
			if !valid.IsDomain(zoneId) {
				zoneId = ""
			} else {
				var scopeName string
				if pkgName := getPkgName(pathname[1:]); strings.HasPrefix(pkgName, "@") {
					scopeName = pkgName[:strings.Index(pkgName, "/")]
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
		}
		if zoneId != "" {
			npmrc.zoneId = zoneId
			cdnOrigin = fmt.Sprintf("https://%s", zoneId)
		}

		// get package info
		pkg, extraQuery, caretVersion, isTargetUrl, err := validateESMPath(npmrc, pathname)
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

		// apply extra query to the url
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
			return rex.Redirect(fmt.Sprintf("%s/%s@%s%s", cdnOrigin, info.Name, info.Version, utils.CleanPath(types)), http.StatusFound)
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

		// parse raw query string
		query := ctx.R.URL.Query()

		// or use `?path=$PATH` query to override the pathname
		if v := query.Get("path"); v != "" {
			pkg.SubModule = utils.CleanPath(v)[1:]
		}

		// check the response type
		resType := ResBare
		if pkg.SubPath != "" {
			ext := path.Ext(pkg.SubPath)
			switch ext {
			case ".js", ".mjs":
				if isTargetUrl {
					resType = ResBuild
				}
			case ".ts", ".mts":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					resType = ResTypes
				}
			case ".css":
				if isTargetUrl {
					resType = ResBuild
				} else {
					resType = ResRaw
				}
			case ".map":
				if isTargetUrl {
					resType = ResBuildMap
				} else {
					resType = ResRaw
				}
			default:
				if ext != "" && assetExts[ext[1:]] {
					resType = ResRaw
				}
			}
		}
		if query.Has("raw") {
			resType = ResRaw
		}

		// redirect to the url with full package version
		if !strings.Contains(pathname, "@"+pkg.Version) {
			if !isTargetUrl {
				skipRedirect := caretVersion && resType == ResBare && !pkg.FromGithub
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
				return rex.Redirect(fmt.Sprintf("%s/%s%s%s", cdnOrigin, pkg.Fullname(), subPath, query), http.StatusFound)
			}
		}

		// serve `*.wasm` as a es module (needs top-level-await support)
		if resType == ResRaw && strings.HasSuffix(pkg.SubPath, ".wasm") && query.Has("module") {
			buf := &bytes.Buffer{}
			wasmUrl := cdnOrigin + pathname
			fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
			fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(mustEncodeJSON(wasmUrl))))
			header.Set("Cache-Control", ccImmutable)
			header.Set("Content-Type", ctJavaScript)
			return buf
		}

		// fix url that is related to `import.meta.url`
		if resType == ResRaw && isTargetUrl && !query.Has("raw") {
			extname := path.Ext(pkg.SubPath)
			dir := path.Join(npmrc.Dir(), pkg.Fullname())
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

		// serve package raw files
		if resType == ResRaw {
			savePath := path.Join(npmrc.Dir(), pkg.Fullname(), "node_modules", pkg.Name, pkg.SubPath)
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
			// limit the file size up to 50MB
			if fi.Size() > 50*1024*1024 {
				return rex.Status(403, "File Too Large")
			}
			f, err := os.Open(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				return rex.Status(404, "File Not Found")
			}
			header.Set("Cache-Control", ccImmutable)
			if endsWith(savePath, ".js", ".mjs", ".jsx") {
				header.Set("Content-Type", ctJavaScript)
			} else if endsWith(savePath, ".ts", ".mts", ".tsx") {
				header.Set("Content-Type", ctTypeScript)
			}
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		// serve build/types files
		if resType == ResBuild || resType == ResBuildMap || resType == ResTypes {
			var savePath string
			if resType == ResTypes {
				savePath = path.Join("types", pathname)
			} else {
				savePath = path.Join("builds", pathname)
			}
			savePath = normalizeSavePath(zoneId, savePath)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound && resType == ResBuildMap {
					return rex.Status(404, "Not found")
				}
				if err != storage.ErrNotFound {
					return rex.Status(500, err.Error())
				}
			}
			if err == nil {
				if query.Has("worker") && resType == ResBuild {
					moduleUrl := cdnOrigin + pathname
					header.Set("Content-Type", ctJavaScript)
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
				if resType == ResTypes {
					header.Set("Content-Type", ctTypeScript)
				} else if resType == ResBuildMap {
					header.Set("Content-Type", ctJSON)
				} else if strings.HasSuffix(pathname, ".css") {
					header.Set("Content-Type", ctCSS)
				} else {
					header.Set("Content-Type", ctJavaScript)
				}
				header.Set("Cache-Control", ccImmutable)
				if resType == ResTypes {
					buffer, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						return rex.Status(500, err.Error())
					}
					return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))
				}
				return rex.Content(savePath, fi.ModTime(), r) // auto closed
			}
		}

		// check `?alias` query
		alias := map[string]string{}
		if query.Has("alias") {
			for _, p := range strings.Split(query.Get("alias"), ",") {
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
		deps := map[string]string{}
		if query.Has("deps") {
			for _, v := range strings.Split(query.Get("deps"), ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					p, _, _, _, err := validateESMPath(npmrc, v)
					if err != nil {
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", v))
					}
					if pkg.Name == "react-dom" && p.Name == "react" {
						// the `react` version always matches `react-dom` version
						continue
					}
					if p.Name != pkg.Name {
						deps[p.Name] = p.Version
					}
				}
			}
		}

		// check `?exports` query
		exports := NewStringSet()
		if query.Has("exports") {
			value := query.Get("exports")
			for _, p := range strings.Split(value, ",") {
				p = strings.TrimSpace(p)
				if regexpJSIdent.MatchString(p) {
					exports.Add(p)
				}
			}
		}

		// check `?conditions` query
		var conditions []string
		conditionsSet := NewStringSet()
		if query.Has("conditions") {
			for _, p := range strings.Split(query.Get("conditions"), ",") {
				p = strings.TrimSpace(p)
				if p != "" && !strings.ContainsRune(p, ' ') && !conditionsSet.Has(p) {
					conditionsSet.Add(p)
					conditions = append(conditions, p)
				}
			}
		}

		// determine build target by `?target` query or `User-Agent` header
		target := strings.ToLower(query.Get("target"))
		targetByUA := targets[target] == 0
		if targetByUA {
			target = getBuildTargetByUA(userAgent)
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

		buildArgs := BuildArgs{
			alias:      alias,
			conditions: conditions,
			deps:       deps,
			exports:    exports,
			external:   external,
		}

		// check if the build args from pathname: `PKG@VERSION/X-${args}/esnext/SUBPATH`
		isBuildArgsFromPath := false
		if resType == ResBuild || resType == ResTypes {
			a := strings.Split(pkg.SubModule, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				pkg.SubModule = strings.Join(a[1:], "/")
				args, err := decodeBuildArgs(npmrc, strings.TrimPrefix(a[0], "X-"))
				if err != nil {
					return throwErrorJS(ctx, err.Error(), false)
				}
				pkg.SubPath = strings.Join(strings.Split(pkg.SubPath, "/")[1:], "/")
				pkg.SubModule = toModuleBareName(pkg.SubPath, true)
				buildArgs = args
				isBuildArgsFromPath = true
			}
		}

		// build and return dts
		if resType == ResTypes {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, pkg, true); a != "" {
					args = "X-" + a
				}
				savePath = normalizeSavePath(zoneId, path.Join(fmt.Sprintf(
					"types%s/%s@%s/%s",
					ghPrefix,
					pkg.Name,
					pkg.Version,
					args,
				), pkg.SubPath))
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
			savePath, _, err := findDts()
			if err != nil {
				if err == storage.ErrNotFound {
					return rex.Status(404, "Types Not Found")
				}
				return rex.Status(500, err.Error())
			}
			r, err := fs.Open(savePath)
			if err != nil {
				return rex.Status(500, err.Error())
			}
			buffer, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				return rex.Status(500, err.Error())
			}
			header.Set("Content-Type", ctTypeScript)
			header.Set("Cache-Control", ccImmutable)
			return bytes.ReplaceAll(buffer, []byte("{ESM_CDN_ORIGIN}"), []byte(cdnOrigin))

		}

		if !isBuildArgsFromPath {
			// check `?jsx-rutnime` query
			var jsxRuntime *Pkg = nil
			if v := query.Get("jsx-runtime"); v != "" {
				m, _, _, _, err := validateESMPath(npmrc, v)
				if err != nil {
					return rex.Status(400, fmt.Sprintf("Invalid jsx-runtime query: %v not found", v))
				}
				jsxRuntime = &m
			}

			externalRequire := query.Has("external-require")
			// workaround: force "unocss/preset-icons" to external `require` calls
			if !externalRequire && pkg.Name == "@unocss/preset-icons" {
				externalRequire = true
			}

			buildArgs.externalRequire = externalRequire
			buildArgs.jsxRuntime = jsxRuntime
			buildArgs.keepNames = query.Has("keep-names")
			buildArgs.ignoreAnnotations = query.Has("ignore-annotations")
		}

		bundleMode := BundleDefault
		if (query.Has("bundle") && query.Get("bundle") != "false") || query.Has("bundle-all") || query.Has("bundle-deps") || query.Has("standalone") {
			bundleMode = BundleAll
		} else if query.Get("bundle") == "false" || query.Has("no-bundle") {
			bundleMode = BundleFalse
		}

		isDev := query.Has("dev")
		isPkgCss := query.Has("css")
		isWorker := query.Has("worker")
		noDts := query.Has("no-dts") || query.Has("no-check")

		// force react/jsx-dev-runtime and react-refresh into `dev` mode
		if !isDev && ((pkg.Name == "react" && pkg.SubModule == "jsx-dev-runtime") || pkg.Name == "react-refresh") {
			isDev = true
		}

		if resType == ResBuild {
			a := strings.Split(pkg.SubModule, "/")
			if len(a) > 0 {
				maybeTarget := a[0]
				if _, ok := targets[maybeTarget]; ok {
					submodule := strings.Join(a[1:], "/")
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
					basename := strings.TrimSuffix(path.Base(pkg.Name), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(pkg.SubPath, ".js") {
						if submodule == basename+".css" {
							pkg.SubModule = ""
							target = maybeTarget
						} else {
							url := fmt.Sprintf("%s/%s", cdnOrigin, pkg.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						isMjs := strings.HasSuffix(pkg.SubPath, ".mjs")
						if isMjs && submodule == basename {
							submodule = ""
						}
						pkg.SubModule = submodule
						target = maybeTarget
					}
				}
			}
		}

		buildCtx := NewBuildContext(zoneId, npmrc, pkg, buildArgs, target, bundleMode, isDev, !config.DisableSourceMap)
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

		// redirect to `*.d.ts` file
		if ret.TypesOnly {
			dtsUrl := cdnOrigin + ret.Dts
			header.Set("X-TypeScript-Types", dtsUrl)
			header.Set("Content-Type", ctJavaScript)
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
			url := fmt.Sprintf("%s%s.css", cdnOrigin, strings.TrimSuffix(buildCtx.Path(), ".mjs"))
			return rex.Redirect(url, 301)
		}

		// if the response type is `ResBuild`, return the build js/css content
		if resType == ResBuild {
			savePath := buildCtx.getSavepath()
			if strings.HasSuffix(pkg.SubPath, ".css") {
				path, _ := utils.SplitByLastByte(savePath, '.')
				savePath = path + ".css"
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
			if endsWith(savePath, ".css") {
				header.Set("Content-Type", ctCSS)
			} else if endsWith(savePath, ".mjs", ".js") {
				header.Set("Content-Type", ctJavaScript)
				if isWorker {
					f.Close()
					moduleUrl := cdnOrigin + buildCtx.Path()
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
			moduleUrl := cdnOrigin + buildCtx.Path()
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
			header.Set("X-ESM-Path", buildCtx.Path())
			fmt.Fprintf(buf, `export * from "%s";%s`, buildCtx.Path(), EOL)
			if (ret.FromCJS || ret.HasDefaultExport) && (exports.Len() == 0 || exports.Has("default")) {
				fmt.Fprintf(buf, `export { default } from "%s";%s`, buildCtx.Path(), EOL)
			}
			if ret.FromCJS && exports.Len() > 0 {
				fmt.Fprintf(buf, `import __cjs_exports$ from "%s";%s`, buildCtx.Path(), EOL)
				fmt.Fprintf(buf, `export const { %s } = __cjs_exports$;%s`, strings.Join(exports.Values(), ", "), EOL)
			}
		}

		if ret.Dts != "" && !noDts && !isWorker {
			dtsUrl := cdnOrigin + ret.Dts
			header.Set("X-TypeScript-Types", dtsUrl)
		}
		if targetByUA {
			appendVaryHeader(header, "User-Agent")
		}
		if caretVersion {
			header.Set("Cache-Control", cc10min)
		} else {
			header.Set("Cache-Control", ccImmutable)
		}
		header.Set("Content-Length", strconv.Itoa(buf.Len()))
		header.Set("Content-Type", ctJavaScript)
		if ctx.R.Method == http.MethodHead {
			return []byte{}
		}
		return buf
	}
}

func throwErrorJS(ctx *rex.Context, message string, static bool) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(buf, "throw new Error(%s);\n", strings.TrimSpace(string(mustEncodeJSON(strings.TrimSpace("[esm.sh] "+message)))))
	fmt.Fprintf(buf, "export default null;\n")
	if static {
		ctx.W.Header().Set("Cache-Control", ccImmutable)
	} else {
		ctx.W.Header().Set("Cache-Control", ccMustRevalidate)
	}
	ctx.W.Header().Set("Content-Type", ctJavaScript)
	return rex.Status(500, buf)
}
