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
	Exports     []string               `json:"exports"`
	PackageInfo map[string]interface{} `json:"packageInfo"`
}

type buildOptions struct {
	bundle []string
	env    string
	target string
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
	bundleID := base64.URLEncoding.EncodeToString([]byte(strings.Join(options.bundle, "+") + "|" + options.target + "|" + options.env))
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

	log.Debug("yarn", "add", strings.Join(options.bundle, " "))
	cmd := exec.Command("yarn", append([]string{"add"}, options.bundle...)...)
	err = cmd.Run()
	if err != nil {
		return
	}

	codeBuf := bytes.NewBufferString("const meta = {};")
	for _, pkg := range options.bundle {
		pgkName, _ := utils.SplitByLastByte(pkg, '@')
		fmt.Fprintf(codeBuf, `meta["%s"] = {exports: Object.keys(require("%s")), packageInfo: require("./node_modules/%s/package.json")};`, pgkName, pgkName, pgkName)
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

	var importMeta map[string]ImportMeta
	err = json.Unmarshal(testOutput, &importMeta)
	if err != nil {
		return
	}
	for name, meta := range importMeta {
		log.Debug(name, meta.Exports)
	}

	codeBuf = bytes.NewBuffer(nil)
	for _, pkg := range options.bundle {
		pgkName, _ := utils.SplitByLastByte(pkg, '@')
		fmt.Fprintf(codeBuf, `export * as %s from "%s";`, rename(pgkName), pgkName)
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
	fmt.Fprintf(jsContentBuf, `/* esm.sh - esbuild bundle(%s) %s %s */%s`, strings.Join(options.bundle, ","), strings.ToLower(options.target), options.env, "\n")
	jsContentBuf.Write(result.OutputFiles[0].Contents)
	err = ioutil.WriteFile(path.Join(etcDir, "builds", hash+".js"), jsContentBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	db.Put(q.Alias(bundleID), q.KV{"hash": []byte(hash), "importMeta": testOutput})

	ret.id = bundleID
	ret.hash = hash
	ret.importMeta = importMeta
	return
}

func rename(pgkName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(pgkName, "-", "_"), "@", "_")
}
