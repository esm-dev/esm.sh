package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
)

const (
	ccMustRevalidate = "public, max-age=0, must-revalidate"
	cc10mins         = "public, max-age=600"
	cc1day           = "public, max-age=86400"
	ccImmutable      = "public, max-age=31536000, immutable"
	ctJavaScript     = "application/javascript; charset=utf-8"
	ctTypeScript     = "application/typescript; charset=utf-8"
	ctJSON           = "application/json; charset=utf-8"
	ctCSS            = "text/css; charset=utf-8"
)

type ResType uint8

const (
	// module bare name
	ResBareName ResType = iota
	// built js/css file
	ResBuild
	// built source map file
	ResBuildSrouceMap
	// *.d.ts or *.d.mts file
	ResDTS
	// package raw file
	ResRaw
)

type Module struct {
	PkgName       string `json:"pkgName"`
	PkgVersion    string `json:"pkgVersion"`
	SubPath       string `json:"subPath"`
	SubModuleName string `json:"subModule"`
	GhPrefix      bool   `json:"gh"`
	PrPrefix      bool   `json:"pr"`
}

func (pkg Module) PackageName() string {
	s := pkg.PkgName
	if pkg.PkgVersion != "" && pkg.PkgVersion != "*" && pkg.PkgVersion != "latest" {
		s += "@" + pkg.PkgVersion
	}
	if pkg.GhPrefix {
		return "gh/" + s
	}
	return s
}

