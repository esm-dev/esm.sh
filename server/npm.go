package server

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/esm-dev/esm.sh/server/common"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

const (
	npmRegistry = "https://registry.npmjs.org/"
	jsrRegistry = "https://npm.jsr.io/"
)

var (
	installLocks = sync.Map{}
	npmNaming    = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('$'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('!'), valid.Eq('~'), valid.Eq('*'), valid.Eq('('), valid.Eq(')')}
)

type PackageId struct {
	Name    string
	Version string
}

func (p *PackageId) String() string {
	return p.Name + "@" + p.Version
}

// NpmPackageVerions defines versions of a NPM package
type NpmPackageVerions struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]PackageJSONRaw `json:"versions"`
}

// PackageJSONRaw defines the package.json of a NPM package
type PackageJSONRaw struct {
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Type             string          `json:"type"`
	Main             string          `json:"main"`
	Module           StringOrMap     `json:"module"`
	ES2015           StringOrMap     `json:"es2015"`
	JsNextMain       StringOrMap     `json:"jsnext:main"`
	Browser          StringOrMap     `json:"browser"`
	Types            string          `json:"types"`
	Typings          string          `json:"typings"`
	SideEffects      any             `json:"sideEffects"`
	Dependencies     any             `json:"dependencies"`
	PeerDependencies any             `json:"peerDependencies"`
	Imports          map[string]any  `json:"imports"`
	TypesVersions    map[string]any  `json:"typesVersions"`
	Exports          json.RawMessage `json:"exports"`
	Files            []string        `json:"files"`
	Esmsh            map[string]any  `json:"esm.sh"`
}

// PackageJSON defines the package.json of a NPM package
type PackageJSON struct {
	Name             string
	PkgName          string
	Version          string
	Type             string
	Main             string
	Module           string
	ES2015           string
	JsNextMain       string
	Types            string
	Typings          string
	SideEffectsFalse bool
	SideEffects      *StringSet
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]any
	TypesVersions    map[string]any
	Exports          *OrderedMap
	Esmsh            map[string]any
}

