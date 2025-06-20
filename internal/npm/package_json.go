package npm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/ije/gox/set"
)

// PackageMetadata defines versions of a NPM package
type PackageMetadata struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]PackageJSONRaw `json:"versions"`
	Time     map[string]string         `json:"time"`
}

// PackageJSONRaw defines the package.json of a NPM package
type PackageJSONRaw struct {
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Type             string          `json:"type"`
	Main             JSONAny         `json:"main"`
	Module           JSONAny         `json:"module"`
	ES2015           JSONAny         `json:"es2015"`
	JsNextMain       JSONAny         `json:"jsnext:main"`
	Browser          JSONAny         `json:"browser"`
	Types            JSONAny         `json:"types"`
	Typings          JSONAny         `json:"typings"`
	SideEffects      any             `json:"sideEffects"`
	Dependencies     any             `json:"dependencies"`
	PeerDependencies any             `json:"peerDependencies"`
	Imports          any             `json:"imports"`
	TypesVersions    any             `json:"typesVersions"`
	Exports          json.RawMessage `json:"exports"`
	Esmsh            any             `json:"esm.sh"`
	Dist             json.RawMessage `json:"dist"`
	Deprecated       any             `json:"deprecated"`
}

// NpmPackageDist defines the dist field of a NPM package
type NpmPackageDist struct {
	Tarball string `json:"tarball"`
}

// PackageJSON defines the package.json of a NPM package
type PackageJSON struct {
	Name             string
	PkgName          string
	Version          string
	Type             string
	Main             string
	Module           string
	Types            string
	Typings          string
	SideEffectsFalse bool
	SideEffects      set.ReadOnlySet[string]
	Browser          map[string]string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	Imports          map[string]any
	TypesVersions    map[string]any
	Exports          JSONObject
	Esmsh            map[string]any
	Dist             NpmPackageDist
	Deprecated       string
}

// ToNpmPackage converts PackageJSONRaw to PackageJSON
func (a *PackageJSONRaw) ToNpmPackage() *PackageJSON {
	browser := map[string]string{}
	if a.Browser.Str != "" && isModule(a.Browser.Str) {
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
		dependencies = make(map[string]string)
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
		peerDependencies = make(map[string]string)
		for k, v := range m {
			if s, ok := v.(string); ok {
				if k != "" && s != "" {
					peerDependencies[k] = s
				}
			}
		}
	}

	sideEffects := set.New[string]()
	sideEffectsFalse := false
	if a.SideEffects != nil {
		if s, ok := a.SideEffects.(string); ok {
			if s == "false" {
				sideEffectsFalse = true
			} else if isModule(s) {
				sideEffects = set.New[string]()
				sideEffects.Add(s)
			}
		} else if b, ok := a.SideEffects.(bool); ok {
			sideEffectsFalse = !b
		} else if m, ok := a.SideEffects.([]any); ok && len(m) > 0 {
			sideEffects = set.New[string]()
			for _, v := range m {
				if name, ok := v.(string); ok && isModule(name) {
					sideEffects.Add(name)
				}
			}
		}
	}

	exports := JSONObject{}
	if rawExports := a.Exports; rawExports != nil {
		var s string
		if json.Unmarshal(rawExports, &s) == nil {
			if len(s) > 0 {
				exports = JSONObject{
					keys:   []string{"."},
					values: map[string]any{".": s},
				}
			}
		} else {
			exports.UnmarshalJSON(rawExports)
		}
	}

	depreacted := ""
	if a.Deprecated != nil {
		if s, ok := a.Deprecated.(string); ok {
			depreacted = s
		}
	}

	var dist NpmPackageDist
	if a.Dist != nil {
		json.Unmarshal(a.Dist, &dist)
	}

	p := &PackageJSON{
		Name:             a.Name,
		Version:          a.Version,
		Type:             a.Type,
		Main:             a.Main.MainString(),
		Module:           a.Module.MainString(),
		Types:            a.Types.MainString(),
		Typings:          a.Typings.MainString(),
		Browser:          browser,
		SideEffectsFalse: sideEffectsFalse,
		SideEffects:      *sideEffects.ReadOnly(),
		Dependencies:     dependencies,
		PeerDependencies: peerDependencies,
		Imports:          toMap(a.Imports),
		TypesVersions:    toMap(a.TypesVersions),
		Exports:          exports,
		Esmsh:            toMap(a.Esmsh),
		Deprecated:       depreacted,
		Dist:             dist,
	}

	// normalize package module field
	if p.Module == "" {
		if es2015 := a.ES2015.MainString(); es2015 != "" {
			p.Module = es2015
		} else if jsNextMain := a.JsNextMain.MainString(); jsNextMain != "" {
			p.Module = jsNextMain
		} else if p.Main != "" && (p.Type == "module" || strings.HasSuffix(p.Main, ".mjs")) {
			p.Module = p.Main
			p.Main = ""
		}
	}

	return p
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (a *PackageJSON) UnmarshalJSON(b []byte) error {
	var raw PackageJSONRaw
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*a = *raw.ToNpmPackage()
	return nil
}

// JSONObject represents a readonly JSON object with ordered keys
type JSONObject struct {
	keys   []string
	values map[string]any
}

// NewJSONObject creates a new JSONObject with the given keys and values
func NewJSONObject(keys []string, values map[string]any) JSONObject {
	return JSONObject{
		keys:   keys,
		values: values,
	}
}

// Len returns the length of the JSON object
func (obj *JSONObject) Len() int {
	return len(obj.keys)
}

// Keys returns the keys of the JSON object
func (obj *JSONObject) Keys() []string {
	return obj.keys
}

// Values returns the values of the JSON object
func (obj *JSONObject) Values() map[string]any {
	return obj.values
}

// Get returns the value of the key in the JSON object
func (obj *JSONObject) Get(key string) (any, bool) {
	v, ok := obj.values[key]
	return v, ok
}

// UnmarshalJSON implements type json.Unmarshaler interface
func (obj *JSONObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))

	// don't convert number to float64
	dec.UseNumber()

	t, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expect JSON object open with '{'")
	}

	err = obj.parse(dec)
	if err != nil {
		return err
	}

	t, err = dec.Token()
	if err != io.EOF {
		return fmt.Errorf("expect end of JSON object but got more token: %T: %v or err: %v", t, t, err)
	}

	return nil
}

