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

// A Country of mmdb record.
type Country struct {
	ISOCode string `maxminddb:"iso_code"`
}

// A Record of mmdb.
type Record struct {
	Country Country `maxminddb:"country"`
}

func registerRoutes(storageDir string, domain string, cdnDomain string, cdnDomainChina string) {
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
		pathname := ctx.Path.String()
		switch pathname {
		case "/":
			readme, err := embedFS.ReadFile("README.md")
			if err != nil {
				return err
			}
			indexHTML, err := embedFS.ReadFile("embed/index.html")
			if err != nil {
				return err
			}
			readmeStr := utils.MustEncodeJSON(string(readme))
			html := bytes.Replace(indexHTML, []byte("'# README'"), readmeStr, -1)
			return rex.Content("index.html", start, bytes.NewReader(html))
		case "/favicon.ico":
			return 404
		case "/_error.js":
			switch ctx.Form.Value("type") {
			case "resolve":
				return throwErrorJS(ctx, 500, fmt.Errorf(`Can't resolve "%s"`, ctx.Form.Value("name")))
			default:
				return throwErrorJS(ctx, 500, fmt.Errorf("Unknown error"))
			}
		}

		if strings.HasPrefix(pathname, "/embed/") {
			data, err := embedFS.ReadFile(pathname[1:])
			if err != nil {
				return err
			}
			return rex.Content(pathname, start, bytes.NewReader(data))
		}

		hasBuildVerPrefix := strings.HasPrefix(pathname, fmt.Sprintf("/v%d/", buildVersion))
		prevBuildVer := ""
		if hasBuildVerPrefix {
			pathname = strings.TrimPrefix(pathname, fmt.Sprintf("/v%d", buildVersion))
		} else if regBuildVerPath.MatchString(pathname) {
			a := strings.Split(pathname, "/")
			pathname = "/" + strings.Join(a[2:], "/")
			hasBuildVerPrefix = true
			prevBuildVer = a[1]
		}

		var storageType string
		switch path.Ext(pathname) {
		case ".js":
			if hasBuildVerPrefix || (strings.HasPrefix(pathname, "/bundle-") && len(strings.Split(pathname, "/")) == 2) {
				storageType = "builds"
			}
		case ".ts":
			if hasBuildVerPrefix && strings.HasSuffix(pathname, ".d.ts") {
				storageType = "types"
			} else if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		case ".css":
			if hasBuildVerPrefix {
				storageType = "builds"
			} else if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		case ".json", ".xml", ".yaml", ".jsx", ".tsx", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm":
			if len(strings.Split(pathname, "/")) > 2 {
				storageType = "raw"
			}
		}
		if storageType == "raw" {
			m, err := parseModule(pathname)
			if err != nil {
				return throwErrorJS(ctx, 500, err)
			}
			if m.submodule != "" {
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
					url := fmt.Sprintf("%s://%s/%s", proto, hostname, m.String())
					return rex.Redirect(url, http.StatusTemporaryRedirect)
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
			storageType = ""
		}
		if storageType != "" {
			var filepath string
			if hasBuildVerPrefix && (storageType == "builds" || storageType == "types") {
				if prevBuildVer != "" {
					filepath = path.Join(storageDir, storageType, prevBuildVer, pathname)
				} else {
					filepath = path.Join(storageDir, storageType, fmt.Sprintf("v%d", buildVersion), pathname)
				}
			} else {
				filepath = path.Join(storageDir, storageType, pathname)
			}
			if fileExists(filepath) {
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(filepath)
			}
		}

		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			if strings.HasPrefix(ctx.R.UserAgent(), "Deno/") {
				target = "deno"
			} else {
				// todo: check browser ua
				target = "esnext"
			}
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

		isCSS := !ctx.Form.IsNil("css")
		isDev := !ctx.Form.IsNil("dev")
		noCheck := !ctx.Form.IsNil("no-check")

		var (
			bundleList    string
			isBare        bool
			currentModule *module
			err           error
		)

		if strings.HasPrefix(pathname, "/[") && strings.Contains(pathname, "]") {
			bundleList, pathname = utils.SplitByFirstByte(strings.TrimPrefix(pathname, "/["), ']')
			if pathname == "" {
				pathname = "/"
			}
		}
		if bundleList == "" && endsWith(pathname, ".js") {
			currentModule, err = parseModule(pathname)
			if err == nil {
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
			currentModule, err = parseModule(pathname)
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

		if isCSS {
			if ret.hasCSS {
				hostname := ctx.R.Host
				proto := "http"
				if ctx.R.TLS != nil {
					proto = "https"
				}
				url := fmt.Sprintf("%s://%s/%s.css", proto, hostname, ret.buildID)
				code := http.StatusTemporaryRedirect
				if regVersionPath.MatchString(pathname) {
					code = http.StatusPermanentRedirect
				}
				return rex.Redirect(url, code)
			}
			return throwErrorJS(ctx, 404, fmt.Errorf("css not found"))
		}

		if bundleList != "" && currentModule.name == "" {
			return ret.importMeta
		}

		if isBare {
			fp := path.Join(storageDir, "builds", fmt.Sprintf("v%d", buildVersion), pathname)
			if fileExists(fp) {
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(fp)
			}
			return 404
		}

		importPath := currentModule.ImportPath()
		importMeta, ok := ret.importMeta[importPath]
		if !ok {
			return throwErrorJS(ctx, 500, fmt.Errorf("package '%s' not found in bundle", importPath))
		}

		buf := bytes.NewBuffer(nil)
		importIdentifier := identify(importPath)
		importPrefix := "/"
		importSuffix := ".js"
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
		if len(packages) == 1 {
			fmt.Fprintf(buf, `export * from "%s%s%s";%s`, importPrefix, ret.buildID, importSuffix, EOL)
			if importMeta.Module != "" {
				for _, name := range importMeta.Exports {
					if name == "default" {
						fmt.Fprintf(buf, `export { default } from "%s%s%s";%s`, importPrefix, ret.buildID, importSuffix, EOL)
						break
					}
				}
			} else {
				fmt.Fprintf(buf, `export { default } from "%s%s%s";%s`, importPrefix, ret.buildID, importSuffix, EOL)
			}
		} else {
			var exports []string
			var hasDefaultExport bool
			for _, name := range importMeta.Exports {
				if name == "default" {
					hasDefaultExport = true
				} else if name != "import" {
					exports = append(exports, name)
				}
			}
			if importMeta.Module != "" {
				fmt.Fprintf(buf, `import { %s_default, %s_star } from "%s%s%s";%s`, importIdentifier, importIdentifier, importPrefix, ret.buildID, importSuffix, EOL)
				fmt.Fprintf(buf, `export const { %s } = %s_star;%s`, strings.Join(exports, ","), importIdentifier, EOL)
			} else {
				fmt.Fprintf(buf, `import { %s_default } from "%s%s%s";%s`, importIdentifier, importPrefix, ret.buildID, importSuffix, EOL)
				fmt.Fprintf(buf, `export const { %s } = %s_default;%s`, strings.Join(exports, ","), importIdentifier, EOL)
			}
			if hasDefaultExport || (importMeta.Main != "" && importMeta.Module == "") {
				fmt.Fprintf(buf, `export default %s_default;%s`, importIdentifier, EOL)
			}
		}
		if importMeta.Dts != "" && !noCheck {
			ctx.SetHeader("X-TypeScript-Types", fmt.Sprintf("%s%s", importPrefix, strings.TrimPrefix(path.Join("/", fmt.Sprintf("v%d", buildVersion), importMeta.Dts), "/")))
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
	ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
	return rex.Status(status, buf)
}
