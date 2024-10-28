package server

import (
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
	"time"

	"github.com/esm-dev/esm.sh/server/storage"

	"github.com/Masterminds/semver/v3"
	"github.com/ije/gox/utils"
	"github.com/ije/gox/valid"
)

const npmRegistry = "https://registry.npmjs.org/"
const jsrRegistry = "https://npm.jsr.io/"

var npmNaming = valid.Validator{valid.Range{'a', 'z'}, valid.Range{'A', 'Z'}, valid.Range{'0', '9'}, valid.Eq('_'), valid.Eq('$'), valid.Eq('.'), valid.Eq('-'), valid.Eq('+'), valid.Eq('!'), valid.Eq('~'), valid.Eq('*'), valid.Eq('('), valid.Eq(')')}

// NpmPackageVerions defines versions of a NPM package
type NpmPackageVerions struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]PackageJSONRaw `json:"versions"`
}

// PackageJSONRaw defines the package.json of a NPM package
type PackageJSONRaw struct {
	Name             string                 `json:"name"`
	Version          string                 `json:"version"`
	Type             string                 `json:"type"`
	Main             string                 `json:"main"`
	Browser          StringOrMap            `json:"browser"`
	Module           StringOrMap            `json:"module"`
	ES2015           StringOrMap            `json:"es2015"`
	JsNextMain       string                 `json:"jsnext:main"`
	Types            string                 `json:"types"`
	Typings          string                 `json:"typings"`
	SideEffects      interface{}            `json:"sideEffects"`
	Dependencies     map[string]string      `json:"dependencies"`
	PeerDependencies map[string]string      `json:"peerDependencies"`
	Imports          map[string]interface{} `json:"imports"`
	TypesVersions    map[string]interface{} `json:"typesVersions"`
	Exports          json.RawMessage        `json:"exports"`
	Files            []string               `json:"files"`
	Deprecated       interface{}            `json:"deprecated"`
	Esmsh            interface{}            `json:"esm.sh"`
}

type PackageID struct {
	Name    string
	Version string
}

