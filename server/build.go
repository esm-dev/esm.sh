package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/postui/postdb"

	"github.com/postui/postdb/q"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
)

// ImportMeta defines import meta
type ImportMeta struct {
	Exports     []string `json:"exports"`
	PackageInfo Package  `json:"packageInfo"`
}

// Package defines the package of npm
type Package struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Types            string            `json:"types"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

type module struct {
	name      string
	version   string
	submodule string
}

type buildOptions struct {
	packages []module
	env      string
	target   string
}

type buildResult struct {
	id         string
	hash       string
	importMeta map[string]ImportMeta
}

var targets = []string{
	"ESNext",
	"ES5",
	"ES2015",
	"ES2016",
	"ES2017",
	"ES2018",
	"ES2019",
	"ES2020",
}

func build(options buildOptions) (ret buildResult, err error) {
	buf := bytes.NewBufferString(options.target + "|" + options.env)
	for _, pkg := range options.packages {
		buf.WriteString("|" + pkg.name + "@" + pkg.version + "/" + pkg.submodule)
	}
	bundleID := base64.URLEncoding.EncodeToString(buf.Bytes())
	p, err := db.Get(q.Alias(bundleID), q.K("hash", "importMeta"))
	if err == nil {
		err = json.Unmarshal(p.KV.Get("importMeta"), &ret.importMeta)
		if err != nil {
			_, err = db.Delete(q.Alias(bundleID))
			if err != nil {
				return
			}
			err = postdb.ErrNotFound
		}

		hash := string(p.KV.Get("hash"))
		_, err = os.Stat(path.Join(etcDir, "builds", hash+".js"))
		if err == nil {
			ret.id = bundleID
			ret.hash = string(p.KV.Get("hash"))
			return
		}
		if os.IsExist(err) {
			return
		}

		_, err = db.Delete(q.Alias(bundleID))
		if err != nil {
			return
		}
		err = postdb.ErrNotFound
	}
	if err != nil && err != postdb.ErrNotFound {
		return
	}

	tmpDir := path.Join(os.TempDir(), bundleID)
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	err = os.Chdir(tmpDir)
	if err != nil {
		return
	}

	args := []string{"add"}
	for _, pkg := range options.packages {
		args = append(args, pkg.name+"@"+pkg.version)
	}
	log.Debug("yarn", strings.Join(args, " "))
	cmd := exec.Command("yarn", args...)
	err = cmd.Run()
	if err != nil {
		return
	}

	var peerDependencies []string
	importMeta := map[string]ImportMeta{}
	for _, pkg := range options.packages {
		var p Package
		err = utils.ParseJSONFile(path.Join(tmpDir, "/node_modules/", pkg.name, "/package.json"), &p)
		if err != nil {
			return
		}
		importName := pkg.name
		if pkg.submodule != "" {
			importName = pkg.name + "/" + pkg.submodule
		}
		importMeta[importName] = ImportMeta{
			PackageInfo: p,
		}
		if len(p.PeerDependencies) > 0 {
			for name := range p.PeerDependencies {
				install := true
				for _, pkg := range options.packages {
					if pkg.name == name {
						install = false
						break
					}
				}
				if install {
					peerDependencies = append(peerDependencies, name)
				}
			}
		}
	}
	if len(peerDependencies) > 0 {
		log.Debug("yarn", "add", strings.Join(peerDependencies, " "))
		cmd = exec.Command("yarn", append([]string{"add"}, peerDependencies...)...)
		err = cmd.Run()
		if err != nil {
			return
		}
	}

	codeBuf := bytes.NewBufferString("const meta = {};")
	for _, pkg := range options.packages {
		importName := pkg.name
		if pkg.submodule != "" {
			importName = pkg.name + "/" + pkg.submodule
		}
		fmt.Fprintf(codeBuf, `meta["%s"] = {exports: Object.keys(require("%s"))};`, importName, importName)
	}
	codeBuf.WriteString("process.stdout.write(JSON.stringify(meta));")
	err = ioutil.WriteFile(path.Join(tmpDir, "test.js"), codeBuf.Bytes(), 0644)
	if err != nil {
		return
	}
	cmd = exec.Command("node", "test.js")
	cmd.Env = append(os.Environ(), `NODE_ENV=`+options.env)
	testOutput, err := cmd.CombinedOutput()
	if err != nil {
		err = errors.New(string(testOutput))
		return
	}

	var m map[string]ImportMeta
	err = json.Unmarshal(testOutput, &m)
	if err != nil {
		return
	}
	for name, meta := range m {
		v, ok := importMeta[name]
		if ok {
			importMeta[name] = ImportMeta{
				PackageInfo: v.PackageInfo,
				Exports:     meta.Exports,
			}
		}
	}

	codeBuf = bytes.NewBuffer(nil)
	for _, pkg := range options.packages {
		importName := pkg.name
		if pkg.submodule != "" {
			importName = pkg.name + "/" + pkg.submodule
		}
		fmt.Fprintf(codeBuf, `export * as %s from "%s";`, rename(importName), importName)
	}

	err = ioutil.WriteFile(path.Join(tmpDir, "entry.js"), codeBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	isDev := options.env == "development"
	target := api.ESNext
	for i, t := range targets {
		if options.target == t {
			target = api.Target(i)
		}
	}
	missingResolved := map[string]struct{}{}
esbuild:
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{"entry.js"},
		Bundle:            true,
		Write:             false,
		Target:            target,
		Format:            api.FormatESModule,
		MinifyWhitespace:  !isDev,
		MinifyIdentifiers: !isDev,
		MinifySyntax:      !isDev,
		Defines:           map[string]string{"process.env.NODE_ENV": `"` + options.env + `"`},
	})
	if len(result.Errors) > 0 {
		fe := result.Errors[0]
		if strings.HasPrefix(fe.Text, "Could not resolve \"") {
			missingModule := strings.Split(fe.Text, "\"")[1]
			if missingModule != "" {
				_, ok := missingResolved[missingModule]
				if !ok {
					log.Debug("yarn", "add", missingModule)
					cmd := exec.Command("yarn", "add", missingModule)
					err = cmd.Run()
					if err != nil {
						return
					}
					missingResolved[missingModule] = struct{}{}
					goto esbuild
				}
			}
		}
		err = errors.New("esbuild: " + fe.Text)
		return
	}

	hasher := sha1.New()
	hasher.Write(result.OutputFiles[0].Contents)
	hash := hex.EncodeToString(hasher.Sum(nil))

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* esm.sh - esbuild bundle(%s) %s %s */%s`, strings.Join(args[1:], ","), strings.ToLower(options.target), options.env, "\n")
	jsContentBuf.Write(result.OutputFiles[0].Contents)
	err = ioutil.WriteFile(path.Join(etcDir, "builds", hash+".js"), jsContentBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	db.Put(q.Alias(bundleID), q.KV{"hash": []byte(hash), "importMeta": utils.MustEncodeJSON(importMeta)})

	ret.id = bundleID
	ret.hash = hash
	ret.importMeta = importMeta
	return
}

func rename(pkgName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(pkgName, "/", "_"), "-", "_"), "@", "_")
}