// ToNpmPackage converts PackageJSONRaw to PackageJSON
func (a *PackageJSONRaw) ToNpmPackage() *PackageJSON {
	browser := map[string]string{}
	if a.Browser.Str != "" {
		browser["."] = a.Browser.Str
	}
	if a.Browser.Map != nil {
		for k, v := range a.Browser.Map {
			s, isStr := v.(string)
			if isStr {
				browser[k] = s
			} else {
				b, ok := v.(bool)
				if ok && !b {
					browser[k] = ""
				}
			}
		}
	}
	var dependencies map[string]string
	if m, ok := a.Dependencies.(map[string]any); ok {
		dependencies = make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					dependencies[k] = s
				}
			}
		}
	}
	var peerDependencies map[string]string
	if m, ok := a.PeerDependencies.(map[string]any); ok {
		peerDependencies = make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					peerDependencies[k] = s
				}
			}
		}
	}
	var sideEffects *StringSet = nil
	sideEffectsFalse := false
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			if s == "false" {
				sideEffectsFalse = true
			} else if endsWith(s, moduleExts...) {
				sideEffects = NewStringSet()
				sideEffects.Add(s)
			}
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffectsFalse = !b
		} else if m, ok := a.SideEffects.([]any); ok && len(m) > 0 {
			sideEffects = NewStringSet()
			for _, v := range m {
				if name, ok := v.(string); ok && endsWith(name, moduleExts...) {
					sideEffects.Add(name)
				}
			}
		}
	}
	exports := newOrderedMap()
	if rawExports := a.Exports; rawExports != nil {
		var v any
		if json.Unmarshal(rawExports, &v) == nil {
			if s, ok := v.(string); ok {
				if len(s) > 0 {
					exports.Set(".", s)
				}
			} else if _, ok := v.(map[string]any); ok {
				exports.UnmarshalJSON(rawExports)
			}
		}
	}
	return &PackageJSON{
		Name:             a.Name,
		Version:          a.Version,
		Type:             a.Type,
		Main:             a.Main,
		Module:           a.Module.MainValue(),
		ES2015:           a.ES2015.MainValue(),
		JsNextMain:       a.JsNextMain.MainValue(),
		Types:            a.Types,
		Typings:          a.Typings,
		Browser:          browser,
		SideEffectsFalse: sideEffectsFalse,
		SideEffects:      sideEffects,
		Dependencies:     dependencies,
		PeerDependencies: peerDependencies,
		Imports:          a.Imports,
		TypesVersions:    a.TypesVersions,
		Exports:          exports,
		Esmsh:            a.Esmsh,
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (a *PackageJSON) UnmarshalJSON(b []byte) error {
	var n PackageJSONRaw
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*a = *n.ToNpmPackage()
	return nil
}

type NpmRegistry struct {
	Registry string `json:"registry"`
	Token    string `json:"token"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type NpmRC struct {
	NpmRegistry
	Registries map[string]NpmRegistry `json:"registries"`
	zoneId     string
}

func NewNpmRcFromConfig() *NpmRC {
	rc := &NpmRC{
		NpmRegistry: NpmRegistry{
			Registry: config.NpmRegistry,
			Token:    config.NpmToken,
			User:     config.NpmUser,
			Password: config.NpmPassword,
		},
		Registries: map[string]NpmRegistry{
			"@jsr": {
				Registry: jsrRegistry,
			},
		},
	}
	if len(config.NpmScopedRegistries) > 0 {
		for scope, reg := range config.NpmScopedRegistries {
			rc.Registries[scope] = NpmRegistry{
				Registry: reg.Registry,
				Token:    reg.Token,
				User:     reg.User,
				Password: reg.Password,
			}
		}
	}
	return rc
}

func NewNpmRcFromJSON(jsonData []byte) (npmrc *NpmRC, err error) {
	var rc NpmRC
	err = json.Unmarshal(jsonData, &rc)
	if err != nil {
		return nil, err
	}
	if rc.Registry == "" {
		rc.Registry = config.NpmRegistry
	} else if !strings.HasSuffix(rc.Registry, "/") {
		rc.Registry += "/"
	}
	if rc.Registries == nil {
		rc.Registries = map[string]NpmRegistry{}
	}
	if _, ok := rc.Registries["@jsr"]; !ok {
		rc.Registries["@jsr"] = NpmRegistry{
			Registry: jsrRegistry,
		}
	}
	for _, reg := range rc.Registries {
		if reg.Registry != "" && !strings.HasSuffix(reg.Registry, "/") {
			reg.Registry += "/"
		}
	}
	return &rc, nil
}

func (rc *NpmRC) StoreDir() string {
	if rc.zoneId != "" {
		return path.Join(config.WorkDir, "npm-"+rc.zoneId)
	}
	return path.Join(config.WorkDir, "npm")
}

func (rc *NpmRC) getPackageInfo(name string, semver string) (info *PackageJSON, err error) {
	// use fixed version for `@types/node`
	if name == "@types/node" {
		info = &PackageJSON{
			Name:    "@types/node",
			Version: nodeTypesVersion,
			Types:   "index.d.ts",
		}
		return
	}

	// strip leading `=` or `v`
	if (strings.HasPrefix(semver, "=") || strings.HasPrefix(semver, "v")) && regexpVersionStrict.MatchString(semver[1:]) {
		semver = semver[1:]
	}

	// check if the package has been installed
	if regexpVersionStrict.MatchString(semver) {
		var raw PackageJSONRaw
		pkgJsonPath := path.Join(rc.StoreDir(), name+"@"+semver, "node_modules", name, "package.json")
		if existsFile(pkgJsonPath) && utils.ParseJSONFile(pkgJsonPath, &raw) == nil {
			info = raw.ToNpmPackage()
			return
		}
	}

	info, err = rc.fetchPackageInfo(name, semver)
	return
}

func (npmrc *NpmRC) fetchPackageInfo(packageName string, semverOrDistTag string) (packageJson *PackageJSON, err error) {
	start := time.Now()
	defer func() {
		if packageJson != nil {
			if semverOrDistTag == packageJson.Version {
				log.Debugf("lookup package(%s@%s) in %v", packageName, semverOrDistTag, time.Since(start))
			} else {
				log.Debugf("lookup package(%s@%s → %s@%s) in %v", packageName, semverOrDistTag, packageName, packageJson.Version, time.Since(start))
			}
		}
	}()

	a := strings.Split(strings.Trim(packageName, "/"), "/")
	packageName = a[0]
	if strings.HasPrefix(packageName, "@") && len(a) > 1 {
		packageName = a[0] + "/" + a[1]
	}

	if semverOrDistTag == "" || semverOrDistTag == "*" {
		semverOrDistTag = "latest"
	} else if (strings.HasPrefix(semverOrDistTag, "=") || strings.HasPrefix(semverOrDistTag, "v")) && regexpVersionStrict.MatchString(semverOrDistTag[1:]) {
		// strip leading `=` or `v` from semver
		semverOrDistTag = semverOrDistTag[1:]
	}

	url := npmrc.Registry + packageName
	token := npmrc.Token
	user := npmrc.User
	password := npmrc.Password

	if strings.HasPrefix(packageName, "@") {
		scope, _ := utils.SplitByFirstByte(packageName, '/')
		reg, ok := npmrc.Registries[scope]
		if ok {
			url = reg.Registry + packageName
			token = reg.Token
			user = reg.User
			password = reg.Password
		}
	}

	isFullVersion := regexpVersionStrict.MatchString(semverOrDistTag)
	isFullVersionFromNpmjsOrg := isFullVersion && strings.HasPrefix(url, npmRegistry)
	if isFullVersionFromNpmjsOrg {
		// npm registry supports url like `https://registry.npmjs.org/<name>/<version>`
		url += "/" + semverOrDistTag
	}

	return withCache(url+"@"+semverOrDistTag+","+token+","+user+":"+password, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() (*PackageJSON, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else if user != "" && password != "" {
			req.SetBasicAuth(user, password)
		}

		c := &http.Client{
			Timeout: 15 * time.Second,
		}
		retryTimes := 0
	do:
		resp, err := c.Do(req)
		if err != nil {
			if retryTimes < 3 {
				retryTimes++
				time.Sleep(time.Duration(retryTimes) * 100 * time.Millisecond)
				goto do
			}
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 || resp.StatusCode == 401 {
			if isFullVersionFromNpmjsOrg {
				err = fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
			} else {
				err = fmt.Errorf("package '%s' not found", packageName)
			}
			return nil, err
		}

		if resp.StatusCode != 200 {
			msg, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("could not get metadata of package '%s' (%s: %s)", packageName, resp.Status, string(msg))
		}

		if isFullVersionFromNpmjsOrg {
			var h PackageJSONRaw
			err = json.NewDecoder(resp.Body).Decode(&h)
			if err != nil {
				return nil, err
			}
			return h.ToNpmPackage(), nil
		}

		var h NpmPackageVerions
		err = json.NewDecoder(resp.Body).Decode(&h)
		if err != nil {
			return nil, err
		}

		if len(h.Versions) == 0 {
			return nil, fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
		}

	lookup:
		distVersion, ok := h.DistTags[semverOrDistTag]
		if ok {
			raw, ok := h.Versions[distVersion]
			if ok {
				return raw.ToNpmPackage(), nil
			}
		} else {
			if semverOrDistTag == "lastest" {
				return nil, fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
			}
			var c *semver.Constraints
			c, err = semver.NewConstraint(semverOrDistTag)
			if err != nil {
				semverOrDistTag = "latest"
				goto lookup
			}
			vs := make([]*semver.Version, len(h.Versions))
			i := 0
			for v := range h.Versions {
				// ignore prerelease versions
				if !strings.ContainsRune(semverOrDistTag, '-') && strings.ContainsRune(v, '-') {
					continue
				}
				var ver *semver.Version
				ver, err = semver.NewVersion(v)
				if err != nil {
					return nil, err
				}
				if c.Check(ver) {
					vs[i] = ver
					i++
				}
			}
			if i > 0 {
				vs = vs[:i]
				if i > 1 {
					sort.Sort(semver.Collection(vs))
				}
				raw, ok := h.Versions[vs[i-1].String()]
				if ok {
					return raw.ToNpmPackage(), nil
				}
			}
		}
		return nil, fmt.Errorf("version %s of '%s' not found", semverOrDistTag, packageName)
	})
}

func (rc *NpmRC) installPackage(esm ESMPath) (packageJson *PackageJSON, err error) {
	installDir := path.Join(rc.StoreDir(), esm.PackageName())
	packageJsonPath := path.Join(installDir, "node_modules", esm.PkgName, "package.json")

	// skip installation if the package has been installed
	if existsFile(packageJsonPath) {
		var raw PackageJSONRaw
		err = utils.ParseJSONFile(packageJsonPath, &raw)
		if err == nil {
			packageJson = raw.ToNpmPackage()
			return
		}
	}

	// only one installation process allowed at the same time for the same package
	v, _ := installLocks.LoadOrStore(esm.PackageName(), &sync.Mutex{})
	defer installLocks.Delete(esm.PackageName())

	v.(*sync.Mutex).Lock()
	defer v.(*sync.Mutex).Unlock()

	// skip installation if the package has been installed
	if existsFile(packageJsonPath) {
		var raw PackageJSONRaw
		err = utils.ParseJSONFile(packageJsonPath, &raw)
		if err == nil {
			packageJson = raw.ToNpmPackage()
			return
		}
	}

	// create '.npmrc' file
	err = rc.createDotNpmRcFile(installDir)
	if err != nil {
		err = fmt.Errorf("failed to create .npmrc file: %v", err)
		return
	}

	// ensure 'package.json' file to prevent read up-levels
	packageJsonRc := path.Join(installDir, "package.json")
	if !existsFile(packageJsonRc) {
		ensureDir(installDir)
		err = os.WriteFile(packageJsonRc, []byte("{}"), 0644)
	}
	if err != nil {
		err = fmt.Errorf("ensure package.json failed: %s", esm.PackageName())
		return
	}

	if esm.GhPrefix {
		err = os.WriteFile(packageJsonRc, []byte(fmt.Sprintf(`{"dependencies":{"%s":"github:%s#%s"}}`, esm.PkgName, esm.PkgName, esm.PkgVersion)), 0644)
		if err == nil {
			err = rc.pnpmi(installDir, "--prefer-offline")
		}
		if err == nil {
			// ensure 'package.json' file if not exists after installing from github
			if !existsFile(packageJsonPath) {
				ensureDir(path.Dir(packageJsonPath))
				packageJson := fmt.Sprintf(`{"name":"%s","version":"%s"}`, esm.PkgName, esm.PkgVersion)
				err = os.WriteFile(packageJsonPath, []byte(packageJson), 0644)
			}
		}
	} else if esm.PrPrefix {
		err = os.WriteFile(packageJsonRc, []byte(fmt.Sprintf(`{"dependencies":{"%s":"https://pkg.pr.new/%s@%s"}}`, esm.PkgName, esm.PkgName, esm.PkgVersion)), 0644)
		if err == nil {
			err = rc.pnpmi(installDir, "--prefer-offline")
		}
	} else if regexpVersionStrict.MatchString(esm.PkgVersion) {
		err = os.WriteFile(packageJsonRc, []byte(fmt.Sprintf(`{"dependencies":{"%s":"%s"}}`, esm.PkgName, esm.PkgVersion)), 0644)
		if err == nil {
			err = rc.pnpmi(installDir, "--prefer-offline")
		}
	} else {
		err = rc.pnpmi(installDir, esm.PackageName())
	}
	if err != nil {
		return
	}

	var raw PackageJSONRaw
	err = utils.ParseJSONFile(packageJsonPath, &raw)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("pnpm i %s: package.json not found", esm.PackageName())
		} else {
			err = fmt.Errorf("pnpm i %s: %v", esm.PackageName(), err)
		}
		return
	}

	// pnpm ignore files are not in 'files' field of the 'package.json'
	// let's download the package from github and extract it
	if esm.GhPrefix && len(raw.Files) > 0 {
		err = ghInstall(installDir, esm.PkgName, esm.PkgVersion)
		if err != nil {
			return
		}
	}

	packageJson = raw.ToNpmPackage()
	return
}

