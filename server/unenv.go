package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

const (
	// https://github.com/unjs/unenv
	unenvVersion = "e666fcf"
)

var (
	unenvNodeRuntimeBulid = map[string][]byte{
		"sys.mjs": []byte(`export*from '/node/util.mjs';export{default}from '/node/util.mjs';`),
	}
)

func buildUnenvNodeRuntime() (err error) {
	wd := path.Join(config.WorkDir, "npm/gh/unjs/unenv@"+unenvVersion)
	err = ensureDir(wd)
	if err != nil {
		return err
	}

	packageJsonFp := path.Join(wd, "package.json")
	if !existsFile(packageJsonFp) {
		err = os.WriteFile(packageJsonFp, []byte(`{"dependencies":{"unenv":"github:unjs/unenv#`+unenvVersion+`"}}`), 0644)
		if err != nil {
			return
		}
	}

	cmd := exec.Command("pnpm", "i", "--prefer-offline")
	cmd.Dir = wd
	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("install unjs/unenv from github: %v", err)
		return
	}

	endpoints := make([]esbuild.EntryPoint, 0, len(nodeBuiltinModules))
	for name := range nodeBuiltinModules {
		// module "sys" is just a alias of "util", no need to build
		if name != "sys" {
			filename := path.Join(wd, "node_modules", "unenv/runtime/node/"+name+"/index.mjs")
			if existsFile(filename) {
				endpoints = append(endpoints, esbuild.EntryPoint{
					InputPath:  filename,
					OutputPath: name,
				})
			}
		}
	}

	ret := esbuild.Build(esbuild.BuildOptions{
		AbsWorkingDir:       wd,
		EntryPointsAdvanced: endpoints,
		Platform:            esbuild.PlatformBrowser,
		Format:              esbuild.FormatESModule,
		Target:              esbuild.ESNext,
		Bundle:              true,
		Splitting:           true,
		MinifyWhitespace:    true,
		MinifyIdentifiers:   true,
		MinifySyntax:        true,
		OutExtension:        map[string]string{".js": ".mjs"},
		Write:               false,
		Outdir:              "/",
		Plugins: []esbuild.Plugin{
			{
				Name: "unenv",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: `^node:`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						return esbuild.OnResolveResult{Path: "/node/" + args.Path[5:] + ".mjs", External: true}, nil
					})
				},
			},
		},
	})

	if len(ret.Errors) > 0 {
		err = errors.New(ret.Errors[0].Text)
		return
	}

	// bundle tiny chunks that are less than 1KB
	tinyChunks := make(map[string][]byte, 0)
	for _, result := range ret.OutputFiles {
		name := result.Path[1:]
		if strings.HasPrefix(name, "chunk-") && len(result.Contents) < 1024 {
			tinyChunks[name] = result.Contents
		} else {
			unenvNodeRuntimeBulid[name] = result.Contents
		}
	}
	for name, data := range unenvNodeRuntimeBulid {
		ret := esbuild.Build(esbuild.BuildOptions{
			Stdin: &esbuild.StdinOptions{
				Contents:   string(data),
				Loader:     esbuild.LoaderJS,
				Sourcefile: "/" + name,
			},
			Platform:          esbuild.PlatformBrowser,
			Format:            esbuild.FormatESModule,
			Target:            esbuild.ES2022,
			Bundle:            true,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			MinifySyntax:      true,
			Write:             false,
			Outdir:            "/",
			Plugins: []esbuild.Plugin{
				{
					Name: "unenv",
					Setup: func(build esbuild.PluginBuild) {
						build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
							if isRelPathSpecifier(args.Path) {
								fullpath := path.Join(path.Dir(args.Importer), args.Path)
								if strings.HasPrefix(fullpath, "/chunk-") {
									if chunk, ok := tinyChunks[fullpath[1:]]; ok {
										return esbuild.OnResolveResult{Path: fullpath, Namespace: "chunk", PluginData: chunk}, nil
									}
								}
							}
							return esbuild.OnResolveResult{External: true}, nil
						})
						build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "chunk"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
							code := string(args.PluginData.([]byte))
							return esbuild.OnLoadResult{Contents: &code}, nil
						})
					},
				},
			},
		})
		if len(ret.Errors) > 0 {
			err = errors.New(ret.Errors[0].Text)
			return
		}
		unenvNodeRuntimeBulid[name] = ret.OutputFiles[0].Contents
	}
	return
}
