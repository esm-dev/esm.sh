package server

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/esm-dev/esm.sh/internal/deno"
	esbuild "github.com/ije/esbuild-internal/api"
)

type LoaderOutput struct {
	Lang  string `json:"lang"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func runLoader(loaderJsPath string, filename string, code string) (out *LoaderOutput, err error) {
	denoPath := deno.ResolveDenoPath(config.WorkDir)
	err = doOnce("check-deno", func() (err error) {
		return deno.CheckDeno(denoPath)
	})
	if err != nil {
		return
	}

	cmd := exec.Command(
		denoPath,
		"run",
		"--no-config",
		"--no-lock",
		"--cached-only",
		"--allow-read=.",
		"--no-prompt",
		"--quiet",
		loaderJsPath,
		filename, // args[0]
	)
	cmd.Env = append(os.Environ(), "DENO_NO_UPDATE_CHECK=1", "DENO_NO_PACKAGE_JSON=1")
	cmd.Stdin = strings.NewReader(code)
	output, err := cmd.Output()
	if err != nil {
		return
	}
	if len(output) < 2 {
		err = errors.New("bad loader output")
		return
	}
	if output[0] != '1' && output[0] != '2' {
		err = errors.New(string(output[2:]))
		return
	}
	lang := "js"
	if output[0] == '2' {
		lang = "ts"
	}
	return &LoaderOutput{Lang: lang, Code: string(output[2:])}, nil
}

func buildLoader(wd, loaderJs, outfile string) (err error) {
	ret := esbuild.Build(esbuild.BuildOptions{
		Outfile:           outfile,
		Stdin:             &esbuild.StdinOptions{Contents: loaderJs, ResolveDir: wd},
		Platform:          esbuild.PlatformBrowser,
		Format:            esbuild.FormatESModule,
		Target:            esbuild.ESNext,
		Bundle:            true,
		MinifySyntax:      true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		Write:             true,
		PreserveSymlinks:  true,
		Plugins: []esbuild.Plugin{{
			Name: "resolver",
			Setup: func(build esbuild.PluginBuild) {
				build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if strings.HasPrefix(args.Path, "node:") || nodeBuiltinModules[args.Path] {
						return esbuild.OnResolveResult{Path: "node:" + strings.TrimPrefix(args.Path, "node:"), External: true}, nil
					}
					if strings.HasPrefix(args.Path, "jsr:") {
						return esbuild.OnResolveResult{Path: args.Path, External: true}, nil
					}
					return esbuild.OnResolveResult{}, nil
				})
			},
		}},
	})
	if len(ret.Errors) > 0 {
		err = errors.New(ret.Errors[0].Text)
	}
	return
}
