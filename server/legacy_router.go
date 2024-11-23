package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/server/storage"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
	"github.com/ije/rex"
)

func esmLegacyRouter(ctx *rex.Context) any {
	pathname := ctx.R.URL.Path
	method := ctx.R.Method

	// Deprecated legacy build script
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

	// stable build artifact
	if strings.HasPrefix(pathname, "/stable/") {
		if endsWith(pathname, ".js", ".mjs", ".map", ".css") && hasTargetSegment(pathname) {
			return proxyLegacyBuildArtifact(ctx, false)
		}
		ctx.R.URL.Path = pathname[7:]
		return nil
	}

	// build artifact
	if strings.HasPrefix(pathname, "/v") {
		legacyBuildVersion, path := utils.SplitByFirstByte(pathname[2:], '/')
		if valid.IsDigtalOnlyString(legacyBuildVersion) {
			bv, _ := strconv.Atoi(legacyBuildVersion)
			if bv <= 0 || bv > 135 {
				return rex.Status(400, "Invalid module path")
			}
			if path == "" && strings.HasPrefix(ctx.UserAgent(), "Deno/") {
				ctx.SetHeader("Content-Type", ctJavaScript)
				return `throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`
			}
			if endsWith(pathname, ".js", ".mjs", ".map", ".css") && hasTargetSegment(pathname) {
				return proxyLegacyBuildArtifact(ctx, false)
			}
			if path == "" {
				path = "/"
			}
			ctx.R.URL.Path = path
			return nil
		}
	}

	// build artifact of the `/build` API
	if len(pathname) == 42 && strings.HasPrefix(pathname, "/~") && valid.IsHexString(pathname[2:]) {
		return rex.Redirect(fmt.Sprintf("/v135%s@0.0.0/%s/mod.mjs", pathname, legacyGetBuildTargetByUA(ctx.UserAgent())), 301)
	}

	return nil // next
}

func hasTargetSegment(pathname string) bool {
	segments := strings.Split(pathname[1:], "/")
	for _, s := range segments {
		if targets[s] > 0 {
			return true
		}
	}
	return false
}

func proxyLegacyBuildArtifact(ctx *rex.Context, varyUA bool) any {
	pathname := ctx.R.URL.Path
	switch path.Ext(pathname) {
	case ".js", ".mjs":
		ctx.SetHeader("Content-Type", ctJavaScript)
	case ".map":
		ctx.SetHeader("Content-Type", ctJSON)
	case ".css":
		ctx.SetHeader("Content-Type", ctCSS)
	}
	ctx.SetHeader("control-cache", ccImmutable)
	if varyUA {
		appendVaryHeader(ctx.W.Header(), "User-Agent")
	}

	savePath := "legacy" + pathname
	if varyUA {
		target := legacyGetBuildTargetByUA(ctx.UserAgent())
		savePath += "." + target
	}
	f, _, e := buildStorage.Get(savePath)
	if e == nil {
		return f // auto closed
	}
	if e != storage.ErrNotFound {
		return rex.Err(500, "Storage Error")
	}

	url, err := ctx.R.URL.Parse(config.LegacyServer + ctx.R.URL.Path)
	if err != nil {
		return rex.Err(http.StatusBadRequest, "Invalid url")
	}
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Host:       url.Host,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{ctx.UserAgent()},
		},
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return rex.Err(http.StatusBadGateway, "Failed to fetch lagecy server")
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		if res.StatusCode == 404 {
			return rex.Status(404, "Not Found")
		}
		return rex.Err(res.StatusCode, "Failed to fetch lagecy server: "+res.Status)
	}

	buf := bytes.NewBuffer(nil)
	err = buildStorage.Put(savePath, io.TeeReader(res.Body, buf))
	if err != nil {
		return rex.Err(500, "Storage Error")
	}

	return buf
}
