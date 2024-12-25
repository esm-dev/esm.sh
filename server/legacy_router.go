package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
)

func esmLegacyRouter(ctx *rex.Context) any {
	method := ctx.R.Method
	pathname := ctx.R.URL.Path

Start:
	// build API (deprecated)
	if pathname == "/build" {
		if method == "POST" {
			return rex.Status(403, "The `/build` API has been deprecated.")
		}
		if method == "GET" {
			ctx.Header.Set("Content-Type", ctJavaScript)
			ctx.Header.Set("Cache-Control", ccImmutable)
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
	if strings.Contains(pathname, "&pin=") {
		return legacyESM(ctx, pathname)
	}

	// `/react-dom@18.3.1?pin=v135`
	if q := ctx.R.URL.RawQuery; strings.HasPrefix(q, "pin=") || strings.Contains(q, "&pin=") {
		query := ctx.R.URL.Query()
		v := query.Get("pin")
		if len(v) > 1 && v[0] == 'v' && valid.IsDigtalOnlyString(v[1:]) {
			bv, _ := strconv.Atoi(v[1:])
			if bv <= 0 || bv > 135 {
				return rex.Status(400, "Invalid `pin` query")
			}
			return legacyESM(ctx, pathname)
		}
	}

	// `/stable/react@18.3.1?dev`
	// `/stable/react@18.3.1/es2022/react.mjs`
	if strings.HasPrefix(pathname, "/stable/") {
		return legacyESM(ctx, pathname[7:])
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
				ctx.Header.Set("Content-Type", ctJavaScript)
				ctx.Header.Set("Cache-Control", ccImmutable)
				return `throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`
			}
			if path == "build" {
				pathname = "/build"
				goto Start
			}
			return legacyESM(ctx, "/"+path)
		}
	}

	// packages created by the `/build` API
	if len(pathname) == 42 && strings.HasPrefix(pathname, "/~") && valid.IsHexString(pathname[2:]) {
		return redirect(ctx, fmt.Sprintf("/v135%s@0.0.0/%s/mod.mjs", pathname, legacyGetBuildTargetByUA(ctx.UserAgent())), true)
	}

	return ctx.Next()
}

