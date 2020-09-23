package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
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

var buildLock sync.Mutex

// ImportMeta defines import meta
type ImportMeta struct {
	Exports []string `json:"exports"`
	NpmPackage
}

// NpmPackage defines the package of npm
type NpmPackage struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Types            string            `json:"types,omitempty"`
	Typings          string            `json:"typings,omitempty"`
	Dependencies     map[string]string `json:"dependencies,omitempty"`
	PeerDependencies map[string]string `json:"peerDependencies,omitempty"`
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

func build(options buildOptions) (ret buildResult, err error) {
	buildLock.Lock()
	defer buildLock.Unlock()

	sort.Sort(options.packages)
	bundleRaw := fmt.Sprintf("bundle %s %s %s", options.packages.String(), options.target, options.env)
	bundleID := base64.URLEncoding.EncodeToString([]byte(bundleRaw))
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

	installList := []string{}
	for _, pkg := range options.packages {
		installList = append(installList, pkg.name+"@"+pkg.version)
	}

	start := time.Now()
	importMeta := map[string]ImportMeta{}
	for _, pkg := range options.packages {
		var p NpmPackage
		p, err = nodeEnv.getPackageInfo(pkg.name, pkg.version)
		if err != nil {
			return
		}
		meta := ImportMeta{
			NpmPackage: NpmPackage{
				Name:             p.Name,
				Version:          p.Version,
				Dependencies:     p.Dependencies,
				PeerDependencies: p.PeerDependencies,
			},
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
					installList = append(installList, name)
				}
			}
		}
		if p.Types != "" {
			meta.Types = fmt.Sprintf("%s@%s/%s", p.Name, p.Version, p.Types)
		} else if p.Typings != "" {
			meta.Types = fmt.Sprintf("%s@%s/%s", p.Name, p.Version, p.Typings)
		} else {
			if !strings.HasPrefix(pkg.name, "@") {
				info, err := nodeEnv.getPackageInfo("@types/"+pkg.name, "latest")
				if err == nil {
					if info.Types != "" {
						meta.Types = fmt.Sprintf("%s@%s/%s", info.Name, info.Version, info.Types)
					} else if info.Typings != "" {
						meta.Types = fmt.Sprintf("%s@%s/%s", info.Name, info.Version, info.Typings)
					} else {
						meta.Types = fmt.Sprintf("%s@%s/?", info.Name, info.Version)
					}
					installList = append(installList, fmt.Sprintf("%s@%s", info.Name, info.Version))
				} else if err.Error() != fmt.Sprintf("npm: package '@types/%s' not found", pkg.name) {
					return ret, err
				}
			}
			if meta.Types == "" {
				meta.Types = fmt.Sprintf("%s@%s/?", p.Name, p.Version)
			}
		}
		importMeta[pkg.ImportPath()] = meta
	}

	log.Debugf("parse importMeta in %v", time.Now().Sub(start))

	buildDir := path.Join(os.TempDir(), bundleID)
	os.MkdirAll(buildDir, 0755)
	defer os.RemoveAll(buildDir)

	err = os.Chdir(buildDir)
	if err != nil {
		return
	}

	err = pnpmAdd(installList...)
	if err != nil {
		return
	}

	codeBuf := bytes.NewBuffer(nil)
	codeBuf.WriteString("const meta = {};")
	codeBuf.WriteString("const isObject = v => typeof v === 'object' && v !== null;")
	for _, m := range options.packages {
		importPath := m.ImportPath()
		importIdentifier := identify(importPath)
		fmt.Fprintf(codeBuf, `const %s = require("%s");`, importIdentifier, importPath)
		fmt.Fprintf(codeBuf, `meta["%s"] = {exports: isObject(%s) ? Object.keys(%s) : []};`, importPath, importIdentifier, importIdentifier)
	}
	codeBuf.WriteString("process.stdout.write(JSON.stringify(meta));")
	err = ioutil.WriteFile(path.Join(buildDir, "peer.js"), codeBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	start = time.Now()
	cmd := exec.Command("node", "peer.js")
	cmd.Env = append(os.Environ(), fmt.Sprintf(`NODE_ENV=%s`, options.env))
	output, err := cmd.CombinedOutput()
	log.Debug("node peer.js in", time.Now().Sub(start))
	if err != nil {
		err = errors.New(string(output))
		return
	}

	var m map[string]ImportMeta
	err = json.Unmarshal(output, &m)
	if err != nil {
		return
	}
	for name, meta := range m {
		v, ok := importMeta[name]
		if ok {
			importMeta[name] = ImportMeta{
				NpmPackage: v.NpmPackage,
				Exports:    meta.Exports,
			}
		}
		if meta.Types != "" {
			err = copyDTS(path.Join(buildDir, "node_modules"), meta.Types, path.Join(etcDir, "types"))
			if err != nil && os.IsExist(err) {
				return
			}
		}
	}

	codeBuf = bytes.NewBuffer(nil)
	for _, m := range options.packages {
		importName := m.ImportPath()
		fmt.Fprintf(codeBuf, `export * as %s from "%s";`, identify(importName), importName)
	}

	err = ioutil.WriteFile(path.Join(buildDir, "bundle.js"), codeBuf.Bytes(), 0644)
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
	defines := map[string]string{
		"process.env.NODE_ENV": fmt.Sprintf(`"%s"`, options.env),
	}
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{"bundle.js"},
		Bundle:            true,
		Write:             false,
		Target:            target,
		Format:            api.FormatESModule,
		MinifyWhitespace:  !isDev,
		MinifyIdentifiers: !isDev,
		MinifySyntax:      !isDev,
		Defines:           defines,
	})
	if len(result.Errors) > 0 {
		fe := result.Errors[0]
		if strings.HasPrefix(fe.Text, "Could not resolve \"") {
			missingModule := strings.Split(fe.Text, "\"")[1]
			if missingModule != "" {
				_, ok := missingResolved[missingModule]
				if !ok {
					err = pnpmAdd(missingModule)
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

	log.Debugf("esbuild bundle %s %s %s in %v", options.packages.String(), options.target, options.env, time.Now().Sub(start))

	hasher := sha1.New()
	hasher.Write(result.OutputFiles[0].Contents)
	hash := strings.ToLower(base32.StdEncoding.EncodeToString(hasher.Sum(nil)))

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* esm.sh - esbuild bundle(%s) %s %s */%s`, options.packages.String(), strings.ToLower(options.target), options.env, EOL)
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

func copyDTS(nmDir string, mainDTS string, saveDir string) (err error) {
	dtsContent, err := ioutil.ReadFile(path.Join(nmDir, mainDTS))
	if err != nil {
		return
	}

	fmt.Println(string(dtsContent))
	return
}

func pnpmAdd(packages ...string) (err error) {
	if len(packages) > 0 {
		start := time.Now()
		args := append([]string{"add", "--no-color", "--prefer-offline"}, packages...)
		output, err := exec.Command("pnpm", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf(string(output))
		}
		log.Debug("pnpm add", strings.Join(packages, " "), "in", time.Now().Sub(start))
	}
	return
}

func identify(importPath string) string {
	p := []byte(importPath)
	for i, c := range p {
		switch c {
		case '/', '-', '@', '.':
			p[i] = '_'
		default:
			p[i] = c
		}
	}
	return string(p)
}

func ensureDir(dir string) (err error) {
	_, err = os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
	}
	return
}
