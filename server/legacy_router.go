package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

func esmLegacyRouter(fs storage.Storage, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		pathname := r.URL.Path

	START:
		// build API (deprecated)
		if pathname == "/build" {
			switch method {
			case "POST":
				writeStatus(w, 403, "The `/build` API has been deprecated.")
			case "GET":
				h := w.Header()
				h.Set("Content-Type", ctJavaScript)
				h.Set("Cache-Control", ccImmutable)
				writeBody(w, []byte(`
					const deprecated = new Error("[esm.sh] The build API has been deprecated.")
					export function build(_) { throw deprecated }
					export function esm(_) { throw deprecated }
					export function transform(_) { throw deprecated }
					export default build
				`))
			default:
				writeStatus(w, 405, "Method Not Allowed")
			}
			return
		}

		// `/react-dom@18.3.1?pin=v135`
		if q := r.URL.RawQuery; q != "" && (strings.HasPrefix(q, "pin=v") || strings.Contains(q, "&pin=v")) {
			query := r.URL.Query()
			v := query.Get("pin")
			if len(v) > 1 && v[0] == 'v' && valid.IsDigtalOnlyString(v[1:]) {
				bv, _ := strconv.Atoi(v[1:])
				if bv <= 0 || bv > 135 {
					writeStatus(w, 400, "Invalid `pin` query")
					return
				}
				if !legacyESM(w, r, fs, "") {
					next.ServeHTTP(w, r)
				}
				return
			}
		}

		// `/react-dom@18.3.1&pin=v135`
		if strings.Contains(pathname, "&pin=v") {
			if !legacyESM(w, r, fs, "") {
				next.ServeHTTP(w, r)
			}
			return
		}

		// `/stable/react@18.3.1?dev`
		// `/stable/react@18.3.1/es2022/react.mjs`
		if strings.HasPrefix(pathname, "/stable/") {
			if !legacyESM(w, r, fs, "stable") {
				next.ServeHTTP(w, r)
			}
			return
		}

		// `/v135/react-dom@18.3.1?dev`
		// `/v135/react-dom@18.3.1/es2022/react-dom.mjs`
		if strings.HasPrefix(pathname, "/v") {
			legacyBuildVersion, path := utils.SplitByFirstByte(pathname[2:], '/')
			if valid.IsDigtalOnlyString(legacyBuildVersion) {
				bv, _ := strconv.Atoi(legacyBuildVersion)
				if bv <= 0 || bv > 135 {
					writeStatus(w, 400, "Invalid Module Path")
					return
				}
				if path == "" && strings.HasPrefix(r.UserAgent(), "Deno/") {
					h := w.Header()
					h.Set("Content-Type", ctJavaScript)
					h.Set("Cache-Control", ccImmutable)
					writeBody(w, []byte(`throw new Error("[esm.sh] The deno CLI has been deprecated, please use our vscode extension instead: https://marketplace.visualstudio.com/items?itemName=ije.esm-vscode")`))
					return
				}
				if path == "build" {
					pathname = "/build"
					goto START
				}
				if !legacyESM(w, r, fs, "v"+legacyBuildVersion) {
					next.ServeHTTP(w, r)
				}
				return
			}
		}

		// packages created by the `/build` API
		if len(pathname) == 42 && strings.HasPrefix(pathname, "/~") && valid.IsHexString(pathname[2:]) {
			redirect(w, fmt.Sprintf("/v135%s@0.0.0/%s/mod.mjs", pathname, getBuildTargetByUA(r.UserAgent())), true)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type LegacyBuildMeta struct {
	EsmId string `json:"esmId,omitempty"`
	Dts   string `json:"dts,omitempty"`
	Code  string `json:"code"`
}

// legacyESM serves legacy esm.sh builds from the storage. It returns false if
// the request is not handled and should be passed to the next handler.
func legacyESM(w http.ResponseWriter, r *http.Request, fs storage.Storage, buildVersionPrefix string) bool {
	pathname := r.URL.Path
	if buildVersionPrefix != "" {
		pathname = pathname[len(buildVersionPrefix)+1:]
	}
	query := ""
	if r.URL.RawQuery != "" {
		query = "?" + r.URL.RawQuery
	}
	var isStatic bool
	if (strings.HasPrefix(pathname, "/node_") && strings.HasSuffix(pathname, ".js")) || pathname == "/node.ns.d.ts" {
		isStatic = true
	} else {
		if strings.HasPrefix(pathname, "/gh/") {
			if !strings.ContainsRune(pathname[4:], '/') {
				writeStatus(w, 400, "invalid path")
				return true
			}
			// add a leading `@` to the package name
			pathname = "/@" + pathname[4:]
		}
		pkgName, pkgVersion, subPath := splitEsmPath(pathname)
		_, target, _ := parseSubPath(subPath)
		var asteriskFlag bool
		if len(pkgName) > 1 && pkgName[0] == '*' {
			asteriskFlag = true
			pkgName = pkgName[1:]
		}
		if !npm.ValidatePackageName(pkgName) {
			writeStatus(w, 400, "Invalid Package Name")
			return true
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
					writeStatus(w, 404, err.Error())
				} else {
					writeStatus(w, 500, err.Error())
				}
				return true
			}
			var b strings.Builder
			b.WriteString(getOrigin(r))
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
			if len(subPath) > 0 {
				b.WriteByte('/')
				b.WriteString(subPath)
			}
			b.WriteString(query)
			redirect(w, b.String(), false)
			return true
		}
		isStatic = target != ""
	}
	savePath := "legacy/" + normalizeSavePath(r.URL.Path[1:])
	if (buildVersionPrefix != "" && isStatic) || endsWith(pathname, ".d.ts", ".d.mts") {
		f, fi, e := fs.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			writeStatus(w, 500, "Storage error: "+e.Error())
			return true
		}
		if e == nil {
			h := w.Header()
			switch path.Ext(pathname) {
			case ".js", ".mjs":
				h.Set("Content-Type", ctJavaScript)
			case ".ts", ".mts":
				h.Set("Content-Type", ctTypeScript)
				// resolve hostname in typescript definition files if the origin is not "https://esm.sh"
				if endsWith(pathname, ".d.ts", ".d.mts") {
					origin := getOrigin(r)
					if origin != "https://esm.sh" {
						defer f.Close()
						data, err := io.ReadAll(f)
						if err != nil {
							writeStatus(w, 500, "Failed to read data from storage")
							return true
						}
						data = bytes.ReplaceAll(data, []byte("https://esm.sh/v"), []byte(origin+"/v"))
						data = bytes.ReplaceAll(data, []byte("https://legacy.esm.sh/v"), []byte(origin+"/v"))
						writeBody(w, data)
						return true
					}
				}
			case ".map":
				h.Set("Content-Type", ctJSON)
			case ".css":
				h.Set("Content-Type", ctCSS)
			default:
				f.Close()
				writeStatus(w, 404, "Module Not Found")
				return true
			}
			h.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
			h.Set("Cache-Control", ccImmutable)
			writeReader(w, f)
			return true
		}
	} else {
		varyUA := false
		if query != "" {
			if !r.URL.Query().Has("target") {
				varyUA = true
				savePath += "." + getBuildTargetByUA(r.UserAgent())
			}
			h := xxhash.New()
			h.Write([]byte(query))
			savePath += "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
		}
		savePath += ".meta"
		f, _, e := fs.Get(savePath)
		if e != nil && e != storage.ErrNotFound {
			writeStatus(w, 500, "Storage error: "+e.Error())
			return true
		}
		if e == nil {
			defer f.Close()
			var ret LegacyBuildMeta
			if json.NewDecoder(f).Decode(&ret) == nil {
				h := w.Header()
				h.Set("Content-Type", ctJavaScript)
				h.Set("Cache-Control", ccImmutable)
				if varyUA {
					appendVaryHeader(h, "User-Agent")
				}
				if ret.EsmId != "" {
					h.Set("X-ESM-Id", ret.EsmId)
				}
				if ret.Dts != "" {
					h.Set("X-TypeScript-Types", getOrigin(r)+ret.Dts)
				}
				writeBody(w, []byte(ret.Code))
				return true
			}
		}
	}

	// strip leading `/stable/*` and `/v<build-version>/*`
	if buildVersionPrefix != "" {
		origin := getOrigin(r)
		if strings.HasPrefix(pathname, "/node_") && strings.HasSuffix(pathname, ".js") {
			pathname = "/node/" + strings.TrimSuffix(strings.TrimPrefix(pathname, "/node_"), ".js") + ".mjs"
		} else if pathname == "/node.ns.d.ts" {
			writeStatus(w, 404, "Not Found")
			return true
		}
		redirect(w, fmt.Sprintf("%s%s%s", origin, pathname, query), true)
		return true
	}

	return false
}