func (obj *JSONObject) parse(dec *json.Decoder) (err error) {
	var t json.Token
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return err
		}

		key, ok := t.(string)
		if !ok {
			return fmt.Errorf("expecting JSON key should be always a string: %T: %v", t, t)
		}

		t, err = dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		var value any
		value, err = handleDelim(t, dec)
		if err != nil {
			return err
		}

		obj.keys = append(obj.keys, key)
		if obj.values == nil {
			obj.values = make(map[string]any)
		}
		obj.values[key] = value
	}

	t, err = dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("expect JSON object close with '}'")
	}

	return nil
}

func parseArray(dec *json.Decoder) (arr []any, err error) {
	var t json.Token
	arr = make([]any, 0)
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return
		}

		var value any
		value, err = handleDelim(t, dec)
		if err != nil {
			return
		}
		arr = append(arr, value)
	}
	t, err = dec.Token()
	if err != nil {
		return
	}
	if delim, ok := t.(json.Delim); !ok || delim != ']' {
		err = fmt.Errorf("expect JSON array close with ']'")
		return
	}

	return
}

func handleDelim(t json.Token, dec *json.Decoder) (res any, err error) {
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			obj := JSONObject{
				values: make(map[string]any),
			}
			err = obj.parse(dec)
			if err != nil {
				return
			}
			return obj, nil
		case '[':
			var value []any
			value, err = parseArray(dec)
			if err != nil {
				return
			}
			return value, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter: %q", delim)
		}
	}
	return t, nil
}

type JSONAny struct {
	Str string
	Map map[string]any
	Any any
}

func (a *JSONAny) MarshalJSON() ([]byte, error) {
	if a.Str != "" {
		return json.Marshal(a.Str)
	}
	if a.Map != nil {
		return json.Marshal(a.Map)
	}
	return json.Marshal(a.Any)
}

func (a *JSONAny) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Str = s
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		a.Map = m
		return nil
	}
	return json.Unmarshal(b, &a.Any)
}

func (a *JSONAny) MainString() string {
	if a.Str != "" {
		return a.Str
	}
	if a.Map != nil {
		if v, ok := a.Map["."]; ok {
			if s, isStr := v.(string); isStr {
				return s
			}
		}
	}
	return ""
}

// isModule checks if the given string is a module file
func isModule(s string) bool {
	switch path.Ext(s) {
	case ".js", ".ts", ".mjs", ".mts", ".jsx", ".tsx", ".cjs", ".cts":
		return true
	default:
		return false
	}
}

// toMap converts any value to a `map[string]any`
func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
