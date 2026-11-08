package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/esm-dev/esm.sh/internal/npm"
	"github.com/ije/gox/term"
)

func generatedLoaderPath(npmrc *NpmRC, name string) string {
	return path.Join(npmrc.StoreDir(), ".loaders", name)
}

func transformWasm(ctx context.Context, npmrc *NpmRC, filename string) (*LoaderOutput, error) {
	wasm, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	loaderExecPath := generatedLoaderPath(npmrc, "wasm-1.js")
	err = doOnce(loaderExecPath, func() error {
		if existsFile(loaderExecPath) {
			return nil
		}
		return buildLoader(npmrc.StoreDir(), `
		  const { stdin, stdout } = Deno;
		  const write = async data => {
		    for (let buf = new TextEncoder().encode(data); buf.length > 0;) buf = buf.subarray(await stdout.write(buf));
		  };
		  try {
		    let base64 = "";
		    for await (const text of stdin.readable.pipeThrough(new TextDecoderStream())) base64 += text;
		    const binary = "Uint8Array.from(atob(" + JSON.stringify(base64) + "), c => c.charCodeAt(0))";
		    // Only compile here; the module's start function must run in the client.
		    const module = await WebAssembly.compile(Uint8Array.from(atob(base64), c => c.charCodeAt(0)));
		    const imports = [...new Set(WebAssembly.Module.imports(module).map(i => i.module))];
		    const lines = imports.map((name, i) => "import * as m" + i + " from " + JSON.stringify(name) + ";");
		    const importObject = imports.map((name, i) => "[" + JSON.stringify(name) + "]: m" + i).join(",");
		    lines.push("const { instance } = await WebAssembly.instantiate(" + binary + ", {" + importObject + "});");
		    WebAssembly.Module.exports(module).forEach(({ name }, i) => {
		      lines.push("const e" + i + " = instance.exports[" + JSON.stringify(name) + "]; export { e" + i + " as " + JSON.stringify(name) + " };");
		    });
		    await write("1\n" + lines.join("\n"));
		  } catch (err) {
		    await write("0\n" + err.message);
		  }
		`, loaderExecPath)
	})
	if err != nil {
		return nil, err
	}
	return runLoaderContext(ctx, loaderExecPath, filename, base64.StdEncoding.EncodeToString(wasm))
}

func transformSvelte(ctx context.Context, npmrc *NpmRC, svelteVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderExecPath := generatedLoaderPath(npmrc, "svelte@"+svelteVersion+".js")

	err = doOnce(loaderExecPath, func() (err error) {
		if !existsFile(loaderExecPath) {
			if DEBUG {
				fmt.Println(term.Dim("Compiling svelte loader..."))
			}
			err = compileSvelteLoader(ctx, npmrc, svelteVersion, loaderExecPath)
		}
		return
	})
	if err != nil {
		err = errors.New("failed to compile svelte loader: " + err.Error())
		return
	}

	return runLoaderContext(ctx, loaderExecPath, filename, code)
}

func compileSvelteLoader(ctx context.Context, npmrc *NpmRC, svelteVersion string, loaderExecPath string) (err error) {
	ctx, release := withNpmPackageScope(ctx)
	defer release()
	wd := path.Join(npmrc.StoreDir(), "svelte@"+svelteVersion)

	// install svelte and its dependencies
	p, err := npmrc.installPackageContext(ctx, npm.Package{Name: "svelte", Version: svelteVersion})
	if err != nil {
		return
	}
	err = npmrc.installDependenciesContext(ctx, wd, p, false, nil)
	if err != nil {
		return
	}

	loaderJS := `
	  import { compile } from "svelte/compiler";
	  const { stdin, stdout } = Deno;
	  const write = async data => {
	    // [warning] stdout.write may write only a part of the buffer
	    for (let buf = new TextEncoder().encode(data); buf.length > 0;) buf = buf.subarray(await stdout.write(buf));
	  };
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

func transformVue(ctx context.Context, npmrc *NpmRC, vueVersion string, filename string, code string) (output *LoaderOutput, err error) {
	loaderVersion := "1.0.1" // @esm.sh/vue-compiler
	loaderExecPath := generatedLoaderPath(npmrc, "vue@"+vueVersion+"-"+loaderVersion+".js")

	err = doOnce(loaderExecPath, func() (err error) {
		if !existsFile(loaderExecPath) {
			if DEBUG {
				fmt.Println(term.Dim("Compiling vue loader..."))
			}
			err = compileVueLoader(ctx, npmrc, vueVersion, loaderVersion, loaderExecPath)
		}
		return
	})
	if err != nil {
		err = errors.New("failed to compile vue loader: " + err.Error())
		return
	}

	return runLoaderContext(ctx, loaderExecPath, filename, code)
}

func compileVueLoader(ctx context.Context, npmrc *NpmRC, vueVersion string, loaderVersion, loaderExecPath string) (err error) {
	ctx, release := withNpmPackageScope(ctx)
	defer release()
	wd := path.Join(npmrc.StoreDir(), "@vue/compiler-sfc@"+vueVersion)

	// install vue sfc compiler
	pkgJson, err := npmrc.installPackageContext(ctx, npm.Package{Name: "@vue/compiler-sfc", Version: vueVersion})
	if err != nil {
		return
	}
	err = npmrc.installDependenciesContext(ctx, wd, pkgJson, false, nil)
	if err != nil {
		return
	}
	err = npmrc.installDependenciesContext(ctx, wd, &npm.PackageJSON{Dependencies: map[string]string{"@esm.sh/vue-compiler": loaderVersion}}, false, nil)
	if err != nil {
		return
	}

	loaderJS := `
	  import * as vueCompilerSFC from "@vue/compiler-sfc";
	  import { transform } from "@esm.sh/vue-compiler";
	  const { stdin, stdout } = Deno;
	  const write = async data => {
	    // [warning] stdout.write may write only a part of the buffer
	    for (let buf = new TextEncoder().encode(data); buf.length > 0;) buf = buf.subarray(await stdout.write(buf));
	  };
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
