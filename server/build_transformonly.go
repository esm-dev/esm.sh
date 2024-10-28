package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

type TransformOptions struct {
	Filename        string          `json:"filename"`
	Lang            string          `json:"lang"`
	Code            string          `json:"code"`
	ImportMap       json.RawMessage `json:"importMap"`
	JsxImportSource string          `json:"jsxImportSource"`
	Target          string          `json:"target"`
	SourceMap       string          `json:"sourceMap"`
	Minify          bool            `json:"minify"`
}

type ResolvedTransformOptions struct {
	TransformOptions
	unocss        *UnoCSSTransformOptions
	importMap     ImportMap
	globalVersion string
}

type UnoCSSTransformOptions struct {
	input     []string
	configCSS string
}

type TransformOutput struct {
	Code string `json:"code"`
	Map  string `json:"map,omitempty"`
}

func transform(npmrc *NpmRC, options ResolvedTransformOptions) (out TransformOutput, err error) {
	target := esbuild.ESNext
	if options.Target != "" {
		if t, ok := targets[options.Target]; ok {
			target = t
		} else {
			err = errors.New("invalid target")
			return
		}
	}

	loader := esbuild.LoaderJS
	sourceCode := options.Code
	jsxImportSource := options.JsxImportSource

	if options.Lang == "" && options.Filename != "" {
		_, options.Lang = utils.SplitByLastByte(options.Filename, '.')
	}
	switch options.Lang {
	case "jsx":
		loader = esbuild.LoaderJSX
	case "ts":
		loader = esbuild.LoaderTS
	case "tsx":
		loader = esbuild.LoaderTSX
	case "css":
		if options.unocss != nil {
			// pre-process uno.css
			o, e := preLoad(npmrc, "unocss", strings.Join(options.unocss.input, "\n"), options.unocss.configCSS, PackageID{"esm-unocss", "0.8.0"}, "@iconify/json@2.2.260")
			if e != nil {
				log.Error("failed to generate uno.css:", e)
				err = errors.New("failed to generate uno.css")
				return
			}
			sourceCode = o.Code
		}
		loader = esbuild.LoaderCSS
	case "vue":
		var vueVersion string
		vueVersion, err = npmrc.getVueVersion(options.importMap)
		if err != nil {
			return
		}
		// pre-process Vue SFC
		o, e := preLoad(npmrc, "vue", options.Filename, sourceCode, PackageID{"@vue/compiler-sfc", vueVersion}, "esm-vue-sfc-compiler@0.1.0")
		if e != nil {
			log.Error("failed to transform vue:", e)
			err = errors.New("failed to transform vue")
			return
		}
		sourceCode = o.Code
	case "svelte":
		var svelteVersion string
		svelteVersion, err = npmrc.getSvelteVersion(options.importMap)
		if err != nil {
			return
		}
		// pre-process svelte component
		o, e := preLoad(npmrc, "svelte", options.Filename, sourceCode, PackageID{"svelte", svelteVersion})
		if e != nil {
			log.Error("failed to transform svelte:", e)
			err = errors.New("failed to transform svelte")
			return
		}
		sourceCode = o.Code
	}

	if jsxImportSource == "" && (loader == esbuild.LoaderJSX || loader == esbuild.LoaderTSX) {
		var ok bool
		for _, key := range []string{"@jsxRuntime", "@jsxImportSource", "preact", "react"} {
			jsxImportSource, ok = options.importMap.Resolve(key)
			if ok {
				break
			}
		}
		if !ok {
			jsxImportSource = "react"
		}
	}

	sourceMap := esbuild.SourceMapNone
	if options.SourceMap == "external" {
		sourceMap = esbuild.SourceMapExternal
	} else if options.SourceMap == "inline" {
		sourceMap = esbuild.SourceMapInline
	}

	filename := options.Filename
	if filename == "" {
		filename = "source." + options.Lang
	}
	stdin := &esbuild.StdinOptions{
		Sourcefile: filename,
		Contents:   sourceCode,
		Loader:     loader,
	}
	opts := esbuild.BuildOptions{
		Stdin:             stdin,
		Platform:          esbuild.PlatformBrowser,
		Format:            esbuild.FormatESModule,
		Target:            target,
		JSX:               esbuild.JSXAutomatic,
		JSXImportSource:   strings.TrimSuffix(jsxImportSource, "/"),
		MinifyWhitespace:  options.Minify,
		MinifySyntax:      options.Minify,
		MinifyIdentifiers: options.Minify,
		Sourcemap:         sourceMap,
		Bundle:            true,
		Outdir:            "/esbuild",
		Write:             false,
		Plugins: []esbuild.Plugin{
			{
				Name: "resolver",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						path, _ := options.importMap.Resolve(args.Path)
						if strings.HasSuffix(path, ".css") {
							path += "?module"
						}
						if isRelativeSpecifier(path) {
							if options.importMap.Src != "" {
								suffix := "N"
								if options.importMap.Support {
									suffix = "y"
								}
								path = appendQueryString(path, "im", suffix+btoaUrl(options.importMap.Src))
							}
							if options.globalVersion != "" {
								path = appendQueryString(path, "v", options.globalVersion)
							}
						}
						return esbuild.OnResolveResult{
							Path:     path,
							External: true,
						}, nil
					})
				},
			},
		},
	}
	ret := esbuild.Build(opts)
	if len(ret.Errors) > 0 {
		err = errors.New("failed to validate code: " + ret.Errors[0].Text)
		return
	}
	if len(ret.OutputFiles) == 0 {
		err = errors.New("failed to validate code: no output files")
		return
	}
	for _, file := range ret.OutputFiles {
		if strings.HasSuffix(file.Path, ".js") || strings.HasSuffix(file.Path, ".css") {
			out.Code = string(file.Contents)
		} else if strings.HasSuffix(file.Path, ".map") {
			out.Map = string(file.Contents)
		}
	}
	return
}

func preLoad(npmrc *NpmRC, loaderName string, specifier string, sourceCode string, mainDependency PackageID, extraDeps ...string) (output *TransformOutput, err error) {
	wd := path.Join(npmrc.StoreDir(), mainDependency.String())
	loaderJsFilename := path.Join(wd, "loader.mjs")
	if !existsFile(loaderJsFilename) {
		var loaderJS []byte
		loaderJS, err = embedFS.ReadFile(fmt.Sprintf("server/embed/internal/%s_loader.js", loaderName))
		if err != nil {
			err = fmt.Errorf("could not find loader: %s", loaderName)
			return
		}
		ensureDir(wd)
		err = os.WriteFile(loaderJsFilename, loaderJS, 0644)
		if err != nil {
			err = fmt.Errorf("could not write loader.js")
			return
		}
		err = npmrc.pnpmi(wd, append([]string{"--prefer-offline", mainDependency.String()}, extraDeps...)...)
		if err != nil {
			err = errors.New("failed to install " + mainDependency.String() + " " + strings.Join(extraDeps, " "))
			return
		}
	}

	stdin := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	err = json.NewEncoder(stdin).Encode([]string{specifier, sourceCode})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", "loader.mjs")
	cmd.Dir = wd
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("preLoad: %s", stderr.String())
		}
		return
	}

	var ret TransformOutput
	err = json.NewDecoder(stdout).Decode(&ret)
	if err != nil {
		return
	}

	output = &ret
	return
}
