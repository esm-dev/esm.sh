package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/esm-dev/esm.sh/internal/npm"
	esbuild "github.com/ije/esbuild-internal/api"
)

var (
	// https://github.com/unjs/unenv
	unenvPkg = npm.Package{
		Name:    "unenv",
		Version: "2.0.0-rc.22",
	}
	unenvNodeRuntimeMap = map[string][]byte{
		"sys.mjs": []byte(`export*from "/node/util.mjs";export{default}from "/node/util.mjs";`),
	}
)

// getNodeRuntimeJS returns the unenv node runtime by the given name.
func getNodeRuntimeJS(name string) (js []byte, ok bool) {
	doOnce("load-node-runtime", func() (err error) {
		return loadNodeRuntime()
	})
	js, ok = unenvNodeRuntimeMap[name]
	return
}

// loadNodeRuntime loads the unenv node runtime from the embed filesystem.
func loadNodeRuntime() (err error) {
	data, err := embedFS.ReadFile("embed/node-runtime.tgz")
	if err == nil {
		tarball, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			defer tarball.Close()
			tr := tar.NewReader(tarball)
			for {
				header, err := tr.Next()
				if err != nil {
					break
				}
				if header.Typeflag == tar.TypeReg {
					name := header.Name
					data := make([]byte, header.Size)
					n, err := io.ReadFull(tr, data)
					if err == nil && int64(n) == header.Size {
						unenvNodeRuntimeMap[name] = data
					}
				}
			}
			return nil
		}
	}
	return buildUnenvNodeRuntime()
}

// slow path
func buildUnenvNodeRuntime() (err error) {
	wd := path.Join(config.WorkDir, "npm/"+unenvPkg.String())
	err = ensureDir(wd)
	if err != nil {
		return err
	}

	npmrc := &NpmRC{globalRegistry: &NpmRegistry{NpmRegistryConfig: NpmRegistryConfig{Registry: npmRegistry}}}
	pkgJson, err := npmrc.installPackage(unenvPkg)
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, pkgJson, false, nil)

	endpoints := make([]esbuild.EntryPoint, 0, len(nodeBuiltinModules))
	for name := range nodeBuiltinModules {
		// currently the module "sys" is just a alias of "util", no need to build it
		if name != "sys" {
			filename := path.Join(wd, "node_modules", unenvPkg.Name+"/dist/runtime/node/"+name+".mjs")
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
				Name: "resolve-node-builtin-modules",
				Setup: func(build esbuild.PluginBuild) {
					// https://github.com/unjs/unenv/issues/365
					build.OnResolve(esbuild.OnResolveOptions{Filter: `^unenv/dist/runtime/node/stream$`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						return esbuild.OnResolveResult{Path: "/node/stream.mjs", External: true}, nil
					})
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

	// bundle tiny chunks that are less than 600 bytes
	tinyChunks := make(map[string][]byte, 0)
	for _, result := range ret.OutputFiles {
		name := result.Path[1:]
		if strings.HasPrefix(name, "chunk-") && len(result.Contents) < 600 {
			tinyChunks[name] = result.Contents
		} else {
			unenvNodeRuntimeMap[name] = result.Contents
		}
	}

	// write the tarball to 'server/embed/' in DEBUG mode
	var tarball *tar.Writer
	if DEBUG {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		file, err := os.OpenFile(path.Join(wd, "server/embed/node-runtime.tgz"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer file.Close()

		gzipWriter := gzip.NewWriter(file)
		defer gzipWriter.Close()

		tarball = tar.NewWriter(gzipWriter)
		defer tarball.Close()
	}

	now := time.Now()
	for name, data := range unenvNodeRuntimeMap {
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
					Name: "bundle-tiny-chunks",
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
		js := ret.OutputFiles[0].Contents
		if tarball != nil {
			tarball.WriteHeader(&tar.Header{
				Name:    name,
				Size:    int64(len(js)),
				Mode:    0644,
				ModTime: now,
			})
			tarball.Write(js)
		}
		unenvNodeRuntimeMap[name] = js
	}
	return
}
