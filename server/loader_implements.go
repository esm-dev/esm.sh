package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
)

func transformSvelte(npmrc *NpmRC, svelteVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderExecPath := path.Join(npmrc.StoreDir(), "svelte@"+svelteVersion, "loader.js")
	if !existsFile(loaderExecPath) {
		log.Debug("compiling svelte loader...")
		err = compileSvelteLoader(npmrc, svelteVersion, loaderExecPath)
		if err != nil {
			return
		}
	}
	return runLoader(loaderExecPath, filename, code)
}

func compileSvelteLoader(npmrc *NpmRC, svelteVersion string, loaderExecPath string) (err error) {
	wd := path.Join(npmrc.StoreDir(), "svelte@"+svelteVersion)

	v, _ := loaderCompileLocks.LoadOrStore(wd, &sync.Mutex{})
	defer loaderCompileLocks.Delete(wd)

	// only one compile process is allowed at the same time
	v.(*sync.Mutex).Lock()
	defer v.(*sync.Mutex).Unlock()

	// check if the loader has been compiled
	if existsFile(loaderExecPath) {
		return
	}

	// install svelte
	pkgJson, err := npmrc.installPackage(Package{Name: "svelte", Version: svelteVersion})
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, pkgJson, false, nil)

	loaderJs := `
		import { compile } from "svelte/compiler";
		const { stdin, stdout } = Deno;
		const write = data => stdout.write(new TextEncoder().encode(data));
		try {
			let sourceCode = "";
			for await (const text of stdin.readable.pipeThrough(new TextDecoderStream())) {
			  sourceCode += text;
			}
			const { js } = compile(sourceCode, { filename: Deno.args[0], css: "injected" });
			await write("1\n" + js.code);
		} catch (err) {
			await write("0\n" + err.message);
		}
	`
	err = buildLoader(wd, loaderJs, loaderExecPath)
	return
}

func generateUnoCSS(npmrc *NpmRC, configCSS string, content string) (output *LoaderOutput, err error) {
	loaderVersion := "0.4.1"
	loaderExecPath := path.Join(config.WorkDir, "bin", "unocss-loader-"+loaderVersion)
	if !existsFile(loaderExecPath) {
		log.Debug("compiling unocss loader...")
		err = compileUnocssLoader(npmrc, loaderVersion, loaderExecPath)
		if err != nil {
			return
		}
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	c := exec.Command(loaderExecPath, strconv.Itoa(len(configCSS)), path.Join(config.WorkDir, "cache/unocss"))
	c.Dir = os.TempDir()
	c.Stdin = strings.NewReader(configCSS + content)
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err = c.Run()
	if err != nil {
		if errBuf.Len() > 0 {
			err = errors.New(errBuf.String())
		}
		return
	}
	if outBuf.Len() < 2 {
		err = errors.New("bad loader output")
		return
	}
	data := outBuf.Bytes()
	if data[0] != '1' {
		err = errors.New(string(data[2:]))
		return
	}
	return &LoaderOutput{Lang: "css", Code: string(data[2:])}, nil
}

func compileUnocssLoader(npmrc *NpmRC, loaderVersion string, loaderExecPath string) (err error) {
	wd := path.Join(npmrc.StoreDir(), "@esm.sh/unocss@"+loaderVersion)

	v, _ := loaderCompileLocks.LoadOrStore(wd, &sync.Mutex{})
	defer loaderCompileLocks.Delete(wd)

	// only one compile process is allowed at the same time
	v.(*sync.Mutex).Lock()
	defer v.(*sync.Mutex).Unlock()

	// check if the loader has been compiled
	if existsFile(loaderExecPath) {
		return
	}

	// install @esm.sh/unocss
	pkgJson, err := npmrc.installPackage(Package{Name: "@esm.sh/unocss", Version: loaderVersion})
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, pkgJson, false, nil)

	loaderJs := `
		import { generate } from "@esm.sh/unocss";
		const { stdin, stdout } = Deno;
		const write = data => stdout.write(new TextEncoder().encode(data));
		const customImport = (name) => {
		  switch (name) {
				case "@unocss/preset-attributify":
					return import("@unocss/preset-attributify");
				case "@unocss/preset-icons/browser":
					return import("@unocss/preset-icons/browser");
				case "@unocss/preset-legacy-compat":
					return import("@unocss/preset-legacy-compat");
				case "@unocss/preset-mini":
					return import("@unocss/preset-mini");
				case "@unocss/preset-rem-to-px":
					return import("@unocss/preset-rem-to-px");
				case "@unocss/preset-tagify":
					return import("@unocss/preset-tagify");
				case "@unocss/preset-typography":
					return import("@unocss/preset-typography");
				case "@unocss/preset-uno":
					return import("@unocss/preset-uno");
				case "@unocss/preset-web-fonts":
					return import("@unocss/preset-web-fonts");
				case "@unocss/preset-wind":
					return import("@unocss/preset-wind");
				default:
					if (name.startsWith("https://esm.sh/@iconify-json/")) {
						const [_, cName, ...path] = name.slice(15).split("/")
						return (async () => {
						  const { UntarStream } = await import("jsr:@std/tar/untar-stream");
							const jsonRes = await fetch("https://registry.npmjs.org/@iconify-json/" + cName + "/latest")
							if (jsonRes.status !== 200) {
								jsonRes.body.cancel()
								throw new Error("Failed to fetch @iconify-json/" + cName)
							}
							const { dist } = await jsonRes.json()
							const tgzRes = await fetch(dist.tarball)
							if (tgzRes.status !== 200) {
								tgzRes.body.cancel()
								throw new Error("Failed to fetch tarball of @iconify-json/" + cName)
							}
							for await (
								const entry of (tgzRes.body
									.pipeThrough(new DecompressionStream("gzip"))
									.pipeThrough(new UntarStream())
							)) {
								if (entry.path === "package/" + path.join("/")) {
									return await new Response(entry.readable).json()
								} else {
								  entry.readable.cancel()
								}
							}
							throw new Error("Failed to find " + path.join("/") + " in " + dist.tarball)
						})()
					}
					throw new Error("Unsupported import: " + name)
			}
		}
		try {
			let content = "";
			for await (const text of stdin.readable.pipeThrough(new TextDecoderStream())) {
			  content += text;
			}
			let configCSS = undefined;
			const n = Number(Deno.args[0]);
			if (n > 0) {
				configCSS = content.slice(0, n);
				content = content.slice(n);
			}
      const code = await generate(content, { configCSS, customImport, customCacheDir: Deno.args[1] });
			await write("1\n" + code);
		} catch (err) {
			await write("0\n" + err.message);
		}
	`
	err = buildLoader(wd, loaderJs, path.Join(wd, "loader.js"))
	if err != nil {
		return
	}

	_, err = run(
		"deno", "compile",
		"--no-config",
		"--no-lock",
		"--include=jsr:@std/tar/untar-stream",
		"--no-prompt",
		"--allow-read="+config.WorkDir+"/cache",
		"--allow-write="+config.WorkDir+"/cache",
		"--allow-net=registry.npmjs.org,fonts.googleapis.com",
		"--quiet",
		"--output", loaderExecPath,
		path.Join(wd, "loader.js"),
	)
	if err != nil {
		err = fmt.Errorf("failed to compile %s: %s", path.Base(loaderExecPath), err.Error())
	}
	return
}

