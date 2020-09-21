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
	// EOL defines char end of line
	EOL = "\n"
)

func init() {
	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		if pathname == "/" {
			return rex.HTML(indexHTML, 200)
		}

		if strings.HasPrefix(pathname, "/bundle-") && strings.HasSuffix(pathname, ".js") {
			return rex.File(path.Join(etcDir, "builds", strings.TrimPrefix(pathname, "/bundle-")))
		}

		packageName, version, submodule := parsePackageName(pathname)
		if version == "" {
			info, err := nodeEnv.getPackageLatestInfo(packageName)
			if err != nil {
				return throwErrorJs(err)
			}
			version = info.Version
		}
		bundleValue := ctx.Form.Value("bundle")
		module0 := module{
			name:      packageName,
			version:   version,
			submodule: submodule,
		}
		packages := moduleSlice{module0}
		if bundleValue != "" {
			for _, dep := range strings.Split(bundleValue, ",") {
				n, v, s := parsePackageName(dep)
				if n == module0.name && s == module0.submodule {
					if v != "" && v != module0.version {
						module0.version = v
					}
					continue
				}
				if v == "" {
					info, err := nodeEnv.getPackageLatestInfo(n)
					if err != nil {
						return throwErrorJs(err)
					}
					v = info.Version
				}
				packages = append(packages, module{
					name:      n,
					version:   v,
					submodule: s,
				})
			}
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
			env:      env,
			target:   target,
		})
		if err != nil {
			return throwErrorJs(err)
		}

		importName := packageName
		if submodule != "" {
			importName = packageName + "/" + submodule
		}

		importMeta, ok := ret.importMeta[importName]
		if !ok {
			return throwErrorJs(fmt.Errorf("package '%s' not found in bundle", importName))
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
		identity := rename(importName)
		if submodule != "" {
			fmt.Fprintf(buf, `/* esm.sh - %s@%s/%s */%s`, packageName, version, submodule, EOL)
		} else {
			fmt.Fprintf(buf, `/* esm.sh - %s@%s */%s`, packageName, version, EOL)
		}
		if cdnDomain != "" {
			fmt.Fprintf(buf, `import { %s } from "https://%s/bundle-%s.js";%s`, identity, cdnDomain, ret.hash, EOL)
		} else {
			fmt.Fprintf(buf, `import { %s } from "/bundle-%s.js";%s`, identity, ret.hash, EOL)
		}
		if len(exports) > 0 {
			fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(exports, ","), identity, EOL)
		}
		if !hasDefaultExport {
			fmt.Fprintf(buf, `export default %s;%s`, identity, EOL)
		}
		return rex.Content(identity+".js", time.Now(), bytes.NewReader(buf.Bytes()))
	})
}

func parsePackageName(pathname string) (string, string, string) {
	a := strings.Split(strings.Trim(pathname, "/"), "/")
	scope := ""
	pkg := a[0]
	submodule := strings.Join(a[1:], "/")
	if strings.HasPrefix(a[0], "@") && len(a) > 1 {
		scope = a[0]
		pkg = a[1]
		submodule = strings.Join(a[2:], "/")
	}
	packageName, version := utils.SplitByLastByte(pkg, '@')
	if scope != "" {
		packageName = scope + "/" + packageName
	}
	return packageName, version, submodule
}

func throwErrorJs(err error) interface{} {
	message := fmt.Sprintf(`throw new Error("[esm.sh] " + %s);`, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))))
	return rex.Content("error.js", time.Now(), bytes.NewReader([]byte(message)))
}
