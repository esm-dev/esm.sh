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

func runLoader(npmrc *NpmRC, loaderName string, args []string, mainDependency Package, extraDeps ...string) (output *LoaderOutput, err error) {
	wd := path.Join(npmrc.StoreDir(), fmt.Sprintf("loader-v%d", VERSION), strings.ReplaceAll(strings.Join(append([]string{mainDependency.String()}, extraDeps...), "+"), "/", "_"))
	loaderJsFilename := path.Join(wd, "loader.mjs")
	if !existsFile(loaderJsFilename) {
		var loaderJS []byte
		loaderJS, err = embedFS.ReadFile(fmt.Sprintf("server/embed/internal/%s_loader.js", loaderName))
		if err != nil {
			err = fmt.Errorf("could not find loader: %s", loaderName)
			return
		}

		ensureDir(wd)

		stderr := bytes.NewBuffer(nil)
		cmd := exec.Command("npm", append([]string{"i", "--ignore-scripts", "--no-bin-links", mainDependency.String()}, extraDeps...)...)
		cmd.Dir = wd
		cmd.Stderr = stderr
		err = cmd.Run()
		if err != nil {
			if stderr.Len() > 0 {
				err = fmt.Errorf("could not install %s %s: %s", mainDependency.String(), strings.Join(extraDeps, " "), stderr.String())
			}
			return
		}

		err = os.WriteFile(loaderJsFilename, loaderJS, 0755)
		if err != nil {
			err = fmt.Errorf("could not write loader.js")
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

func transformVue(npmrc *NpmRC, vueVersion string, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "vue", args, Package{Name: "@vue/compiler-sfc", Version: vueVersion}, "@esm.sh/vue-loader@1.0.3")
}

func transformSvelte(npmrc *NpmRC, svelteVersion string, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "svelte", args, Package{Name: "svelte", Version: svelteVersion})
}

func generateUnoCSS(npmrc *NpmRC, args []string) (output *LoaderOutput, err error) {
	return runLoader(npmrc, "unocss", args, Package{Name: "@esm.sh/unocss", Version: "0.3.1"}, "@iconify/json@2.2.280")
}
