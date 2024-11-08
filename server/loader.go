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
)

type LoaderOutput struct {
	Lang  string `json:"lang"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

func runLoader(npmrc *NpmRC, loaderName string, args []string, mainDependency PackageId, extraDeps ...string) (output *LoaderOutput, err error) {
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
	err = json.NewEncoder(stdin).Encode(args)
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

	var out LoaderOutput
	err = json.NewDecoder(stdout).Decode(&out)
	if err != nil {
		return
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	return &out, nil
}

func transformVue(npmrc *NpmRC, options *ResolvedTransformOptions) (output *LoaderOutput, err error) {
	var vueVersion string
	vueVersion, err = npmrc.getVueVersion(options.importMap)
	if err != nil {
		return
	}
	return runLoader(npmrc, "vue", []string{options.Filename, options.Code}, PackageId{"@vue/compiler-sfc", vueVersion}, "@esm.sh/vue-loader@1.0.3")
}

func transformSvelte(npmrc *NpmRC, options *ResolvedTransformOptions) (output *LoaderOutput, err error) {
	var svelteVersion string
	svelteVersion, err = npmrc.getSvelteVersion(options.importMap)
	if err != nil {
		return
	}
	return runLoader(npmrc, "svelte", []string{options.Filename, options.Code}, PackageId{"svelte", svelteVersion})
}

func generateUnoCSS(npmrc *NpmRC, options *ResolvedTransformOptions) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "unocss", []string{options.unocss.configCSS, strings.Join(options.unocss.content, "\n")}, PackageId{"@esm.sh/unocss", "0.2.1"}, "@iconify/json@2.2.269")
}
