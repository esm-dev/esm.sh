package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	esbuild "github.com/ije/esbuild-internal/api"
)

// each loader process boots a deno runtime with a bundled compiler, so cap them to avoid exhausting memory
var loaderSema = make(chan struct{}, runtime.NumCPU())

type LoaderOutput struct {
	Lang string
	Code string
}

func runLoaderContext(ctx context.Context, loaderJsPath string, filename string, code string) (out *LoaderOutput, err error) {
	denoPath := resolveDenoPath(config.WorkDir)
	err = doOnce("check-deno", func() (err error) {
		return checkDenoPath(denoPath)
	})
	if err != nil {
		return
	}

	select {
	case loaderSema <- struct{}{}:
		defer func() { <-loaderSema }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	cancelCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cancelCtx,
		denoPath,
		"run",
		"--no-config",
		"--no-lock",
		"--cached-only",
		"--no-prompt",
		"--quiet",
		loaderJsPath,
		filename, // args[0]
	)
	cmd.Dir = filepath.Dir(loaderJsPath)
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
		return errors.New(ret.Errors[0].Text)
	}
	if err = ensureDir(filepath.Dir(outfile)); err != nil {
		return
	}
	// write to a temp file first, callers treat an existing loader file as complete
	tmpfile := outfile + ".tmp"
	if err = os.WriteFile(tmpfile, ret.OutputFiles[0].Contents, 0644); err != nil {
		return
	}
	return os.Rename(tmpfile, outfile)
}
