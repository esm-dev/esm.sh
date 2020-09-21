package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/postui/postdb"
	"github.com/postui/postdb/q"
)

// ImportMeta defines import meta
type ImportMeta struct {
	Exports []string   `json:"exports"`
	Package NpmPackage `json:"package"`
}

// NpmPackage defines the package of npm
type NpmPackage struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Typings          string            `json:"typings"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

type module struct {
	name      string
	version   string
	submodule string
}

func (m module) String() string {
	s := m.name + "@" + m.version
	if m.submodule != "" {
		s = s + "/" + m.submodule
	}
	return s
}

type moduleSlice []module

func (a moduleSlice) Len() int           { return len(a) }
func (a moduleSlice) Less(i, j int) bool { return a[i].String() < a[j].String() }
func (a moduleSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (a moduleSlice) String() string {
	s := make([]string, a.Len())
	for i, m := range a {
		s[i] = m.String()
	}
	return strings.Join(s, ",")
}

type buildOptions struct {
	packages moduleSlice
	target   string
	env      string
}

type buildResult struct {
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

var lock sync.Mutex

func build(options buildOptions) (ret buildResult, err error) {
	lock.Lock()
	defer lock.Unlock()

	sort.Sort(options.packages)

	bundleID := "bundle-" + options.packages.String() + " " + options.target + " " + options.env
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
	start := time.Now()
	err = exec.Command("yarn", args...).Run()
	if err != nil {
		return
	}
	log.Debug("yarn", strings.Join(args, " "), "in", time.Now().Sub(start))

	var peerDependencies []string
	importMeta := map[string]ImportMeta{}
	for _, pkg := range options.packages {
		var p NpmPackage
		err = utils.ParseJSONFile(path.Join(tmpDir, "/node_modules/", pkg.name, "/package.json"), &p)
		if err != nil {
			return
		}
		importName := pkg.name
		if pkg.submodule != "" {
			importName = pkg.name + "/" + pkg.submodule
		}
		importMeta[importName] = ImportMeta{
			Package: p,
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
		start := time.Now()
		err = exec.Command("yarn", append([]string{"add"}, peerDependencies...)...).Run()
		if err != nil {
			return
		}
		log.Debug("yarn", "add", strings.Join(peerDependencies, " "), "in", time.Now().Sub(start))
	}

	codeBuf := bytes.NewBuffer(nil)
	codeBuf.WriteString("const meta = {};")
	codeBuf.WriteString("const isObject = v => typeof v === 'object' && v !== null;")
	for _, pkg := range options.packages {
		importName := pkg.name
		if pkg.submodule != "" {
			importName = pkg.name + "/" + pkg.submodule
		}
		importIdentifier := rename(importName)
		fmt.Fprintf(codeBuf, `const %s = require("%s");`, importIdentifier, importName)
		fmt.Fprintf(codeBuf, `meta["%s"] = {exports: isObject(%s) ? Object.keys(%s) : []};`, importName, importIdentifier, importIdentifier)
	}
	codeBuf.WriteString("process.stdout.write(JSON.stringify(meta));")
	err = ioutil.WriteFile(path.Join(tmpDir, "test.js"), codeBuf.Bytes(), 0644)
	if err != nil {
		return
	}
	cmd := exec.Command("node", "test.js")
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
				Package: v.Package,
				Exports: meta.Exports,
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
	start = time.Now()
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
					start := time.Now()
					err = exec.Command("yarn", "add", missingModule).Run()
					if err != nil {
						return
					}
					log.Debug("yarn", "add", missingModule, "in", time.Now().Sub(start))
					missingResolved[missingModule] = struct{}{}
					goto esbuild
				}
			}
		}
		err = errors.New("esbuild: " + fe.Text)
		return
	}

	log.Debug("esbuild", bundleID, "in", time.Now().Sub(start))

	hasher := sha1.New()
	hasher.Write(result.OutputFiles[0].Contents)
	hash := base32.StdEncoding.EncodeToString(hasher.Sum(nil))

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* esm.sh - esbuild bundle(%s) %s %s */%s`, strings.Join(args[1:], ","), strings.ToLower(options.target), options.env, EOL)
	jsContentBuf.Write(result.OutputFiles[0].Contents)
	err = ioutil.WriteFile(path.Join(etcDir, "builds", hash+".js"), jsContentBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	db.Put(
		q.Alias(bundleID),
		q.Tags("bundle"),
		q.KV{
			"hash":       []byte(hash),
			"importMeta": utils.MustEncodeJSON(importMeta),
		},
	)

	ret.hash = hash
	ret.importMeta = importMeta
	return
}

func rename(pkgName string) string {
	p := make([]byte, len([]byte(pkgName)))
	for i, c := range []byte(pkgName) {
		switch c {
		case '/', '-', '@', '.':
			p[i] = '_'
		default:
			p[i] = c
		}
	}
	return string(p)
}