func transformVue(npmrc *NpmRC, vueVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderVersion := "1.0.1" // @esm.sh/vue-compiler
	loaderExecPath := path.Join(npmrc.StoreDir(), "@vue/compiler-sfc@"+vueVersion, "loader-"+loaderVersion+".js")
	if !existsFile(loaderExecPath) {
		log.Debug("compiling vue loader...")
		err = compileVueLoader(npmrc, vueVersion, loaderVersion, loaderExecPath)
		if err != nil {
			return
		}
	}
	return runLoader(loaderExecPath, filename, code)
}

func compileVueLoader(npmrc *NpmRC, vueVersion string, loaderVersion, loaderExecPath string) (err error) {
	wd := path.Join(npmrc.StoreDir(), "@vue/compiler-sfc@"+vueVersion)

	v, _ := loaderCompileLocks.LoadOrStore(wd, &sync.Mutex{})
	defer loaderCompileLocks.Delete(wd)

	// only one compile process is allowed at the same time
	v.(*sync.Mutex).Lock()
	defer v.(*sync.Mutex).Unlock()

	// check if the loader has been compiled
	if existsFile(loaderExecPath) {
		return
	}

	// install vue sfc compiler
	pkgJson, err := npmrc.installPackage(Package{Name: "@vue/compiler-sfc", Version: vueVersion})
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, pkgJson, false, nil)
	npmrc.installDependencies(wd, &PackageJSON{Dependencies: map[string]string{"@esm.sh/vue-compiler": loaderVersion}}, false, nil)

	loaderJs := `
	  import * as vueCompilerSFC from "@vue/compiler-sfc";
		import { transform } from "@esm.sh/vue-compiler";
		const { stdin, stdout } = Deno;
		const write = data => stdout.write(new TextEncoder().encode(data));
		try {
			let sourceCode = "";
			for await (const text of stdin.readable.pipeThrough(new TextDecoderStream())) {
			  sourceCode += text;
			}
      const { lang, code } = await transform(Deno.args[0], sourceCode, { imports: { "@vue/compiler-sfc": vueCompilerSFC } });
			await write((lang === "ts" ? '2' : '1') + '\n' + code);
		} catch (err) {
			await write("0\n" + err.message);
		}
	`
	err = buildLoader(wd, loaderJs, loaderExecPath)
	return
}
