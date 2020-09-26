package server

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

func registerAPI(storageDir string, cdnDomain string) {
	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		switch pathname {
		case "/":
			return rex.HTML(indexHTML)
		case "/favicon.ico":
			return 404
		}

		var storageType string
		switch path.Ext(pathname) {
		case ".js":
			storageType = "builds"
		case ".ts":
			if strings.HasSuffix(pathname, ".d.ts") {
				storageType = "types"
			}
			fallthrough
		case ".json", ".jsx", ".tsx", ".css", ".less", ".sass", ".scss", ".stylus", ".styl", ".wasm":
			storageType = "raw"
		}
		if storageType != "" {
			fp := path.Join(storageDir, storageType, pathname)
			if fileExists(fp) {
				if storageType == "types" {
					ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				}
				ctx.SetHeader("Cache-Control", "public, max-age=31536000, immutable")
				return rex.File(fp)
			}
		}

		var bundleList string
		if strings.HasPrefix(pathname, "/[") && strings.Contains(pathname, "]") {
			bundleList, pathname = utils.SplitByFirstByte(strings.TrimPrefix(pathname, "/["), ']')
			if pathname == "" {
				pathname = "/"
			}
		}

		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			target = "esnext"
		}
		isDev := !ctx.Form.IsNil("dev")

		var currentModule *module
		var isBare bool
		var err error
		if bundleList == "" && endsWith(pathname, ".js") {
			currentModule, err = parseModule(pathname)
			if err == nil && !endsWith(currentModule.name, ".js") {
				a := strings.Split(currentModule.submodule, "/")
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
			return throwErrorJS(ctx, err)
		}

		var packages moduleSlice
		if bundleList != "" {
			containsPackage := currentModule.name == ""
			for _, dep := range strings.Split(bundleList, ",") {
				m, err := parseModule(strings.TrimSpace(dep))
				if err != nil {
					return throwErrorJS(ctx, err)
				}
				if !containsPackage && m.Equels(*currentModule) {
					containsPackage = true
				}
				packages = append(packages, *m)
			}
			if len(packages) > 10 {
				return throwErrorJS(ctx, fmt.Errorf("too many packages in the bundle list, up to 10 but get %d", len(packages)))
			}
			if !containsPackage {
				return throwErrorJS(ctx, fmt.Errorf("package '%s' not found in the bundle list", currentModule.ImportPath()))
			}
		} else {
			packages = moduleSlice{*currentModule}
		}

		ret, err := build(storageDir, buildOptions{
			packages: packages,
			target:   target,
			dev:      isDev,
		})
		if err != nil {
			return throwErrorJS(ctx, err)
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
			return throwErrorJS(ctx, fmt.Errorf("package '%s' not found in bundle", importPath))
		}

		var exports []string
		var hasDefaultExport bool
		for _, name := range importMeta.Exports {
			if name != "import" {
				exports = append(exports, name)
			}
			if name == "default" {
				hasDefaultExport = true
			}
		}

		buf := bytes.NewBuffer(nil)
		importIdentifier := identify(importPath)
		importPrefix := "/"
		if cdnDomain != "" {
			importPrefix = fmt.Sprintf("https://%s/", cdnDomain)
		}
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, currentModule, EOL)
		if ret.single {
			fmt.Fprintf(buf, `import %s from "%s%s.js";%s`, importIdentifier, importPrefix, ret.buildID, EOL)
		} else {
			fmt.Fprintf(buf, `import { %s } from "%s%s.js";%s`, importIdentifier, importPrefix, ret.buildID, EOL)
		}
		if len(exports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(exports, ","), importIdentifier, EOL)
		}
		if !hasDefaultExport {
			fmt.Fprintf(buf, `export default %s;%s`, importIdentifier, EOL)
		}
		if importMeta.TypesPath != "" {
			ctx.SetHeader("X-TypeScript-Types", importMeta.TypesPath)
		}
		ctx.SetHeader("Cache-Control", fmt.Sprintf("private, max-age=%d", refreshDuration))
		ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
		return buf.String()
	})
}

func throwErrorJS(ctx *rex.Context, err error) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, `/* esm.sh - error */%s`, EOL)
	fmt.Fprintf(buf, `throw new Error("[esm.sh] " + %s);%s`, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))), EOL)
	fmt.Fprintf(buf, `export default null;%s`, EOL)
	ctx.SetHeader("Cache-Control", "private, no-store, no-cache, must-revalidate")
	ctx.SetHeader("Content-Type", "application/javascript; charset=utf-8")
	return buf.String()
}
