package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

// A Record of mmdb
type Record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

func registerAPI(storageDir string, domain string, cdnDomain string, cdnDomainChina string) {
	start := time.Now()
	httpClient := &http.Client{
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
			MaxIdleConnsPerHost:   5,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}

	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		switch pathname {
		case "/":
			mdStr := strings.TrimSpace(string(utils.MustEncodeJSON(readme)))
			return rex.Content("index.html", start, bytes.NewReader([]byte(fmt.Sprintf(indexHTML, "`", mdStr))))
		case "/favicon.ico":
			return 404
		case "/_error.js":
			t := ctx.Form.Value("type")
			switch t {
			case "resolve":
				return throwErrorJS(ctx, 500, fmt.Errorf(`Can't resolve "%s"`, ctx.Form.Value("name")))
			default:
				return throwErrorJS(ctx, 500, fmt.Errorf("Unknown error"))
			}
		case fmt.Sprintf("/v%d/_process_browser.js", builderID):
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content("process/browser.js", start, bytes.NewReader([]byte(polyfills["process_browser.js"])))
		case fmt.Sprintf("/v%d/_node_fs.js", builderID):
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content("node/fs.js", start, bytes.NewReader([]byte(polyfills["node_fs.js"])))
		case fmt.Sprintf("/v%d/_node_readline.js", builderID):
			ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
			return rex.Content("node/readline.js", start, bytes.NewReader([]byte(polyfills["node_readline.js"])))
		}

		if len(strings.Split(pathname, "/")) > 2 || (strings.HasPrefix(pathname, "/bundle-") && strings.HasSuffix(pathname, ".js")) {
			var storageType string
			switch path.Ext(pathname) {
			case ".js":
				storageType = "builds"
			case ".ts":
				if strings.HasSuffix(pathname, ".d.ts") {
					storageType = "types"
				} else {
					storageType = "raw"
				}
			case ".json", ".jsx", ".tsx", ".css", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm":
				storageType = "raw"
			}
			if storageType != "" {
				if storageType == "raw" {
					m, err := parseModule(pathname)
					if err != nil {
						return throwErrorJS(ctx, 500, err)
					}
					shouldRedirect := !regVersionPath.MatchString(pathname)
					hostname := ctx.R.Host
					proto := "http"
					if ctx.R.TLS != nil {
						proto = "https"
					}
					if hostname == domain {
						if cdnDomain != "" {
							shouldRedirect = true
							hostname = cdnDomain
							proto = "https"
						}
						if cdnDomainChina != "" {
							var record Record
							err = mmdbr.Lookup(net.ParseIP(ctx.RemoteIP()), &record)
							if err == nil && record.Country.ISOCode == "CN" {
								shouldRedirect = true
								hostname = cdnDomainChina
								proto = "https"
							}
						}
					}
					if shouldRedirect {
						return rex.Redirect(fmt.Sprintf("%s://%s/%s", proto, hostname, m.String()), 302)
					}
					cacheFile := path.Join(storageDir, "raw", m.String())
					if fileExists(cacheFile) {
						if strings.HasSuffix(pathname, ".ts") {
							ctx.SetHeader("Content-Type", "application/typescript")
						}
						ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
						return rex.File(cacheFile)
					}
					resp, err := httpClient.Get(fmt.Sprintf("https://unpkg.com/%s", m.String()))
					if err != nil {
						return err
					}
					if resp.StatusCode != 200 {
						return http.StatusBadGateway
					}
					defer resp.Body.Close()
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
				fp := path.Join(storageDir, storageType, pathname)
				if fileExists(fp) {
					if storageType == "types" {
						ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
					}
					ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
					return rex.File(fp)
				}
			}
		}

		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			target = "esnext"
		}
		external := moduleSlice{}
		for _, p := range strings.Split(ctx.Form.Value("external"), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				m, err := parseModule(p)
				if err != nil {
					if strings.HasSuffix(err.Error(), "not found") {
						continue
					}
					return throwErrorJS(ctx, 500, err)
				}
				if !external.Has(m.name) {
					external = append(external, *m)
				}
			}
		}
		isDev := !ctx.Form.IsNil("dev")
		noCheck := !ctx.Form.IsNil("nocheck") || !ctx.Form.IsNil("noCheck") || !ctx.Form.IsNil("no-check")

		var bundleList string
		var isBare bool
		var currentModule *module
		var err error
		if strings.HasPrefix(pathname, "/[") && strings.Contains(pathname, "]") {
			bundleList, pathname = utils.SplitByFirstByte(strings.TrimPrefix(pathname, "/["), ']')
			if pathname == "" {
				pathname = "/"
			}
		}
		if bundleList == "" && endsWith(pathname, ".js") {
			currentModule, err = parseModule(strings.TrimPrefix(pathname, fmt.Sprintf("/v%d", builderID)))
			if err == nil && !endsWith(currentModule.name, ".js") {
				a := strings.Split(currentModule.submodule, "/")
				if len(a) > 1 {
					if strings.HasPrefix(a[0], "external=") {
						for _, p := range strings.Split(strings.TrimPrefix(a[0], "external="), ",") {
							p = strings.TrimSpace(p)
							if p != "" {
								if strings.HasPrefix(p, "@") {
									scope, name := utils.SplitByFirstByte(p, '_')
									p = scope + "/" + name
								}
								m, err := parseModule(p)
								if err != nil {
									if strings.HasSuffix(err.Error(), "not found") {
										continue
									}
									return throwErrorJS(ctx, 500, err)
								}
								if !external.Has(m.name) {
									external = append(external, *m)
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
						if submodule == path.Base(currentModule.name) {
							submodule = ""
						}
						currentModule.submodule = submodule
						target = a[0]
						isBare = true
					}
				}
			}
		} else {
			currentModule, err = parseModule(strings.TrimPrefix(pathname, fmt.Sprintf("/v%d", builderID)))
		}
		if err != nil {
			if strings.HasSuffix(err.Error(), "not found") {
				return throwErrorJS(ctx, 404, err)
			}
			return throwErrorJS(ctx, 500, err)
		}

		var packages moduleSlice
		if bundleList != "" {
			containsPackage := currentModule.name == ""
			for _, dep := range strings.Split(bundleList, ",") {
				m, err := parseModule(strings.TrimSpace(dep))
				if err != nil {
					return throwErrorJS(ctx, 500, err)
				}
				if !containsPackage && m.Equels(*currentModule) {
					containsPackage = true
				}
				if !packages.Has(m.name) {
					packages = append(packages, *m)
				}
			}
			if len(packages) > 10 {
				return throwErrorJS(ctx, 400, fmt.Errorf("too many packages in the bundle list, up to 10 but get %d", len(packages)))
			}
			if !containsPackage {
				return throwErrorJS(ctx, 400, fmt.Errorf("package '%s' not found in the bundle list", currentModule.ImportPath()))
			}
		} else {
			packages = moduleSlice{*currentModule}
		}

		ret, err := build(storageDir, domain, buildOptions{
			packages: packages,
			external: external,
			target:   target,
			isDev:    isDev,
		})
		if err != nil {
			return throwErrorJS(ctx, 500, err)
		}

		if isBare {
			fp := path.Join(storageDir, "builds", pathname)
			if fileExists(fp) {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(fp)
			}
			return 404
		}

		if bundleList != "" && currentModule.name == "" {
			return ret.importMeta
		}

		importPath := currentModule.ImportPath()
		importMeta, ok := ret.importMeta[importPath]
		if !ok {
			return throwErrorJS(ctx, 500, fmt.Errorf("package '%s' not found in bundle", importPath))
		}

		buf := bytes.NewBuffer(nil)
		importIdentifier := identify(importPath)
		importPrefix := "/"
		if cdnDomain != "" {
			importPrefix = fmt.Sprintf("https://%s/", cdnDomain)
		}
		if cdnDomainChina != "" {
			var record Record
			err = mmdbr.Lookup(net.ParseIP(ctx.RemoteIP()), &record)
			if err == nil && record.Country.ISOCode == "CN" {
				importPrefix = fmt.Sprintf("https://%s/", cdnDomainChina)
			}
		}

		fmt.Fprintf(buf, `/* %s - %v */%s`, jsCopyrightName, currentModule, EOL)
		var exported bool
		if len(packages) == 1 {
			if importMeta.Module != "" {
				fmt.Fprintf(buf, `export * from "%s%s.js";%s`, importPrefix, ret.buildID, EOL)
				for _, name := range importMeta.Exports {
					if name == "default" {
						fmt.Fprintf(buf, `export {default} from "%s%s.js";%s`, importPrefix, ret.buildID, EOL)
						break
					}
				}
				exported = true
			} else {
				fmt.Fprintf(buf, `import %s_default from "%s%s.js";%s`, importIdentifier, importPrefix, ret.buildID, EOL)
			}
		} else {
			if importMeta.Module != "" {
				fmt.Fprintf(buf, `import { %s_default, %s_star } from "%s%s.js";%s`, importIdentifier, importIdentifier, importPrefix, ret.buildID, EOL)
			} else {
				fmt.Fprintf(buf, `import { %s_default } from "%s%s.js";%s`, importIdentifier, importPrefix, ret.buildID, EOL)
			}
		}
		if !exported {
			var exports []string
			var hasDefaultExport bool
			for _, name := range importMeta.Exports {
				if name == "default" {
					hasDefaultExport = true
				} else if name != "import" {
					exports = append(exports, name)
				}
			}
			if len(exports) > 0 {
				if importMeta.Module != "" {
					fmt.Fprintf(buf, `export const { %s } = %s_star;%s`, strings.Join(exports, ","), importIdentifier, EOL)
				} else {
					fmt.Fprintf(buf, `export const { %s } = %s_default;%s`, strings.Join(exports, ","), importIdentifier, EOL)
				}
			}
			if hasDefaultExport || (importMeta.Main != "" && importMeta.Module == "") {
				fmt.Fprintf(buf, `export default %s_default;%s`, importIdentifier, EOL)
			}
		}
		if importMeta.Dts != "" && !noCheck {
			ctx.SetHeader("X-TypeScript-Types", importMeta.Dts)
		}
		ctx.SetHeader("Cache-Control", fmt.Sprintf("private, max-age=%d", refreshDuration))
		ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
		return buf.String()
	})
}

func throwErrorJS(ctx *rex.Context, status int, err error) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, `/* %s - error */%s`, jsCopyrightName, EOL)
	fmt.Fprintf(buf, `throw new Error("[%s] " + %s);%s`, jsCopyrightName, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))), EOL)
	fmt.Fprintf(buf, `export default null;%s`, EOL)
	ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
	return &rex.TypedContent{
		Status:      status,
		Content:     buf.Bytes(),
		ContentType: "application/javascript; charset=utf-8",
	}
}
