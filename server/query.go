package server

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

func registerAPI(storageDir string, cdnDomain string) {
	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		if pathname == "/" {
			return rex.HTML(indexHTML)
		}

		switch path.Ext(pathname) {
		case ".js":
			return rex.File(path.Join(storageDir, "builds", pathname))
		case ".ts":
			if strings.HasSuffix(pathname, ".d.ts") {
				ctx.SetHeader("Content-Type", "application/typescript; charset=utf-8")
				return rex.File(path.Join(storageDir, "types", pathname))
			}
			fallthrough
		case ".json", ".jsx", ".tsx", ".css", ".less", ".sass", ".scss", "stylus", "styl", ".wasm":
			return rex.File(path.Join(storageDir, "raw", pathname))
		}

		var bundleList string
		if strings.HasPrefix(pathname, "/[") && strings.Contains(pathname, "]") {
			bundleList, pathname = utils.SplitByFirstByte(strings.TrimPrefix(pathname, "/["), ']')
			if pathname == "" {
				pathname = "/"
			}
		}

		currentModule, err := parseModule(pathname)
		if err != nil {
			return throwErrorJS(err)
		}

		var packages moduleSlice
		if bundleList != "" {
			containsPackage := currentModule.name == ""
			for _, dep := range strings.Split(bundleList, ",") {
				m, err := parseModule(strings.TrimSpace(dep))
				if err != nil {
					return throwErrorJS(err)
				}
				if !containsPackage && m.Equels(*currentModule) {
					containsPackage = true
				}
				packages = append(packages, *m)
			}
			if len(packages) > 10 {
				return throwErrorJS(fmt.Errorf("too many packages in the bundle list, up to 10 but get %d", len(packages)))
			}
			if !containsPackage {
				return throwErrorJS(fmt.Errorf("package '%s' not found in the bundle list", currentModule.ImportPath()))
			}
		} else {
			packages = moduleSlice{*currentModule}
		}

		target := strings.ToLower(strings.TrimSpace(ctx.Form.Value("target")))
		if _, ok := targets[target]; !ok {
			target = "esnext"
		}
		ret, err := build(storageDir, buildOptions{
			packages: packages,
			target:   target,
			dev:      !ctx.Form.IsNil("dev"),
		})
		if err != nil {
			return throwErrorJS(err)
		}

		if currentModule.name == "" {
			return ret.importMeta
		}

		importPath := currentModule.ImportPath()
		importMeta, ok := ret.importMeta[importPath]
		if !ok {
			return throwErrorJS(fmt.Errorf("package '%s' not found in bundle", importPath))
		}

		var exports []string
		hasDefaultExport := false
		for _, name := range importMeta.Exports {
			if name == "default" {
				hasDefaultExport = true
				break
			} else if name != "import" {
				exports = append(exports, name)
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
		return rex.Content(importIdentifier+".js", time.Now(), bytes.NewReader(buf.Bytes()))
	})
}

func throwErrorJS(err error) interface{} {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, `/* esm.sh - error */%s`, EOL)
	fmt.Fprintf(buf, `throw new Error("[esm.sh] " + %s);%s`, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))), EOL)
	fmt.Fprintf(buf, `export default null;%s`, EOL)
	return rex.Content("error.js", time.Now(), bytes.NewReader(buf.Bytes()))
}