func (rc *NpmRC) pnpmi(dir string, packages ...string) (err error) {
	start := time.Now()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := []string{
		"i",
		"--ignore-pnpmfile",
		"--ignore-scripts",
		"--ignore-workspace",
		"--loglevel=warn",
		"--prod",
		"--no-color",
		"--no-lockfile",
		"--no-optional",
		"--no-verify-store-integrity",
	}
	if len(packages) > 0 {
		args = append(args, packages...)
	}
	cmd := exec.Command("pnpm", args...)
	cmd.Env = os.Environ()
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 30 * time.Second

	// for security, we don't put token and password in the `.npmrc` file
	// instead, we pass them as environment variables to the `pnpm` subprocess
	if rc.Token != "" {
		cmd.Env = append(cmd.Environ(), "ESM_NPM_TOKEN="+rc.Token)
	} else if rc.User != "" && rc.Password != "" {
		data := []byte(rc.Password)
		password := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(password, data)
		cmd.Env = append(
			cmd.Environ(),
			"ESM_NPM_USER="+rc.User,
			"ESM_NPM_PASSWORD="+string(password),
		)
	}
	for scope, reg := range rc.Registries {
		if reg.Token != "" {
			cmd.Env = append(cmd.Environ(), fmt.Sprintf("ESM_NPM_TOKEN_%s=%s", toEnvName(scope[1:]), reg.Token))
		} else if reg.User != "" && reg.Password != "" {
			data := []byte(reg.Password)
			password := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
			base64.StdEncoding.Encode(password, data)
			cmd.Env = append(
				cmd.Env,
				fmt.Sprintf("ESM_NPM_USER_%s=%s", toEnvName(scope[1:]), reg.User),
				fmt.Sprintf("ESM_NPM_PASSWORD_%s=%s", toEnvName(scope[1:]), string(password)),
			)
		}
	}

	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s", stderr.String())
		}
		return err
	}

	// check 'deprecated' warning in the output
	r := bufio.NewReader(stdout)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		t, m := utils.SplitByFirstByte(string(line), ':')
		if strings.Contains(t, " deprecated ") {
			os.WriteFile(path.Join(dir, "deprecated.txt"), []byte(strings.TrimSpace(m)), 0644)
			break
		}
	}

	log.Debug("pnpm i", strings.Join(packages, " "), "in", time.Since(start))
	return
}

