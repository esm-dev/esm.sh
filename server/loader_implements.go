package server

import (
	"errors"
	"fmt"
	"path"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
)

func transformSvelte(npmrc *NpmRC, svelteVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderExecPath := path.Join(npmrc.StoreDir(), "svelte@"+svelteVersion, "loader.js")

	err = doOnce(loaderExecPath, func() (err error) {
		if !existsFile(loaderExecPath) {
			if DEBUG {
				fmt.Println(term.Dim("Compiling svelte loader..."))
			}
			err = compileSvelteLoader(npmrc, svelteVersion, loaderExecPath)
		}
		return
	})
	if err != nil {
		err = errors.New("failed to compile svelte loader: " + err.Error())
		return
	}

	return runLoader(loaderExecPath, filename, code)
}

func compileSvelteLoader(npmrc *NpmRC, svelteVersion string, loaderExecPath string) (err error) {
	wd := path.Join(npmrc.StoreDir(), "svelte@"+svelteVersion)

	// install svelte and its dependencies
	p, err := npmrc.installPackage(npm.Package{Name: "svelte", Version: svelteVersion})
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, p, false, nil)

	loaderJS := `
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
	err = buildLoader(wd, loaderJS, loaderExecPath)
	return
}

func transformVue(npmrc *NpmRC, vueVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderVersion := "1.0.1" // @esm.sh/vue-compiler
	loaderExecPath := path.Join(npmrc.StoreDir(), "@vue/compiler-sfc@"+vueVersion, "loader-"+loaderVersion+".js")

	err = doOnce(loaderExecPath, func() (err error) {
		if !existsFile(loaderExecPath) {
			if DEBUG {
				fmt.Println(term.Dim("Compiling vue loader..."))
			}
			err = compileVueLoader(npmrc, vueVersion, loaderVersion, loaderExecPath)
		}
		return
	})
	if err != nil {
		err = errors.New("failed to compile vue loader: " + err.Error())
		return
	}

	return runLoader(loaderExecPath, filename, code)
}

func compileVueLoader(npmrc *NpmRC, vueVersion string, loaderVersion, loaderExecPath string) (err error) {
	wd := path.Join(npmrc.StoreDir(), "@vue/compiler-sfc@"+vueVersion)

	// install vue sfc compiler
	pkgJson, err := npmrc.installPackage(npm.Package{Name: "@vue/compiler-sfc", Version: vueVersion})
	if err != nil {
		return
	}
	npmrc.installDependencies(wd, pkgJson, false, nil)
	npmrc.installDependencies(wd, &npm.PackageJSON{Dependencies: map[string]string{"@esm.sh/vue-compiler": loaderVersion}}, false, nil)

	loaderJS := `
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
	err = buildLoader(wd, loaderJS, loaderExecPath)
	return
}