func (p *PackageID) String() string {
	return p.Name + "@" + p.Version
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
	deprecated := ""
	if a.Deprecated != nil {
		if s, ok := a.Deprecated.(string); ok {
			deprecated = s
		}
	}
	esmsh := map[string]interface{}{}
	if a.Esmsh != nil {
		if v, ok := a.Esmsh.(map[string]interface{}); ok {
			esmsh = v
		}
	}
	var sideEffects *StringSet = nil
	sideEffectsFalse := false
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			if s == "false" {
				sideEffectsFalse = true
			} else if endsWith(s, jsExts...) {
				sideEffects = NewStringSet()
				sideEffects.Add(s)
			}
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffectsFalse = !b
		} else if m, ok := a.SideEffects.([]interface{}); ok && len(m) > 0 {
			sideEffects = NewStringSet()
			for _, v := range m {
				if name, ok := v.(string); ok && endsWith(name, jsExts...) {
					sideEffects.Add(name)
				}
			}
		}
	}
	var exports interface{} = nil
	if rawExports := a.Exports; rawExports != nil {
		var v interface{}
		if json.Unmarshal(rawExports, &v) == nil {
			if s, ok := v.(string); ok {
				if len(s) > 0 {
					exports = s
				}
			} else if _, ok := v.(map[string]interface{}); ok {
				om := newOrderedMap()
				if om.UnmarshalJSON(rawExports) == nil {
					exports = om
				}
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
		JsNextMain:       a.JsNextMain,
		Types:            a.Types,
		Typings:          a.Typings,
		Browser:          browser,
		SideEffectsFalse: sideEffectsFalse,
		SideEffects:      sideEffects,
		Dependencies:     a.Dependencies,
		PeerDependencies: a.PeerDependencies,
		Imports:          a.Imports,
		TypesVersions:    a.TypesVersions,
		Exports:          exports,
		Files:            a.Files,
		Deprecated:       deprecated,
		Esmsh:            esmsh,
	}
}

// PackageJSON defines defines the package.json of a NPM package
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
	Imports          map[string]interface{}
	TypesVersions    map[string]interface{}
	Exports          interface{}
	Files            []string
	Deprecated       string
	Esmsh            map[string]interface{}
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
	if len(config.NpmRegistries) > 0 {
		for scope, reg := range config.NpmRegistries {
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

func (rc *NpmRC) getPackageInfo(name string, semver string) (info PackageJSON, err error) {
	// use fixed version for `@types/node`
	if name == "@types/node" {
		info = PackageJSON{
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
		pkgJsonPath := path.Join(rc.StoreDir(), name+"@"+semver, "node_modules", name, "package.json")
		if existsFile(pkgJsonPath) && utils.ParseJSONFile(pkgJsonPath, &info) == nil {
			return
		}
	}

	info, err = rc.fetchPackageInfo(name, semver)
	return
}

func (rc *NpmRC) fetchPackageInfo(name string, semverOrDistTag string) (info PackageJSON, err error) {
	a := strings.Split(strings.Trim(name, "/"), "/")
	name = a[0]
	if strings.HasPrefix(name, "@") && len(a) > 1 {
		name = a[0] + "/" + a[1]
	}

	if semverOrDistTag == "" || semverOrDistTag == "*" {
		semverOrDistTag = "latest"
	} else if (strings.HasPrefix(semverOrDistTag, "=") || strings.HasPrefix(semverOrDistTag, "v")) && regexpVersionStrict.MatchString(semverOrDistTag[1:]) {
		// strip leading `=` or `v` from semver
		semverOrDistTag = semverOrDistTag[1:]
	}

	cacheKey := fmt.Sprintf("npm:%s/%s@%s", rc.zoneId, name, semverOrDistTag)
	lock := getFetchLock(cacheKey)
	lock.Lock()
	defer lock.Unlock()

	// check cache firstly
	if cache != nil {
		var data []byte
		data, err = cache.Get(cacheKey)
		if err == nil && json.Unmarshal(data, &info) == nil {
			return
		}
		if err != nil && err != storage.ErrNotFound && err != storage.ErrExpired {
			log.Error("cache:", err)
		}
	}

	start := time.Now()
	defer func() {
		if err == nil {
			log.Debugf("lookup package(%s@%s) in %v", name, info.Version, time.Since(start))
		}
	}()

	url := rc.Registry + name
	token := rc.Token
	user := rc.User
	password := rc.Password

	if strings.HasPrefix(name, "@") {
		scope, _ := utils.SplitByFirstByte(name, '/')
		reg, ok := rc.Registries[scope]
		if ok {
			url = reg.Registry + name
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
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
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 401 {
		if isFullVersionFromNpmjsOrg {
			err = fmt.Errorf("version %s of '%s' not found", semverOrDistTag, name)
		} else {
			err = fmt.Errorf("package '%s' not found", name)
		}
		return
	}

	if resp.StatusCode != 200 {
		ret, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("could not get metadata of package '%s' (%s: %s)", name, resp.Status, string(ret))
		return
	}

	if isFullVersionFromNpmjsOrg {
		err = json.NewDecoder(resp.Body).Decode(&info)
		if err != nil {
			return
		}
		if cache != nil {
			cache.Set(cacheKey, utils.MustEncodeJSON(info), 7*24*time.Hour)
		}
		return
	}

	var h NpmPackageVerions
	err = json.NewDecoder(resp.Body).Decode(&h)
	if err != nil {
		return
	}

	if len(h.Versions) == 0 {
		err = fmt.Errorf("missing `versions` field")
		return
	}

	var jsonBytes []byte

	distVersion, ok := h.DistTags[semverOrDistTag]
	if ok {
		d := h.Versions[distVersion]
		info = *d.ToNpmPackage()
		jsonBytes = utils.MustEncodeJSON(d)
	} else {
		var c *semver.Constraints
		c, err = semver.NewConstraint(semverOrDistTag)
		if err != nil && semverOrDistTag != "latest" {
			return rc.fetchPackageInfo(name, "latest")
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
				return
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
			d := h.Versions[vs[i-1].String()]
			info = *d.ToNpmPackage()
			jsonBytes = utils.MustEncodeJSON(d)
		}
	}

	if info.Version == "" {
		err = fmt.Errorf("version %s of '%s' not found", semverOrDistTag, name)
		return
	}

	// cache package info for 10 minutes
	if cache != nil {
		cache.Set(cacheKey, jsonBytes, 10*time.Minute)
	}
	return
}

func (rc *NpmRC) installPackage(url ESM) (pkgJson PackageJSON, err error) {
	installDir := path.Join(rc.StoreDir(), url.PackageName())
	pkgJsonFilepath := path.Join(installDir, "node_modules", url.PkgName, "package.json")

	// only one installation process allowed at the same time for the same package
	lock := getInstallLock(installDir)
	lock.Lock()
	defer lock.Unlock()

	// skip installation if the package has been installed
	if existsFile(pkgJsonFilepath) {
		err = utils.ParseJSONFile(pkgJsonFilepath, &pkgJson)
		if err == nil {
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
	packageJsonFp := path.Join(installDir, "package.json")
	if !existsFile(packageJsonFp) {
		ensureDir(installDir)
		err = os.WriteFile(packageJsonFp, []byte("{}"), 0644)
	}
	if err != nil {
		err = fmt.Errorf("ensure package.json failed: %s", url.PackageName())
		return
	}

	attemptMaxTimes := 5
	for i := 1; i <= attemptMaxTimes; i++ {
		if url.GhPrefix {
			err = os.WriteFile(packageJsonFp, []byte(fmt.Sprintf(`{"dependencies":{"%s":"github:%s#%s"}}`, url.PkgName, url.PkgName, url.PkgVersion)), 0644)
			if err == nil {
				err = rc.pnpmi(installDir)
			}
			// pnpm will ignore github package which has been installed without `package.json` file
			// so we install it manually
			if err == nil {
				packageJsonFp := path.Join(installDir, "node_modules", url.PkgName, "package.json")
				if !existsFile(packageJsonFp) {
					ensureDir(path.Dir(packageJsonFp))
					err = os.WriteFile(packageJsonFp, utils.MustEncodeJSON(PackageJSONRaw{
						Name:    url.PkgName,
						Version: url.PkgVersion,
					}), 0644)
				} else {
					var p PackageJSON
					err = utils.ParseJSONFile(packageJsonFp, &p)
					if err == nil && len(p.Files) > 0 {
						// install github package with ignoring `files` field
						err = ghInstall(installDir, url.PkgName, url.PkgVersion)
					}
				}
			}
		} else if regexpVersionStrict.MatchString(url.PkgVersion) {
			err = rc.pnpmi(installDir, "--prefer-offline", url.PackageName())
		} else {
			err = rc.pnpmi(installDir, url.PackageName())
		}
		if err == nil {
			err = utils.ParseJSONFile(pkgJsonFilepath, &pkgJson)
			if err != nil {
				err = fmt.Errorf("pnpm install %s: package.json not found", url)
			}
		}
		if err == nil || i == attemptMaxTimes {
			break
		}
		time.Sleep(time.Duration(i) * 100 * time.Millisecond)
	}
	return
}

func (rc *NpmRC) pnpmi(dir string, packages ...string) (err error) {
	args := []string{
		"i",
		"--no-lockfile",
		"--no-color",
		"--ignore-pnpmfile",
		"--ignore-workspace",
		"--ignore-scripts",
		"--loglevel=error",
	}
	if len(packages) > 0 {
		args = append(args, packages...)
	}
	start := time.Now()
	out := &bytes.Buffer{}
	errout := &bytes.Buffer{}
	cmd := exec.Command("pnpm", args...)
	cmd.Env = os.Environ()
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = errout
	cmd.WaitDelay = 10 * time.Minute

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
	if err == nil && errout.Len() > 0 {
		return fmt.Errorf("%s", errout.String())
	}
	if err != nil {
		return fmt.Errorf("pnpm %s: %s", strings.Join(args, " "), out.String())
	}
	if len(packages) > 0 {
		log.Debug("pnpm add", strings.Join(packages, " "), "in", time.Since(start))
	} else {
		log.Debug("pnpm install in", time.Since(start))
	}
	return
}

func (rc *NpmRC) createDotNpmRcFile(dir string) error {
	buf := bytes.NewBuffer(nil)
	if rc.Registry != "" {
		buf.WriteString(fmt.Sprintf("registry=%s\n", rc.Registry))
		if rc.Token != "" {
			authPerfix := removeHttpUrlProtocol(rc.Registry)
			buf.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN}\n", authPerfix))
		}
		if rc.User != "" && rc.Password != "" {
			authPerfix := removeHttpUrlProtocol(rc.Registry)
			buf.WriteString(fmt.Sprintf("%s:username=${ESM_NPM_USER}\n", authPerfix))
			buf.WriteString(fmt.Sprintf("%s:_password=${ESM_NPM_PASSWORD}\n", authPerfix))
		}
	}
	for scope, reg := range rc.Registries {
		if reg.Registry != "" {
			buf.WriteString(fmt.Sprintf("%s:registry=%s\n", scope, reg.Registry))
			if reg.Token != "" {
				authPerfix := removeHttpUrlProtocol(reg.Registry)
				buf.WriteString(fmt.Sprintf("%s:_authToken=${ESM_NPM_TOKEN_%s}\n", authPerfix, toEnvName(scope[1:])))
			}
			if reg.User != "" && reg.Password != "" {
				authPerfix := removeHttpUrlProtocol(reg.Registry)
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

func (npmrc *NpmRC) getSvelteVersion(importMap ImportMap) (svelteVersion string, err error) {
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
		var info PackageJSON
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

func (npmrc *NpmRC) getVueVersion(importMap ImportMap) (vueVersion string, err error) {
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
		var info PackageJSON
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
		return npmNaming.Is(scope[1:]) && npmNaming.Is(name)
	}
	return npmNaming.Is(pkgName)
}

// added by @jimisaacs
func toTypesPkgName(pkgName string) string {
	if strings.HasPrefix(pkgName, "@") {
		pkgName = strings.Replace(pkgName[1:], "/", "__", 1)
	}
	return "@types/" + pkgName
}