func (rc *NpmRC) createDotNpmRcFile(dir string) error {
	buf := bytes.NewBuffer(nil)
	if rc.Registry != "" {
		buf.WriteString(fmt.Sprintf("registry=%s\n", rc.Registry))
		if rc.Token != "" {
			authPerfix := stripHttpScheme(rc.Registry)
			buf.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN}\n", authPerfix))
		}
		if rc.User != "" && rc.Password != "" {
			authPerfix := stripHttpScheme(rc.Registry)
			buf.WriteString(fmt.Sprintf("%s:username=${ESM_NPM_USER}\n", authPerfix))
			buf.WriteString(fmt.Sprintf("%s:_password=${ESM_NPM_PASSWORD}\n", authPerfix))
		}
	}
	for scope, reg := range rc.Registries {
		if reg.Registry != "" {
			buf.WriteString(fmt.Sprintf("%s:registry=%s\n", scope, reg.Registry))
			if reg.Token != "" {
				authPerfix := stripHttpScheme(reg.Registry)
				buf.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN_%s}\n", authPerfix, toEnvName(scope[1:])))
			}
			if reg.User != "" && reg.Password != "" {
				authPerfix := stripHttpScheme(reg.Registry)
				buf.WriteString(fmt.Sprintf("%s:username=${ESM_NPM_USER_%s}\n", authPerfix, toEnvName(scope[1:])))
				buf.WriteString(fmt.Sprintf("%s:_password=${ESM_NPM_PASSWORD_%s}\n", authPerfix, toEnvName(scope[1:])))
			}
		}
	}
	err := ensureDir(dir)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(dir, ".npmrc"), buf.Bytes(), 0644)
}