func (pkg Module) String() string {
	s := pkg.PackageName()
	if pkg.SubModuleName != "" {
		s += "/" + pkg.SubModuleName
	}
	return s
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
				h.Write(input.ImportMap)
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
				zoneId := ctx.Form.Value("zoneId")
				packageName := ctx.Form.Value("package")
				version := ctx.Form.Value("version")
				if packageName == "" {
					return rex.Err(400, "param `package` is required")
				}
				prefix := "/" + packageName + "@"
				if version != "" {
					prefix += version
				}
				if zoneId != "" {
					prefix = zoneId + prefix
				}
				deletedKeys, err := db.DeleteAll(prefix)
				if err != nil {
					return rex.Err(500, err.Error())
				}
				deletedPkgs := NewStringSet()
				for _, esmPath := range deletedKeys {
					pathname := esmPath
					if zoneId != "" {
						pathname = pathname[len(zoneId):]
					}
					fromGithub := strings.HasPrefix(pathname, "/gh/")
					if fromGithub {
						pathname = pathname[3:]
					}
					pkgName, version, _, _ := splitPkgPath(pathname)
					pkgId := pkgName + "@" + version
					if fromGithub {
						pkgId = "gh/" + pkgId
					}
					deletedPkgs.Add(pkgId)
				}
				deletedFiles := []string{}
				for _, pkgId := range deletedPkgs.Values() {
					buildPrefix := fmt.Sprintf("builds/%s", pkgId)
					buildFiles, err := fs.List(buildPrefix)
					if err == nil && len(buildFiles) > 0 {
						err = fs.RemoveAll(buildPrefix)
						if err != nil {
							return rex.Err(500, "FS error")
						}
						for i, filepath := range buildFiles {
							buildFiles[i] = fmt.Sprintf("%s/%s", pkgId, filepath)
						}
						deletedFiles = append(deletedFiles, buildFiles...)
					}
					dtsPrefix := fmt.Sprintf("types/%s", pkgId)
					dtsFiles, err := fs.List(dtsPrefix)
					if err == nil && len(dtsFiles) > 0 {
						err = fs.RemoveAll(dtsPrefix)
						if err != nil {
							return rex.Err(500, "FS error")
						}
						for i, filepath := range dtsFiles {
							dtsFiles[i] = fmt.Sprintf("%s/%s", pkgId, filepath)
						}
						deletedFiles = append(deletedFiles, dtsFiles...)
					}
					log.Info("purged", pkgId)
				}
				ret := map[string]interface{}{
					"deletedPkgs":  deletedPkgs.Values(),
					"deletedFiles": deletedFiles,
				}
				if zoneId != "" {
					ret["zoneId"] = zoneId
				}
				return ret
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
			readmeStrLit := utils.MustEncodeJSON(string(readme))
			html := bytes.ReplaceAll(indexHTML, []byte("'# README'"), readmeStrLit)
			html = bytes.ReplaceAll(html, []byte("{VERSION}"), []byte(fmt.Sprintf("%d", VERSION)))
			header.Set("Cache-Control", ccMustRevalidate)
			header.Set("Etag", globalETag)
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

		// serve the internal script
		if pathname == "/run" || pathname == "/tsx" {
			ifNoneMatch := ctx.R.Header.Get("If-None-Match")
			if ifNoneMatch != "" && ifNoneMatch == globalETag {
				return rex.Status(http.StatusNotModified, "")
			}

			data, err := embedFS.ReadFile(fmt.Sprintf("server/embed/%s.ts", pathname[1:]))
			if err != nil {
				return rex.Status(404, "Not Found")
			}

			// determine build target by `?target` query or `User-Agent` header
			query := ctx.R.URL.Query()
			target := strings.ToLower(query.Get("target"))
			targetByUA := targets[target] == 0
			if targetByUA {
				target = getBuildTargetByUA(userAgent)
			}

			// replace `$TARGET` with the target
			data = bytes.ReplaceAll(data, []byte("$TARGET"), []byte(fmt.Sprintf(`"%s"`, target)))

			var code []byte
			if pathname == "/run" {
				referer := ctx.R.Header.Get("Referer")
				isLocalhost := strings.HasPrefix(referer, "http://localhost:") || strings.HasPrefix(referer, "http://localhost/")
				ret := api.Build(api.BuildOptions{
					Stdin: &api.StdinOptions{
						Sourcefile: "run.ts",
						Loader:     api.LoaderTS,
						Contents:   string(data),
					},
					Target:            targets[target],
					Format:            api.FormatESModule,
					Platform:          api.PlatformBrowser,
					MinifyWhitespace:  true,
					MinifyIdentifiers: true,
					MinifySyntax:      true,
					Bundle:            true,
					Write:             false,
					Outfile:           "-",
					LegalComments:     api.LegalCommentsExternal,
					Plugins: []api.Plugin{{
						Name: "loader",
						Setup: func(build api.PluginBuild) {
							build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
								if strings.HasPrefix(args.Path, "/") {
									return api.OnResolveResult{Path: args.Path, External: true}, nil
								}
								if args.Path == "./run-tsx" {
									return api.OnResolveResult{Path: args.Path, Namespace: "tsx"}, nil
								}
								return api.OnResolveResult{}, nil
							})
							build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "tsx"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
								sourceFile := "server/embed/run-tsx.ts"
								if isLocalhost {
									sourceFile = "server/embed/run-tsx.dev.ts"
								}
								data, err := embedFS.ReadFile(sourceFile)
								if err != nil {
									return api.OnLoadResult{}, err
								}
								sourceCode := string(bytes.ReplaceAll(data, []byte("$TARGET"), []byte(target)))
								return api.OnLoadResult{Contents: &sourceCode, Loader: api.LoaderTS}, nil
							})
						},
					}},
				})
				if ret.Errors != nil {
					return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", ret.Errors), false)
				}
				code = concatBytes(ret.OutputFiles[0].Contents, ret.OutputFiles[1].Contents)
				appendVaryHeader(header, "Referer")
			} else {
				code, err = minify(string(data), targets[target], api.LoaderTS)
				if err != nil {
					return throwErrorJS(ctx, fmt.Sprintf("Transform error: %v", err), false)
				}
			}
			if targetByUA {
				appendVaryHeader(header, "User-Agent")
			}
			header.Set("Content-Type", ctJavaScript)
			header.Set("Cache-Control", cc1day)
			header.Set("Etag", globalETag)
			if pathname == "/run" {
				header.Set("X-Typescript-Types", fmt.Sprintf("%s/run.d.ts", cdnOrigin))
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
				header.Set("Cache-Control", cc1day)
				header.Set("Etag", globalETag)
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

		// use embed types
		if strings.HasSuffix(pathname, ".d.ts") && strings.Count(pathname, "/") == 1 {
			data, err := embedFS.ReadFile("server/embed/types" + pathname)
			if err == nil {
				ifNoneMatch := ctx.R.Header.Get("If-None-Match")
				if ifNoneMatch != "" && ifNoneMatch == globalETag {
					return rex.Status(http.StatusNotModified, "")
				}
				header.Set("Cache-Control", cc1day)
				header.Set("Etag", globalETag)
				header.Set("Content-Type", ctTypeScript)
				return rex.Content(pathname, startTime, bytes.NewReader(data))
			}
		}

		// check `/*pathname` or `/gh/*pathname` pattern
		externalAll := false
		if strings.HasPrefix(pathname, "/*") {
			externalAll = true
			pathname = "/" + pathname[2:]
		} else if strings.HasPrefix(pathname, "/gh/*") {
			externalAll = true
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

		module, extraQuery, isFixedVersion, isTargetUrl, err := praseESMPath(npmrc, pathname)
		if err != nil {
			status := 500
			message := err.Error()
			if strings.HasPrefix(message, "invalid") {
				status = 400
			} else if strings.HasSuffix(message, " not found") {
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

		pkgAllowed := config.AllowList.IsPackageAllowed(module.PkgName)
		pkgBanned := config.BanList.IsPackageBanned(module.PkgName)
		if !pkgAllowed || pkgBanned {
			return rex.Status(403, "forbidden")
		}

		ghPrefix := ""
		if module.GhPrefix {
			ghPrefix = "/gh"
		}

		// redirect `/@types/PKG` to it's main dts file
		if strings.HasPrefix(module.PkgName, "@types/") && module.SubModuleName == "" {
			info, err := npmrc.getPackageInfo(module.PkgName, module.PkgVersion)
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
		if css := cssPackages[module.PkgName]; css != "" && module.SubModuleName == "" {
			url := fmt.Sprintf("%s/%s/%s", cdnOrigin, module.String(), css)
			return rex.Redirect(url, http.StatusFound)
		}

		// support `https://esm.sh/react?dev&target=es2020/jsx-runtime` pattern for jsx transformer
		for _, jsxRuntime := range []string{"jsx-runtime", "jsx-dev-runtime"} {
			if strings.HasSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime) {
				if module.SubModuleName == "" {
					module.SubModuleName = jsxRuntime
				} else {
					module.SubModuleName = module.SubModuleName + "/" + jsxRuntime
				}
				pathname = fmt.Sprintf("/%s/%s", module.PkgName, module.SubModuleName)
				ctx.R.URL.RawQuery = strings.TrimSuffix(ctx.R.URL.RawQuery, "/"+jsxRuntime)
			}
		}

		// parse raw query string
		query := ctx.R.URL.Query()

		// use `?path=$PATH` query to override the pathname
		if v := query.Get("path"); v != "" {
			module.SubModuleName = utils.CleanPath(v)[1:]
		}

		// check the response type
		resType := ResBareName
		if module.SubPath != "" {
			ext := path.Ext(module.SubPath)
			switch ext {
			case ".js", ".mjs":
				if isTargetUrl {
					resType = ResBuild
				}
			case ".ts", ".mts":
				if endsWith(pathname, ".d.ts", ".d.mts") {
					resType = ResDTS
				}
			case ".css":
				if isTargetUrl {
					resType = ResBuild
				} else {
					resType = ResRaw
				}
			case ".map":
				if isTargetUrl {
					resType = ResBuildSrouceMap
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

		// redirect to the url with fixed package version
		if !isFixedVersion {
			if isTargetUrl {
				subPath := ""
				query := ""
				if module.SubPath != "" {
					subPath = "/" + module.SubPath
				}
				if ctx.R.URL.RawQuery != "" {
					query = "?" + ctx.R.URL.RawQuery
				}
				header.Set("Cache-Control", cc10mins)
				return rex.Redirect(fmt.Sprintf("%s/%s%s%s", cdnOrigin, module.PackageName(), subPath, query), http.StatusFound)
			}
			if resType != ResBareName {
				pkgName := module.PkgName
				asteriskPrefix := ""
				subPath := ""
				query := ""
				if strings.HasPrefix(pkgName, "@jsr/") {
					pkgName = "jsr/@" + strings.ReplaceAll(pkgName[5:], "__", "/")
				}
				if externalAll {
					asteriskPrefix = "*"
				}
				if module.SubPath != "" {
					subPath = "/" + module.SubPath
				}
				if rawQuery := ctx.R.URL.RawQuery; rawQuery != "" {
					if extraQuery != "" {
						query = "&" + rawQuery
						return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, asteriskPrefix, pkgName, module.PkgVersion, query, subPath), http.StatusFound)
					}
					query = "?" + rawQuery
				}
				header.Set("Cache-Control", cc10mins)
				return rex.Redirect(fmt.Sprintf("%s%s/%s%s@%s%s%s", cdnOrigin, ghPrefix, asteriskPrefix, pkgName, module.PkgVersion, subPath, query), http.StatusFound)
			}
		}

		// serve `*.wasm` as a es module (needs top-level-await support)
		if resType == ResRaw && strings.HasSuffix(module.SubPath, ".wasm") && query.Has("module") {
			buf := &bytes.Buffer{}
			wasmUrl := cdnOrigin + pathname
			fmt.Fprintf(buf, "/* esm.sh - wasm module */\n")
			fmt.Fprintf(buf, "const data = await fetch(%s).then(r => r.arrayBuffer());\nexport default new WebAssembly.Module(data);", strings.TrimSpace(string(utils.MustEncodeJSON(wasmUrl))))
			header.Set("Cache-Control", ccImmutable)
			header.Set("Content-Type", ctJavaScript)
			return buf
		}

		// fix url that is related to `import.meta.url`
		if resType == ResRaw && isTargetUrl && !query.Has("raw") {
			extname := path.Ext(module.SubPath)
			dir := path.Join(npmrc.NpmDir(), module.PackageName())
			if !existsDir(dir) {
				_, err := npmrc.installPackage(module)
				if err != nil {
					return rex.Status(500, err.Error())
				}
			}
			pkgRoot := path.Join(dir, "node_modules", module.PkgName)
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
				sort.Sort(sort.Reverse(SortablePaths(files)))
				for _, f := range files {
					if strings.HasSuffix(module.SubPath, f) {
						file = f
						break
					}
				}
				if file == "" {
					for _, f := range files {
						if path.Base(module.SubPath) == path.Base(f) {
							file = f
							break
						}
					}
				}
			}
			if file == "" {
				return rex.Status(404, "File not found")
			}
			url := fmt.Sprintf("%s/%s@%s/%s", cdnOrigin, module.PkgName, module.PkgVersion, file)
			return rex.Redirect(url, http.StatusMovedPermanently)
		}

		// serve package raw files
		if resType == ResRaw {
			savePath := path.Join(npmrc.NpmDir(), module.PackageName(), "node_modules", module.PkgName, module.SubPath)
			fi, err := os.Lstat(savePath)
			if err != nil {
				if os.IsExist(err) {
					return rex.Status(500, err.Error())
				}
				// if the file not found, try to install the package
				_, err = npmrc.installPackage(module)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				fi, err = os.Lstat(savePath)
				if err != nil {
					if os.IsExist(err) {
						return rex.Status(500, err.Error())
					}
					return rex.Status(404, "File Not Found")
				}
			}
			// limit the file size up to 50MB
			if fi.Size() > assetMaxSize {
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
			if strings.HasSuffix(savePath, ".json") && query.Has("module") {
				defer f.Close()
				data, err := io.ReadAll(f)
				if err != nil {
					return rex.Status(500, err.Error())
				}
				header.Set("Content-Type", ctJavaScript)
				return concatBytes([]byte("export default "), data)
			}
			if endsWith(savePath, ".js", ".mjs", ".jsx") {
				header.Set("Content-Type", ctJavaScript)
			} else if endsWith(savePath, ".ts", ".mts", ".tsx") {
				header.Set("Content-Type", ctTypeScript)
			}
			return rex.Content(savePath, fi.ModTime(), f) // auto closed
		}

		// serve build/types files
		if resType == ResBuild || resType == ResBuildSrouceMap || resType == ResDTS {
			var savePath string
			if resType == ResDTS {
				savePath = path.Join("types", pathname)
			} else {
				savePath = path.Join("builds", pathname)
			}
			savePath = normalizeSavePath(zoneId, savePath)
			fi, err := fs.Stat(savePath)
			if err != nil {
				if err == storage.ErrNotFound && resType == ResBuildSrouceMap {
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
				if resType == ResDTS {
					header.Set("Content-Type", ctTypeScript)
				} else if resType == ResBuildSrouceMap {
					header.Set("Content-Type", ctJSON)
				} else if strings.HasSuffix(pathname, ".css") {
					header.Set("Content-Type", ctCSS)
				} else {
					header.Set("Content-Type", ctJavaScript)
				}
				header.Set("Cache-Control", ccImmutable)
				if resType == ResDTS {
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
					if name != "" && to != "" && name != module.PkgName {
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
					m, _, _, _, err := praseESMPath(npmrc, v)
					if err != nil {
						return rex.Status(400, fmt.Sprintf("Invalid deps query: %v not found", v))
					}
					if module.PkgName == "react-dom" && m.PkgName == "react" {
						// make sure react-dom and react are in the same version
						continue
					}
					if m.PkgName != module.PkgName {
						deps[m.PkgName] = m.PkgVersion
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
		external := NewStringSet()
		for _, p := range strings.Split(query.Get("external"), ",") {
			p = strings.TrimSpace(p)
			if p == "*" {
				external.Reset()
				externalAll = true
				break
			}
			if p != "" {
				external.Add(p)
			}
		}

		buildArgs := BuildArgs{
			alias:       alias,
			conditions:  conditions,
			deps:        deps,
			exports:     exports,
			externalAll: externalAll,
			external:    external,
		}

		// check if the build args from pathname: `PKG@VERSION/X-${args}/esnext/SUBPATH`
		isBuildArgsFromPath := false
		if resType == ResBuild || resType == ResDTS {
			a := strings.Split(module.SubModuleName, "/")
			if len(a) > 1 && strings.HasPrefix(a[0], "X-") {
				module.SubModuleName = strings.Join(a[1:], "/")
				args, err := decodeBuildArgs(npmrc, strings.TrimPrefix(a[0], "X-"))
				if err != nil {
					return throwErrorJS(ctx, "Invalid build args: "+a[0], false)
				}
				module.SubPath = strings.Join(strings.Split(module.SubPath, "/")[1:], "/")
				module.SubModuleName = toModuleBareName(module.SubPath, true)
				buildArgs = args
				isBuildArgsFromPath = true
			}
		}

		// fix the build args that are from the query
		if !isBuildArgsFromPath {
			err := fixBuildArgs(npmrc, path.Join(npmrc.NpmDir(), module.PackageName()), &buildArgs, module)
			if err != nil {
				return throwErrorJS(ctx, err.Error(), false)
			}
		}

		// build and return `.d.ts`
		if resType == ResDTS {
			findDts := func() (savePath string, fi storage.FileStat, err error) {
				args := ""
				if a := encodeBuildArgs(buildArgs, module, true); a != "" {
					args = "X-" + a
				}
				savePath = normalizeSavePath(zoneId, path.Join(fmt.Sprintf(
					"types%s/%s@%s/%s",
					ghPrefix,
					module.PkgName,
					module.PkgVersion,
					args,
				), module.SubPath))
				fi, err = fs.Stat(savePath)
				return savePath, fi, err
			}
			_, _, err := findDts()
			if err == storage.ErrNotFound {
				buildCtx := NewBuildContext(zoneId, npmrc, module, buildArgs, "types", BundleDefault, false, false)
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
			var jsxRuntime *Module = nil
			if v := query.Get("jsx-runtime"); v != "" {
				m, _, _, _, err := praseESMPath(npmrc, v)
				if err != nil {
					return rex.Status(400, fmt.Sprintf("Invalid jsx-runtime query: %v not found", v))
				}
				jsxRuntime = &m
			}

			externalRequire := query.Has("external-require")
			// workaround: force "unocss/preset-icons" to external `require` calls
			if !externalRequire && module.PkgName == "@unocss/preset-icons" {
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
		if !isDev && ((module.PkgName == "react" && module.SubModuleName == "jsx-dev-runtime") || module.PkgName == "react-refresh") {
			isDev = true
		}

		if resType == ResBuild {
			a := strings.Split(module.SubModuleName, "/")
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
					basename := strings.TrimSuffix(path.Base(module.PkgName), ".js")
					if strings.HasSuffix(submodule, ".css") && !strings.HasSuffix(module.SubPath, ".js") {
						if submodule == basename+".css" {
							module.SubModuleName = ""
							target = maybeTarget
						} else {
							url := fmt.Sprintf("%s/%s", cdnOrigin, module.String())
							return rex.Redirect(url, http.StatusFound)
						}
					} else {
						isMjs := strings.HasSuffix(module.SubPath, ".mjs")
						if isMjs && submodule == basename {
							submodule = ""
						}
						module.SubModuleName = submodule
						target = maybeTarget
					}
				}
			}
		}

		buildCtx := NewBuildContext(zoneId, npmrc, module, buildArgs, target, bundleMode, isDev, !config.DisableSourceMap)
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
						if strings.HasSuffix(module.SubPath, "/"+module.PkgName+".js") {
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
		if isPkgCss && module.SubModuleName == "" {
			if !ret.PackageCSS {
				return rex.Status(404, "Package CSS not found")
			}
			url := fmt.Sprintf("%s%s.css", cdnOrigin, strings.TrimSuffix(buildCtx.Path(), ".mjs"))
			return rex.Redirect(url, 301)
		}

		// if the response type is `ResBuild`, return the build js/css content
		if resType == ResBuild {
			savePath := buildCtx.getSavepath()
			if strings.HasSuffix(module.SubPath, ".css") {
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
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, module, EOL)

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
		if isFixedVersion {
			header.Set("Cache-Control", ccImmutable)
		} else {
			header.Set("Cache-Control", cc10mins)
		}
		header.Set("Content-Length", strconv.Itoa(buf.Len()))
		header.Set("Content-Type", ctJavaScript)
		if ctx.R.Method == http.MethodHead {
			return []byte{}
		}
		return buf
	}
}

func auth(secret string) rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if secret != "" && ctx.R.Header.Get("Authorization") != "Bearer "+secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
	}
}

func praseESMPath(rc *NpmRC, pathname string) (module Module, extraQuery string, isFixedVersion bool, hasTargetSegment bool, err error) {
	// see https://pkg.pr.new
	if strings.HasPrefix(pathname, "/pr/") {
		pkgName, rest := utils.SplitByFirstByte(pathname[4:], '@')
		if rest == "" {
			err = errors.New("invalid path")
			return
		}
		version, subPath := utils.SplitByFirstByte(rest, '/')
		if !valid.IsHexString(version) || len(version) < 7 {
			err = errors.New("invalid path")
			return
		}
		module = Module{
			PkgName:       pkgName,
			PkgVersion:    version,
			SubPath:       subPath,
			SubModuleName: toModuleBareName(subPath, true),
			PrPrefix:      true,
		}
		isFixedVersion = true
		return
	}

	ghPrefix := strings.HasPrefix(pathname, "/gh/")
	if ghPrefix {
		if len(pathname) == 4 {
			err = errors.New("invalid path")
			return
		}
		// add a leading `@` to the package name
		pathname = "/@" + pathname[4:]
	} else if strings.HasPrefix(pathname, "/jsr/") {
		segs := strings.Split(pathname[5:], "/")
		if len(segs) < 2 || !strings.HasPrefix(segs[0], "@") {
			err = errors.New("invalid path")
			return
		}
		pathname = "/@jsr/" + segs[0][1:] + "__" + segs[1]
		if len(segs) > 2 {
			pathname += "/" + strings.Join(segs[2:], "/")
		}
	}

	pkgName, maybeVersion, subPath, hasTargetSegment := splitPkgPath(pathname)
	if !validatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	// strip the leading `@` added before
	if ghPrefix {
		pkgName = pkgName[1:]
	}

	version, extraQuery := utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}

	module = Module{
		PkgName:       pkgName,
		PkgVersion:    version,
		SubPath:       subPath,
		SubModuleName: toModuleBareName(subPath, true),
		GhPrefix:      ghPrefix,
	}

	// workaround for es5-ext "../#/.." path
	if module.SubModuleName != "" && module.PkgName == "es5-ext" {
		module.SubModuleName = strings.ReplaceAll(module.SubModuleName, "/%23/", "/#/")
	}

	if ghPrefix {
		if (valid.IsHexString(module.PkgVersion) && len(module.PkgVersion) >= 7) || regexpFullVersion.MatchString(strings.TrimPrefix(module.PkgVersion, "v")) {
			isFixedVersion = true
			return
		}
		var refs []GitRef
		refs, err = listRepoRefs(fmt.Sprintf("https://github.com/%s", module.PkgName))
		if err != nil {
			return
		}
		if module.PkgVersion == "" {
			for _, ref := range refs {
				if ref.Ref == "HEAD" {
					module.PkgVersion = ref.Sha[:7]
					return
				}
			}
		} else {
			// try to find the exact tag or branch
			for _, ref := range refs {
				if ref.Ref == "refs/tags/"+module.PkgVersion || ref.Ref == "refs/heads/"+module.PkgVersion {
					module.PkgVersion = ref.Sha[:7]
					return
				}
			}
			// try to find the semver tag
			var c *semver.Constraints
			c, err = semver.NewConstraint(strings.TrimPrefix(module.PkgVersion, "semver:"))
			if err == nil {
				vs := make([]*semver.Version, len(refs))
				i := 0
				for _, ref := range refs {
					if strings.HasPrefix(ref.Ref, "refs/tags/") {
						v, e := semver.NewVersion(strings.TrimPrefix(ref.Ref, "refs/tags/"))
						if e == nil && c.Check(v) {
							vs[i] = v
							i++
						}
					}
				}
				if i > 0 {
					vs = vs[:i]
					if i > 1 {
						sort.Sort(semver.Collection(vs))
					}
					module.PkgVersion = vs[i-1].String()
					return
				}
			}
		}
		err = errors.New("tag or branch not found")
		return
	}

	isFixedVersion = regexpFullVersion.MatchString(module.PkgVersion)
	if !isFixedVersion {
		var p PackageJSON
		p, err = rc.fetchPackageInfo(pkgName, module.PkgVersion)
		if err == nil {
			module.PkgVersion = p.Version
		}
	}
	return
}

func throwErrorJS(ctx *rex.Context, message string, static bool) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "/* esm.sh - error */\n")
	fmt.Fprintf(buf, "throw new Error(%s);\n", strings.TrimSpace(string(utils.MustEncodeJSON(strings.TrimSpace("[esm.sh] "+message)))))
	fmt.Fprintf(buf, "export default null;\n")
	if static {
		ctx.W.Header().Set("Cache-Control", ccImmutable)
	} else {
		ctx.W.Header().Set("Cache-Control", ccMustRevalidate)
	}
	ctx.W.Header().Set("Content-Type", ctJavaScript)
	return rex.Status(500, buf)
}

func toModuleBareName(path string, stripIndexSuffier bool) string {
	if path != "" {
		subModule := path
		if strings.HasSuffix(subModule, ".mjs") {
			subModule = strings.TrimSuffix(subModule, ".mjs")
		} else if strings.HasSuffix(subModule, ".cjs") {
			subModule = strings.TrimSuffix(subModule, ".cjs")
		} else {
			subModule = strings.TrimSuffix(subModule, ".js")
		}
		if stripIndexSuffier {
			subModule = strings.TrimSuffix(subModule, "/index")
		}
		return subModule
	}
	return ""
}

func splitPkgPath(pathname string) (pkgName string, version string, subPath string, hasTargetSegment bool) {
	a := strings.Split(strings.TrimPrefix(pathname, "/"), "/")
	nameAndVersion := ""
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		nameAndVersion = a[0] + "/" + a[1]
		subPath = strings.Join(a[2:], "/")
		hasTargetSegment = checkTargetSegment(a[2:])
	} else {
		nameAndVersion = a[0]
		subPath = strings.Join(a[1:], "/")
		hasTargetSegment = checkTargetSegment(a[1:])
	}
	if len(nameAndVersion) > 0 && nameAndVersion[0] == '@' {
		pkgName, version = utils.SplitByLastByte(nameAndVersion[1:], '@')
		pkgName = "@" + pkgName
	} else {
		pkgName, version = utils.SplitByLastByte(nameAndVersion, '@')
	}
	if version != "" {
		version = strings.TrimSpace(version)
	}
	return
}

func checkTargetSegment(segments []string) bool {
	if len(segments) < 2 {
		return false
	}
	if strings.HasPrefix(segments[0], "X-") && len(segments) > 2 {
		_, ok := targets[segments[1]]
		return ok
	}
	_, ok := targets[segments[0]]
	return ok
}

func getPkgName(specifier string) string {
	name, _, _, _ := splitPkgPath(specifier)
	return name
}
