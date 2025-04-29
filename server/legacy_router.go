package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/goccy/go-json"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
)

func esmLegacyRouter(buildStorage storage.Storage) rex.Handle {
	return func(ctx *rex.Context) any {
		method := ctx.R.Method
		pathname := ctx.R.URL.Path

	START:
		// build API (deprecated)
		if pathname == "/build" {
			if method == "POST" {
				return rex.Status(403, "The `/build` API has been deprecated.")
			}
			if method == "GET" {
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Cache-Control", ccImmutable)
				return `
					const deprecated = new Error("[esm.sh] The build API has been deprecated.")
					export function build(_) { throw deprecated }
					export function esm(_) { throw deprecated }
					export function transform(_) { throw deprecated }
					export default build
				`
			}
			return rex.Status(405, "Method Not Allowed")
		}

		// `/react-dom@18.3.1&pin=v135`
		if strings.Contains(pathname, "&pin=v") {
			return legacyESM(ctx, buildStorage, "")
		}

		// `/react-dom@18.3.1?pin=v135`
		if q := ctx.R.URL.RawQuery; strings.HasPrefix(q, "pin=v") || strings.Contains(q, "&pin=v") {
			query := ctx.R.URL.Query()
			v := query.Get("pin")
			if len(v) > 1 && v[0] == 'v' && valid.IsDigtalOnlyString(v[1:]) {
				bv, _ := strconv.Atoi(v[1:])
				if bv <= 0 || bv > 135 {
					return rex.Status(400, "Invalid `pin` query")
				}
				return legacyESM(ctx, buildStorage, "")
			}
		}

		// `/stable/react@18.3.1?dev`
		// `/stable/react@18.3.1/es2022/react.mjs`
		if strings.HasPrefix(pathname, "/stable/") {
			return legacyESM(ctx, buildStorage, "stable")
		}

		// `/v135/react-dom@18.3.1?dev`
		// `/v135/react-dom@18.3.1/es2022/react-dom.mjs`
		if strings.HasPrefix(pathname, "/v") {
			legacyBuildVersion, path := utils.SplitByFirstByte(pathname[2:], '/')
			if valid.IsDigtalOnlyString(legacyBuildVersion) {
				bv, _ := strconv.Atoi(legacyBuildVersion)
				if bv <= 0 || bv > 135 {
					return rex.Status(400, "Invalid Module Path")
				}
				if path == "" && strings.HasPrefix(ctx.UserAgent(), "Deno/") {
					ctx.SetHeader("Content-Type", ctJavaScript)
					ctx.SetHeader("Cache-Control", ccImmutable)
					return `throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`
				}
				if path == "build" {
					pathname = "/build"
					goto START
				}
				return legacyESM(ctx, buildStorage, "v"+legacyBuildVersion)
			}
		}

		// packages created by the `/build` API
		if len(pathname) == 42 && strings.HasPrefix(pathname, "/~") && valid.IsHexString(pathname[2:]) {
			return redirect(ctx, fmt.Sprintf("/v135%s@0.0.0/%s/mod.mjs", pathname, legacyGetBuildTargetByUA(ctx.UserAgent())), true)
		}

		return ctx.Next()
	}
}

type LegacyBuildMeta struct {
	EsmId string `json:"esmId,omitempty"`
	Dts   string `json:"dts,omitempty"`
	Code  string `json:"code"`
}