func (npmrc *NpmRC) isDeprecated(pkgName string, pkgVersion string) (string, error) {
	installDir := path.Join(npmrc.StoreDir(), pkgName+"@"+pkgVersion)
	data, err := os.ReadFile(path.Join(installDir, "deprecated.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (npmrc *NpmRC) getSvelteVersion(importMap common.ImportMap) (svelteVersion string, err error) {
	svelteVersion = "5"
	if len(importMap.Imports) > 0 {
		sveltePath, ok := importMap.Imports["svelte"]
		if ok {
			a := regexpSveltePath.FindAllStringSubmatch(sveltePath, 1)
			if len(a) > 0 {
				svelteVersion = a[0][1]
			}
		}
	}
	if !regexpVersionStrict.MatchString(svelteVersion) {
		var info *PackageJSON
		info, err = npmrc.getPackageInfo("svelte", svelteVersion)
		if err != nil {
			return
		}
		svelteVersion = info.Version
	}
	if semverLessThan(svelteVersion, "4.0.0") {
		err = errors.New("unsupported svelte version, only 4.0.0+ is supported")
	}
	return
}

func (npmrc *NpmRC) getVueVersion(importMap common.ImportMap) (vueVersion string, err error) {
	vueVersion = "3"
	if len(importMap.Imports) > 0 {
		vuePath, ok := importMap.Imports["vue"]
		if ok {
			a := regexpVuePath.FindAllStringSubmatch(vuePath, 1)
			if len(a) > 0 {
				vueVersion = a[0][1]
			}
		}
	}
	if !regexpVersionStrict.MatchString(vueVersion) {
		var info *PackageJSON
		info, err = npmrc.getPackageInfo("vue", vueVersion)
		if err != nil {
			return
		}
		vueVersion = info.Version
	}
	if semverLessThan(vueVersion, "3.0.0") {
		err = errors.New("unsupported vue version, only 3.0.0+ is supported")
	}
	return
}

// based on https://github.com/npm/validate-npm-package-name
func validatePackageName(pkgName string) bool {
	if len(pkgName) > 214 {
		return false
	}
	if strings.HasPrefix(pkgName, "@") {
		scope, name := utils.SplitByFirstByte(pkgName, '/')
		return npmNaming.Match(scope[1:]) && npmNaming.Match(name)
	}
	return npmNaming.Match(pkgName)
}

// added by @jimisaacs
func toTypesPackageName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}

// stripHttpScheme removes the `http[s]:` protocol from the given url.
func stripHttpScheme(url string) string {
	if strings.HasPrefix(url, "https://") {
		return url[6:]
	}
	if strings.HasPrefix(url, "http://") {
		return url[5:]
	}
	return url
}
