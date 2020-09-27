package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/crypto/rs"
	"github.com/ije/gox/utils"
	"github.com/postui/postdb"
	"github.com/postui/postdb/q"
)

var targets = map[string]api.Target{
	"es2015": api.ES2015,
	"es2016": api.ES2016,
	"es2017": api.ES2017,
	"es2018": api.ES2018,
	"es2019": api.ES2019,
	"es2020": api.ES2020,
}

// todo: use queue to replace lock
var buildLock sync.Mutex

// ImportMeta defines import meta
type ImportMeta struct {
	NpmPackage
	Exports   []string `json:"exports"`
	TypesPath string   `json:"typespath"`
}

type buildOptions struct {
	packages moduleSlice
	target   string
	dev      bool
}

type buildResult struct {
	buildID    string
	importMeta map[string]*ImportMeta
	single     bool
}

func build(storageDir string, options buildOptions) (ret buildResult, err error) {
	buildLock.Lock()
	defer buildLock.Unlock()

	n := len(options.packages)
	if n == 0 {
		err = fmt.Errorf("no packages")
		return
	}

	ret.single = n == 1
	if ret.single {
		pkg := options.packages[0]
		filename := path.Base(pkg.name)
		if pkg.submodule != "" {
			filename = pkg.submodule
		}
		if options.dev {
			filename += ".development"
		}
		ret.buildID = fmt.Sprintf("%s@%s/%s/%s", pkg.name, pkg.version, options.target, filename)
	} else {
		hasher := sha1.New()
		sort.Sort(options.packages)
		fmt.Fprintf(hasher, "%s %s %v", options.packages.String(), options.target, options.dev)
		ret.buildID = "bundle-" + strings.ToLower(base32.StdEncoding.EncodeToString(hasher.Sum(nil)))
	}

	p, err := db.Get(q.Alias(ret.buildID), q.K("hash", "importMeta"))
	if err == nil {
		err = json.Unmarshal(p.KV.Get("importMeta"), &ret.importMeta)
		if err != nil {
			_, err = db.Delete(q.Alias(ret.buildID))
			if err != nil {
				return
			}
		}

		_, err = os.Stat(path.Join(storageDir, "builds", ret.buildID+".js"))
		if err == nil || os.IsExist(err) {
			return
		}

		_, err = db.Delete(q.Alias(ret.buildID))
		if err != nil {
			return
		}
	}
	if err != nil && err != postdb.ErrNotFound {
		return
	}

	installList := []string{}
	for _, pkg := range options.packages {
		installList = append(installList, pkg.name+"@"+pkg.version)
	}

	start := time.Now()
	importMeta := map[string]*ImportMeta{}
	peerDependencies := map[string]struct{}{}
	for _, pkg := range options.packages {
		var p NpmPackage
		p, err = nodeEnv.getPackageInfo(pkg.name, pkg.version)
		if err != nil {
			return
		}
		meta := &ImportMeta{
			NpmPackage: p,
		}
		for name := range p.PeerDependencies {
			peerDependencies[name] = struct{}{}
		}
		if meta.Types == "" && meta.Typings == "" && !strings.HasPrefix(pkg.name, "@") {
			var info NpmPackage
			info, err = nodeEnv.getPackageInfo("@types/"+pkg.name, "latest")
			if err == nil {
				if info.Types != "" || info.Typings != "" || info.Main != "" {
					installList = append(installList, fmt.Sprintf("%s@%s", info.Name, info.Version))
				}
			} else if err.Error() != fmt.Sprintf("npm: package '@types/%s' not found", pkg.name) {
				return
			}
		}
		importMeta[pkg.ImportPath()] = meta
	}

	peerPackages := map[string]string{}
	for name := range peerDependencies {
		peer := true
		for _, pkg := range options.packages {
			if pkg.name == name {
				peer = false
				break
			}
		}
		if peer {
			for _, meta := range importMeta {
				for dep := range meta.Dependencies {
					if dep == name {
						peer = false
						break
					}
				}
			}
		}
		if peer {
			peerPackages[name] = "latest"
			installList = append(installList, name)
		}
	}

	log.Debugf("parse importMeta in %v", time.Now().Sub(start))

	buildDir := path.Join(os.TempDir(), "esmd-build", rs.Hex.String(16))
	ensureDir(buildDir)
	defer os.RemoveAll(buildDir)

	err = os.Chdir(buildDir)
	if err != nil {
		return
	}

	err = yarnAdd(installList...)
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

	env := "production"
	if options.dev {
		env = "development"
	}

	start = time.Now()
	cmd := exec.Command("node", "peer.js")
	cmd.Env = append(os.Environ(), fmt.Sprintf(`NODE_ENV=%s`, env))
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = errors.New(string(output))
		return
	}
	log.Debug("node peer.js in", time.Now().Sub(start))

	var m map[string]ImportMeta
	err = json.Unmarshal(output, &m)
	if err != nil {
		return
	}
	for name, meta := range m {
		_meta, ok := importMeta[name]
		if ok {
			_meta.Exports = meta.Exports
		}
	}

	start = time.Now()
	for i, pkg := range options.packages {
		var types string
		meta := importMeta[pkg.ImportPath()]
		nv := fmt.Sprintf("%s@%s", meta.Name, meta.Version)
		if pkg.submodule == "" {
			if meta.Types == "" && meta.Typings == "" && !strings.HasPrefix(pkg.name, "@") {
				var info NpmPackage
				err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", "@types/"+pkg.name, "package.json"), &info)
				if err == nil {
					types = getTypesPath(info)
				} else if !os.IsNotExist(err) {
					return
				}
			}
			if types == "" {
				types = getTypesPath(meta.NpmPackage)
			}
		} else {
			var p NpmPackage
			err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", pkg.name, pkg.submodule, "package.json"), &p)
			if err == nil {
				var tp string
				if p.Types != "" {
					tp = p.Types
				} else if p.Typings != "" {
					tp = p.Typings
				} else if p.Main != "" {
					tp = strings.TrimSuffix(p.Main, ".js")
				}
				if tp != "" {
					types = fmt.Sprintf("%s/%s", nv, ensureExt(path.Join(pkg.submodule, tp), ".d.ts"))
				}
				if p.PeerDependencies != nil {
					_, ok := p.PeerDependencies[meta.Name]
					if ok {
						// copy submodule to node_modules dir since the esbuild external will ignore the submodule too
						err = utils.CopyDir(
							path.Join(buildDir, "node_modules", pkg.name, pkg.submodule),
							path.Join(buildDir, "node_modules", identify(pkg.ImportPath())),
						)
						if err != nil {
							return
						}
						options.packages[i] = module{
							name:    identify(pkg.ImportPath()),
							version: pkg.version,
						}
					}
					for name := range p.PeerDependencies {
						peerPackages[name] = "latest"
					}
				}
			}
			if types == "" {
				if fileExists(path.Join(buildDir, "node_modules", pkg.name, pkg.submodule, "index.d.ts")) {
					types = fmt.Sprintf("%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
				} else if fileExists(path.Join(buildDir, "node_modules", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
					types = fmt.Sprintf("%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
				} else if fileExists(path.Join(buildDir, "node_modules/@types", pkg.name, pkg.submodule, "index.d.ts")) {
					types = fmt.Sprintf("@types/%s/%s", nv, path.Join(pkg.submodule, "index.d.ts"))
				} else if fileExists(path.Join(buildDir, "node_modules/@types", pkg.name, ensureExt(pkg.submodule, ".d.ts"))) {
					types = fmt.Sprintf("@types/%s/%s", nv, ensureExt(pkg.submodule, ".d.ts"))
				}
			}
		}
		if types != "" {
			err = copyDTS(path.Join(buildDir, "node_modules"), path.Join(storageDir, "types"), types)
			if err != nil {
				return
			}
			meta.TypesPath = "/" + types
		}
	}
	log.Debug("copy dts in", time.Now().Sub(start))

	codeBuf = bytes.NewBuffer(nil)
	// todo: has submodule
	if ret.single {
		pkg := options.packages[0]
		importPath := pkg.ImportPath()
		importIdentifier := identify(importPath)
		meta := importMeta[importPath]
		if meta.Module != "" {
			if len(meta.Exports) > 0 {
				fmt.Fprintf(codeBuf, `export * from "%s";`, importPath)
				for _, name := range meta.Exports {
					if name == "default" {
						fmt.Fprintf(codeBuf, `export {default} from "%s";`, importPath)
					}
				}
			} else {
				fmt.Fprintf(codeBuf, `export {default} from "%s";`, importPath)
			}
		} else if meta.Main != "" {
			fmt.Fprintf(codeBuf, `import %s from "%s";`, importIdentifier, importPath)
			fmt.Fprintf(codeBuf, `export default %s;`, importIdentifier)
		} else {
			fmt.Fprintf(codeBuf, `export default null;`)
		}
	} else {
		for _, pkg := range options.packages {
			importPath := pkg.ImportPath()
			importIdentifier := identify(importPath)
			meta := importMeta[importPath]
			if meta.Module != "" {
				fmt.Fprintf(codeBuf, `export * as %s from "%s";`, importIdentifier, importPath)
			} else if meta.Main != "" {
				fmt.Fprintf(codeBuf, `import %s_ from "%s";`, importIdentifier, importPath)
				fmt.Fprintf(codeBuf, `export {%s_ as %s};`, importIdentifier, importIdentifier)
			} else {
				fmt.Fprintf(codeBuf, `export const %s = null;`, importIdentifier)
			}
		}
	}
	err = ioutil.WriteFile(path.Join(buildDir, "export.js"), codeBuf.Bytes(), 0644)
	if err != nil {
		return
	}

	externals := make([]string, len(peerPackages))
	i := 0
	for name := range peerPackages {
		var p NpmPackage
		err = utils.ParseJSONFile(path.Join(buildDir, "node_modules", name, "package.json"), &p)
		if err != nil {
			return
		}
		peerPackages[name] = p.Version
		externals[i] = name
		i++
	}

	missingResolved := map[string]struct{}{}
esbuild:
	start = time.Now()
	minify := !options.dev
	defines := map[string]string{
		"process.env.NODE_ENV": fmt.Sprintf(`"%s"`, env),
	}
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{"export.js"},
		Externals:         externals,
		Bundle:            true,
		Write:             false,
		Target:            targets[options.target],
		Format:            api.FormatESModule,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Defines:           defines,
	})
	if len(result.Errors) > 0 {
		fe := result.Errors[0]
		if strings.HasPrefix(fe.Text, `Could not resolve "`) {
			missingModule := strings.Split(fe.Text, `"`)[1]
			if missingModule != "" {
				_, ok := missingResolved[missingModule]
				if !ok {
					err = yarnAdd(missingModule)
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

	log.Debugf("esbuild bundle %s %s %s in %v", options.packages.String(), options.target, env, time.Now().Sub(start))

	jsContentBuf := bytes.NewBuffer(nil)
	fmt.Fprintf(jsContentBuf, `/* esm.sh - esbuild bundle(%s) %s %s */%s`, options.packages.String(), strings.ToLower(options.target), env, EOL)
	if len(peerPackages) > 0 {
		var esModules []string
		var eol, indent string
		if options.dev {
			indent = "  "
			eol = EOL
		}
		for name, version := range peerPackages {
			identifier := identify(name)
			filename := path.Base(name)
			if options.dev {
				filename += ".development"
			}
			esModules = append(esModules, fmt.Sprintf(`"%s": %s`, name, identifier))
			fmt.Fprintf(jsContentBuf, `import %s from "/%s@%s/%s/%s";%s`, identifier, name, version, options.target, ensureExt(filename, ".js"), eol)
		}
		fmt.Fprintf(jsContentBuf, `var __esModules = {%s`, eol)
		fmt.Fprintf(jsContentBuf, `%s%s%s`, indent, strings.Join(esModules, fmt.Sprintf(",%s%s", eol, indent)), eol)
		fmt.Fprintf(jsContentBuf, `};%s`, eol)
		fmt.Fprintf(jsContentBuf, `var require = name => __esModules[name];%s`, eol)
		jsContentBuf.Write(rewriteImportPath(result.OutputFiles[0].Contents, func(importPath string) string {
			version, ok := peerPackages[importPath]
			if ok {
				filename := path.Base(importPath)
				if options.dev {
					filename += ".development"
				}
				return fmt.Sprintf("/%s@%s/%s/%s", importPath, version, options.target, ensureExt(filename, ".js"))
			}
			return importPath
		}))
	} else {
		jsContentBuf.Write(result.OutputFiles[0].Contents)
	}

	saveFilePath := path.Join(storageDir, "builds", ret.buildID+".js")
	ensureDir(path.Dir(saveFilePath))
	file, err := os.Create(saveFilePath)
	if err != nil {
		return
	}
	defer file.Close()

	_, err = io.Copy(file, jsContentBuf)
	if err != nil {
		return
	}

	db.Put(
		q.Alias(ret.buildID),
		q.Tags("build"),
		q.KV{
			"importMeta": utils.MustEncodeJSON(importMeta),
		},
	)

	ret.importMeta = importMeta
	return
}

func yarnAdd(packages ...string) (err error) {
	if len(packages) > 0 {
		start := time.Now()
		args := append([]string{"add"}, packages...)
		output, err := exec.Command("yarn", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf(string(output))
		}
		log.Debug("yarn add", strings.Join(packages, " "), "in", time.Now().Sub(start))
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

func getTypesPath(p NpmPackage) string {
	types := ""
	if p.Types != "" {
		types = p.Types
	} else if p.Typings != "" {
		types = p.Typings
	} else if p.Main != "" {
		types = strings.TrimSuffix(p.Main, ".js")
	}
	if types != "" {
		return fmt.Sprintf("%s@%s%s", p.Name, p.Version, ensureExt(path.Join("/", types), ".d.ts"))
	}
	return ""
}
