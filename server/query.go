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

const (
	// EOL defines the char of end of line
	EOL = "\n"
)

func init() {
	rex.Query("bundle", func(ctx *rex.Context) interface{} {
		return rex.HTML("<p>todo: bundle management<p>")
	})

	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		if pathname == "/" {
			return rex.HTML(indexHTML, 200)
		}

		if strings.HasPrefix(pathname, "/bundle-") && strings.HasSuffix(pathname, ".js") {
			return rex.File(path.Join(etcDir, "builds", strings.TrimPrefix(pathname, "/bundle-")))
		}

		var bundleList string
		if strings.HasPrefix(pathname, "/[") && strings.Contains(pathname, "]/") {
			bundleList, pathname = utils.SplitByFirstByte(strings.TrimPrefix(pathname, "/["), ']')
		}

		currentModule, err := parseModule(pathname)
		if err != nil {
			return throwErrorJs(err)
		}

		var packages moduleSlice
		if bundleList != "" {
			var containsPackage bool
			for _, dep := range strings.Split(bundleList, ",") {
				m, err := parseModule(strings.TrimSpace(dep))
				if err != nil {
					return throwErrorJs(err)
				}
				if m.name == currentModule.name && m.version == currentModule.version && m.submodule == currentModule.submodule {
					containsPackage = true
				}
				packages = append(packages, *m)
			}
			if !containsPackage {
				return throwErrorJs(fmt.Errorf("package '%s' not found in the bundle list", currentModule.ImportPath()))
			}
		} else {
			packages = moduleSlice{*currentModule}
		}

		if len(packages) > 10 {
			return throwErrorJs(fmt.Errorf("too many packages in the bundle list, up to 10 but get %d", len(packages)))
		}

		env := "production"
		if !ctx.Form.IsNil("dev") {
			env = "development"
		}
		target := strings.ToUpper(strings.TrimSpace(ctx.Form.Value("target")))
		if target == "" {
			target = "ESNEXT"
		}
		ret, err := build(buildOptions{
			packages: packages,
			target:   target,
			env:      env,
		})
		if err != nil {
			return throwErrorJs(err)
		}

		importPath := currentModule.ImportPath()
		importMeta, ok := ret.importMeta[importPath]
		if !ok {
			return throwErrorJs(fmt.Errorf("package '%s' not found in bundle", importPath))
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
		fmt.Fprintf(buf, `/* esm.sh - %v */%s`, currentModule, EOL)
		if cdnDomain != "" {
			fmt.Fprintf(buf, `import { %s } from "https://%s/bundle-%s.js";%s`, importIdentifier, cdnDomain, ret.hash, EOL)
		} else {
			fmt.Fprintf(buf, `import { %s } from "/bundle-%s.js";%s`, importIdentifier, ret.hash, EOL)
		}
		if len(exports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(exports, ","), importIdentifier, EOL)
		}
		if !hasDefaultExport {
			fmt.Fprintf(buf, `export default %s;%s`, importIdentifier, EOL)
		}
		return rex.Content(importIdentifier+".js", time.Now(), bytes.NewReader(buf.Bytes()))
	})
}

func parseModule(pathname string) (*module, error) {
	a := strings.Split(strings.Trim(pathname, "/"), "/")
	for i, s := range a {
		a[i] = strings.TrimSpace(s)
	}
	scope := ""
	packageName := a[0]
	submodule := strings.Join(a[1:], "/")
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		scope = a[0]
		packageName = a[1]
		submodule = strings.Join(a[2:], "/")
	}
	name, version := utils.SplitByLastByte(packageName, '@')
	if scope != "" {
		name = scope + "/" + name
	}
	if version == "" {
		info, err := nodeEnv.getPackageLatestInfo(name)
		if err != nil {
			return nil, err
		}
		version = info.Version
	}
	return &module{
		name:      name,
		version:   version,
		submodule: submodule,
	}, nil
}

func throwErrorJs(err error) interface{} {
	message := fmt.Sprintf(`throw new Error("[esm.sh] " + %s);`, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))))
	return rex.Content("error.js", time.Now(), bytes.NewReader([]byte(message)))
}