func legacyESM(ctx *rex.Context, buildStorage storage.Storage, buildVersionPrefix string) any {
	pathname := ctx.R.URL.Path
	if buildVersionPrefix != "" {
		pathname = pathname[len(buildVersionPrefix)+1:]
	}
	query := ""
	if ctx.R.URL.RawQuery != "" {
		query = "?" + ctx.R.URL.RawQuery
	}
	var isStatic bool
	if (strings.HasPrefix(pathname, "/node_") && strings.HasSuffix(pathname, ".js")) || pathname == "/node.ns.d.ts" {
		isStatic = true
	} else {
		if strings.HasPrefix(pathname, "/gh/") {
			if !strings.ContainsRune(pathname[4:], '/') {
				return rex.Status(400, "invalid path")
			}
			// add a leading `@` to the package name
			pathname = "/@" + pathname[4:]
		}
		pkgName, pkgVersion, subPath, hasTargetSegment := splitEsmPath(pathname)
		var asteriskFlag bool
		if len(pkgName) > 1 && pkgName[0] == '*' {
			asteriskFlag = true
			pkgName = pkgName[1:]
		}
		if !npm.ValidatePackageName(pkgName) {
			return rex.Status(400, "Invalid Package Name")
		}
		var extraQuery string
		if pkgVersion != "" {
			pkgVersion, extraQuery = utils.SplitByFirstByte(pkgVersion, '&')
			if v, e := url.QueryUnescape(pkgVersion); e == nil {
				pkgVersion = v
			}
		}
		if !npm.IsExactVersion(pkgVersion) {
			npmrc := DefaultNpmRC()
			pkgInfo, err := npmrc.getPackageInfo(pkgName, pkgVersion)
			if err != nil {
				if strings.Contains(err.Error(), " not found") {
					return rex.Status(404, err.Error())
				}
				return rex.Status(500, err.Error())
			}
			var b strings.Builder
			b.WriteString(getOrigin(ctx))
			if buildVersionPrefix != "" {
				b.WriteByte('/')
				b.WriteString(buildVersionPrefix)
			}
			b.WriteByte('/')
			if asteriskFlag {
				b.WriteByte('*')
			}
			b.WriteString(pkgName)
			b.WriteByte('@')
			b.WriteString(pkgInfo.Version)
			if extraQuery != "" {
				b.WriteByte('&')
				b.WriteString(extraQuery)
			}
			if subPath != "" {
				b.WriteByte('/')
				b.WriteString(subPath)
			}
			b.WriteString(query)
			return redirect(ctx, b.String(), false)
		}
		isStatic = hasTargetSegment
	}
	savePath := "legacy/" + normalizeSavePath("", ctx.R.URL.Path[1:])
	if (buildVersionPrefix != "" && isStatic) || endsWith(pathname, ".d.ts", ".d.mts") {
		f, _, e := buildStorage.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			return rex.Status(500, "Storage error: "+e.Error())
		}
		if e == nil {
			switch path.Ext(pathname) {
			case ".js", ".mjs":
				ctx.SetHeader("Content-Type", ctJavaScript)
			case ".ts", ".mts":
				ctx.SetHeader("Content-Type", ctTypeScript)
				// resolve hostname in typescript definition files if the origin is not "https://esm.sh"
				if endsWith(pathname, ".d.ts", ".d.mts") {
					origin := getOrigin(ctx)
					if origin != "https://esm.sh" {
						defer f.Close()
						data, err := io.ReadAll(f)
						if err != nil {
							return rex.Status(500, "Failed to read data from storage")
						}
						data = bytes.ReplaceAll(data, []byte("https://esm.sh/v"), []byte(origin+"/v"))
						data = bytes.ReplaceAll(data, []byte(config.LegacyServer+"/v"), []byte(origin+"/v"))
						return data
					}
				}
			case ".map":
				ctx.SetHeader("Content-Type", ctJSON)
			case ".css":
				ctx.SetHeader("Content-Type", ctCSS)
			default:
				f.Close()
				return rex.Status(404, "Module Not Found")
			}
			ctx.SetHeader("Control-Cache", ccImmutable)
			return f // auto closed
		}
	} else {
		varyUA := false
		if query != "" {
			if !ctx.R.URL.Query().Has("target") {
				varyUA = true
				savePath += "." + legacyGetBuildTargetByUA(ctx.UserAgent())
			}
			h := xxhash.New()
			h.Write([]byte(query))
			savePath += "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
		}
		savePath += ".meta"
		f, _, e := buildStorage.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			return rex.Status(500, "Storage error: "+e.Error())
		}
		if e == nil {
			defer f.Close()
			var ret LegacyBuildMeta
			if json.NewDecoder(f).Decode(&ret) == nil {
				ctx.SetHeader("Content-Type", ctJavaScript)
				ctx.SetHeader("Control-Cache", ccImmutable)
				if varyUA {
					appendVaryHeader(ctx.W.Header(), "User-Agent")
				}
				if ret.EsmId != "" {
					ctx.SetHeader("X-ESM-Id", ret.EsmId)
				}
				if ret.Dts != "" {
					ctx.SetHeader("X-TypeScript-Types", getOrigin(ctx)+ret.Dts)
				}
				return ret.Code
			}
		}
	}

	url, err := ctx.R.URL.Parse(config.LegacyServer + ctx.R.URL.Path + query)
	if err != nil {
		return rex.Status(http.StatusBadRequest, "Invalid url")
	}

	client, recycle := fetch.NewClient(ctx.UserAgent(), 60, true)
	defer recycle()

	res, err := client.Fetch(url, nil)
	if err != nil {
		return rex.Status(http.StatusBadGateway, "Failed to connect the lagecy esm.sh server")
	}
	defer res.Body.Close()

	if res.StatusCode == 301 || res.StatusCode == 302 {
		url := res.Header.Get("Location")
		if strings.HasPrefix(url, "https://legacy.esm.sh") {
			url = getOrigin(ctx) + strings.TrimPrefix(url, "https://legacy.esm.sh")
		}
		return redirect(ctx, url, res.StatusCode == 301)
	}

	if res.StatusCode != 200 {
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return rex.Status(500, "Failed to fetch data from the legacy esm.sh server")
		}
		ctx.SetHeader("Cache-Control", "public, max-age=600")
		return rex.Status(res.StatusCode, data)
	}

	if (buildVersionPrefix != "" && isStatic) || endsWith(pathname, ".d.ts", ".d.mts") {
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return rex.Status(500, "Failed to fetch data from the legacy esm.sh server")
		}
		err = buildStorage.Put(savePath, bytes.NewReader(data))
		if err != nil {
			return rex.Status(500, "Storage error: "+err.Error())
		}
		ctx.SetHeader("Content-Type", res.Header.Get("Content-Type"))
		ctx.SetHeader("Control-Cache", ccImmutable)
		// resolve hostname in typescript definition files if the origin is not "https://esm.sh"
		if endsWith(pathname, ".d.ts", ".d.mts") {
			origin := getOrigin(ctx)
			if origin != "https://esm.sh" {
				data = bytes.ReplaceAll(data, []byte("https://esm.sh/v"), []byte(origin+"/v"))
				data = bytes.ReplaceAll(data, []byte(config.LegacyServer+"/v"), []byte(origin+"/v"))
			}
		}
		return data
	} else {
		code, err := io.ReadAll(res.Body)
		if err != nil {
			return rex.Status(500, "Failed to fetch data from the legacy esm.sh server")
		}
		esmId := res.Header.Get("X-Esm-Id")
		dts := res.Header.Get("X-TypeScript-Types")
		if dts != "" {
			u, err := url.Parse(dts)
			if err != nil {
				dts = ""
			} else {
				dts = u.Path
			}
		}
		ret := LegacyBuildMeta{
			EsmId: esmId,
			Dts:   dts,
			Code:  string(code),
		}
		err = buildStorage.Put(savePath, bytes.NewReader(utils.MustEncodeJSON(ret)))
		if err != nil {
			return rex.Status(500, "Storage error: "+err.Error())
		}
		ctx.SetHeader("Content-Type", res.Header.Get("Content-Type"))
		ctx.SetHeader("Control-Cache", ccImmutable)
		if query != "" && !ctx.R.URL.Query().Has("target") {
			appendVaryHeader(ctx.W.Header(), "User-Agent")
		}
		if esmId != "" {
			ctx.SetHeader("X-ESM-Id", esmId)
		}
		if dts != "" {
			ctx.SetHeader("X-TypeScript-Types", getOrigin(ctx)+dts)
		}
		return code
	}
}
