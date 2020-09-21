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

func init() {
	rex.Query("*", func(ctx *rex.Context) interface{} {
		pathname := utils.CleanPath(ctx.R.URL.Path)
		if pathname == "/" {
			return rex.HTML(indexHTML, 200)
		}

		if strings.HasPrefix(pathname, "/bundle-") && strings.HasSuffix(pathname, ".js") {
			return rex.File(path.Join(etcDir, "builds", strings.TrimPrefix(pathname, "/bundle-")))
		}

		var bundle []string
		packageName, version := utils.SplitByLastByte(strings.Trim(pathname, "/"), '@')
		if version == "" {
			info, err := nodeEnv.getPackageLatestInfo(packageName)
			if err != nil {
				return throwErrorJs(err)
			}
			version = info.Version
		}
		bundleValue := ctx.Form.Value("bundle")
		if bundleValue != "" {
			for _, dep := range strings.Split(bundleValue, ",") {
				packageName, version := utils.SplitByLastByte(dep, '@')
				if version == "" {
					info, err := nodeEnv.getPackageLatestInfo(packageName)
					if err != nil {
						return throwErrorJs(err)
					}
					version = info.Version
				}
				bundle = append(bundle, packageName+"@"+version)
			}
		} else {
			bundle = []string{packageName + "@" + version}
		}
		env := strings.ToLower(ctx.Form.Value("env"))
		if env != "development" {
			env = "production"
		}
		target := strings.ToUpper(strings.TrimSpace(ctx.Form.Value("target")))
		if target == "" {
			target = "ESNEXT"
		}
		ret, err := build(buildOptions{
			bundle: bundle,
			env:    env,
			target: target,
		})
		if err != nil {
			return throwErrorJs(err)
		}

		importMeta, ok := ret.importMeta[packageName]
		if !ok {
			return throwErrorJs(fmt.Errorf("package '%s' not found in bundle", packageName))
		}

		hasDefaultExport := false
		for _, name := range importMeta.Exports {
			if name == "default" {
				hasDefaultExport = true
				break
			}
		}

		eof := "\n"
		buf := bytes.NewBuffer(nil)
		identity := rename(packageName)
		fmt.Fprintf(buf, `/* esm.sh - %s@%s */%s`, packageName, version, eof)
		if cdnDomain != "" {
			fmt.Fprintf(buf, `import { %s } from "https://%s/bundle-%s.js";%s`, identity, cdnDomain, ret.hash, eof)
		} else {
			fmt.Fprintf(buf, `import { %s } from "/bundle-%s.js";%s`, identity, ret.hash, eof)
		}
		fmt.Fprintf(buf, `export const { %s } = %s;%s`, strings.Join(importMeta.Exports, ","), identity, eof)
		if !hasDefaultExport {
			fmt.Fprintf(buf, `export default %s;%s`, identity, eof)
		}
		return rex.Content(packageName+".js", time.Now(), bytes.NewReader(buf.Bytes()))
	})
}

func throwErrorJs(err error) interface{} {
	message := fmt.Sprintf(`throw new Error("[esm.sh] " + %s);`, strings.TrimSpace(string(utils.MustEncodeJSON(err.Error()))))
	return rex.Content("error.js", time.Now(), bytes.NewReader([]byte(message)))
}