func legacyESM(ctx *rex.Context, pathname string) any {
	pkgName, pkgVersion, isBuildDist, err := splitLegacyESMPath(pathname)
	if err != nil {
		return rex.Status(400, err.Error())
	}
	query := ""
	if ctx.R.URL.RawQuery != "" {
		query = "?" + ctx.R.URL.RawQuery
	}
	isFixedVersion := regexpVersionStrict.MatchString(pkgVersion)
	if !isFixedVersion {
		npmrc := DefaultNpmRC()
		pkgInfo, err := npmrc.fetchPackageInfo(pkgName, pkgVersion)
		if err != nil {
			if strings.Contains(err.Error(), " not found") {
				return rex.Status(404, err.Error())
			}
			return rex.Status(500, err.Error())
		}
		return redirect(ctx, getCdnOrigin(ctx)+strings.Replace(ctx.R.URL.Path, "@"+pkgVersion, "@"+pkgInfo.Version, 1)+query, false)
	}
	savePath := "legacy/" + normalizeSavePath("", ctx.R.URL.Path[1:])
	if isBuildDist || endsWith(pathname, ".d.ts", ".d.mts") {
		f, _, e := buildStorage.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			return rex.Status(500, "Storage Error: "+e.Error())
		}
		if e == nil {
			switch path.Ext(pathname) {
			case ".js", ".mjs":
				ctx.Header.Set("Content-Type", ctJavaScript)
			case ".ts", ".mts":
				ctx.Header.Set("Content-Type", ctTypeScript)
			case ".map":
				ctx.Header.Set("Content-Type", ctJSON)
			case ".css":
				ctx.Header.Set("Content-Type", ctCSS)
			default:
				f.Close()
				return rex.Status(404, "Module Not Found")
			}
			ctx.Header.Set("Control-Cache", ccImmutable)
			return f // auto closed
		}
	} else {
		varyUA := false
		if query != "" {
			if !ctx.R.URL.Query().Has("target") {
				varyUA = true
				savePath += "+" + legacyGetBuildTargetByUA(ctx.UserAgent())
			}
			h := xxhash.New()
			h.Write([]byte(query))
			savePath += "+" + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
		}
		savePath += "+mjs"
		f, _, e := buildStorage.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			return rex.Status(500, "Storage Error: "+e.Error())
		}
		if e == nil {
			defer f.Close()
			var ret []string
			if json.NewDecoder(f).Decode(&ret) == nil && len(ret) >= 2 {
				ctx.Header.Set("Content-Type", ctJavaScript)
				ctx.Header.Set("Control-Cache", ccImmutable)
				ctx.Header.Set("X-ESM-Id", ret[0])
				if varyUA {
					appendVaryHeader(ctx.W.Header(), "User-Agent")
				}
				if len(ret) == 3 {
					ctx.Header.Set("X-TypeScript-Types", getCdnOrigin(ctx)+ret[1])
					return ret[2]
				}
				return ret[1]
			}
		}
	}

	url, err := ctx.R.URL.Parse(config.LegacyServer + ctx.R.URL.Path + query)
	if err != nil {
		return rex.Status(http.StatusBadRequest, "Invalid url")
	}
	fetchClient, recycle := NewFetchClient(30, ctx.UserAgent())
	defer recycle()
	res, err := fetchClient.Fetch(url, nil)
	if err != nil {
		return rex.Status(http.StatusBadGateway, "Failed to connect the lagecy esm.sh server")
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return rex.Status(500, "Failed to fetch data from the legacy esm.sh server")
		}
		ctx.Header.Set("Cache-Control", "public, max-age=600")
		return rex.Status(res.StatusCode, data)
	}

	if isBuildDist || endsWith(pathname, ".d.ts", ".d.mts") {
		buf, recycle := NewBuffer()
		defer recycle()
		err := buildStorage.Put(savePath, io.TeeReader(res.Body, buf))
		if err != nil {
			return rex.Status(500, "Storage Error")
		}
		ctx.Header.Set("Content-Type", res.Header.Get("Content-Type"))
		ctx.Header.Set("Control-Cache", ccImmutable)
		return buf.Bytes()
	} else {
		code, err := io.ReadAll(res.Body)
		if err != nil {
			return rex.Status(500, "Failed to fetch data from the legacy esm.sh server")
		}
		esmId := res.Header.Get("X-Esm-Id")
		if esmId == "" {
			ctx.Header.Set("Cache-Control", "public, max-age=600")
			return rex.Status(502, "Unexpected response from the legacy esm.sh server")
		}
		dts := res.Header.Get("X-TypeScript-Types")
		if dts != "" {
			u, err := url.Parse(dts)
			if err != nil {
				dts = ""
			} else {
				dts = u.Path
			}
		}
		ret := []string{esmId}
		if dts != "" {
			ret = append(ret, dts)
		}
		ret = append(ret, string(code))
		err = buildStorage.Put(savePath, bytes.NewReader(utils.MustEncodeJSON(ret)))
		if err != nil {
			return rex.Status(500, "Storage Error")
		}
		ctx.Header.Set("Content-Type", res.Header.Get("Content-Type"))
		ctx.Header.Set("Control-Cache", ccImmutable)
		ctx.Header.Set("X-ESM-Id", esmId)
		if query != "" && !ctx.R.URL.Query().Has("target") {
			appendVaryHeader(ctx.W.Header(), "User-Agent")
		}
		if dts != "" {
			ctx.Header.Set("X-TypeScript-Types", getCdnOrigin(ctx)+dts)
		}
		return code
	}
}

func splitLegacyESMPath(pathname string) (pkgName string, version string, isBuildDist bool, err error) {
	if strings.HasPrefix(pathname, "/gh/") {
		if !strings.ContainsRune(pathname[4:], '/') {
			err = errors.New("invalid path")
			return
		}
		// add a leading `@` to the package name
		pathname = "/@" + pathname[4:]
	}

	pkgName, maybeVersion, _, isBuildDist := splitEsmPath(pathname)
	if !validatePackageName(pkgName) {
		err = fmt.Errorf("invalid package name '%s'", pkgName)
		return
	}

	version, _ = utils.SplitByFirstByte(maybeVersion, '&')
	if v, e := url.QueryUnescape(version); e == nil {
		version = v
	}
	return
}
